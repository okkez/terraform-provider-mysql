package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccGrantRoleResource(t *testing.T) {
	user := NewRandomUser("test-user", "%")
	role1 := NewRandomRole("test-role1", "%")
	role2 := NewRandomRole("test-role2", "%")
	t.Logf("user: %s, role1: %s, role2: %s", user.GetName(), role1.GetName(), role2.GetName())
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccGrantRoleResource_Config(user.GetName(), role1.GetName(), role2.GetName(), "role1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_grant_role.test", "to.name", user.GetName()),
					resource.TestCheckResourceAttr("mysql_grant_role.test", "to.host", user.GetHost()),
					resource.TestCheckResourceAttr("mysql_grant_role.test", "roles.#", "1"),
					resource.TestCheckResourceAttr("mysql_grant_role.test", "roles.0", role1.GetName()),
					resource.TestCheckResourceAttr("mysql_grant_role.test", "id", user.GetID()),
				),
			},
			// ImportState testing
			{
				ResourceName:      "mysql_grant_role.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and Read testing
			{
				Config: testAccGrantRoleResource_Config(user.GetName(), role1.GetName(), role2.GetName(), "role2"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_grant_role.test", "to.name", user.GetName()),
					resource.TestCheckResourceAttr("mysql_grant_role.test", "to.host", user.GetHost()),
					resource.TestCheckResourceAttr("mysql_grant_role.test", "roles.#", "1"),
					resource.TestCheckResourceAttr("mysql_grant_role.test", "roles.0", role2.GetName()),
					resource.TestCheckResourceAttr("mysql_grant_role.test", "id", user.GetID()),
				),
			},
			{
				Config: testAccGrantRoleResource_ConfigWithRoles(user.GetName(), role1.GetName(), role2.GetName()),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_grant_role.test", "to.name", user.GetName()),
					resource.TestCheckResourceAttr("mysql_grant_role.test", "to.host", user.GetHost()),
					resource.TestCheckResourceAttr("mysql_grant_role.test", "roles.#", "2"),
					resource.TestCheckResourceAttr("mysql_grant_role.test", "roles.0", role1.GetName()),
					resource.TestCheckResourceAttr("mysql_grant_role.test", "roles.1", role2.GetName()),
					resource.TestCheckResourceAttr("mysql_grant_role.test", "id", user.GetID()),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccGrantRoleResource_Config(user, role1, role2, roleLabel string) string {
	config := fmt.Sprintf(`
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
  roles = [mysql_role.%s.name]
}
`, user, role1, role2, roleLabel)
	return buildConfig(config)
}

func testAccGrantRoleResource_ConfigWithRoles(user, role1, role2 string) string {
	config := fmt.Sprintf(`
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
`, user, role1, role2)
	return buildConfig(config)
}
