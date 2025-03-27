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
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/okkez/terraform-provider-mysql/internal/utils"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &DefaultRolesResource{}
	_ resource.ResourceWithConfigure   = &DefaultRolesResource{}
	_ resource.ResourceWithImportState = &DefaultRolesResource{}
)

func NewDefaultRolesResource() resource.Resource {
	return &DefaultRolesResource{}
}

// DefaultRolesResource defines the resource implementation.
type DefaultRolesResource struct {
	mysqlConfig *MySQLConfiguration
}

// DefaultRolesResourceModel describes the resource data model.
type DefaultRolesResourceModel struct {
	ID           types.String `tfsdk:"id"`
	User         types.String `tfsdk:"user"`
	Host         types.String `tfsdk:"host"`
	DefaultRoles types.Set    `tfsdk:"default_role"`
}

func (r *DefaultRolesResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_default_roles"
}

func (r *DefaultRolesResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The `mysql_default_role` resource manages default roles for the user.",

		Attributes: map[string]schema.Attribute{
			"id":   utils.IDAttribute(),
			"user": utils.NameAttribute("user", true),
			"host": utils.HostAttribute("user", true),
		},
		Blocks: map[string]schema.Block{
			"default_role": schema.SetNestedBlock{
				MarkdownDescription: "Set default roles",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": utils.NameAttribute("role", false),
						"host": utils.HostAttribute("role", false),
					},
				},
			},
		},
	}
}

func (r *DefaultRolesResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *DefaultRolesResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	db, err := getDatabase(ctx, r.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed to connect MySQL", err.Error())
		return
	}

	var data *DefaultRolesResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	user := data.User.ValueString()
	host := data.Host.ValueString()
	err = alterDefaultRoles(ctx, db, data)
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Failed setting default role to user (%s@%s)", user, host),
			err.Error())
		return
	}

	data.ID = types.StringValue(fmt.Sprintf("%s@%s", user, host))
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DefaultRolesResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	db, err := getDatabase(ctx, r.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed to connect MySQL", err.Error())
		return
	}

	var data *DefaultRolesResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	user := data.User.ValueString()
	host := data.Host.ValueString()
	if !utils.UserExists(ctx, db, user, host) {
		resp.State.RemoveResource(ctx)
		return
	}

	var args []interface{}
	args = append(args, user)
	args = append(args, host)
	sql := `
SELECT
  DEFAULT_ROLE_USER
, DEFAULT_ROLE_HOST
FROM
  mysql.default_roles
WHERE
  USER = ?
  AND HOST = ?
`

	rows, err := db.QueryContext(ctx, sql, args...)
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Failed querying default roles for user (%s@%s)", user, host), err.Error())
		return
	}
	defer func() { _ = rows.Close() }()

	defaultRoles := []attr.Value{}
	for rows.Next() {
		var roleName, roleHost string
		if err := rows.Scan(&roleName, &roleHost); err != nil {
			resp.Diagnostics.AddError("Failed scanning MySQL rows", err.Error())
			return
		}
		roleValues := map[string]attr.Value{}
		roleValues["name"] = types.StringValue(roleName)
		roleValues["host"] = types.StringValue(roleHost)
		defaultRoles = append(defaultRoles, types.ObjectValueMust(RoleTypes, roleValues))
	}
	data.DefaultRoles = types.SetValueMust(types.ObjectType{AttrTypes: RoleTypes}, defaultRoles)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DefaultRolesResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	db, err := getDatabase(ctx, r.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed to connect MySQL", err.Error())
		return
	}

	var data *DefaultRolesResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	user := data.User.ValueString()
	host := data.Host.ValueString()
	err = alterDefaultRoles(ctx, db, data)
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Failed setting default role to user (%s@%s)", user, host),
			err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *DefaultRolesResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	db, err := getDatabase(ctx, r.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed to connect MySQL", err.Error())
		return
	}

	var data *DefaultRolesResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	user := data.User.ValueString()
	host := data.Host.ValueString()
	var args []interface{}
	args = append(args, user)
	args = append(args, host)
	sql := `ALTER USER ?@? DEFAULT ROLE NONE`
	tflog.Info(ctx, sql, map[string]any{"args": args})

	_, err = db.ExecContext(ctx, sql, args...)
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Failed deleting default roles for user (%s@%s)", user, host), err.Error())
		return
	}
}

func (r *DefaultRolesResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	userHost := strings.SplitN(req.ID, "@", 2)
	if len(userHost) != 2 {
		resp.Diagnostics.AddAttributeError(path.Root("id"), fmt.Sprintf("Invalid ID format. %s", req.ID), "The valid ID format is `user@host`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(req.ID))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("user"), types.StringValue(userHost[0]))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("host"), types.StringValue(userHost[1]))...)
}

func alterDefaultRoles(ctx context.Context, db *sql.DB, data *DefaultRolesResourceModel) error {
	user := data.User.ValueString()
	host := data.Host.ValueString()

	var args []interface{}
	args = append(args, user)
	args = append(args, host)
	sql := `ALTER USER ?@? DEFAULT ROLE`

	if data.DefaultRoles.IsNull() {
		sql += ` NONE`
	} else {
		var defaultRoles []RoleModel
		data.DefaultRoles.ElementsAs(ctx, &defaultRoles, false)
		var placeholders []string
		for _, role := range defaultRoles {
			placeholders = append(placeholders, "?@?")
			args = append(args, role.Name.ValueString())
			args = append(args, role.Host.ValueString())
		}
		sql += fmt.Sprintf(` %s`, strings.Join(placeholders, ","))
	}

	tflog.Info(ctx, sql, map[string]any{"args": args})
	_, err := db.ExecContext(ctx, sql, args...)
	if err != nil {
		return err
	}

	return nil
}
