package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/okkez/terraform-provider-mysql/internal/utils"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &RoleResource{}
	_ resource.ResourceWithConfigure   = &RoleResource{}
	_ resource.ResourceWithImportState = &RoleResource{}
)

func NewRoleResource() resource.Resource {
	return &RoleResource{}
}

// RoleResource defines the resource implementation.
type RoleResource struct {
	mysqlConfig *MySQLConfiguration
}

// RoleResourceModel describes the resource data model.
type RoleResourceModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
	Host types.String `tfsdk:"host"`
}

func (r *RoleResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_role"
}

func (r *RoleResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "MySQL role",

		Attributes: map[string]schema.Attribute{
			"id":   utils.IDAttribute(),
			"name": utils.NameAttribute("role", true),
			"host": utils.HostAttribute("role", true),
		},
	}
}

func (r *RoleResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *RoleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	db, err := getDatabase(ctx, r.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed to connect MySQL", err.Error())
		return
	}

	var data *RoleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := data.Name.ValueString()
	host := data.Host.ValueString()
	var args []interface{}
	args = append(args, name)
	args = append(args, host)
	sql := "CREATE ROLE ?@?"
	tflog.Info(ctx, sql, map[string]any{"args": args})
	_, err = db.ExecContext(ctx, sql, args...)
	if err != nil {
		resp.Diagnostics.AddError("Failed creating role", err.Error())
		return
	}

	data.ID = types.StringValue(fmt.Sprintf("%s@%s", args...))
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RoleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	db, err := getDatabase(ctx, r.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed to connect MySQL", err.Error())
		return
	}

	var data *RoleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var args []interface{}
	args = append(args, data.Name.ValueString())
	args = append(args, data.Host.ValueString())

	sql := `
SELECT
  User
, Host
FROM
  mysql.user
WHERE
  User = ?
  AND Host = ?
  AND authentication_string = ''
  AND password_expired = 'Y'
`
	tflog.Info(ctx, sql, map[string]any{"args": args})

	var name, host string
	if err = db.QueryRowContext(ctx, sql, args...).Scan(&name, &host); err != nil {
		resp.State.RemoveResource(ctx)
		return
	} else {
		data.Name = types.StringValue(name)
		data.Host = types.StringValue(host)
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *RoleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	panic("must not happen")
}

func (r *RoleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	db, err := getDatabase(ctx, r.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed to connect MySQL", err.Error())
		return
	}

	var data *RoleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := data.Name.ValueString()
	host := data.Host.ValueString()
	sql := "DROP ROLE IF EXISTS ?@?"
	var args []interface{}
	args = append(args, name)
	args = append(args, host)
	tflog.Info(ctx, sql, map[string]any{"args": args})

	_, err = db.ExecContext(ctx, sql, name, host)
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Failed deleting role (%s@%s)", args...), err.Error())
		return
	}
}

func (r *RoleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	nameHost := strings.SplitN(req.ID, "@", 2)
	if len(nameHost) != 2 {
		resp.Diagnostics.AddAttributeError(path.Root("id"), fmt.Sprintf("Invalid ID format. %s", req.ID), "The valid ID format is `name@host`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), types.StringValue(req.ID))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), types.StringValue(nameHost[0]))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("host"), types.StringValue(nameHost[1]))...)
}
