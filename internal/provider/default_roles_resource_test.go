package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccDefaultRoleResource(t *testing.T) {
	user := NewRandomUser("test-user", "%")
	role1 := NewRandomRole("test-role", "%")
	role2 := NewRandomRole("test-role", "%")
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		CheckDestroy:             testAccTestAccDefaultRoleResource_CheckDestroy(user),
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccDefaultRoleResource_Config(user.GetName(), role1.GetName()),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_default_role.test", "id", user.GetID()),
					resource.TestCheckResourceAttr("mysql_default_role.test", "user", user.GetName()),
					resource.TestCheckResourceAttr("mysql_default_role.test", "host", user.GetHost()),
					resource.TestCheckResourceAttr("mysql_default_role.test", "default_role.0.name", role1.GetName()),
					resource.TestCheckResourceAttr("mysql_default_role.test", "default_role.0.host", "%"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "mysql_default_role.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and Read testing
			{
				Config: testAccDefaultRoleResource_Config(user.GetName(), role2.GetName()),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_default_role.test", "id", user.GetID()),
					resource.TestCheckResourceAttr("mysql_default_role.test", "user", user.GetName()),
					resource.TestCheckResourceAttr("mysql_default_role.test", "host", user.GetHost()),
					resource.TestCheckResourceAttr("mysql_default_role.test", "default_role.0.name", role2.GetName()),
					resource.TestCheckResourceAttr("mysql_default_role.test", "default_role.0.host", "%"),
				),
			},
			{
				Config: testAccDefaultRoleResource_ConfigWithRoles(user.GetName(), role1.GetName(), role2.GetName()),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_default_role.test", "id", user.GetID()),
					resource.TestCheckResourceAttr("mysql_default_role.test", "user", user.GetName()),
					resource.TestCheckResourceAttr("mysql_default_role.test", "host", user.GetHost()),
					resource.TestCheckResourceAttr("mysql_default_role.test", "default_role.0.name", role1.GetName()),
					resource.TestCheckResourceAttr("mysql_default_role.test", "default_role.0.host", "%"),
					resource.TestCheckResourceAttr("mysql_default_role.test", "default_role.1.name", role2.GetName()),
					resource.TestCheckResourceAttr("mysql_default_role.test", "default_role.1.host", "%"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccDefaultRoleResource_Config(user, role string) string {
	return fmt.Sprintf(`
resource "mysql_user" "test" {
  name = %q
}
resource "mysql_role" "test" {
  name = %q
}
resource "mysql_default_role" "test" {
  user = mysql_user.test.name
  default_role {
    name = mysql_role.test.name
  }
}
`, user, role)
}

func testAccDefaultRoleResource_ConfigWithRoles(user, role1, role2 string) string {
	return fmt.Sprintf(`
resource "mysql_user" "test" {
  name = %q
}
resource "mysql_role" "test1" {
  name = %q
}
resource "mysql_role" "test2" {
  name = %q
}
resource "mysql_default_role" "test" {
  user = mysql_user.test.name
  default_role {
    name = mysql_role.test1.name
  }
  default_role {
    name = mysql_role.test2.name
  }
}
`, user, role1, role2)
}

func testAccTestAccDefaultRoleResource_CheckDestroy(user UserModel) resource.TestCheckFunc {
	return func(t *terraform.State) error {
		db := testDatabase()
		sql := `SELECT COUNT(*) FROM mysql.default_roles WHERE USER = ? AND HOST = ?`
		var count string
		if err := db.QueryRow(sql, user.GetName(), user.GetHost()).Scan(&count); err != nil {
			return err
		}
		if count != "0" {
			return fmt.Errorf("Default roles still exist (%s)", user.GetID())
		}
		return nil
	}
}
