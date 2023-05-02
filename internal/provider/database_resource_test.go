package provider

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccDatabaseResource(t *testing.T) {
	name := fmt.Sprintf("test-%04d", rand.Intn(10000))
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccDatabaseResourceConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_database.test", "id", name),
					resource.TestCheckResourceAttr("mysql_database.test", "name", name),
					resource.TestCheckResourceAttr("mysql_database.test", "default_character_set", "utf8mb4"),
					resource.TestCheckResourceAttr("mysql_database.test", "default_collation", "utf8mb4_0900_ai_ci"),
				),
			},
			// ImportState testing
			// {
			// 	ResourceName:      "mysql_database.test",
			// 	ImportState:       true,
			// 	ImportStateVerify: true,
			// },
			// Update and Read testing
			{
				Config: testAccDatabaseResourceConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_database.test", "name", name),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccDatabaseResourceConfig(name string) string {
	config := fmt.Sprintf(`
resource "mysql_database" "test" {
  name = "%s"
}
`, name)
	return buildConfig(config)
}
