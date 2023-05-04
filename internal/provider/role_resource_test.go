package provider

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

type testRole struct{
	name string
	host string
}

func TestAccRoleResource(t *testing.T) {
	roles := []testRole{}
	roles = append(roles, testRole{name: fmt.Sprintf("test-role-%d", rand.Intn(100)), host: "%"})
	roles = append(roles, testRole{name: fmt.Sprintf("test-role-%d", rand.Intn(100)), host: "example.com"})
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		CheckDestroy: testAccRoleResource_CheckDestroy(roles),
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccRoleResourceConfig(roles[0].name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_role.test", "name", roles[0].name),
					resource.TestCheckResourceAttr("mysql_role.test", "host", roles[0].host),
					resource.TestCheckResourceAttr("mysql_role.test", "id", fmt.Sprintf("%s@%s", roles[0].name, roles[0].host)),
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
				Config: testAccRoleResourceConfigWithHost(roles[1].name, roles[1].host),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_role.test", "name", roles[1].name),
					resource.TestCheckResourceAttr("mysql_role.test", "host", roles[1].host),
					resource.TestCheckResourceAttr("mysql_role.test", "id", fmt.Sprintf("%s@%s", roles[1].name, roles[1].host)),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccRoleResourceConfig(name string) string {
	config := fmt.Sprintf(`
resource "mysql_role" "test" {
  name = %q
}
`, name)
	return buildConfig(config)
}

func testAccRoleResourceConfigWithHost(name, host string) string {
	config := fmt.Sprintf(`
resource "mysql_role" "test" {
  name = %q
  host = %q
}
`, name, host)
	return buildConfig(config)
}

func testAccRoleResource_CheckDestroy(roles []testRole) resource.TestCheckFunc {
	return func(t *terraform.State) error {
		db := testDatabase()
		sql := "SELECT COUNT(*) FROM mysql.user WHERE user = ? AND host = ?"
		for _, role := range roles {
			var count string
			if err := db.QueryRow(sql, role.name, role.host).Scan(&count); err != nil {
				return err
			}
			if count != "0" {
				return fmt.Errorf("Role still exist (%s@%s): %s", role.name, role.host, count)
			}
		}
		return nil
	}
}

