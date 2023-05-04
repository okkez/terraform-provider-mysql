package provider

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccRoleResource(t *testing.T) {
	roles := []RoleModel{}
	roles = append(roles, NewRole(fmt.Sprintf("test-role-%d", rand.Intn(100)), "%"))
	roles = append(roles, NewRole(fmt.Sprintf("test-role-%d", rand.Intn(100)), "example.com"))
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		CheckDestroy: testAccRoleResource_CheckDestroy(roles),
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccRoleResource_Config(roles[0].Name.ValueString()),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_role.test", "name", roles[0].GetName()),
					resource.TestCheckResourceAttr("mysql_role.test", "host", roles[0].GetHost()),
					resource.TestCheckResourceAttr("mysql_role.test", "id", fmt.Sprintf("%s@%s", roles[0].GetName(), roles[0].GetHost())),
				),
			},
			// ImportState testing
			{
				ResourceName:      "mysql_role.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and Read testing
			{
				Config: testAccRoleResource_ConfigWithHost(roles[1].GetName(), roles[1].GetHost()),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_role.test", "name", roles[1].GetName()),
					resource.TestCheckResourceAttr("mysql_role.test", "host", roles[1].GetHost()),
					resource.TestCheckResourceAttr("mysql_role.test", "id", fmt.Sprintf("%s@%s", roles[1].GetName(), roles[1].GetHost())),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccRoleResource_Config(name string) string {
	config := fmt.Sprintf(`
resource "mysql_role" "test" {
  name = %q
}
`, name)
	return buildConfig(config)
}

func testAccRoleResource_ConfigWithHost(name, host string) string {
	config := fmt.Sprintf(`
resource "mysql_role" "test" {
  name = %q
  host = %q
}
`, name, host)
	return buildConfig(config)
}

func testAccRoleResource_CheckDestroy(roles []RoleModel) resource.TestCheckFunc {
	return func(t *terraform.State) error {
		db := testDatabase()
		sql := "SELECT COUNT(*) FROM mysql.user WHERE user = ? AND host = ?"
		for _, role := range roles {
			var count string
			if err := db.QueryRow(sql, role.GetName(), role.GetHost()).Scan(&count); err != nil {
				return err
			}
			if count != "0" {
				return fmt.Errorf("Role still exist (%s@%s): %s", role.GetName(), role.GetHost(), count)
			}
		}
		return nil
	}
}

