package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces.
var _ resource.Resource = &roleResource{}
var _ resource.ResourceWithImportState = &roleResource{}

func NewRoleResource() resource.Resource {
	return &roleResource{}
}

// roleResource defines the resource implementation.
type roleResource struct {
	mysqlConfig *MySQLConfiguration
}

// roleResourceModel describes the resource data model.
type roleResourceModel struct {
	Id   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
	Host types.String `tfsdk:"host"`
}

func (r *roleResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_role"
}

func (r *roleResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "MySQL role",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "MySQL role name",
				Required:            true,
			},
			"host": schema.StringAttribute{
				MarkdownDescription: "Host for the role",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("%"),
			},
		},
	}
}

func (r *roleResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	r.mysqlConfig = req.ProviderData.(*MySQLConfiguration)
}

func (r *roleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	db, err := getDatabase(ctx, r.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed to connect MySQL", err.Error())
		return
	}

	var data *roleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := data.Name.ValueString()
	host := data.Host.ValueString()
	sql := "CREATE ROLE ?@?"
	tflog.Debug(ctx, sql, map[string]any{"role": name, "host": host})
	_, err = db.ExecContext(ctx, sql, name, host)
	if err != nil {
		resp.Diagnostics.AddError("Failed creating role", err.Error())
		return
	}

	data.Id = types.StringValue(fmt.Sprintf("%s@%s", name, host))
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *roleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	db, err := getDatabase(ctx, r.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed to connect MySQL", err.Error())
		return
	}

	var data *roleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	nameHost := strings.SplitN(data.Id.ValueString(), "@", 2)
	if len(nameHost) != 2 {
		resp.Diagnostics.AddAttributeError(path.Root("id"), "Invalid id format", data.Id.ValueString())
		return
	}
	name := nameHost[0]
	host := nameHost[1]

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
	tflog.Debug(ctx, sql, map[string]any{"role": name, "host": host})

	var _user, _host string
	if err = db.QueryRowContext(ctx, sql, name, host).Scan(&_user, &_host); err != nil {
		resp.Diagnostics.AddWarning(fmt.Sprintf("Role (%s) not found. Removing from state.", name), err.Error())
		data.Id = types.StringNull()
	} else {
		data.Name = types.StringValue(name)
		data.Host = types.StringValue(host)
	}

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *roleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	db, err := getDatabase(ctx, r.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed to connect MySQL", err.Error())
		return
	}

	var data, state *roleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if data.Name.Equal(state.Name) && data.Host.Equal(state.Host) {
		return
	}

	oldName := state.Name.ValueString()
	oldHost := state.Host.ValueString()
	newName := data.Name.ValueString()
	newHost := data.Host.ValueString()

	var args []interface{}
	args = append(args, oldName)
	args = append(args, oldHost)
	args = append(args, newName)
	args = append(args, newHost)

	sql := "RENAME USER ?@? TO ?@?"
	tflog.Debug(ctx, sql, map[string]any{"args": args})

	_, err = db.ExecContext(ctx, sql, args...)
	if err != nil {
		resp.Diagnostics.AddError("Failed to rename role", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *roleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	db, err := getDatabase(ctx, r.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed to connect MySQL", err.Error())
		return
	}

	var data *roleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := data.Name.ValueString()
	host := data.Host.ValueString()
	sql := "DROP ROLE IF EXISTS ?@?"
	tflog.Debug(ctx, sql, map[string]any{"role": name, "host": host})

	_, err = db.ExecContext(ctx, sql, name, host)
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Failed deleting role (%s)", name), err.Error())
		return
	}
}

func (r *roleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
