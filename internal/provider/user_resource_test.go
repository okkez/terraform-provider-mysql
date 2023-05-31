package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/okkez/terraform-provider-mysql/internal/utils"
)

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
				Config: testAccUserResource_ConfigWithAuthPlugin(t, users[2].GetName(), users[2].GetHost(), "mysql_native_password"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_user.test", "name", users[2].GetName()),
					resource.TestCheckResourceAttr("mysql_user.test", "host", users[2].GetHost()),
					resource.TestCheckResourceAttr("mysql_user.test", "id", users[2].GetID()),
					resource.TestCheckResourceAttr("mysql_user.test", "auth_option.plugin", "mysql_native_password"),
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
