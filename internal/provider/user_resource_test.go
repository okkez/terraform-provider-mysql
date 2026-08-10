package provider

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/okkez/terraform-provider-mysql/internal/utils"
)

// TestSupportsDualPassword checks the version comparison against the version strings
// MySQL actually reports. `@@GLOBAL.version` often carries a suffix such as `-log`,
// which `go-version` treats as a prerelease and sorts below the release itself.
func TestSupportsDualPassword(t *testing.T) {
	t.Parallel()
	tests := []struct {
		versionString string
		want          bool
	}{
		{"5.7.44", false},
		{"8.0.13", false},
		{"8.0.13-log", false},
		{"8.0.14", true},
		{"8.0.14-log", true},
		{"8.0.14-commercial", true},
		{"8.0.14-0ubuntu0.22.04.1", true},
		{"8.0.39", true},
		{"8.0.36-28", true},
		{"8.4.0", true},
	}
	for _, tt := range tests {
		t.Run(tt.versionString, func(t *testing.T) {
			t.Parallel()
			currentVersion, err := version.NewVersion(tt.versionString)
			if err != nil {
				t.Fatalf("failed parsing version %q: %v", tt.versionString, err)
			}
			if got := supportsDualPassword(currentVersion); got != tt.want {
				t.Errorf("supportsDualPassword(%q): got %t, want %t", tt.versionString, got, tt.want)
			}
		})
	}
}

// TestSecondaryPasswordExpression checks that `User_attributes` is not referenced on a server
// which does not have the column, because referencing it fails with ER_BAD_FIELD_ERROR.
func TestSecondaryPasswordExpression(t *testing.T) {
	t.Parallel()
	tests := []struct {
		versionString        string
		wantColumnReferenced bool
	}{
		{"8.0.13", false},
		{"8.0.13-log", false},
		{"8.0.14", true},
		{"8.0.14-log", true},
		{"8.0.39", true},
		{"8.4.0", true},
	}
	for _, tt := range tests {
		t.Run(tt.versionString, func(t *testing.T) {
			t.Parallel()
			currentVersion, err := version.NewVersion(tt.versionString)
			if err != nil {
				t.Fatalf("failed parsing version %q: %v", tt.versionString, err)
			}
			got := secondaryPasswordExpression(currentVersion)
			if strings.Contains(got, "User_attributes") != tt.wantColumnReferenced {
				t.Errorf("secondaryPasswordExpression(%q) = %q, want User_attributes referenced: %t",
					tt.versionString, got, tt.wantColumnReferenced)
			}
		})
	}
}

// TestStateWithoutDiscardOldPassword checks that `discard_old_password` is unset while the
// other attributes are kept, so that a failed discard still leaves a diff to retry.
func TestStateWithoutDiscardOldPassword(t *testing.T) {
	t.Parallel()
	authOption := types.ObjectValueMust(AuthOptionModelTypes, map[string]attr.Value{
		"plugin":                  types.StringNull(),
		"auth_string":             types.StringValue("password"),
		"random_password":         types.BoolNull(),
		"retain_current_password": types.BoolValue(false),
		"discard_old_password":    types.BoolValue(true),
	})
	data := &UserResourceModel{
		Name:       types.StringValue("test-user"),
		Host:       types.StringValue("%"),
		Lock:       types.BoolValue(true),
		AuthOption: authOption,
	}

	got, diags := stateWithoutDiscardOldPassword(data)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	var gotAuthOption AuthOptionModel
	if diags := got.AuthOption.As(context.Background(), &gotAuthOption, basetypes.ObjectAsOptions{}); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !gotAuthOption.DiscardOldPassword.IsNull() {
		t.Errorf("discard_old_password: got %v, want null", gotAuthOption.DiscardOldPassword)
	}
	if gotAuthOption.AuthString.ValueString() != "password" {
		t.Errorf("auth_string: got %q, want %q", gotAuthOption.AuthString.ValueString(), "password")
	}
	if gotAuthOption.RetainCurrentPassword.ValueBool() {
		t.Errorf("retain_current_password: got true, want false")
	}
	if !got.Lock.ValueBool() {
		t.Errorf("lock: got false, want true")
	}

	// The argument must not be modified, because it is still used to report the error.
	var originalAuthOption AuthOptionModel
	if diags := data.AuthOption.As(context.Background(), &originalAuthOption, basetypes.ObjectAsOptions{}); diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if !originalAuthOption.DiscardOldPassword.ValueBool() {
		t.Errorf("the argument was modified: discard_old_password got %v, want true", originalAuthOption.DiscardOldPassword)
	}
}

func TestAccUserResource(t *testing.T) {
	users := []UserModel{
		NewRandomUser("test-user", "%"),
		NewRandomUser("test-user", "%"),
		NewRandomUser("test-user", "example.com"),
	}
	t.Logf("%+v\n", users)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		CheckDestroy:             testAccUserResource_CheckDestroy(users),
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccUserResource_Config(t, users[0].GetName(), ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_user.test", "name", users[0].GetName()),
					resource.TestCheckResourceAttr("mysql_user.test", "id", users[0].GetID()),
				),
			},
			// ImportState testing
			{
				ResourceName:      "mysql_user.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and Read testing
			{
				Config: testAccUserResource_Config(t, users[1].GetName(), ""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_user.test", "name", users[1].GetName()),
					resource.TestCheckResourceAttr("mysql_user.test", "id", users[1].GetID()),
				),
			},
			{
				Config: testAccUserResource_Config(t, users[2].GetName(), users[2].GetHost()),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_user.test", "name", users[2].GetName()),
					resource.TestCheckResourceAttr("mysql_user.test", "host", users[2].GetHost()),
					resource.TestCheckResourceAttr("mysql_user.test", "id", users[2].GetID()),
				),
			},
			{
				Config: testAccUserResource_ConfigWithAuth(t, users[2].GetName(), users[2].GetHost()),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_user.test", "name", users[2].GetName()),
					resource.TestCheckResourceAttr("mysql_user.test", "host", users[2].GetHost()),
					resource.TestCheckResourceAttr("mysql_user.test", "id", users[2].GetID()),
					resource.TestCheckResourceAttr("mysql_user.test", "auth_option.auth_string", "password"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func TestAccUserResource_Plugin(t *testing.T) {
	users := []UserModel{
		NewRandomUser("test-user", "%"),
		NewRandomUser("test-user", "%"),
		NewRandomUser("test-user", "example.com"),
	}
	t.Logf("%+v\n", users)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		CheckDestroy:             testAccUserResource_CheckDestroy(users),
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccUserResource_ConfigWithAuthPlugin(t, users[2].GetName(), users[2].GetHost(), "sha256_password"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_user.test", "name", users[2].GetName()),
					resource.TestCheckResourceAttr("mysql_user.test", "host", users[2].GetHost()),
					resource.TestCheckResourceAttr("mysql_user.test", "id", users[2].GetID()),
					resource.TestCheckResourceAttr("mysql_user.test", "auth_option.plugin", "sha256_password"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "mysql_user.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and Read testing
			{
				Config: testAccUserResource_ConfigWithAuthPlugin(t, users[2].GetName(), users[2].GetHost(), "caching_sha2_password"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_user.test", "name", users[2].GetName()),
					resource.TestCheckResourceAttr("mysql_user.test", "host", users[2].GetHost()),
					resource.TestCheckResourceAttr("mysql_user.test", "id", users[2].GetID()),
					resource.TestCheckResourceAttr("mysql_user.test", "auth_option.plugin", "caching_sha2_password"),
				),
			},
			{
				Config: testAccUserResource_Config(t, users[2].GetName(), users[2].GetHost()),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_user.test", "name", users[2].GetName()),
					resource.TestCheckResourceAttr("mysql_user.test", "host", users[2].GetHost()),
					resource.TestCheckResourceAttr("mysql_user.test", "id", users[2].GetID()),
					resource.TestCheckResourceAttr("mysql_user.test", "auth_option.%", "0"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func TestAccUserResource_Lock(t *testing.T) {
	users := []UserModel{
		NewRandomUser("test-user", "%"),
		NewRandomUser("test-user", "%"),
		NewRandomUser("test-user", "example.com"),
	}
	t.Logf("%+v\n", users)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		CheckDestroy:             testAccUserResource_CheckDestroy(users),
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccUserResource_ConfigWithLock(t, users[2].GetName(), users[2].GetHost(), true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_user.test", "name", users[2].GetName()),
					resource.TestCheckResourceAttr("mysql_user.test", "host", users[2].GetHost()),
					resource.TestCheckResourceAttr("mysql_user.test", "id", users[2].GetID()),
					resource.TestCheckResourceAttr("mysql_user.test", "lock", "true"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "mysql_user.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and Read testing
			{
				Config: testAccUserResource_ConfigWithLock(t, users[2].GetName(), users[2].GetHost(), false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_user.test", "name", users[2].GetName()),
					resource.TestCheckResourceAttr("mysql_user.test", "host", users[2].GetHost()),
					resource.TestCheckResourceAttr("mysql_user.test", "id", users[2].GetID()),
					resource.TestCheckResourceAttr("mysql_user.test", "lock", "false"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func TestAccUserResource_DualPassword(t *testing.T) {
	user := NewRandomUser("test-user", "%")
	t.Logf("%+v\n", user)
	users := []UserModel{user}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckDualPassword(t) },
		CheckDestroy:             testAccUserResource_CheckDestroy(users),
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccUserResource_ConfigWithDualPassword(t, user.GetName(), user.GetHost(), "password1", false, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_user.test", "id", user.GetID()),
					resource.TestCheckResourceAttr("mysql_user.test", "auth_option.auth_string", "password1"),
					resource.TestCheckResourceAttr("mysql_user.test", "has_secondary_password", "false"),
					testAccUserResource_CheckSecondaryPassword(user, false),
					testAccUserResource_CheckLogin(user, "password1"),
					testAccUserResource_CheckLoginFailure(user, "password2"),
				),
			},
			// Change the primary password and retain the current password as the secondary password
			{
				Config: testAccUserResource_ConfigWithDualPassword(t, user.GetName(), user.GetHost(), "password2", true, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_user.test", "auth_option.auth_string", "password2"),
					resource.TestCheckResourceAttr("mysql_user.test", "auth_option.retain_current_password", "true"),
					resource.TestCheckResourceAttr("mysql_user.test", "has_secondary_password", "true"),
					testAccUserResource_CheckSecondaryPassword(user, true),
					testAccUserResource_CheckLogin(user, "password1"),
					testAccUserResource_CheckLogin(user, "password2"),
				),
			},
			// Discard the secondary password
			{
				Config: testAccUserResource_ConfigWithDualPassword(t, user.GetName(), user.GetHost(), "password2", false, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_user.test", "auth_option.auth_string", "password2"),
					resource.TestCheckResourceAttr("mysql_user.test", "auth_option.discard_old_password", "true"),
					resource.TestCheckResourceAttr("mysql_user.test", "has_secondary_password", "false"),
					testAccUserResource_CheckSecondaryPassword(user, false),
					testAccUserResource_CheckLogin(user, "password2"),
					testAccUserResource_CheckLoginFailure(user, "password1"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

// TestAccUserResource_DualPasswordRotateTwice checks that MySQL keeps only one secondary password,
// so the second rotation invalidates the password retained by the first rotation.
func TestAccUserResource_DualPasswordRotateTwice(t *testing.T) {
	user := NewRandomUser("test-user", "%")
	t.Logf("%+v\n", user)
	users := []UserModel{user}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckDualPassword(t) },
		CheckDestroy:             testAccUserResource_CheckDestroy(users),
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccUserResource_ConfigWithDualPassword(t, user.GetName(), user.GetHost(), "password1", false, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccUserResource_CheckLogin(user, "password1"),
				),
			},
			{
				Config: testAccUserResource_ConfigWithDualPassword(t, user.GetName(), user.GetHost(), "password2", true, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccUserResource_CheckLogin(user, "password1"),
					testAccUserResource_CheckLogin(user, "password2"),
				),
			},
			{
				Config: testAccUserResource_ConfigWithDualPassword(t, user.GetName(), user.GetHost(), "password3", true, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccUserResource_CheckLoginFailure(user, "password1"),
					testAccUserResource_CheckLogin(user, "password2"),
					testAccUserResource_CheckLogin(user, "password3"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

// TestAccUserResource_DualPasswordAbandonedRotation checks that `has_secondary_password` reports
// a rotation which was never finished with `discard_old_password`. Removing
// `retain_current_password` does not discard the secondary password, so the retained password
// stays valid and only this attribute makes it visible.
func TestAccUserResource_DualPasswordAbandonedRotation(t *testing.T) {
	user := NewRandomUser("test-user", "%")
	t.Logf("%+v\n", user)
	users := []UserModel{user}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckDualPassword(t) },
		CheckDestroy:             testAccUserResource_CheckDestroy(users),
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccUserResource_ConfigWithDualPassword(t, user.GetName(), user.GetHost(), "password1", false, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_user.test", "has_secondary_password", "false"),
				),
			},
			// Retain the current password, which keeps `password1` as the secondary password
			{
				Config: testAccUserResource_ConfigWithDualPassword(t, user.GetName(), user.GetHost(), "password2", true, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_user.test", "has_secondary_password", "true"),
					testAccUserResource_CheckLogin(user, "password1"),
					testAccUserResource_CheckLogin(user, "password2"),
				),
			},
			// Abandon the rotation by dropping `retain_current_password` instead of discarding.
			// `password1` is still the secondary password, so it keeps working.
			{
				Config: testAccUserResource_ConfigWithDualPassword(t, user.GetName(), user.GetHost(), "password3", false, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_user.test", "has_secondary_password", "true"),
					testAccUserResource_CheckSecondaryPassword(user, true),
					testAccUserResource_CheckLogin(user, "password1"),
					testAccUserResource_CheckLoginFailure(user, "password2"),
					testAccUserResource_CheckLogin(user, "password3"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

// TestAccUserResource_DualPasswordWithPlugin checks the dual password options
// with `IDENTIFIED WITH <plugin> BY ?`.
func TestAccUserResource_DualPasswordWithPlugin(t *testing.T) {
	user := NewRandomUser("test-user", "%")
	t.Logf("%+v\n", user)
	users := []UserModel{user}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckDualPassword(t) },
		CheckDestroy:             testAccUserResource_CheckDestroy(users),
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccUserResource_ConfigWithDualPasswordAndPlugin(t, user.GetName(), user.GetHost(), "caching_sha2_password", "password1", false, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_user.test", "auth_option.plugin", "caching_sha2_password"),
					testAccUserResource_CheckLogin(user, "password1"),
				),
			},
			{
				Config: testAccUserResource_ConfigWithDualPasswordAndPlugin(t, user.GetName(), user.GetHost(), "caching_sha2_password", "password2", true, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_user.test", "has_secondary_password", "true"),
					testAccUserResource_CheckSecondaryPassword(user, true),
					testAccUserResource_CheckLogin(user, "password1"),
					testAccUserResource_CheckLogin(user, "password2"),
				),
			},
			{
				Config: testAccUserResource_ConfigWithDualPasswordAndPlugin(t, user.GetName(), user.GetHost(), "caching_sha2_password", "password2", false, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_user.test", "has_secondary_password", "false"),
					testAccUserResource_CheckSecondaryPassword(user, false),
					testAccUserResource_CheckLoginFailure(user, "password1"),
					testAccUserResource_CheckLogin(user, "password2"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

// TestAccUserResource_DualPasswordUnsupportedVersion checks that the provider reports a clear error
// instead of letting MySQL fail with a syntax error. It runs only against MySQL earlier than 8.0.14.
func TestAccUserResource_DualPasswordUnsupportedVersion(t *testing.T) {
	user := NewRandomUser("test-user", "%")
	t.Logf("%+v\n", user)
	users := []UserModel{user}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheckDualPasswordUnsupported(t) },
		CheckDestroy:             testAccUserResource_CheckDestroy(users),
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccUserResource_ConfigWithDualPassword(t, user.GetName(), user.GetHost(), "password1", false, false),
			},
			{
				Config: testAccUserResource_ConfigWithDualPassword(t, user.GetName(), user.GetHost(), "password2", true, false),
				// Asserting on the version specific message, because the summary
				// `Could not use dual password` is reported for unrelated errors too.
				ExpectError: regexp.MustCompile(fmt.Sprintf("requires MySQL %s or later",
					regexp.QuoteMeta(dualPasswordMinVersion.String()))),
			},
		},
	})
}

// TestAccUserResource_DualPasswordBothTrueFromExpression checks that the both options cannot be
// true at the same time when one of them is derived from an expression rather than a literal.
// It does not tell the two layers apart: `ValidateConfig` rejects the combination as soon as the
// value is resolved, and `Update` is only reached for a value which is still unknown by then, so
// either message is accepted. The check in `Update` was verified by disabling the one in
// `ValidateConfig`, which makes this apply succeed with no error and no warning.
func TestAccUserResource_DualPasswordBothTrueFromExpression(t *testing.T) {
	user := NewRandomUser("test-user", "%")
	helper := NewRandomUser("test-user", "%")
	t.Logf("%+v %+v\n", user, helper)
	users := []UserModel{user, helper}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		CheckDestroy:             testAccUserResource_CheckDestroy(users),
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccUserResource_ConfigWithDualPassword(t, user.GetName(), user.GetHost(), "password1", false, false),
			},
			{
				Config:      testAccUserResource_ConfigWithRetainCurrentPasswordFromExpression(t, user.GetName(), helper.GetName(), user.GetHost(), "password2"),
				ExpectError: regexp.MustCompile("Invalid Attribute Combination|Conflicting dual password options"),
			},
		},
	})
}

// TestAccUserResource_DualPasswordBothTrue checks that the both options cannot be true at the same time.
func TestAccUserResource_DualPasswordBothTrue(t *testing.T) {
	user := NewRandomUser("test-user", "%")
	t.Logf("%+v\n", user)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccUserResource_ConfigWithDualPassword(t, user.GetName(), user.GetHost(), "password1", true, true),
				ExpectError: regexp.MustCompile("Invalid Attribute Combination"),
			},
		},
	})
}

func TestAccUserResource_ImportNonExistentRemoteObject(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// ImportState testing
			{
				ResourceName:      "mysql_user.test",
				ImportState:       true,
				ImportStateId:     "non-existent-user@%",
				ImportStateVerify: false,
				Config:            testAccUserResource_Config(t, "non-existent-user", "%"),
				ExpectError:       regexp.MustCompile("Cannot import non-existent remote object"),
			},
		},
	})
}

func testAccUserResource_Config(t *testing.T, name, host string) string {
	source := `
resource "mysql_user" "test" {
  name = "{{ .Name }}"
  {{- if gt (len .Host) 0 }}
  host = "{{ .Host }}"
  {{- end }}
}
`
	data := struct {
		Name string
		Host string
	}{
		Name: name,
		Host: host,
	}
	config, err := utils.Render(source, data)
	if err != nil {
		t.Fatal(err)
		t.Fail()
	}
	return config
}

func testAccUserResource_ConfigWithAuth(t *testing.T, name, host string) string {
	source := `
resource "mysql_user" "test" {
  name = "{{ .Name }}"
  {{- if gt (len .Host) 0 }}
  host = "{{ .Host }}"
  {{- end }}
  auth_option {
    auth_string = "password"
  }
}
`
	data := struct {
		Name string
		Host string
	}{
		Name: name,
		Host: host,
	}
	config, err := utils.Render(source, data)
	if err != nil {
		t.Fatal(err)
		t.Fail()
	}
	return config
}

func testAccUserResource_ConfigWithAuthPlugin(t *testing.T, name, host, plugin string) string {
	source := `
resource "mysql_user" "test" {
  name = "{{ .Name }}"
  host = "{{ .Host }}"
  auth_option {
    plugin = "{{ .Plugin }}"
  }
}
`
	data := struct {
		Name   string
		Host   string
		Plugin string
	}{
		Name:   name,
		Host:   host,
		Plugin: plugin,
	}
	config, err := utils.Render(source, data)
	if err != nil {
		t.Fatal(err)
		t.Fail()
	}
	return config
}

func testAccUserResource_ConfigWithLock(t *testing.T, name, host string, lock bool) string {
	source := `
resource "mysql_user" "test" {
  name = "{{ .Name }}"
  host = "{{ .Host }}"
  lock = {{ .Lock }}
}
`
	data := struct {
		Name string
		Host string
		Lock bool
	}{
		Name: name,
		Host: host,
		Lock: lock,
	}
	config, err := utils.Render(source, data)
	if err != nil {
		t.Fatal(err)
		t.Fail()
	}
	return config
}

func testAccUserResource_ConfigWithDualPassword(t *testing.T, name, host, authString string, retainCurrentPassword, discardOldPassword bool) string {
	source := `
resource "mysql_user" "test" {
  name = "{{ .Name }}"
  host = "{{ .Host }}"
  auth_option {
    auth_string             = "{{ .AuthString }}"
    retain_current_password = {{ .RetainCurrentPassword }}
    discard_old_password    = {{ .DiscardOldPassword }}
  }
}
`
	data := struct {
		Name                  string
		Host                  string
		AuthString            string
		RetainCurrentPassword bool
		DiscardOldPassword    bool
	}{
		Name:                  name,
		Host:                  host,
		AuthString:            authString,
		RetainCurrentPassword: retainCurrentPassword,
		DiscardOldPassword:    discardOldPassword,
	}
	config, err := utils.Render(source, data)
	if err != nil {
		t.Fatal(err)
		t.Fail()
	}
	return config
}

// testAccUserResource_ConfigWithRetainCurrentPasswordFromExpression derives `retain_current_password`
// from a resource which is created by the same apply, so that the value is unknown while the
// configuration is validated and only known when the user is updated.
func testAccUserResource_ConfigWithRetainCurrentPasswordFromExpression(t *testing.T, name, helperName, host, authString string) string {
	source := `
resource "mysql_user" "helper" {
  name = "{{ .HelperName }}"
  host = "{{ .Host }}"
}

resource "mysql_user" "test" {
  name = "{{ .Name }}"
  host = "{{ .Host }}"
  auth_option {
    auth_string             = "{{ .AuthString }}"
    retain_current_password = mysql_user.helper.id != ""
    discard_old_password    = true
  }
}
`
	data := struct {
		Name       string
		HelperName string
		Host       string
		AuthString string
	}{
		Name:       name,
		HelperName: helperName,
		Host:       host,
		AuthString: authString,
	}
	config, err := utils.Render(source, data)
	if err != nil {
		t.Fatal(err)
		t.Fail()
	}
	return config
}

func testAccUserResource_ConfigWithDualPasswordAndPlugin(t *testing.T, name, host, plugin, authString string, retainCurrentPassword, discardOldPassword bool) string {
	source := `
resource "mysql_user" "test" {
  name = "{{ .Name }}"
  host = "{{ .Host }}"
  auth_option {
    plugin                  = "{{ .Plugin }}"
    auth_string             = "{{ .AuthString }}"
    retain_current_password = {{ .RetainCurrentPassword }}
    discard_old_password    = {{ .DiscardOldPassword }}
  }
}
`
	data := struct {
		Name                  string
		Host                  string
		Plugin                string
		AuthString            string
		RetainCurrentPassword bool
		DiscardOldPassword    bool
	}{
		Name:                  name,
		Host:                  host,
		Plugin:                plugin,
		AuthString:            authString,
		RetainCurrentPassword: retainCurrentPassword,
		DiscardOldPassword:    discardOldPassword,
	}
	config, err := utils.Render(source, data)
	if err != nil {
		t.Fatal(err)
		t.Fail()
	}
	return config
}

// testAccUserResource_CheckSecondaryPassword checks whether the user has a secondary password.
// The secondary password is stored in the mysql.user.User_attributes column as `additional_password`.
func testAccUserResource_CheckSecondaryPassword(user UserModel, expected bool) resource.TestCheckFunc {
	return func(t *terraform.State) error {
		db := testDatabase()
		sql := `
SELECT
  JSON_CONTAINS_PATH(User_attributes, 'one', '$.additional_password') IS TRUE
FROM
  mysql.user
WHERE
  User = ?
  AND Host = ?
`
		var actual bool
		if err := db.QueryRow(sql, user.GetName(), user.GetHost()).Scan(&actual); err != nil {
			return err
		}
		if actual != expected {
			return fmt.Errorf("Unexpected secondary password state (%s): expected=%t, actual=%t", user.GetID(), expected, actual)
		}
		return nil
	}
}

// testAccUserResource_CheckLogin checks that the user can log in with the given password.
func testAccUserResource_CheckLogin(user UserModel, password string) resource.TestCheckFunc {
	return func(t *terraform.State) error {
		if err := testLogin(user, password); err != nil {
			return fmt.Errorf("Could not log in as %s with the password %q: %v", user.GetID(), password, err)
		}
		return nil
	}
}

// testAccUserResource_CheckLoginFailure checks that the user cannot log in with the given password.
func testAccUserResource_CheckLoginFailure(user UserModel, password string) resource.TestCheckFunc {
	return func(t *terraform.State) error {
		if err := testLogin(user, password); err == nil {
			return fmt.Errorf("Could log in as %s with the password %q unexpectedly", user.GetID(), password)
		}
		return nil
	}
}

// testServerVersion returns the version of the server under test.
// A failure here is a broken test environment, not an unsupported server, so it fails the test
// instead of being reported as a missing dual password support.
func testServerVersion(t *testing.T) *version.Version {
	currentVersion, err := serverVersion(testDatabase())
	if err != nil {
		t.Fatalf("failed getting the server version: %v", err)
	}
	return currentVersion
}

// testAccPreCheckDualPassword skips the test unless the server supports dual password.
// `mysql.user.User_attributes` does not exist before MySQL 8.0.14 either, so a test which
// reads the secondary password state cannot run on an earlier server at all.
func testAccPreCheckDualPassword(t *testing.T) {
	testAccPreCheck(t)
	if err := checkDualPasswordSupport(testServerVersion(t)); err != nil {
		t.Skipf("%v", err)
	}
}

// testAccPreCheckDualPasswordUnsupported skips the test unless the server is earlier than the
// version which added dual password.
func testAccPreCheckDualPasswordUnsupported(t *testing.T) {
	testAccPreCheck(t)
	if err := checkDualPasswordSupport(testServerVersion(t)); err == nil {
		t.Skipf("The server supports dual password")
	}
}

// testLogin opens a new connection without using the connection cache,
// because the cached connection stays usable after the password is changed.
func testLogin(user UserModel, password string) error {
	conf := testMySQLConfig()
	conf.Config.User = user.GetName()
	conf.Config.Passwd = password
	db, err := sql.Open("mysql", conf.Config.FormatDSN())
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	return db.PingContext(context.Background())
}

func testAccUserResource_CheckDestroy(users []UserModel) resource.TestCheckFunc {
	return func(t *terraform.State) error {
		db := testDatabase()
		sql := "SELECT COUNT(*) FROM mysql.user WHERE user = ? AND host = ?"
		for _, user := range users {
			var count string
			if err := db.QueryRow(sql, user.GetName(), user.GetHost()).Scan(&count); err != nil {
				return err
			}
			if count != "0" {
				return fmt.Errorf("User still exist (%s): %s", user.GetID(), count)
			}
		}
		return nil
	}
}
