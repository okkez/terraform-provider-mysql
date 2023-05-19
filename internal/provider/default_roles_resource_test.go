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
	t.Logf("user: %s, role1: %s, role2: %s", user.GetID(), role1.GetID(), role2.GetID())
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		CheckDestroy:             testAccTestAccDefaultRoleResource_CheckDestroy(user),
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccDefaultRoleResource_Config(user.GetName(), role1.GetName(), role2.GetName(), "role1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_default_roles.test", "id", user.GetID()),
					resource.TestCheckResourceAttr("mysql_default_roles.test", "user", user.GetName()),
					resource.TestCheckResourceAttr("mysql_default_roles.test", "host", user.GetHost()),
					resource.TestCheckResourceAttr("mysql_default_roles.test", "default_role.#", "1"),
					resource.TestCheckResourceAttr("mysql_default_roles.test", "default_role.0.name", role1.GetName()),
					resource.TestCheckResourceAttr("mysql_default_roles.test", "default_role.0.host", "%"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "mysql_default_roles.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and Read testing
			{
				Config: testAccDefaultRoleResource_Config(user.GetName(), role1.GetName(), role2.GetName(), "role2"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_default_roles.test", "id", user.GetID()),
					resource.TestCheckResourceAttr("mysql_default_roles.test", "user", user.GetName()),
					resource.TestCheckResourceAttr("mysql_default_roles.test", "host", user.GetHost()),
					resource.TestCheckResourceAttr("mysql_default_roles.test", "default_role.#", "1"),
					resource.TestCheckResourceAttr("mysql_default_roles.test", "default_role.0.name", role2.GetName()),
					resource.TestCheckResourceAttr("mysql_default_roles.test", "default_role.0.host", "%"),
				),
			},
			{
				Config: testAccDefaultRoleResource_ConfigWithRoles(user.GetName(), role1.GetName(), role2.GetName()),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_default_roles.test", "id", user.GetID()),
					resource.TestCheckResourceAttr("mysql_default_roles.test", "user", user.GetName()),
					resource.TestCheckResourceAttr("mysql_default_roles.test", "host", user.GetHost()),
					resource.TestCheckResourceAttr("mysql_default_roles.test", "default_role.#", "2"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccDefaultRoleResource_Config(user, role1, role2, roleLabel string) string {
	return fmt.Sprintf(`
resource "mysql_user" "test" {
  name = %q
}
resource "mysql_role" "role1" {
  name = %q
}
resource "mysql_role" "role2" {
  name = %q
}
resource "mysql_grant_role" "test" {
  to {
    name = mysql_user.test.name
  }
  roles = [mysql_role.role1.name, mysql_role.role2.name]
}
resource "mysql_default_roles" "test" {
  user = mysql_user.test.name
  default_role {
    name = mysql_role.%s.name
  }
  depends_on = [mysql_grant_role.test]
}
`, user, role1, role2, roleLabel)
}

func testAccDefaultRoleResource_ConfigWithRoles(user, role1, role2 string) string {
	return fmt.Sprintf(`
resource "mysql_user" "test" {
  name = %q
}
resource "mysql_role" "role1" {
  name = %q
}
resource "mysql_role" "role2" {
  name = %q
}
resource "mysql_grant_role" "test" {
  to {
    name = mysql_user.test.name
  }
  roles = [mysql_role.role1.name, mysql_role.role2.name]
}
resource "mysql_default_roles" "test" {
  user = mysql_user.test.name
  default_role {
    name = mysql_role.role1.name
  }
  default_role {
    name = mysql_role.role2.name
  }
  depends_on = [mysql_grant_role.test]
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
