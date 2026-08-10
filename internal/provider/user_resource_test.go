package provider

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/go-version"
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
		PreCheck:                 func() { testAccPreCheck(t) },
		CheckDestroy:             testAccUserResource_CheckDestroy(users),
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccUserResource_ConfigWithDualPassword(t, user.GetName(), user.GetHost(), "password1", false, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_user.test", "id", user.GetID()),
					resource.TestCheckResourceAttr("mysql_user.test", "auth_option.auth_string", "password1"),
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
		PreCheck:                 func() { testAccPreCheck(t) },
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

// TestAccUserResource_DualPasswordWithPlugin checks the dual password options
// with `IDENTIFIED WITH <plugin> BY ?`.
func TestAccUserResource_DualPasswordWithPlugin(t *testing.T) {
	user := NewRandomUser("test-user", "%")
	t.Logf("%+v\n", user)
	users := []UserModel{user}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
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
					testAccUserResource_CheckSecondaryPassword(user, true),
					testAccUserResource_CheckLogin(user, "password1"),
					testAccUserResource_CheckLogin(user, "password2"),
				),
			},
			{
				Config: testAccUserResource_ConfigWithDualPasswordAndPlugin(t, user.GetName(), user.GetHost(), "caching_sha2_password", "password2", false, true),
				Check: resource.ComposeAggregateTestCheckFunc(
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
		PreCheck: func() {
			testAccPreCheck(t)
			if err := checkDualPasswordSupport(testDatabase()); err == nil {
				t.Skipf("The server supports dual password")
			}
		},
		CheckDestroy:             testAccUserResource_CheckDestroy(users),
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccUserResource_ConfigWithDualPassword(t, user.GetName(), user.GetHost(), "password1", false, false),
			},
			{
				Config:      testAccUserResource_ConfigWithDualPassword(t, user.GetName(), user.GetHost(), "password2", true, false),
				ExpectError: regexp.MustCompile("Could not use dual password"),
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
