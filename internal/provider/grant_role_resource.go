package provider

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/okkez/terraform-provider-mysql/internal/utils"

	"github.com/r3labs/diff/v3"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &GrantRoleResource{}
var _ resource.ResourceWithImportState = &GrantRoleResource{}

func NewGrantRoleResource() resource.Resource {
	return &GrantRoleResource{}
}

// GrantRoleResource defines the resource implementation.
type GrantRoleResource struct {
	mysqlConfig *MySQLConfiguration
}

// GrantRoleResourceModel describes the resource data model.
type GrantRoleResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Roles       types.Set    `tfsdk:"roles"`
	To          types.Object `tfsdk:"to"`
	AdminOption types.Bool   `tfsdk:"admin_option"`
}

func (r *GrantRoleResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_grant_role"
}

func (r *GrantRoleResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The `mysql_grant_role` resource grants a role to a user." +
			"See MySQL Reference Manual [GRANT Statement](https://dev.mysql.com/doc/refman/8.0/en/grant.html) for more detauls.\n\n" +
			"Use the [`mysql_grant_privilege`](./grant_privilege) resource to grant privileges to a user or a role.",
		Attributes: map[string]schema.Attribute{
			"id": utils.IDAttribute(),
			"roles": schema.SetAttribute{
				MarkdownDescription: "Sets the role to be granted to the user specified in the `to` block.",
				Required:            true,
				ElementType:         types.StringType,
			},
			"admin_option": schema.BoolAttribute{
				MarkdownDescription: "If `true`, add `WITH ADMIN OPTION`. Defaults to `false`.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
		},
		Blocks: map[string]schema.Block{
			"to": schema.SingleNestedBlock{
				MarkdownDescription: "Set the user or role to be granted roles.",
				Attributes: map[string]schema.Attribute{
					"name": utils.NameAttribute("user or role", true),
					"host": utils.HostAttribute("user or role", true),
				},
			},
		},
	}
}

func (r *GrantRoleResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	if mysqlConfig, ok := req.ProviderData.(*MySQLConfiguration); ok {
		r.mysqlConfig = mysqlConfig
	} else {
		resp.Diagnostics.AddError("Failed type assertion", "")
	}
}

func (r *GrantRoleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	db, err := getDatabase(ctx, r.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed to connect MySQL", err.Error())
		return
	}

	var data *GrantRoleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var roles []string
	data.Roles.ElementsAs(ctx, &roles, false)

	var userOrRole UserModel
	resp.Diagnostics.Append(data.To.As(ctx, &userOrRole, basetypes.ObjectAsOptions{})...)
	if resp.Diagnostics.HasError() {
		return
	}

	err = grantRoles(ctx, db, userOrRole, roles, data.AdminOption.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Failed executing GRANT statement (%s@%s)", userOrRole.Name.ValueString(), userOrRole.Host.ValueString()),
			err.Error())
		return
	}

	data.ID = types.StringValue(fmt.Sprintf("%s@%s", userOrRole.Name.ValueString(), userOrRole.Host.ValueString()))

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *GrantRoleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	db, err := getDatabase(ctx, r.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed to connect MySQL", err.Error())
		return
	}

	var data *GrantRoleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var userOrRole UserModel
	resp.Diagnostics.Append(data.To.As(ctx, &userOrRole, basetypes.ObjectAsOptions{})...)
	if resp.Diagnostics.HasError() {
		return
	}

	sql := `
SELECT
  FROM_USER
, FROM_HOST
, WITH_ADMIN_OPTION
FROM
  mysql.role_edges
WHERE
  TO_USER = ? 
  AND TO_HOST = ?
`
	var args []interface{}
	args = append(args, userOrRole.Name.ValueString())
	args = append(args, userOrRole.Host.ValueString())
	tflog.Info(ctx, sql, map[string]any{"args": args})

	rows, err := db.QueryContext(ctx, sql, args...)
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Failed querying roles (%s@%s)", userOrRole.Name.ValueString(), userOrRole.Host.ValueString()),
			err.Error())
		return
	}
	defer rows.Close()

	var roles []attr.Value
	for rows.Next() {
		var fromUser, fromHost, adminOption string
		if err := rows.Scan(&fromUser, &fromHost, &adminOption); err != nil {
			resp.Diagnostics.AddError("Failed scanning MySQL rows", err.Error())
			return
		}
		roles = append(roles, types.StringValue(fromUser))
		if adminOption == "Y" {
			data.AdminOption = types.BoolValue(true)
		} else {
			data.AdminOption = types.BoolValue(false)
		}
	}
	data.Roles = types.SetValueMust(types.StringType, roles)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *GrantRoleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	db, err := getDatabase(ctx, r.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed to connect MySQL", err.Error())
		return
	}

	var data, state *GrantRoleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var dataRoles, stateRoles []string
	data.Roles.ElementsAs(ctx, &dataRoles, false)
	state.Roles.ElementsAs(ctx, &stateRoles, false)

	changelog, err := diff.Diff(stateRoles, dataRoles)
	if err != nil {
		resp.Diagnostics.AddError("Failed calculating diff", err.Error())
		return
	}

	var rolesToGrant, rolesToRevoke []string
	for _, change := range changelog {
		switch change.Type {
		case "create":
			if role, ok := change.To.(string); ok {
				rolesToGrant = append(rolesToGrant, role)
			}
		case "update":
			if role, ok := change.To.(string); ok {
				rolesToGrant = append(rolesToGrant, role)
			}
			if role, ok := change.From.(string); ok {
				rolesToRevoke = append(rolesToRevoke, role)
			}
		case "delete":
			if role, ok := change.From.(string); ok {
				rolesToRevoke = append(rolesToRevoke, role)
			}
		}
	}

	var userOrRole UserModel
	resp.Diagnostics.Append(data.To.As(ctx, &userOrRole, basetypes.ObjectAsOptions{})...)
	if resp.Diagnostics.HasError() {
		return
	}

	if len(rolesToRevoke) > 0 {
		err := revokeRoles(ctx, db, userOrRole, rolesToRevoke)
		if err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("[Update] Failed executing REVOKE statement (%s@%s)", userOrRole.Name.ValueString(), userOrRole.Host.ValueString()),
				err.Error())
			return
		}
	}

	if len(rolesToGrant) > 0 {
		err := grantRoles(ctx, db, userOrRole, rolesToGrant, data.AdminOption.ValueBool())
		if err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("[Update] Failed executing GRANT statement (%s@%s)", userOrRole.Name.ValueString(), userOrRole.Host.ValueString()),
				err.Error())
			return
		}
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *GrantRoleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	db, err := getDatabase(ctx, r.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed to connect MySQL", err.Error())
		return
	}

	var data *GrantRoleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var userOrRole UserModel
	resp.Diagnostics.Append(data.To.As(ctx, &userOrRole, basetypes.ObjectAsOptions{})...)
	if resp.Diagnostics.HasError() {
		return
	}
	var roles []string
	data.Roles.ElementsAs(ctx, &roles, false)

	err = revokeRoles(ctx, db, userOrRole, roles)

	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("[Delete] Failed executing REVOKE statement (%s@%s)", userOrRole.Name.ValueString(), userOrRole.Host.ValueString()),
			err.Error())
		return
	}
}

func grantRoles(ctx context.Context, db *sql.DB, to UserModel, roles []string, adminOption bool) error {
	var args []interface{}
	sql := `GRANT`

	placeholders := []string{}
	for _, role := range roles {
		placeholders = append(placeholders, "?")
		args = append(args, role)
	}
	sql += fmt.Sprintf(` %s`, strings.Join(placeholders, ","))

	sql += ` TO ?@?`
	args = append(args, to.Name.ValueString())
	args = append(args, to.Host.ValueString())

	if adminOption {
		sql += ` WITH ADMIN OPTION`
	}

	tflog.Info(ctx, sql, map[string]any{"args": args})

	_, err := db.ExecContext(ctx, sql, args...)
	if err != nil {
		return err
	}

	return nil
}

func revokeRoles(ctx context.Context, db *sql.DB, to UserModel, roles []string) error {
	var args []interface{}
	sql := `REVOKE`

	placeholders := []string{}
	for _, role := range roles {
		placeholders = append(placeholders, "?")
		args = append(args, role)
	}
	sql += fmt.Sprintf(` %s`, strings.Join(placeholders, ","))

	sql += ` FROM ?@?`
	args = append(args, to.Name.ValueString())
	args = append(args, to.Host.ValueString())

	tflog.Info(ctx, sql, map[string]any{"args": args})

	_, err := db.ExecContext(ctx, sql, args...)
	if err != nil {
		return err
	}

	return nil
}

func (r *GrantRoleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	nameHost := strings.SplitN(req.ID, "@", 2)
	if len(nameHost) != 2 {
		resp.Diagnostics.AddAttributeError(path.Root("id"), fmt.Sprintf("Invalid ID format. %s", req.ID), "The valid ID format is `name@host`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(req.ID))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("to").AtName("name"), types.StringValue(nameHost[0]))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("to").AtName("host"), types.StringValue(nameHost[1]))...)
}
