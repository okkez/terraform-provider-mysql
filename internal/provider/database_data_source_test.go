package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccDatabaseDataSource(t *testing.T) {
	database := "mysql"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: testAccDatabaseDataSource_Config(database),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.mysql_database.test", "id", database),
					resource.TestCheckResourceAttr("data.mysql_database.test", "database", database),
					resource.TestCheckResourceAttr("data.mysql_database.test", "default_character_set", "utf8mb4"),
					resource.TestCheckResourceAttr("data.mysql_database.test", "default_collation", "utf8mb4_0900_ai_ci"),
				),
			},
			{
				Config:      testAccDatabaseDataSource_Config("non-existent-database"),
				ExpectError: regexp.MustCompile("Error: Failed querying database"),
			},
		},
	})
}

func testAccDatabaseDataSource_Config(database string) string {
	return fmt.Sprintf(`
data "mysql_database" "test" {
  database = %q
}
`, database)
}
