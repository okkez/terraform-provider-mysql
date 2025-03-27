package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"

	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/okkez/terraform-provider-mysql/internal/utils"
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
	ID         types.String `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	Host       types.String `tfsdk:"host"`
	Lock       types.Bool   `tfsdk:"lock"`
	AuthOption types.Object `tfsdk:"auth_option"`
}

type AuthOptionModel struct {
	Plugin         types.String `tfsdk:"plugin"`
	AuthString     types.String `tfsdk:"auth_string"`
	RandomPassword types.Bool   `tfsdk:"random_password"`
}

var AuthOptionModelTypes = map[string]attr.Type{
	"plugin":          types.StringType,
	"auth_string":     types.StringType,
	"random_password": types.BoolType,
}

func (r *UserResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_user"
}

func (r *UserResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "The `mysql_user` resource creates and manages a user on a MySQL server.\n\n" +
			"~> **Note:** The password for the user is provided in plain text, and is obscured by an unsalted hash in the " +
			"state [Read more about sensitive data in state](https://www.terraform.io/language/state/sensitive-data). " +
			"Care is required when using this resource, to avoid disclosing the password.\n\n" +
			"~> **Note about random password:** The generated random password will be shown in the log immediately after running `terraform apply`. " +
			"Be sure to save the password, as there is no way to check it after that.",
		Attributes: map[string]schema.Attribute{
			"id":   utils.IDAttribute(),
			"name": utils.NameAttribute("user", true),
			"host": utils.HostAttribute("user", true),
			"lock": schema.BoolAttribute{
				MarkdownDescription: "Lock account if set to `true`. Defaults to `false`",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
		},
		Blocks: map[string]schema.Block{
			"auth_option": schema.SingleNestedBlock{
				MarkdownDescription: "Authentication configuration for the user",
				Attributes: map[string]schema.Attribute{
					"plugin": schema.StringAttribute{
						MarkdownDescription: "An authentication plugin name. " +
							"See MySQL Reference Manual [6.4.1 Authentication Plugins](https://dev.mysql.com/doc/refman/8.0/en/authentication-plugins.html) for more details. " +
							"Conflicts with `auth_string`, `random_password` if set `AWSAuthenticationPlugin`.",
						Optional: true,
					},
					"auth_string": schema.StringAttribute{
						MarkdownDescription: "Plain text password. Conflicts with `random_password`.",
						Optional:            true,
					},
					"random_password": schema.BoolAttribute{
						MarkdownDescription: "Generate random password when create user. Display generated password after creating user. Conflicts with `auth_string`.",
						Optional:            true,
					},
				},
			},
		},
	}
}

func (r *UserResource) ConfigValidators(ctx context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.Conflicting(
			path.MatchRoot("auth_option").AtName("auth_string"),
			path.MatchRoot("auth_option").AtName("random_password"),
		),
	}
}

func (r *UserResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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
	args = append(args, data.Name.ValueString())
	args = append(args, data.Host.ValueString())
	sql := `CREATE USER ?@?`
	if !data.AuthOption.IsNull() {
		var authOption *AuthOptionModel
		resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("auth_option"), &authOption)...)
		if authOption.Plugin.IsNull() {
			if authOption.RandomPassword.ValueBool() {
				sql += ` IDENTIFIED BY RANDOM PASSWORD`
			} else if !authOption.AuthString.IsNull() {
				sql += ` IDENTIFIED BY ?`
				args = append(args, authOption.AuthString.ValueString())
			} else {
				resp.Diagnostics.AddWarning("Could not add IDENTIFIED clause without plugin", "")
			}
		} else {
			plugin := authOption.Plugin.ValueString()
			if plugin == awsAuthenticationPlugin {
				sql += fmt.Sprintf(` IDENTIFIED WITH %s AS 'RDS'`, plugin)
			} else {
				sql += fmt.Sprintf(` IDENTIFIED WITH %s`, plugin)
				if authOption.RandomPassword.ValueBool() {
					sql += ` BY RANDOM PASSWORD`
				} else if !authOption.AuthString.IsNull() {
					sql += ` BY ?`
					args = append(args, authOption.AuthString.ValueString())
				}
			}
		}
	}
	if data.Lock.ValueBool() {
		sql += ` ACCOUNT LOCK`
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
		defer func() { _ = rows.Close() }()
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

	data.ID = types.StringValue(fmt.Sprintf("%s@%s", data.Name.ValueString(), data.Host.ValueString()))

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

	user := data.Name.ValueString()
	host := data.Host.ValueString()
	var args []interface{}
	args = append(args, host)
	args = append(args, user)

	sql := `
SELECT
  Host
, User
, plugin
, authentication_string
, account_locked
FROM
   mysql.user
WHERE
  Host = ?
  AND User = ?
`
	tflog.Info(ctx, sql, map[string]any{"args": args})
	var _host, _user, plugin, authString, accountLocked string
	if err = db.QueryRowContext(ctx, sql, args...).Scan(&_host, &_user, &plugin, &authString, &accountLocked); err != nil {
		resp.State.RemoveResource(ctx)
		return
	} else {
		data.Name = types.StringValue(user)
		data.Host = types.StringValue(host)
		data.Lock = types.BoolValue(accountLocked == "Y")

		if data.AuthOption.IsNull() {
			// See https://dev.mysql.com/doc/refman/8.0/en/server-system-variables.html#sysvar_default_authentication_plugin
			var defaultAuthenticationPlugin string
			if err := db.QueryRowContext(ctx, "SELECT @@default_authentication_plugin").Scan(&defaultAuthenticationPlugin); err != nil {
				resp.Diagnostics.AddError("Failed to fetching @@default_authentication_plugin", err.Error())
				return
			}
			tflog.Info(ctx, fmt.Sprintf("default_authentication_plugin=%s", defaultAuthenticationPlugin))
			if plugin != defaultAuthenticationPlugin {
				attributes := map[string]attr.Value{
					"plugin":          types.StringValue(plugin),
					"auth_string":     types.StringNull(),
					"random_password": types.BoolNull(),
				}
				data.AuthOption = types.ObjectValueMust(AuthOptionModelTypes, attributes)
			}
		} else {
			var authOption AuthOptionModel
			resp.Diagnostics.Append(data.AuthOption.As(ctx, &authOption, basetypes.ObjectAsOptions{})...)

			attributes := map[string]attr.Value{}
			attributes["plugin"] = types.StringNull()
			if !authOption.Plugin.IsNull() {
				attributes["plugin"] = types.StringValue(plugin)
			}
			attributes["auth_string"] = types.StringNull()
			if !authOption.AuthString.IsNull() {
				attributes["auth_string"] = authOption.AuthString
			}
			attributes["random_password"] = authOption.RandomPassword

			data.AuthOption = types.ObjectValueMust(AuthOptionModelTypes, attributes)
		}
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
	args = append(args, data.Name.ValueString())

	sql := `ALTER USER ?`
	if !data.Host.IsNull() {
		sql += `@?`
		args = append(args, data.Host.ValueString())
	}

	if !data.AuthOption.IsNull() {
		var authOption *AuthOptionModel
		resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("auth_option"), &authOption)...)
		if authOption.Plugin.IsNull() {
			if authOption.RandomPassword.ValueBool() {
				sql += ` IDENTIFIED BY RANDOM PASSWORD`
			} else if !authOption.AuthString.IsNull() {
				sql += ` IDENTIFIED BY ?`
				args = append(args, authOption.AuthString.ValueString())
			} else {
				resp.Diagnostics.AddWarning("Could not add IDENTIFIED clause without plugin", "")
			}
		} else {
			plugin := authOption.Plugin.ValueString()
			if plugin == awsAuthenticationPlugin {
				sql += fmt.Sprintf(` IDENTIFIED WITH %s AS 'RDS'`, plugin)
			} else {
				sql += fmt.Sprintf(` IDENTIFIED WITH %s`, plugin)
				if authOption.RandomPassword.ValueBool() {
					sql += ` BY RANDOM PASSWORD`
				} else if !authOption.AuthString.IsNull() {
					sql += ` BY ?`
					args = append(args, authOption.AuthString.ValueString())
				}
			}
		}
	}
	if data.Lock.ValueBool() {
		sql += ` ACCOUNT LOCK`
	} else {
		sql += ` ACCOUNT UNLOCK`
	}

	tflog.Info(ctx, sql, map[string]any{"args": args})
	rows, err := db.QueryContext(ctx, sql, args...)
	if err != nil {
		resp.Diagnostics.AddError("Failed creating user", err.Error())
	}
	defer func() { _ = rows.Close() }()
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

	user := data.Name.ValueString()
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
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), types.StringValue(userHost[0]))...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("host"), types.StringValue(userHost[1]))...)
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
