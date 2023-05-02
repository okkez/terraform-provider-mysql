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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/okkez/terraform-provider-mysql/internal/utils"

	"github.com/r3labs/diff/v3"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &GrantPrivilegeResource{}
	_ resource.ResourceWithImportState = &GrantPrivilegeResource{}
)

func NewGrantPrivilegeResource() resource.Resource {
	return &GrantPrivilegeResource{}
}

// GrantPrivilegeResource defines the resource implementation.
type GrantPrivilegeResource struct {
	mysqlConfig *MySQLConfiguration
}

// GrantPrivilegeResourceModel describes the resource data model.
type GrantPrivilegeResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Privileges  types.Set    `tfsdk:"privilege"`
	On          types.Object `tfsdk:"on"`
	To          types.Object `tfsdk:"to"`
	GrantOption types.Bool   `tfsdk:"grant_option"`
}

type PrivilegeTypeModel struct {
	PrivType types.String `tfsdk:"priv_type" diff:"priv_type"`
	Columns  types.Set    `tfsdk:"columns" diff:"columns"`
}

var PrivlilegeTypeModelTypes = map[string]attr.Type{
	"priv_type": types.StringType,
	"columns":   types.SetType{ElemType: types.StringType},
}

type PrivilegeLevelModel struct {
	Database types.String `tfsdk:"database"`
	Table    types.String `tfsdk:"table"`
}

func (r *GrantPrivilegeResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_grant_privilege"
}

func (r *GrantPrivilegeResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Example resource",

		Attributes: map[string]schema.Attribute{
			"id": utils.IDAttribute(),
			"grant_option": schema.BoolAttribute{
				MarkdownDescription: "",
				Computed:            true,
				Optional:            true,
				Default:             booldefault.StaticBool(false),
			},
		},
		Blocks: map[string]schema.Block{
			"privilege": schema.SetNestedBlock{
				MarkdownDescription: "",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"priv_type": schema.StringAttribute{
							MarkdownDescription: "",
							Required:            true,
						},
						"columns": schema.SetAttribute{
							MarkdownDescription: "",
							ElementType:         types.StringType,
							Optional:            true,
						},
					},
				},
			},
			"on": schema.SingleNestedBlock{
				MarkdownDescription: "",
				Attributes: map[string]schema.Attribute{
					"database": schema.StringAttribute{
						MarkdownDescription: "",
						Required:            true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.RequiresReplace(),
						},
					},
					"table": schema.StringAttribute{
						MarkdownDescription: "",
						Required:            true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.RequiresReplace(),
						},
					},
				},
			},
			"to": schema.SingleNestedBlock{
				MarkdownDescription: "",
				Attributes: map[string]schema.Attribute{
					"name": utils.NameAttribute(),
					"host": utils.HostAttribute(),
				},
			},
		},
	}
}

func (r *GrantPrivilegeResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *GrantPrivilegeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	db, err := getDatabase(ctx, r.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed to connect MySQL", err.Error())
		return
	}

	var data *GrantPrivilegeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var privileges []PrivilegeTypeModel
	data.Privileges.ElementsAs(ctx, &privileges, false)

	var privilegeLevel PrivilegeLevelModel
	resp.Diagnostics.Append(data.On.As(ctx, &privilegeLevel, basetypes.ObjectAsOptions{})...)
	var userOrRole UserModel
	resp.Diagnostics.Append(data.To.As(ctx, &userOrRole, basetypes.ObjectAsOptions{})...)
	if resp.Diagnostics.HasError() {
		return
	}

	err = grantPrivileges(ctx, db, privileges, privilegeLevel, userOrRole, data.GrantOption.ValueBool())
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Failed executing GRANT statement (%s@%s)", userOrRole.Name.ValueString(), userOrRole.Host.ValueString()),
			err.Error())
		return
	}

	data.ID = types.StringValue(
		fmt.Sprintf(
			"%s@%s%s@%s",
			privilegeLevel.Database.ValueString(),
			privilegeLevel.Table.ValueString(),
			userOrRole.Name.ValueString(),
			userOrRole.Host.ValueString()))

	// Save data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *GrantPrivilegeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	db, err := getDatabase(ctx, r.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed to connect MySQL", err.Error())
		return
	}

	var data *GrantPrivilegeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var args []interface{}
	sql := `SHOW GRANTS FOR ?@?`

	var privilegeLevel PrivilegeLevelModel
	resp.Diagnostics.Append(data.On.As(ctx, &privilegeLevel, basetypes.ObjectAsOptions{})...)
	var userOrRole UserModel
	resp.Diagnostics.Append(data.To.As(ctx, &userOrRole, basetypes.ObjectAsOptions{})...)

	args = append(args, userOrRole.Name.ValueString())
	args = append(args, userOrRole.Host.ValueString())

	tflog.Info(ctx, sql, map[string]any{"args": args})

	rows, err := db.QueryContext(ctx, sql, args...)
	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Failed showing grants (%s@%s)", userOrRole.Name.ValueString(), userOrRole.Host.ValueString()),
			err.Error())
	}

	privileges := []attr.Value{}
	for rows.Next() {
		var grantStatement string
		if err := rows.Scan(&grantStatement); err != nil {
			resp.Diagnostics.AddError("Failed scanning MySQL rows", err.Error())
			return
		}
		grantPrivilege, err := ParseGrantPrivilegeStatement(grantStatement)
		if err != nil {
			resp.Diagnostics.AddError("Failed parsing grant statement", err.Error())
			return
		}
		if !grantPrivilege.Match(privilegeLevel.Database.ValueString(), privilegeLevel.Table.ValueString(), userOrRole.Name.ValueString(), userOrRole.Host.ValueString()) {
			continue
		}

		for _, priv := range grantPrivilege.Privileges {
			if len(priv.Priv.String()) == 0 {
				continue
			}
			privilegeTypeModelValue := map[string]attr.Value{}
			privilegeTypeModelValue["priv_type"] = types.StringValue(strings.ToUpper(priv.Priv.String()))
			if len(priv.Cols) == 0 {
				privilegeTypeModelValue["columns"] = types.SetNull(types.StringType)
			} else {
				columns := []attr.Value{}
				for _, col := range priv.Cols {
					columns = append(columns, types.StringValue(col.Name.O))
				}
				privilegeTypeModelValue["columns"] = types.SetValueMust(types.StringType, columns)
			}
			privileges = append(privileges, types.ObjectValueMust(PrivlilegeTypeModelTypes, privilegeTypeModelValue))
		}
	}

	tflog.Info(ctx, fmt.Sprintf("%+v\n", privileges))

	data.Privileges = types.SetValueMust(types.ObjectType{AttrTypes: PrivlilegeTypeModelTypes}, privileges)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *GrantPrivilegeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	db, err := getDatabase(ctx, r.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed to connect MySQL", err.Error())
		return
	}

	var data, state *GrantPrivilegeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var dataPrivileges, statePrivileges []PrivilegeTypeModel
	data.Privileges.ElementsAs(ctx, &dataPrivileges, false)
	state.Privileges.ElementsAs(ctx, &statePrivileges, false)

	changelog, err := diff.Diff(statePrivileges, dataPrivileges, diff.DiscardComplexOrigin(), diff.StructMapKeySupport())
	if err != nil {
		resp.Diagnostics.AddError("Failed calculating diff", err.Error())
		return
	}

	tflog.Info(ctx, fmt.Sprintf("%+v\nstate=%+v\ndata=%+v\n", changelog, statePrivileges, dataPrivileges))

	var privilegesToGrant, privilegesToRevoke []PrivilegeTypeModel
	var prev string
	var toGrant, toRevoke PrivilegeTypeModel
	for _, change := range changelog {
		tflog.Info(ctx, fmt.Sprintf("%+v\n", change))
		if prev != change.Path[0] {
			if !toGrant.PrivType.IsNull() {
				privilegesToGrant = append(privilegesToGrant, toGrant)
			}
			if !toRevoke.PrivType.IsNull() {
				privilegesToRevoke = append(privilegesToRevoke, toRevoke)
			}
			toGrant = PrivilegeTypeModel{}
			toRevoke = PrivilegeTypeModel{}
		}
		if change.Path[1] == "priv_type" && change.Path[2] == "state" {
			prev = change.Path[0]
			continue
		}
		// TODO support columns
		if change.Path[1] == "priv_type" && change.Path[2] == "value" {
			switch change.Type {
			case "create":
				if priv, ok := change.To.(string); ok {
					toGrant.PrivType = types.StringValue(priv)
				}
			case "update":
				if priv, ok := change.To.(string); ok {
					toGrant.PrivType = types.StringValue(priv)
				}
				if priv, ok := change.From.(string); ok {
					toRevoke.PrivType = types.StringValue(priv)
				}
			case "delete":
				if priv, ok := change.From.(string); ok {
					toRevoke.PrivType = types.StringValue(priv)
				}
			}
		}
		prev = change.Path[0]
	}

	if !toGrant.PrivType.IsNull() {
		privilegesToGrant = append(privilegesToGrant, toGrant)
	}
	if !toRevoke.PrivType.IsNull() {
		privilegesToRevoke = append(privilegesToRevoke, toRevoke)
	}

	tflog.Info(ctx, fmt.Sprintf("grant %+v\n", privilegesToGrant))
	tflog.Info(ctx, fmt.Sprintf("revoke %+v\n", privilegesToRevoke))

	var privilegeLevel PrivilegeLevelModel
	resp.Diagnostics.Append(data.On.As(ctx, &privilegeLevel, basetypes.ObjectAsOptions{})...)
	var userOrRole UserModel
	resp.Diagnostics.Append(data.To.As(ctx, &userOrRole, basetypes.ObjectAsOptions{})...)
	if resp.Diagnostics.HasError() {
		return
	}

	if len(privilegesToRevoke) > 0 {
		err = revokePrivileges(ctx, db, privilegesToRevoke, privilegeLevel, userOrRole, data.GrantOption.ValueBool())
		if err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Failed executing REVOKE statement (%s)", data.ID.ValueString()),
				err.Error())
			return
		}
	}
	if len(privilegesToGrant) > 0 {
		err = grantPrivileges(ctx, db, privilegesToGrant, privilegeLevel, userOrRole, data.GrantOption.ValueBool())
		if err != nil {
			resp.Diagnostics.AddError(
				fmt.Sprintf("Failed executing GRANT statement (%s)", data.ID.ValueString()),
				err.Error())
			return
		}
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *GrantPrivilegeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	db, err := getDatabase(ctx, r.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed to connect MySQL", err.Error())
		return
	}

	var data *GrantPrivilegeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var privileges []PrivilegeTypeModel
	data.Privileges.ElementsAs(ctx, &privileges, false)

	var privilegeLevel PrivilegeLevelModel
	resp.Diagnostics.Append(data.On.As(ctx, &privilegeLevel, basetypes.ObjectAsOptions{})...)
	var userOrRole UserModel
	resp.Diagnostics.Append(data.To.As(ctx, &userOrRole, basetypes.ObjectAsOptions{})...)
	if resp.Diagnostics.HasError() {
		return
	}

	err = revokePrivileges(ctx, db, privileges, privilegeLevel, userOrRole, data.GrantOption.ValueBool())

	if err != nil {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Failed executing REVOKE statement (%s@%s)", userOrRole.Name.ValueString(), userOrRole.Host.ValueString()),
			err.Error())
		return
	}
}

func (r *GrantPrivilegeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func buildPrivilege(ctx context.Context, db *sql.DB, privilege PrivilegeTypeModel) (string, error) {
	normalizedPrivType := strings.ToUpper(privilege.PrivType.ValueString())
	if privilege.Columns.IsNull() {
		return normalizedPrivType, nil
	}

	var columns, quotedColumns []string
	privilege.Columns.ElementsAs(ctx, &columns, false)
	quotedColumns, err := quoteIdentifiers(ctx, db, columns...)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s (%s)", normalizedPrivType, strings.Join(quotedColumns, ",")), nil
}

func grantPrivileges(ctx context.Context, db *sql.DB, privileges []PrivilegeTypeModel, privilegeLevel PrivilegeLevelModel, userOrRole UserModel, grantOption bool) error {
	var args []interface{}
	sql := `GRANT `

	var privilegesWithColumns []string

	for _, privilege := range privileges {
		priv, err := buildPrivilege(ctx, db, privilege)
		if err != nil {
			return fmt.Errorf("Failed building privilege: %w", err)
		}
		privilegesWithColumns = append(privilegesWithColumns, priv)
	}

	sql += strings.Join(privilegesWithColumns, ",")

	sql += ` ON`
	var database, table string
	if privilegeLevel.Database.ValueString() == "*" {
		database = privilegeLevel.Database.ValueString()
	} else {
		database, _ = quoteIdentifier(ctx, db, privilegeLevel.Database.ValueString())
	}
	if privilegeLevel.Table.ValueString() == "*" {
		table = privilegeLevel.Table.ValueString()
	} else {
		table, _ = quoteIdentifier(ctx, db, privilegeLevel.Table.ValueString())
	}
	sql += fmt.Sprintf(" %s.%s", database, table)
	sql += ` TO ?@?`
	args = append(args, userOrRole.Name.ValueString())
	args = append(args, userOrRole.Host.ValueString())

	if grantOption {
		sql += ` WITH GRANT OPTION`
	}

	tflog.Info(ctx, sql, map[string]any{"args": args})

	_, err := db.ExecContext(ctx, sql, args...)
	if err != nil {
		return err
	}

	return nil
}

func revokePrivileges(ctx context.Context, db *sql.DB, privileges []PrivilegeTypeModel, privilegeLevel PrivilegeLevelModel, userOrRole UserModel, grantOption bool) error {
	var args []interface{}
	sql := `REVOKE `

	var privilegesWithColumns []string
	for _, privilege := range privileges {
		priv, err := buildPrivilege(ctx, db, privilege)
		if err != nil {
			return fmt.Errorf("Failed to building privileges: %w", err)
		}
		privilegesWithColumns = append(privilegesWithColumns, priv)
	}

	sql += strings.Join(privilegesWithColumns, ",")

	if grantOption {
		sql += `,GRANT OPTION`
	}

	sql += ` ON`
	var database, table string
	if privilegeLevel.Database.ValueString() == "*" {
		database = privilegeLevel.Database.ValueString()
	} else {
		database, _ = quoteIdentifier(ctx, db, privilegeLevel.Database.ValueString())
	}
	if privilegeLevel.Table.ValueString() == "*" {
		table = privilegeLevel.Table.ValueString()
	} else {
		table, _ = quoteIdentifier(ctx, db, privilegeLevel.Table.ValueString())
	}
	sql += fmt.Sprintf(" %s.%s", database, table)
	sql += ` FROM ?@?`
	args = append(args, userOrRole.Name.ValueString())
	args = append(args, userOrRole.Host.ValueString())

	tflog.Info(ctx, sql, map[string]any{"args": args})

	_, err := db.ExecContext(ctx, sql, args...)
	if err != nil {
		return err
	}

	return nil
}
