package provider

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccGrantPrivilegeResource(t *testing.T) {
	database := fmt.Sprintf("test-database-%04d", rand.Intn(1000))
	user := NewRandomUser("test-user", "%")
	t.Logf("database: %s user: %s", database, user.GetID())
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccGrantPrivilegeResource_Config(database, user.GetName(), "SELECT"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.#", "1"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.priv_type", "SELECT"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.columns.#", "0"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "on.database", database),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "on.table", "*"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "to.name", user.GetName()),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "to.host", user.GetHost()),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "grant_option", "false"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "mysql_grant_privilege.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and Read testing
			{
				Config: testAccGrantPrivilegeResource_Config(database, user.GetName(), "ALL PRIVILEGES"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.#", "1"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.priv_type", "ALL PRIVILEGES"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.columns.#", "0"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "on.database", database),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "on.table", "*"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "to.name", user.GetName()),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "to.host", user.GetHost()),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "grant_option", "false"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccGrantPrivilegeResource_Config(database, user, privilege string) string {
	config := fmt.Sprintf(`
resource "mysql_database" "test" {
  name = %q
}
resource "mysql_user" "test" {
  name = %q
}
resource "mysql_grant_privilege" "test" {
  privilege {
    priv_type = %q
  }
  on {
    database = mysql_database.test.name
    table = "*"
  }
  to {
    name = mysql_user.test.name
    host = mysql_user.test.host
  }
}
`, database, user, privilege)
	return buildConfig(config)
}
