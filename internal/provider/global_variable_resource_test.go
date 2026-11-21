package provider

import (
	"context"
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
				PreCheck: func() {
					testAccPreCheck(t)
					testAccGlobalVariableResource_RestoreAfterTest(t, c.name)
				},
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

// TestAccGlobalVariableResource_DetectsRevertToAppliedValue checks that a value changed back
// to the applied one is still reported, after the configuration followed an out-of-band
// change without running apply.
func TestAccGlobalVariableResource_DetectsRevertToAppliedValue(t *testing.T) {
	const name = "max_connections"
	setOutOfBand := func(value string) func() {
		return func() {
			if err := setGlobalVariable(context.Background(), testDatabase(), name, value); err != nil {
				t.Fatalf("failed setting %s: %v", name, err)
			}
		}
	}
	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)
			testAccGlobalVariableResource_RestoreAfterTest(t, name)
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccGlobalVariableResourceConfig(name, "500"),
			},
			testAccStepExpectOutOfBandChange(setOutOfBand("1000")),
			// The configuration follows the server, so apply has nothing to do.
			{
				Config: testAccGlobalVariableResourceConfig(name, "1000"),
			},
			testAccStepExpectOutOfBandChange(setOutOfBand("500")),
		},
	})
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
				PreCheck: func() {
					testAccPreCheck(t)
					testAccGlobalVariableResource_RestoreAfterTest(t, c.name)
				},
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config: testAccGlobalVariableResourceConfig(c.name, c.value),
					},
					testAccStepExpectOutOfBandChange(func() {
						db := testDatabase()
						if _, err := db.Exec(fmt.Sprintf("SET GLOBAL %s = %s", c.name, c.outOfBandLiteral)); err != nil {
							t.Fatalf("failed setting %s: %v", c.name, err)
						}
					}),
				},
			})
		})
	}
}

// TestGlobalVariableValueEquivalent checks the guess deciding whether to warn about a value
// MySQL stored differently from the configured one.
func TestGlobalVariableValueEquivalent(t *testing.T) {
	runEquivalenceCases(t, "globalVariableValueEquivalent", globalVariableValueEquivalent, []equivalenceCase{
		// Exactly the same values.
		{"2", "2", true},
		{"", "", true},
		// Boolean words against what MySQL reports for boolean variables.
		{"1", "ON", true},
		{"1", "on", true},
		{"1", "TRUE", true},
		{"0", "OFF", true},
		{"0", "false", true},
		{"1", "OFF", false},
		{"0", "ON", false},
		// Words separated by commas, ignoring letter case and order.
		{"MINIMAL", "minimal", true},
		{"READ-COMMITTED", "read-committed", true},
		{"ONLY_FULL_GROUP_BY,STRICT_TRANS_TABLES,NO_ENGINE_SUBSTITUTION", "STRICT_TRANS_TABLES,NO_ENGINE_SUBSTITUTION,ONLY_FULL_GROUP_BY", true},
		{"STRICT_TRANS_TABLES", "STRICT_TRANS_TABLES,ONLY_FULL_GROUP_BY", false},
		// A number capped by MySQL.
		{"100000", "1000000", false},
		// Values with other characters, such as paths, are compared exactly.
		{"/var/lib/A", "/var/lib/a", false},
		{"a.b", "A.B", false},
	})
}

// testAccGlobalVariableResource_RestoreAfterTest restores the value the server had before the
// test, because destroying the resource sets the default value, which is not necessarily the
// value the server was configured with.
func testAccGlobalVariableResource_RestoreAfterTest(t *testing.T, name string) {
	t.Helper()
	ctx := context.Background()
	db := testDatabase()
	originalValue, err := readGlobalVariable(ctx, db, name)
	if err != nil {
		if mysqlErrorNumber(err) == unknownSystemVariableErrorNumber {
			t.Skipf("the server has no %s", name)
		}
		t.Fatalf("failed reading %s: %v", name, err)
	}
	t.Cleanup(func() {
		if err := setGlobalVariable(ctx, db, name, originalValue); err != nil {
			t.Errorf("failed restoring %s: %v", name, err)
		}
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
