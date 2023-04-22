package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccUserResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccUserResourceConfig("test-user-one"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_user.test", "user", "test-user-one"),
					resource.TestCheckResourceAttr("mysql_user.test", "id", "test-user-one@%"),
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
				Config: testAccUserResourceConfig("test-user-two"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_user.test", "user", "test-user-two"),
					resource.TestCheckResourceAttr("mysql_user.test", "id", "test-user-two@%"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccUserResourceConfig(user string) string {
	config := fmt.Sprintf(`
resource "mysql_user" "test" {
  user = "%s"
}
`, user)
	return buildConfig(config)
}
