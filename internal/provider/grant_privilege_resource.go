package provider

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
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
	PrivType types.String `tfsdk:"priv_type"`
	Columns  types.Set    `tfsdk:"columns"`
}

var PrivlilegeTypeModelTypes = map[string]attr.Type{
	"priv_type": types.StringType,
	"columns":   types.SetType{ElemType: types.StringType},
}

type PrivilegeLevelModel struct {
	Database types.String `tfsdk:"database"`
	Table    types.String `tfsdk:"table"`
}

type PrivilegeTypeRaw struct {
	PrivType string   `diff:"priv_type"`
	Columns  []string `diff:"columns"`
}

func (r *GrantPrivilegeResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_grant_privilege"
}

func (r *GrantPrivilegeResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The `mysql_grant_privilege` resource grants privileges to a user or a role. " +
			"See MySQL Reference Manual [GRANT Statement](https://dev.mysql.com/doc/refman/8.0/en/grant.html) for more detauls.\n\n" +
			"Use the [`mysql_grant_role`](./grant_role) resource to grant a role to a user.",

		Attributes: map[string]schema.Attribute{
			"id": utils.IDAttribute(),
			"grant_option": schema.BoolAttribute{
				MarkdownDescription: "If `true`, add `WITH GRANT OPTION`. Defaults to `false`.",
				Computed:            true,
				Optional:            true,
				Default:             booldefault.StaticBool(false),
			},
		},
		Blocks: map[string]schema.Block{
			"privilege": schema.SetNestedBlock{
				MarkdownDescription: "Set privilege name and columns.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"priv_type": schema.StringAttribute{
							MarkdownDescription: "The privilege name.",
							Required:            true,
						},
						"columns": schema.SetAttribute{
							MarkdownDescription: "Column names.",
							ElementType:         types.StringType,
							Optional:            true,
						},
					},
				},
			},
			"on": schema.SingleNestedBlock{
				MarkdownDescription: "Set the target to grant privileges.",
				Attributes: map[string]schema.Attribute{
					"database": schema.StringAttribute{
						MarkdownDescription: "The database name to grant privileges.",
						Required:            true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.RequiresReplace(),
						},
						Validators: []validator.String{
							stringvalidator.NoneOf("*"),
						},
					},
					"table": schema.StringAttribute{
						MarkdownDescription: "The table name to grant privileges.",
						Required:            true,
						PlanModifiers: []planmodifier.String{
							stringplanmodifier.RequiresReplace(),
						},
					},
				},
			},
			"to": schema.SingleNestedBlock{
				MarkdownDescription: "Set the user or role to be granted privileges.",
				Attributes: map[string]schema.Attribute{
					"name": utils.NameAttribute("user or role", true),
					"host": utils.HostAttribute("user or role", true),
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
			"%s@%s@%s@%s",
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

		data.GrantOption = types.BoolValue(grantPrivilege.GrantOption)

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
	dataPrivilegesRaw := convertPrivilegesToRaws(ctx, dataPrivileges)
	statePrivilegesRaw := convertPrivilegesToRaws(ctx, statePrivileges)

	// Note: need to change types to check diff properly.
	changelog, err := diff.Diff(statePrivilegesRaw, dataPrivilegesRaw, diff.DiscardComplexOrigin(), diff.StructMapKeySupport())
	if err != nil {
		resp.Diagnostics.AddError("Failed calculating diff", err.Error())
		return
	}

	tflog.Info(ctx, fmt.Sprintf("\nchangelog=%+v\nstate=%+v\ndata=%+v\n", changelog, statePrivileges, dataPrivileges))

	var prev string
	var privilegesToGrantRaw, privilegesToRevokeRaw []PrivilegeTypeRaw
	toGrantRaw := PrivilegeTypeRaw{Columns: []string{}}
	toRevokeRaw := PrivilegeTypeRaw{Columns: []string{}}
	for _, change := range changelog {
		tflog.Info(ctx, fmt.Sprintf("\nchange %+v\n", change))
		if prev != change.Path[0] {
			if len(toGrantRaw.PrivType) > 0 {
				privilegesToGrantRaw = append(privilegesToGrantRaw, toGrantRaw)
			}
			if len(toRevokeRaw.PrivType) > 0 {
				privilegesToRevokeRaw = append(privilegesToRevokeRaw, toRevokeRaw)
			}
			toGrantRaw = PrivilegeTypeRaw{Columns: []string{}}
			toRevokeRaw = PrivilegeTypeRaw{Columns: []string{}}
		}
		if change.Path[1] == "priv_type" {
			switch change.Type {
			case "create":
				if priv, ok := change.To.(string); ok {
					toGrantRaw.PrivType = priv
				}
			case "update":
				if priv, ok := change.To.(string); ok {
					toGrantRaw.PrivType = priv
				}
				if priv, ok := change.From.(string); ok {
					toRevokeRaw.PrivType = priv
				}
			case "delete":
				if priv, ok := change.From.(string); ok {
					toRevokeRaw.PrivType = priv
				}
			}
		}
		if change.Path[1] == "columns" {
			if len(toGrantRaw.PrivType) == 0 && (change.Type == "create" || change.Type == "update") {
				i, _ := strconv.ParseUint(change.Path[0], 10, 32)
				toGrantRaw.PrivType = dataPrivilegesRaw[i].PrivType
			}
			if len(toGrantRaw.PrivType) == 0 && (change.Type == "update" || change.Type == "delete") {
				i, _ := strconv.ParseUint(change.Path[0], 10, 32)
				toRevokeRaw.PrivType = dataPrivilegesRaw[i].PrivType
			}
			switch change.Type {
			case "create":
				if column, ok := change.To.(string); ok {
					toGrantRaw.Columns = append(toGrantRaw.Columns, column)
				}
			case "update":
				if column, ok := change.To.(string); ok {
					toGrantRaw.Columns = append(toGrantRaw.Columns, column)
				}
				if column, ok := change.From.(string); ok {
					toRevokeRaw.Columns = append(toGrantRaw.Columns, column)
				}
			case "delete":
				if column, ok := change.From.(string); ok {
					toRevokeRaw.Columns = append(toGrantRaw.Columns, column)
				}
			}
		}
		prev = change.Path[0]
	}

	if len(toGrantRaw.PrivType) > 0 {
		privilegesToGrantRaw = append(privilegesToGrantRaw, toGrantRaw)
	}
	if len(toRevokeRaw.PrivType) > 0 {
		privilegesToRevokeRaw = append(privilegesToRevokeRaw, toRevokeRaw)
	}

	tflog.Info(ctx, fmt.Sprintf("\ngrant raw%+v\nrevoke raw %+v\n", privilegesToGrantRaw, privilegesToRevokeRaw))

	privilegesToGrant := convertRawsToPrivileges(privilegesToGrantRaw)
	privilegesToRevoke := convertRawsToPrivileges(privilegesToRevokeRaw)
	tflog.Info(ctx, fmt.Sprintf("\ngrant %+v\nrevoke %+v\n", privilegesToGrant, privilegesToRevoke))

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
	idParts := strings.SplitN(req.ID, "@", 4)
	if len(idParts) != 4 {
		resp.Diagnostics.AddAttributeError(path.Root("id"), fmt.Sprintf("Invalid ID format. %s", req.ID), "The valid ID format is `database@table@name@host`")
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("on").AtName("database"), types.StringValue(idParts[0]))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("on").AtName("table"), types.StringValue(idParts[1]))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("to").AtName("name"), types.StringValue(idParts[2]))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("to").AtName("host"), types.StringValue(idParts[3]))...)
}

func buildPrivilege(ctx context.Context, db *sql.DB, privilege PrivilegeTypeModel) (string, error) {
	normalizedPrivType := strings.ToUpper(privilege.PrivType.ValueString())
	if privilege.Columns.IsNull() || len(privilege.Columns.Elements()) == 0 {
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

func convertPrivilegesToRaws(ctx context.Context, privileges []PrivilegeTypeModel) []PrivilegeTypeRaw {
	var result []PrivilegeTypeRaw
	for _, p := range privileges {
		var raw PrivilegeTypeRaw
		raw.PrivType = p.PrivType.ValueString()
		var columns []string
		p.Columns.ElementsAs(ctx, &columns, false)
		raw.Columns = columns
		result = append(result, raw)
	}

	return result
}

func convertRawsToPrivileges(raws []PrivilegeTypeRaw) []PrivilegeTypeModel {
	var result []PrivilegeTypeModel
	for _, r := range raws {
		var p PrivilegeTypeModel
		p.PrivType = types.StringValue(r.PrivType)
		var columns []attr.Value
		for _, c := range r.Columns {
			columns = append(columns, types.StringValue(c))
		}
		p.Columns = types.SetValueMust(types.StringType, columns)
		result = append(result, p)
	}
	return result
}
