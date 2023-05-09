package provider

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccDatabaseResource(t *testing.T) {
	name := fmt.Sprintf("test-%04d", rand.Intn(10000))
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccDatabaseResource_CheckDestroy(name),
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
			{
				ResourceName:      "mysql_database.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and Read testing
			{
				Config: testAccDatabaseResourceConfig_full(name, "latin1", "latin1_bin"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_database.test", "name", name),
					resource.TestCheckResourceAttr("mysql_database.test", "default_character_set", "latin1"),
					resource.TestCheckResourceAttr("mysql_database.test", "default_collation", "latin1_bin"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccDatabaseResourceConfig(name string) string {
	return fmt.Sprintf(`
resource "mysql_database" "test" {
  name = %q
}
`, name)
}

func testAccDatabaseResourceConfig_full(name, charset, collation string) string {
	return fmt.Sprintf(`
resource "mysql_database" "test" {
  name = %q
  default_character_set = %q
  default_collation = %q
}
`, name, charset, collation)
}

func testAccDatabaseResource_CheckDestroy(name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		db := testDatabase()
		sql := `SELECT count(*) FROM information_schema.schemata WHERE SCHEMA_NAME = ?`
		args := []interface{}{name}
		var count string
		if err := db.QueryRow(sql, args...).Scan(&count); err != nil {
			return err
		}

		if count == "0" {
			return nil
		} else {
			return fmt.Errorf("database still exists after descroy (%s)", count)
		}
	}
}
