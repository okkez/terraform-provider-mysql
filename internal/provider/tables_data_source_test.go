package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccTablesDataSource(t *testing.T) {
	database := "mysql"
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: testAccTablesDataSource_config(database),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.mysql_tables.test", "id", "mysql:"),
					resource.TestCheckResourceAttr("data.mysql_tables.test", "database", database),
					resource.TestCheckNoResourceAttr("data.mysql_tables.test", "pattern"),
					resource.TestCheckResourceAttr("data.mysql_tables.test", "tables.#", testAccTablesDataSource_tableCount(t, database, "%")),
				),
			},
			{
				Config: testAccTablesDataSource_configWithPattern(database, "time%"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.mysql_tables.test", "id", "mysql:time%"),
					resource.TestCheckResourceAttr("data.mysql_tables.test", "database", database),
					resource.TestCheckResourceAttr("data.mysql_tables.test", "pattern", "time%"),
					resource.TestCheckResourceAttr("data.mysql_tables.test", "tables.#", testAccTablesDataSource_tableCount(t, database, "time%")),
				),
			},
		},
	})
}

func testAccTablesDataSource_config(database string) string {
	config := fmt.Sprintf(`
data "mysql_tables" "test" {
  database = %q
}
`, database)
	return buildConfig(config)
}

func testAccTablesDataSource_configWithPattern(database, pattern string) string {
	config := fmt.Sprintf(`
data "mysql_tables" "test" {
  database = %q
  pattern = %q
}
`, database, pattern)
	return buildConfig(config)
}

func testAccTablesDataSource_tableCount(t *testing.T, database, pattern string) string {
	db := testDatabase()
	sql := "SELECT COUNT(*) FROM information_schema.tables WHERE TABLE_SCHEMA = ? AND TABLE_NAME LIKE ?"
	var count string
	if err := db.QueryRow(sql, database, pattern).Scan(&count); err != nil {
		t.Fatalf("Failed querying table count (%s %s): %s", database, pattern, err.Error())
	}
	return count
}
