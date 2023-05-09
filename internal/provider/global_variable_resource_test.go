package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccGlobalVariableResource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccGlobalVariableResourceConfig("binlog_format", "MIXED"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_global_variable.test", "name", "binlog_format"),
					resource.TestCheckResourceAttr("mysql_global_variable.test", "value", "MIXED"),
					resource.TestCheckResourceAttr("mysql_global_variable.test", "id", "binlog_format"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "mysql_global_variable.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and Read testing
			{
				Config: testAccGlobalVariableResourceConfig("binlog_format", "STATEMENT"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_global_variable.test", "value", "STATEMENT"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
		CheckDestroy: func(s *terraform.State) error {
			db := testDatabase()
			var value string
			if err := db.QueryRow(`SELECT @@GLOBAL.binlog_format`).Scan(&value); err != nil {
				return err
			}
			if value != "ROW" {
				return fmt.Errorf(`expected "ROW" but was %q`, value)
			}
			return nil
		},
	})
}

func testAccGlobalVariableResourceConfig(name, value string) string {
	return fmt.Sprintf(`
resource "mysql_global_variable" "test" {
  name = %q
  value = %q
}
`, name, value)
}
