package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccDefaultRoleResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccExampleResourceConfig("one"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_default_role.test", "id", "test-user@%"),
					resource.TestCheckResourceAttr("mysql_default_role.test", "user", "test-user"),
					resource.TestCheckResourceAttr("mysql_default_role.test", "host", "%"),
					resource.TestCheckResourceAttr("mysql_default_role.test", "default_role.0.name", "test-role"),
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
				Config: testAccExampleResourceConfig("two"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_default_role.test", "default_role.0.name", "test-role2"),
					resource.TestCheckResourceAttr("mysql_default_role.test", "default_role.0.host", "%"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccDefaultRoleResourceConfig(user, role string) string {
	return fmt.Sprintf(`
resource "mysql_default_role" "test" {
  user = %q
  default_role {
    name = %q
  }
}
`, user, role)
}
