package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccTablesDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Read testing
			{
				Config: buildConfig(`
data "mysql_tables" "test" {
  database = "test"
}
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.mysql_tables.test", "id", "test:"),
					resource.TestCheckResourceAttr("data.mysql_tables.test", "database", "test"),
					resource.TestCheckNoResourceAttr("data.mysql_tables.test", "pattern"),
					resource.TestCheckResourceAttr("data.mysql_tables.test", "tables.0", "test"),
					resource.TestCheckResourceAttr("data.mysql_tables.test", "tables.1", "test2"),
					resource.TestCheckResourceAttr("data.mysql_tables.test", "tables.2", "test3"),
					resource.TestCheckNoResourceAttr("data.mysql_tables.test", "tables.3"),
				),
			},
			{
				Config: buildConfig(`
data "mysql_tables" "test" {
  database = "test"
  pattern  = "%2"
}
`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.mysql_tables.test", "id", "test:%2"),
					resource.TestCheckResourceAttr("data.mysql_tables.test", "database", "test"),
					resource.TestCheckResourceAttr("data.mysql_tables.test", "pattern", "%2"),
					resource.TestCheckResourceAttr("data.mysql_tables.test", "tables.0", "test2"),
					resource.TestCheckNoResourceAttr("data.mysql_tables.test", "tables.1"),
				),
			},
		},
	})
}
