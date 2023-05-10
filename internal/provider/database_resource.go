package provider

import (
	"context"
	"fmt"

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
var _ resource.Resource = &databaseResource{}
var _ resource.ResourceWithImportState = &databaseResource{}

func NewDatabaseResource() resource.Resource {
	return &databaseResource{}
}

// databaseResource defines the resource implementation.
type databaseResource struct {
	mysqlConfig *MySQLConfiguration
}

// databaseResourceModel describes the resource data model.
type databaseResourceModel struct {
	Id                  types.String `tfsdk:"id"`
	Name                types.String `tfsdk:"name"`
	DefaultCharacterSet types.String `tfsdk:"default_character_set"`
	DefaultCollation    types.String `tfsdk:"default_collation"`
}

func (r *databaseResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_database"
}

func (r *databaseResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The `mysql_database` resource creates and manages a database.",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "The database name.",
				Required:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"default_character_set": schema.StringAttribute{
				MarkdownDescription: "The default character set. Defaults to `utf8mb4`.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("utf8mb4"),
			},
			"default_collation": schema.StringAttribute{
				MarkdownDescription: "The default collation. Defaults to `utf8mb4_0900_ai_ci`.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("utf8mb4_0900_ai_ci"),
			},
		},
	}
}

func (r *databaseResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *databaseResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	db, err := getDatabase(ctx, r.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed to connect MySQL", err.Error())
		return
	}

	var data *databaseResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	database, _ := quoteIdentifier(ctx, db, data.Name.ValueString())
	sql := fmt.Sprintf("CREATE DATABASE %s", database)
	var args []interface{}
	if !data.DefaultCharacterSet.IsNull() {
		sql += " CHARACTER SET ?"
		args = append(args, data.DefaultCharacterSet.ValueString())
	}
	if !data.DefaultCollation.IsNull() {
		sql += " COLLATE ?"
		args = append(args, data.DefaultCollation.ValueString())
	}
	tflog.Info(ctx, sql, map[string]any{"args": args})

	_, err = db.ExecContext(ctx, sql, args...)
	if err != nil {
		resp.Diagnostics.AddError("Failed creating DB", err.Error())
		return
	}
	tflog.Trace(ctx, "created a resource")

	data.Id = data.Name
	resp.Diagnostics.Append(resp.State.Set(ctx, data)...)
}

func (r *databaseResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	db, err := getDatabase(ctx, r.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed to connect MySQL", err.Error())
		return
	}
	var data *databaseResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var characterSet, collation string
	sql := "SELECT DEFAULT_CHARACTER_SET_NAME, DEFAULT_COLLATION_NAME FROM INFORMATION_SCHEMA.SCHEMATA WHERE SCHEMA_NAME = ?"
	err = db.QueryRowContext(ctx, sql, data.Id.ValueString()).Scan(&characterSet, &collation)
	if err != nil {
		resp.Diagnostics.AddError("Failed executing query", err.Error())
		return
	}

	data.Name = data.Id
	data.DefaultCharacterSet = types.StringValue(characterSet)
	data.DefaultCollation = types.StringValue(collation)

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *databaseResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	db, err := getDatabase(ctx, r.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed to connect MySQL", err.Error())
		return
	}
	var data, state *databaseResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	database, _ := quoteIdentifier(ctx, db, data.Name.ValueString())
	sql := fmt.Sprintf("ALTER DATABASE %s", database)
	var args []interface{}
	if !data.DefaultCharacterSet.Equal(state.DefaultCharacterSet) {
		sql += " CHARACTER SET ?"
		args = append(args, data.DefaultCharacterSet.ValueString())
	}
	if !data.DefaultCollation.Equal(state.DefaultCollation) {
		sql += " COLLATE ?"
		args = append(args, data.DefaultCollation.ValueString())
	}
	tflog.Info(ctx, sql, map[string]any{"args": args})

	_, err = db.ExecContext(ctx, sql, args...)
	if err != nil {
		resp.Diagnostics.AddError("Failed updating DB", err.Error())
		return
	}

	data.Id = data.Name

	// Save updated data into Terraform state
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *databaseResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	db, err := getDatabase(ctx, r.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed to connect MySQL", err.Error())
		return
	}
	var data *databaseResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	database, _ := quoteIdentifier(ctx, db, data.Name.ValueString())
	sql := fmt.Sprintf("DROP DATABASE %s", database)
	tflog.Info(ctx, sql)

	_, err = db.ExecContext(ctx, sql)
	if err != nil {
		resp.Diagnostics.AddError("Failed deleting DB", err.Error())
		return
	}
}

func (r *databaseResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
