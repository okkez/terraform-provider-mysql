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
		// MySQL caps the value at 100000. Apply warns about it instead of leaving a diff.
		"clamped": {"max_connections", "1000000"},
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
	// outOfBandLiteral is written into `SET GLOBAL` as is, because a placeholder passes a
	// string, which a boolean variable does not accept as `0`.
	cases := map[string]struct {
		name, value, outOfBandLiteral string
	}{
		"boolean": {"log_bin_trust_function_creators", "ON", "0"},
		// Role names are case-sensitive, and the order of a list is kept as written.
		"letter_case": {"mandatory_roles", "AppReader", "'appreader'"},
		"list_order":  {"mandatory_roles", "role_a,role_b", "'role_b,role_a'"},
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
					{
						PreConfig: func() {
							db := testDatabase()
							if _, err := db.Exec(fmt.Sprintf("SET GLOBAL %s = %s", c.name, c.outOfBandLiteral)); err != nil {
								t.Fatalf("failed setting %s: %v", c.name, err)
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
		})
	}
}

// TestGlobalVariableValueEquivalent checks the guess deciding whether to warn about a value
// MySQL stored differently from the configured one.
func TestGlobalVariableValueEquivalent(t *testing.T) {
	runEquivalenceCases(t, "globalVariableValueEquivalent", globalVariableValueEquivalent, []equivalenceCase{
		{"1", "ON", true},
		{"1", "on", true},
		{"1", "TRUE", true},
		{"0", "OFF", true},
		{"0", "false", true},
		{"1", "OFF", false},
		{"0", "ON", false},
		{"MINIMAL", "minimal", true},
		{"ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,NO_ENGINE_SUBSTITUTION", "STRICT_TRANS_TABLES,NO_ENGINE_SUBSTITUTION,ONLY_FULL_GROUP_BY", true},
		{"STRICT_TRANS_TABLES", "STRICT_TRANS_TABLES,ONLY_FULL_GROUP_BY", false},
		{"100000", "1000000", false},
		{"2", "2", true},
		{"", "", true},
		{"/var/lib/A", "/var/lib/a", false},
		{"a.b", "A.B", false},
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
