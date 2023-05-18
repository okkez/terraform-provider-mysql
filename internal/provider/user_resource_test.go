package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccUserResource(t *testing.T) {
	users := []UserModel{
		NewRandomUser("test-user", "%"),
		NewRandomUser("test-user", "%"),
		NewRandomUser("test-user", "example.com"),
	}
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		CheckDestroy:             testAccUserResource_CheckDestroy(users),
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccUserResource_Config(users[0].GetName()),
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
				Config: testAccUserResource_Config(users[1].GetName()),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_user.test", "name", users[1].GetName()),
					resource.TestCheckResourceAttr("mysql_user.test", "id", users[1].GetID()),
				),
			},
			{
				Config: testAccUserResource_ConfigWithHost(users[2].GetName(), users[2].GetHost()),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_user.test", "name", users[2].GetName()),
					resource.TestCheckResourceAttr("mysql_user.test", "host", users[2].GetHost()),
					resource.TestCheckResourceAttr("mysql_user.test", "id", users[2].GetID()),
				),
			},
			{
				Config: testAccUserResource_ConfigWithAuth(users[2].GetName(), users[2].GetHost()),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_user.test", "name", users[2].GetName()),
					resource.TestCheckResourceAttr("mysql_user.test", "host", users[2].GetHost()),
					resource.TestCheckResourceAttr("mysql_user.test", "id", users[2].GetID()),
					resource.TestCheckResourceAttr("mysql_user.test", "auth_option.plugin", "caching_sha2_password"),
					resource.TestCheckResourceAttr("mysql_user.test", "auth_option.auth_string", "password"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccUserResource_Config(name string) string {
	return fmt.Sprintf(`
resource "mysql_user" "test" {
  name = %q
}
`, name)
}

func testAccUserResource_ConfigWithHost(name, host string) string {
	return fmt.Sprintf(`
resource "mysql_user" "test" {
  name = %q
  host = %q
}
`, name, host)
}

func testAccUserResource_ConfigWithAuth(name, host string) string {
	return fmt.Sprintf(`
resource "mysql_user" "test" {
  name = %q
  host = %q
  auth_option {
    auth_string = "password"
    plugin = "caching_sha2_password"
  }
}
`, name, host)
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
