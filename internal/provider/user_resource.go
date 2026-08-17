package provider

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/go-version"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
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
	_ resource.Resource                   = &UserResource{}
	_ resource.ResourceWithConfigure      = &UserResource{}
	_ resource.ResourceWithImportState    = &UserResource{}
	_ resource.ResourceWithValidateConfig = &UserResource{}
)

const (
	awsAuthenticationPlugin = "AWSAuthenticationPlugin"
)

// dualPasswordMinVersion is the minimum MySQL version that supports dual password.
// The dual password support was added in MySQL 8.0.14, while this provider supports MySQL 8.0 or later.
// See https://dev.mysql.com/doc/refman/8.0/en/password-management.html#dual-passwords for more details.
var dualPasswordMinVersion = version.Must(version.NewVersion("8.0.14"))

// supportsDualPassword reports whether the server version supports dual password.
// `@@GLOBAL.version` often carries a suffix such as `-log` with binary logging enabled,
// `-commercial`, or a distribution specific one. `go-version` treats the suffix as a
// prerelease, which sorts below the release itself, so compare only the core version.
func supportsDualPassword(currentVersion *version.Version) bool {
	return !currentVersion.Core().LessThan(dualPasswordMinVersion)
}

// secondaryPasswordExpression returns the SQL expression reporting whether an account has a
// secondary password. `mysql.user.User_attributes` exists only from MySQL 8.0.14, the version
// which added dual password, so an earlier server never has a secondary password and the column
// must not be referenced at all to avoid ER_BAD_FIELD_ERROR.
func secondaryPasswordExpression(currentVersion *version.Version) string {
	if !supportsDualPassword(currentVersion) {
		return `FALSE`
	}
	return `JSON_CONTAINS_PATH(User_attributes, 'one', '$.additional_password') IS TRUE`
}

// checkDualPasswordSupport reports a clear error instead of letting MySQL fail with a syntax error.
func checkDualPasswordSupport(currentVersion *version.Version) error {
	if !supportsDualPassword(currentVersion) {
		return fmt.Errorf("dual password requires MySQL %s or later, but the server version is %s", dualPasswordMinVersion, currentVersion)
	}
	return nil
}

func NewUserResource() resource.Resource {
	return &UserResource{}
}

// UserResource defines the resource implementation.
type UserResource struct {
	mysqlConfig *MySQLConfiguration
}

// UserResourceModel describes the resource data model.
type UserResourceModel struct {
	ID                   types.String `tfsdk:"id"`
	Name                 types.String `tfsdk:"name"`
	Host                 types.String `tfsdk:"host"`
	Lock                 types.Bool   `tfsdk:"lock"`
	HasSecondaryPassword types.Bool   `tfsdk:"has_secondary_password"`
	AuthOption           types.Object `tfsdk:"auth_option"`
}

type AuthOptionModel struct {
	Plugin                types.String `tfsdk:"plugin"`
	AuthString            types.String `tfsdk:"auth_string"`
	RandomPassword        types.Bool   `tfsdk:"random_password"`
	RetainCurrentPassword types.Bool   `tfsdk:"retain_current_password"`
	DiscardOldPassword    types.Bool   `tfsdk:"discard_old_password"`
}

var AuthOptionModelTypes = map[string]attr.Type{
	"plugin":                  types.StringType,
	"auth_string":             types.StringType,
	"random_password":         types.BoolType,
	"retain_current_password": types.BoolType,
	"discard_old_password":    types.BoolType,
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
			"Be sure to save the password, as there is no way to check it after that.\n\n" +
			"!> **Warning about dual password:** `discard_old_password` is the only way to invalidate a password retained by " +
			"`retain_current_password`. Removing `retain_current_password` from the configuration is **not** equivalent to discarding it, " +
			"because MySQL keeps the secondary password until `DISCARD OLD PASSWORD` is issued. " +
			"An abandoned rotation therefore leaves the old password valid forever. " +
			"Use `has_secondary_password` to detect an account which still has a secondary password.",
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
			"has_secondary_password": schema.BoolAttribute{
				MarkdownDescription: "Whether the account currently has a secondary password, read from `mysql.user.User_attributes`. " +
					"Unlike `retain_current_password` and `discard_old_password`, which are write-only options for `ALTER USER`, " +
					"this reports the state of the account on the server, so it can be used to detect a rotation which was never " +
					"finished with `discard_old_password`. Always `false` on MySQL earlier than 8.0.14, which has no dual password support.",
				Computed: true,
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
					"retain_current_password": schema.BoolAttribute{
						MarkdownDescription: "Keep the current password as the secondary password when changing the password. Requires MySQL 8.0.14 or later. " +
							"See MySQL Reference Manual [8.2.15 Password Management](https://dev.mysql.com/doc/refman/8.0/en/password-management.html#dual-passwords) for more details. " +
							"This option is ignored when creating a user because `CREATE USER` does not accept `RETAIN CURRENT PASSWORD`. " +
							"Cannot be true at the same time as `discard_old_password`.",
						Optional: true,
					},
					"discard_old_password": schema.BoolAttribute{
						MarkdownDescription: "Discard the secondary password. Requires MySQL 8.0.14 or later. " +
							"See MySQL Reference Manual [8.2.15 Password Management](https://dev.mysql.com/doc/refman/8.0/en/password-management.html#dual-passwords) for more details. " +
							"This option is ignored when creating a user because a new user has no secondary password. " +
							"Cannot be true at the same time as `retain_current_password`.",
						Optional: true,
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

// ValidateConfig rejects only the combination of `retain_current_password` and `discard_old_password` being true.
// `resourcevalidator.Conflicting` cannot be used here because it rejects an explicit `false`,
// which is common when the value comes from an expression.
func (r *UserResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var data *UserResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() || data.AuthOption.IsNull() || data.AuthOption.IsUnknown() {
		return
	}

	var authOption AuthOptionModel
	resp.Diagnostics.Append(data.AuthOption.As(ctx, &authOption, basetypes.ObjectAsOptions{})...)
	if resp.Diagnostics.HasError() {
		return
	}

	if authOption.RetainCurrentPassword.ValueBool() && authOption.DiscardOldPassword.ValueBool() {
		resp.Diagnostics.AddAttributeError(
			path.Root("auth_option").AtName("discard_old_password"),
			"Invalid Attribute Combination",
			"`retain_current_password` and `discard_old_password` cannot be true at the same time.")
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
		if resp.Diagnostics.HasError() {
			return
		}
		if authOption.RetainCurrentPassword.ValueBool() || authOption.DiscardOldPassword.ValueBool() {
			resp.Diagnostics.AddWarning(
				"Ignored dual password options on creating user",
				"`retain_current_password` and `discard_old_password` are available only in `ALTER USER`.")
		}
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
			return
		}
	} else {
		rows, err := db.QueryContext(ctx, sql, args...)
		if err != nil {
			resp.Diagnostics.AddError("Failed creating user", err.Error())
			return
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
	// `CREATE USER` does not accept `RETAIN CURRENT PASSWORD`, so a new user never has one.
	data.HasSecondaryPassword = types.BoolValue(false)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *UserResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	db, err := getDatabase(ctx, r.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed to connect MySQL", err.Error())
		return
	}

	currentVersion, err := getDatabaseVersion(ctx, r.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed getting the server version", err.Error())
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

	query := fmt.Sprintf(`
SELECT
  Host
, User
, plugin
, authentication_string
, account_locked
, %s
FROM
   mysql.user
WHERE
  Host = ?
  AND User = ?
`, secondaryPasswordExpression(currentVersion))
	tflog.Info(ctx, query, map[string]any{"args": args})
	var _host, _user, plugin, authString, accountLocked string
	var hasSecondaryPassword bool
	if err = db.QueryRowContext(ctx, query, args...).Scan(&_host, &_user, &plugin, &authString, &accountLocked, &hasSecondaryPassword); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// The user is gone on the server, so remove it from the state.
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(fmt.Sprintf("Failed reading user (%s@%s)", user, host), err.Error())
		return
	}
	data.Name = types.StringValue(user)
	data.Host = types.StringValue(host)
	data.HasSecondaryPassword = types.BoolValue(hasSecondaryPassword)
	data.Lock = types.BoolValue(accountLocked == "Y")

	if data.AuthOption.IsNull() {
		// https://dev.mysql.com/doc/refman/8.4/en/native-pluggable-authentication.html
		// The mysql_native_password authentication plugin is deprecated as of MySQL 8.0.34, disabled by default in MySQL 8.4,
		// and removed as of MySQL 9.0.0.
		// See https://dev.mysql.com/doc/refman/8.0/en/server-system-variables.html#sysvar_default_authentication_plugin
		var defaultAuthenticationPlugin string
		err := db.QueryRowContext(ctx, "SELECT @@default_authentication_plugin").Scan(&defaultAuthenticationPlugin)
		if err != nil {
			if mysqlErrorNumber(err) == unknownSystemVariableErrorNumber {
				// ER_UNKNOWN_SYSTEM_VARIABLE: For MySQL 8.4+ where default_authentication_plugin is removed
				// Default authentication plugin is caching_sha2_password
				defaultAuthenticationPlugin = "caching_sha2_password"
				tflog.Info(ctx, fmt.Sprintf("Using hardcoded default plugin for MySQL 8.4+: %s", defaultAuthenticationPlugin))
			} else {
				// Other database errors should be surfaced
				resp.Diagnostics.AddError("Failed to query default authentication plugin", err.Error())
				return
			}
		} else {
			tflog.Info(ctx, fmt.Sprintf("default_authentication_plugin=%s", defaultAuthenticationPlugin))
		}
		if plugin != defaultAuthenticationPlugin {
			attributes := map[string]attr.Value{
				"plugin":                  types.StringValue(plugin),
				"auth_string":             types.StringNull(),
				"random_password":         types.BoolNull(),
				"retain_current_password": types.BoolNull(),
				"discard_old_password":    types.BoolNull(),
			}
			data.AuthOption = types.ObjectValueMust(AuthOptionModelTypes, attributes)
		}
	} else {
		var authOption AuthOptionModel
		resp.Diagnostics.Append(data.AuthOption.As(ctx, &authOption, basetypes.ObjectAsOptions{})...)
		if resp.Diagnostics.HasError() {
			return
		}

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
		// The dual password options are write-only options for `ALTER USER`, so keep the configured values.
		attributes["retain_current_password"] = authOption.RetainCurrentPassword
		attributes["discard_old_password"] = authOption.DiscardOldPassword

		data.AuthOption = types.ObjectValueMust(AuthOptionModelTypes, attributes)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (r *UserResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	db, err := getDatabase(ctx, r.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed to connect MySQL", err.Error())
		return
	}

	currentVersion, err := getDatabaseVersion(ctx, r.mysqlConfig)
	if err != nil {
		resp.Diagnostics.AddError("Failed getting the server version", err.Error())
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

	discardOldPassword := false
	if !data.AuthOption.IsNull() {
		var authOption *AuthOptionModel
		resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("auth_option"), &authOption)...)
		if resp.Diagnostics.HasError() {
			return
		}
		discardOldPassword = authOption.DiscardOldPassword.ValueBool()
		// `ValidateConfig` sees an unknown value as false, so repeat the check on the resolved
		// values. Without it the statement would retain the current password and the following
		// `DISCARD OLD PASSWORD` would immediately discard it, with no error and no warning.
		if authOption.RetainCurrentPassword.ValueBool() && discardOldPassword {
			resp.Diagnostics.AddError(
				"Conflicting dual password options",
				"`retain_current_password` and `discard_old_password` cannot be true at the same time. "+
					"Applying both would retain the current password and discard it right away.")
			return
		}
		// `RETAIN CURRENT PASSWORD` is available only when the statement changes the password.
		passwordChanged := false
		if authOption.Plugin.IsNull() {
			if authOption.RandomPassword.ValueBool() {
				sql += ` IDENTIFIED BY RANDOM PASSWORD`
				passwordChanged = true
			} else if !authOption.AuthString.IsNull() {
				sql += ` IDENTIFIED BY ?`
				args = append(args, authOption.AuthString.ValueString())
				passwordChanged = true
			} else if !discardOldPassword {
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
					passwordChanged = true
				} else if !authOption.AuthString.IsNull() {
					sql += ` BY ?`
					args = append(args, authOption.AuthString.ValueString())
					passwordChanged = true
				}
			}
		}
		retainCurrentPassword := authOption.RetainCurrentPassword.ValueBool() && passwordChanged
		if authOption.RetainCurrentPassword.ValueBool() && !passwordChanged {
			resp.Diagnostics.AddWarning(
				"Ignored retain_current_password",
				"`retain_current_password` requires changing the password. Set `auth_string` or `random_password` to retain the current password.")
		}
		// The server version is checked where a dual password statement is actually issued, so
		// that an ignored `retain_current_password` does not fail on a server which never sees it.
		if retainCurrentPassword || discardOldPassword {
			if err := checkDualPasswordSupport(currentVersion); err != nil {
				resp.Diagnostics.AddError("Could not use dual password", err.Error())
				return
			}
		}
		if retainCurrentPassword {
			sql += ` RETAIN CURRENT PASSWORD`
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
		resp.Diagnostics.AddError("Failed updating user", err.Error())
		return
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
	// `DISCARD OLD PASSWORD` cannot be combined with `IDENTIFIED BY` in a single statement.
	var discardErr error
	if discardOldPassword {
		_ = rows.Close()
		discardErr = alterUserDiscardOldPassword(ctx, db, data)
	}

	// The secondary password is read back because it survives a password change which does not
	// specify `RETAIN CURRENT PASSWORD`, so it cannot be derived from this update alone.
	if hasSecondaryPassword, err := readHasSecondaryPassword(ctx, db, currentVersion, data); err == nil {
		data.HasSecondaryPassword = types.BoolValue(hasSecondaryPassword)
	} else {
		// Failing here would throw away the state although `ALTER USER` is already committed,
		// which is the very thing saving the state below avoids. Keep the recorded value, which
		// the next successful read corrects, and report the failure as a warning instead.
		resp.Diagnostics.AddWarning(
			"Could not read the secondary password state",
			fmt.Sprintf("Keeping the value recorded in the state for `has_secondary_password`: %v", err))
		// `ValueBool` turns the null of a state written before this attribute existed into false,
		// which a condition such as `!has_secondary_password` would otherwise fail to evaluate.
		data.HasSecondaryPassword = types.BoolValue(state.HasSecondaryPassword.ValueBool())
	}

	if discardErr != nil {
		// MySQL DDL is not transactional, so the `ALTER USER` above is already committed.
		// Save the state to keep it in sync with the server before reporting the error.
		partialState, diags := stateWithoutDiscardOldPassword(data)
		resp.Diagnostics.Append(diags...)
		resp.Diagnostics.Append(resp.State.Set(ctx, partialState)...)
		resp.Diagnostics.AddError("Failed discarding old password", discardErr.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// readHasSecondaryPassword reports whether the account currently has a secondary password.
// `ALTER USER` does not report it, so it has to be read back after updating the user.
func readHasSecondaryPassword(ctx context.Context, db *sql.DB, currentVersion *version.Version, data *UserResourceModel) (bool, error) {
	if !supportsDualPassword(currentVersion) {
		return false, nil
	}

	var args []interface{}
	args = append(args, data.Name.ValueString())
	args = append(args, data.Host.ValueString())

	query := fmt.Sprintf(`SELECT %s FROM mysql.user WHERE User = ? AND Host = ?`, secondaryPasswordExpression(currentVersion))
	tflog.Info(ctx, query, map[string]any{"args": args})
	var hasSecondaryPassword bool
	if err := db.QueryRowContext(ctx, query, args...).Scan(&hasSecondaryPassword); err != nil {
		return false, err
	}
	return hasSecondaryPassword, nil
}

// stateWithoutDiscardOldPassword returns a copy of `data` with `discard_old_password` unset.
// It is used when `DISCARD OLD PASSWORD` fails after the preceding `ALTER USER` succeeded.
// Keeping the attribute unset leaves a diff against the configuration, so that the next
// apply retries the discard instead of reporting no changes while the secondary password
// is still alive on the server.
func stateWithoutDiscardOldPassword(data *UserResourceModel) (*UserResourceModel, diag.Diagnostics) {
	var diags diag.Diagnostics
	if data.AuthOption.IsNull() || data.AuthOption.IsUnknown() {
		return data, diags
	}

	attributes := make(map[string]attr.Value, len(AuthOptionModelTypes))
	for name, value := range data.AuthOption.Attributes() {
		attributes[name] = value
	}
	attributes["discard_old_password"] = types.BoolNull()

	authOption, d := types.ObjectValue(AuthOptionModelTypes, attributes)
	diags.Append(d...)
	if diags.HasError() {
		return data, diags
	}

	partialState := *data
	partialState.AuthOption = authOption
	return &partialState, diags
}

func alterUserDiscardOldPassword(ctx context.Context, db *sql.DB, data *UserResourceModel) error {
	var args []interface{}
	args = append(args, data.Name.ValueString())

	query := `ALTER USER ?`
	if !data.Host.IsNull() {
		query += `@?`
		args = append(args, data.Host.ValueString())
	}
	query += ` DISCARD OLD PASSWORD`

	tflog.Info(ctx, query, map[string]any{"args": args})
	_, err := db.ExecContext(ctx, query, args...)
	return err
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
