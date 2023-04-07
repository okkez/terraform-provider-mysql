package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccRoleResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccRoleResourceConfig("test-role"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_role.test", "name", "test-role"),
					resource.TestCheckResourceAttr("mysql_role.test", "host", "%"),
					resource.TestCheckResourceAttr("mysql_role.test", "id", "test-role@%"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "mysql_role.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and Read testing
			{
				Config: testAccRoleResourceConfigWithHost("test-role-2", "localhost"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_role.test", "name", "test-role-2"),
					resource.TestCheckResourceAttr("mysql_role.test", "host", "localhost"),
					resource.TestCheckResourceAttr("mysql_role.test", "id", "test-role-2@localhost"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccRoleResourceConfig(name string) string {
	config := fmt.Sprintf(`
resource "mysql_role" "test" {
  name = "%s"
}
`, name)
	return buildConfig(config)
}

func testAccRoleResourceConfigWithHost(name, host string) string {
	config := fmt.Sprintf(`
resource "mysql_role" "test" {
  name = "%s"
  host = "%s"
}
`, name, host)
	return buildConfig(config)
}
