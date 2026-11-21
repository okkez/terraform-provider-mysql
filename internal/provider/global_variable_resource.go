package provider

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"regexp"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
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
		MarkdownDescription: "The `mysql_global_variable` resource manages a global variable.\n\n" +
			"~> **Note on MySQL: ** MySQL global variables art [not persistent](https://dev.mysql.com/doc/refman/8.0/en/set-variable.html).\n\n" +
			"~> **Note about `destroy`: ** `destroy` will try setting `DEFAULT` value for the global variable. Unfortunately not every variable support this.",

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
	resp.Diagnostics.Append(applyGlobalVariable(ctx, db, resp.Private, name, value)...)
	if resp.Diagnostics.HasError() {
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
	value, err := readGlobalVariable(ctx, db, name)
	if err != nil {
		if mysqlErrorNumber(err) == unknownSystemVariableErrorNumber {
			// The variable does not exist on the server (e.g. a plugin variable after
			// the plugin was uninstalled), so remove it from the state.
			resp.Diagnostics.AddWarning(fmt.Sprintf("Unknown system variable (%s)", name), err.Error())
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError(fmt.Sprintf("Failed reading global variable (%s)", name), err.Error())
		return
	}
	data.ID = types.StringValue(name)
	data.Name = types.StringValue(name)

	// MySQL reports the value in its own form, such as `1` for `ON`. While the server still has
	// the value it stored on apply, keep the configured form in the state. Otherwise the value
	// was changed outside of Terraform, so report it as it is.
	appliedValue, saved, diags := loadPrivateString(ctx, req.Private, appliedValuePrivateKey)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !saved || value != appliedValue {
		data.Value = types.StringValue(value)
		// The state now holds the server value, so the applied value no longer tells the
		// configured form. Keeping it would hide a later change back to the applied value.
		resp.Diagnostics.Append(resp.Private.SetKey(ctx, appliedValuePrivateKey, nil)...)
	}

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
	resp.Diagnostics.Append(applyGlobalVariable(ctx, db, resp.Private, name, value)...)
	if resp.Diagnostics.HasError() {
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

func readGlobalVariable(ctx context.Context, db *sql.DB, name string) (string, error) {
	sql := fmt.Sprintf(`SELECT @@GLOBAL.%s`, name)
	tflog.Info(ctx, sql)

	var value string
	err := db.QueryRowContext(ctx, sql).Scan(&value)
	return value, err
}

// appliedValuePrivateKey is the private state key holding the value MySQL reported right
// after the configured value was applied, which is the form MySQL normalizes the value to.
const appliedValuePrivateKey = "applied_value"

// applyGlobalVariable sets the configured value and saves the value MySQL stored for it in the
// private state, so that Read can tell the form MySQL normalizes the configured value to from
// a change made outside of Terraform. Create and Update must both go through it.
func applyGlobalVariable(ctx context.Context, db *sql.DB, private privateStateSetter, name, configuredValue string) diag.Diagnostics {
	var diags diag.Diagnostics

	if err := setGlobalVariable(ctx, db, name, configuredValue); err != nil {
		diags.AddError(fmt.Sprintf("Failed setting global variable (%s)", name), err.Error())
		return diags
	}

	appliedValue, err := readGlobalVariable(ctx, db, name)
	if err != nil {
		// The value is already applied, so do not fail. Without the saved value, Read reports
		// the value MySQL has, which shows a diff once if it is written differently.
		diags.AddWarning(fmt.Sprintf("Could not read back global variable (%s)", name), err.Error())
		diags.Append(private.SetKey(ctx, appliedValuePrivateKey, nil)...)
		return diags
	}

	diags.Append(warnIfStoredDifferently(name, configuredValue, appliedValue)...)
	diags.Append(savePrivateString(ctx, private, appliedValuePrivateKey, appliedValue)...)
	return diags
}

// warnIfStoredDifferently warns when MySQL stored a value which does not look like the
// configured one, such as a value capped at its maximum, as the state keeps the configured one.
func warnIfStoredDifferently(name, configuredValue, appliedValue string) diag.Diagnostics {
	var diags diag.Diagnostics
	if !globalVariableValueEquivalent(configuredValue, appliedValue) {
		diags.AddWarning(
			fmt.Sprintf("MySQL stored a different value for global variable (%s)", name),
			fmt.Sprintf("MySQL stored %q for the configured value %q, for example because the value is out of range. "+
				"Terraform keeps the configured value in the state.", appliedValue, configuredValue))
	}
	return diags
}

// wordListPattern matches a value made only of words separated by commas, such as an enum
// value like `READ-COMMITTED` or `sql_mode`.
var wordListPattern = regexp.MustCompile(`^[A-Za-z0-9_,-]*$`)

// globalVariableValueEquivalent guesses whether MySQL stored the configured value only in its
// own form. It treats `ON`/`TRUE` as `1` and `OFF`/`FALSE` as `0`, and compares comma separated
// words ignoring letter case and order. The guess is loose, so it decides only whether to warn
// about a value MySQL changed, never whether a diff is reported.
func globalVariableValueEquivalent(a, b string) bool {
	if a == b {
		return true
	}
	if normalizeBooleanValue(a) == normalizeBooleanValue(b) {
		return true
	}
	if !wordListPattern.MatchString(a) || !wordListPattern.MatchString(b) {
		return false
	}
	return maps.Equal(upperWordSet(a), upperWordSet(b))
}

func normalizeBooleanValue(value string) string {
	switch strings.ToUpper(value) {
	case "ON", "TRUE":
		return "1"
	case "OFF", "FALSE":
		return "0"
	}
	return value
}

func upperWordSet(value string) map[string]struct{} {
	words := map[string]struct{}{}
	for _, word := range strings.Split(strings.ToUpper(value), ",") {
		words[word] = struct{}{}
	}
	return words
}
