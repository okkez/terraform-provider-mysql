package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	// "github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Ensure provider defined types fully satisfy framework interfaces.
var (
	_ resource.Resource                = &UserResource{}
	_ resource.ResourceWithConfigure   = &UserResource{}
	_ resource.ResourceWithImportState = &UserResource{}
)

const (
	awsAuthenticationPlugin = "AWSAuthenticationPlugin"
)

func NewUserResource() resource.Resource {
	return &UserResource{}
}

// UserResource defines the resource implementation.
type UserResource struct {
	mysqlConfig *MySQLConfiguration
}

// UserResourceModel describes the resource data model.
type UserResourceModel struct {
	ID           types.String `tfsdk:"id"`
	User         types.String `tfsdk:"user"`
	Host         types.String `tfsdk:"host"`
	AuthOption   types.Object `tfsdk:"auth_option"`
}

type AuthOption struct {
	Plugin         types.String `tfsdk:"plugin"`
	AuthString     types.String `tfsdk:"auth_string"`
	RandomPassword types.Bool   `tfsdk:"random_password"`
}

type RoleModel struct {
	Name types.String `tfsdk:"name"`
	Host types.String `tfsdk:"host"`
}

var RoleTypes = map[string]attr.Type{
	"name": types.StringType,
	"host": types.StringType,
}

func (r *UserResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (r *UserResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "user resource",

		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "user identifier",
				Computed:            true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"user": schema.StringAttribute{
				MarkdownDescription: "",
				Required:            true,
				Validators: []validator.String{
					stringvalidator.LengthAtMost(32),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"host": schema.StringAttribute{
				MarkdownDescription: "",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("%"),
				Validators: []validator.String{
					stringvalidator.LengthAtMost(255),
				},
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"auth_option": schema.SingleNestedAttribute{
				MarkdownDescription: "",
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"plugin": schema.StringAttribute{
						MarkdownDescription: "",
						Optional:            true,
					},
					"auth_string": schema.StringAttribute{
						MarkdownDescription: "",
						Optional:            true,
					},
					"random_password": schema.BoolAttribute{
						MarkdownDescription: "",
						Optional:            true,
					},
				},
			},
		},
	}
}

func (r *UserResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		// resourcevalidator.Conflicting(
		// 	path.MatchRoot("auth_option.auth_string"),
		// 	path.MatchRoot("auth_option.random_password"),
		// ),
	}
}

func (r *UserResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Prevent panic if the provider has not been configured.
	if req.ProviderData == nil {
		return
	}

	r.mysqlConfig = req.ProviderData.(*MySQLConfiguration)
}

func (r *UserResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	db, err := getDatabase(ctx, r.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed to connect MySQL", err.Error())
		return
	}

	var data *UserResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	callExec := true
	var args []interface{}
	args = append(args, data.User.ValueString())
	args = append(args, data.Host.ValueString())
	sql := `CREATE USER ?@?`
	if !data.AuthOption.IsNull() {
		var authOption *AuthOption
		resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("auth_option"), &authOption)...)
		if !authOption.Plugin.IsNull() {
			plugin := authOption.Plugin.ValueString()
			if !authOption.AuthString.IsNull() {
				if plugin == awsAuthenticationPlugin {
					sql += fmt.Sprintf(` IDENTIFIED WITH %s AS 'RDS'`, plugin)
				} else {
					sql += fmt.Sprintf(` IDENTIFIED WITH %s BY ?`, plugin)
					args = append(args, authOption.AuthString.ValueString())
				}
			} else if authOption.RandomPassword.ValueBool() {
				sql += fmt.Sprintf(` IDENTIFIED WITH %s BY RANDOM PASSWORD`, plugin)
			}
		} else {
			if !authOption.AuthString.IsNull() {
				sql += ` IDENTIFIED BY ?`
				args = append(args, authOption.AuthString.ValueString())
			} else if authOption.RandomPassword.ValueBool() {
				sql += ` IDENTIFIED BY RANDOM PASSWORD`
				callExec = false
			}
		}
	}

	tflog.Info(ctx, sql, map[string]any{"args": args})
	if callExec {
		_, err = db.ExecContext(ctx, sql, args...)
		if err != nil {
			resp.Diagnostics.AddError("Failed creating user", err.Error())
		}
	} else {
		rows, err := db.QueryContext(ctx, sql, args...)
		if err != nil {
			resp.Diagnostics.AddError("Failed creating user", err.Error())
		}
		defer rows.Close()
		for rows.Next() {
			var _host, _user, generatedPassword, _authFactor string
			if err = rows.Scan(&_host, &_user, &generatedPassword, &_authFactor); err != nil {
				resp.Diagnostics.AddError("Failed scanning MySQL rows", err.Error())
				return
			}
			resp.Diagnostics.AddWarning(
				fmt.Sprintf("Generated password: %s", generatedPassword),
				"The generated password is not saved in tfstate")
		}
	}

	data.ID = types.StringValue(fmt.Sprintf("%s@%s", data.User.ValueString(), data.Host.ValueString()))

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *UserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	db, err := getDatabase(ctx, r.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed to connect MySQL", err.Error())
		return
	}

	var data *UserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	user := data.User.ValueString()
	host := data.Host.ValueString()
	var args []interface{}
	args = append(args, host)
	args = append(args, user)

	sql := `SELECT Host, User, plugin, authentication_string FROM mysql.user WHERE Host = ? AND User = ?`
	tflog.Info(ctx, sql, map[string]any{"args": args})
	var _host, _user, plugin, authString string
	if err = db.QueryRowContext(ctx, sql, args...).Scan(&_host, &_user, &plugin, &authString); err != nil {
		resp.State.RemoveResource(ctx)
		return
	} else {
		data.User = types.StringValue(user)
		data.Host = types.StringValue(host)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *UserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	db, err := getDatabase(ctx, r.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed to connect MySQL", err.Error())
		return
	}

	var data, state *UserResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var args []interface{}
	args = append(args, data.User.ValueString())

	sql := `ALTER USER ?`
	if !data.Host.IsNull() {
		sql += `@?`
		args = append(args, data.Host.ValueString())
	}

	if !data.AuthOption.IsNull() {
		var authOption *AuthOption
		resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("auth_option"), &authOption)...)
		if !authOption.Plugin.IsNull() {
			plugin := authOption.Plugin.ValueString()
			if !authOption.AuthString.IsNull() {
				if plugin == awsAuthenticationPlugin {
					sql += fmt.Sprintf(` IDENTIFIED WITH %s AS 'RDS'`, plugin)
				} else {
					sql += fmt.Sprintf(` IDENTIFIED WITH %s BY ?`, plugin)
					args = append(args, authOption.AuthString.ValueString())
				}
			} else if authOption.RandomPassword.ValueBool() {
				sql += fmt.Sprintf(` IDENTIFIED WITH %s BY RANDOM PASSWORD`, plugin)
			}
		} else {
			if !authOption.AuthString.IsNull() {
				sql += ` IDENTIFIED BY ?`
				args = append(args, authOption.AuthString.ValueString())
			} else if authOption.RandomPassword.ValueBool() {
				sql += ` IDENTIFIED BY RANDOM PASSWORD`
			}
		}
	}

	tflog.Info(ctx, sql, map[string]any{"args": args})
	rows, err := db.QueryContext(ctx, sql, args...)
	if err != nil {
		resp.Diagnostics.AddError("Failed creating user", err.Error())
	}
	defer rows.Close()
	for rows.Next() {
		var _host, _user, generatedPassword, _authFactor string
		if err = rows.Scan(&_host, &_user, &generatedPassword, &_authFactor); err != nil {
			resp.Diagnostics.AddError("Failed scanning MySQL rows", err.Error())
			return
		}
		resp.Diagnostics.AddWarning(
			fmt.Sprintf("Generated password: %s", generatedPassword),
			"The generated password is not saved in tfstate")
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *UserResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	db, err := getDatabase(ctx, r.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed to connect MySQL", err.Error())
		return
	}

	var data *UserResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	user := data.User.ValueString()
	host := data.Host.ValueString()

	sql := `DROP USER ?@?`
	var args []interface{}
	args = append(args, user)
	args = append(args, host)
	tflog.Info(ctx, sql, map[string]any{"args": args})

	_, err = db.ExecContext(ctx, sql, args...)
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Failed deleting user (%s@%s)", args...), err.Error())
		return
	}
}

func (r *UserResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	userHost := strings.SplitN(req.ID, "@", 2)
	if len(userHost) != 2 {
		resp.Diagnostics.AddAttributeError(path.Root("id"), fmt.Sprintf("Invalid ID format. %s", req.ID), "The valid ID format is `name@host`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("user"), types.StringValue(userHost[0]))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("host"), types.StringValue(userHost[1]))...)
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
