package provider

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &GlobalVariableResource{}
	_ resource.ResourceWithConfigure   = &GlobalVariableResource{}
	_ resource.ResourceWithImportState = &GlobalVariableResource{}
)

func NewGlobalVariableResource() resource.Resource {
	return &GlobalVariableResource{}
}

// GlobalVariableResource defines the resource implementation.
type GlobalVariableResource struct {
	mysqlConfig *MySQLConfiguration
}

// GlobalVariableResourceModel describes the resource data model.
type GlobalVariableResourceModel struct {
	ID    types.String `tfsdk:"id"`
	Name  types.String `tfsdk:"name"`
	Value types.String `tfsdk:"value"`
}

func (r *GlobalVariableResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_global_variable"
}

func (r *GlobalVariableResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Global variable",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Global variable name",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"value": schema.StringAttribute{
				MarkdownDescription: "Global variable value",
				Required:            true,
			},
		},
	}
}

func (r *GlobalVariableResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *GlobalVariableResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	db, err := getDatabase(ctx, r.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed to connect MySQL", err.Error())
		return
	}

	var data *GlobalVariableResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := data.Name.ValueString()
	value := data.Value.ValueString()
	err = setGlobalVariable(ctx, db, name, value)
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Failed setting global variable (%s)", name), err.Error())
		return
	}

	data.ID = types.StringValue(name)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *GlobalVariableResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	db, err := getDatabase(ctx, r.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed to connect MySQL", err.Error())
		return
	}

	var data *GlobalVariableResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := data.Name.ValueString()
	sql := fmt.Sprintf(`SELECT @@GLOBAL.%s`, name)
	tflog.Info(ctx, sql)

	var value string
	err = db.QueryRowContext(ctx, sql).Scan(&value)
	if err != nil {
		resp.Diagnostics.AddWarning("Failed scanning MySQL rows", err.Error())
		resp.State.RemoveResource(ctx)
		return
	}
	data.ID = types.StringValue(name)
	data.Name = types.StringValue(name)
	data.Value = types.StringValue(value)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *GlobalVariableResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	db, err := getDatabase(ctx, r.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed to connect MySQL", err.Error())
		return
	}

	var data *GlobalVariableResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := data.Name.ValueString()
	value := data.Value.ValueString()
	err = setGlobalVariable(ctx, db, name, value)
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Failed setting global variable (%s)", name), err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *GlobalVariableResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	db, err := getDatabase(ctx, r.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed to connect MySQL", err.Error())
		return
	}

	var data *GlobalVariableResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := data.Name.ValueString()
	sql := fmt.Sprintf(`SET GLOBAL %s = DEFAULT`, name)
	tflog.Info(ctx, sql)
	_, err = db.ExecContext(ctx, sql)
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Failed resetting global variable (%s)", name), err.Error())
		return
	}
}

func (r *GlobalVariableResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

func setGlobalVariable(ctx context.Context, db *sql.DB, name, value string) error {
	var args []interface{}
	sql := fmt.Sprintf(`SET GLOBAL %s = ?`, name)
	if intValue, err := strconv.ParseInt(value, 10, 64); err == nil {
		args = append(args, intValue)
	} else if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
		args = append(args, floatValue)
	} else {
		args = append(args, value)
	}

	tflog.Info(ctx, sql, map[string]any{"args": args})

	_, err := db.ExecContext(ctx, sql, args...)
	if err != nil {
		return err
	}

	return nil
}
