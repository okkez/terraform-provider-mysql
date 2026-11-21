package provider

import (
	"fmt"
	"math/rand"
	"regexp"
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
				Config: testAccDatabaseResource_Config(name),
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
				Config: testAccDatabaseResource_ConfigFull(name, "latin1", "latin1_bin"),
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

func TestAccDatabaseResource_ImportNonExistentRemoteObject(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// ImportState testing
			{
				ResourceName:      "mysql_database.test",
				ImportState:       true,
				ImportStateId:     "non-existent-database",
				ImportStateVerify: false,
				Config:            testAccDatabaseResource_Config("non-existent-database"),
				ExpectError:       regexp.MustCompile("Cannot import non-existent remote object"),
			},
		},
	})
}

// TestAccDatabaseResource_RemovedOutOfBand checks that a database dropped outside of
// Terraform is removed from the state on refresh, instead of failing the refresh.
func TestAccDatabaseResource_RemovedOutOfBand(t *testing.T) {
	name := fmt.Sprintf("test-%04d", rand.Intn(10000))
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccDatabaseResource_CheckDestroy(name),
		Steps: []resource.TestStep{
			{
				Config: testAccDatabaseResource_Config(name),
			},
			{
				PreConfig: func() {
					db := testDatabase()
					if _, err := db.Exec(fmt.Sprintf("DROP DATABASE `%s`", name)); err != nil {
						t.Fatalf("failed dropping database %s: %v", name, err)
					}
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TestAccDatabaseResource_EquivalentNotation checks that a character set or a collation
// written differently from what MySQL reports leaves no diff after apply.
func TestAccDatabaseResource_EquivalentNotation(t *testing.T) {
	cases := map[string]struct{ charset, collation string }{
		"upper_charset":   {"UTF8MB4", "utf8mb4_0900_ai_ci"},
		"upper_collation": {"utf8mb4", "UTF8MB4_BIN"},
		"utf8_alias":      {"utf8", "utf8_general_ci"},
	}
	for caseName, c := range cases {
		t.Run(caseName, func(t *testing.T) {
			name := fmt.Sprintf("test-%04d", rand.Intn(10000))
			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				CheckDestroy:             testAccDatabaseResource_CheckDestroy(name),
				Steps: []resource.TestStep{
					{
						Config: testAccDatabaseResource_ConfigFull(name, c.charset, c.collation),
					},
				},
			})
		})
	}
}

// TestAccDatabaseResource_DetectsOutOfBandCharsetChange checks that a real change made
// outside of Terraform is still reported, even with a configuration written differently
// from what MySQL reports.
func TestAccDatabaseResource_DetectsOutOfBandCharsetChange(t *testing.T) {
	name := fmt.Sprintf("test-%04d", rand.Intn(10000))
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccDatabaseResource_CheckDestroy(name),
		Steps: []resource.TestStep{
			{
				Config: testAccDatabaseResource_ConfigFull(name, "UTF8MB4", "utf8mb4_bin"),
			},
			testAccStepExpectOutOfBandChange(func() {
				db := testDatabase()
				if _, err := db.Exec(fmt.Sprintf("ALTER DATABASE `%s` CHARACTER SET latin1 COLLATE latin1_bin", name)); err != nil {
					t.Fatalf("failed altering database %s: %v", name, err)
				}
			}),
		},
	})
}

// TestAccDatabaseResource_UpdateCharsetKeepsCollation checks that updating the character set
// keeps the configured collation, as `ALTER DATABASE ... CHARACTER SET` alone resets the
// collation to the default one of the character set.
func TestAccDatabaseResource_UpdateCharsetKeepsCollation(t *testing.T) {
	name := fmt.Sprintf("test-%04d", rand.Intn(10000))
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccDatabaseResource_CheckDestroy(name),
		Steps: []resource.TestStep{
			{
				Config: testAccDatabaseResource_ConfigFull(name, "utf8mb4", "utf8mb4_bin"),
			},
			// Only the notation of the character set changes, which plan still reports as a
			// change, so Update issues `ALTER DATABASE ... CHARACTER SET` with the same collation.
			{
				Config: testAccDatabaseResource_ConfigFull(name, "UTF8MB4", "utf8mb4_bin"),
				Check:  testAccDatabaseResource_CheckCollation(name, "utf8mb4_bin"),
			},
		},
	})
}

func testAccDatabaseResource_CheckCollation(name, expected string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		db := testDatabase()
		var collation string
		query := `SELECT DEFAULT_COLLATION_NAME FROM INFORMATION_SCHEMA.SCHEMATA WHERE SCHEMA_NAME = ?`
		if err := db.QueryRow(query, name).Scan(&collation); err != nil {
			return err
		}
		if collation != expected {
			return fmt.Errorf("expected collation %q but was %q", expected, collation)
		}
		return nil
	}
}

func testAccDatabaseResource_Config(name string) string {
	return fmt.Sprintf(`
resource "mysql_database" "test" {
  name = %q
}
`, name)
}

func testAccDatabaseResource_ConfigFull(name, charset, collation string) string {
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
