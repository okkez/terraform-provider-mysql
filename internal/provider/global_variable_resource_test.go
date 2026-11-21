package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
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

// TestAccGlobalVariableResource_EquivalentNotation checks that a value written differently
// from what MySQL reports leaves no diff after apply.
func TestAccGlobalVariableResource_EquivalentNotation(t *testing.T) {
	cases := map[string]struct{ name, value string }{
		"boolean_on":     {"log_bin_trust_function_creators", "ON"},
		"enum_lowercase": {"binlog_row_image", "minimal"},
		"set_order":      {"sql_mode", "STRICT_TRANS_TABLES,NO_ENGINE_SUBSTITUTION,ONLY_FULL_GROUP_BY"},
	}
	for caseName, c := range cases {
		t.Run(caseName, func(t *testing.T) {
			resource.Test(t, resource.TestCase{
				PreCheck:                 func() { testAccPreCheck(t) },
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: testAccGlobalVariableResourceConfig(c.name, c.value),
					},
				},
			})
		})
	}
}

// TestAccGlobalVariableResource_DetectsOutOfBandChange checks that a real change made
// outside of Terraform is still reported, even with a value written differently from
// what MySQL reports.
func TestAccGlobalVariableResource_DetectsOutOfBandChange(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccGlobalVariableResourceConfig("log_bin_trust_function_creators", "ON"),
			},
			{
				PreConfig: func() {
					db := testDatabase()
					if _, err := db.Exec("SET GLOBAL log_bin_trust_function_creators = 0"); err != nil {
						t.Fatalf("failed setting log_bin_trust_function_creators: %v", err)
					}
				},
				RefreshState:       true,
				ExpectNonEmptyPlan: true,
				// `ExpectNonEmptyPlan` only allows a non-empty plan, so require one explicitly.
				RefreshPlanChecks: resource.RefreshPlanChecks{
					PostRefresh: []plancheck.PlanCheck{plancheck.ExpectNonEmptyPlan()},
				},
			},
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
