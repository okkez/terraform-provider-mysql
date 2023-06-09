package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccRoleResource(t *testing.T) {
	roles := []RoleModel{}
	roles = append(roles, NewRandomRole("test-role", "%"))
	roles = append(roles, NewRandomRole("test-role", "example.com"))
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		CheckDestroy:             testAccRoleResource_CheckDestroy(roles),
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccRoleResource_Config(roles[0].Name.ValueString()),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_role.test", "name", roles[0].GetName()),
					resource.TestCheckResourceAttr("mysql_role.test", "host", roles[0].GetHost()),
					resource.TestCheckResourceAttr("mysql_role.test", "id", roles[0].GetID()),
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
					resource.TestCheckResourceAttr("mysql_role.test", "id", roles[1].GetID()),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func TestAccRoleResource_ImportNonExistentRemoteObject(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// ImportState testing
			{
				ResourceName:      "mysql_role.test",
				ImportState:       true,
				ImportStateId:     "non-existent-role@%",
				ImportStateVerify: false,
				Config:            testAccRoleResource_Config("non-existent-role"),
				ExpectError:       regexp.MustCompile("Cannot import non-existent remote object"),
			},
		},
	})
}

func testAccRoleResource_Config(name string) string {
	return fmt.Sprintf(`
resource "mysql_role" "test" {
  name = %q
}
`, name)
}

func testAccRoleResource_ConfigWithHost(name, host string) string {
	return fmt.Sprintf(`
resource "mysql_role" "test" {
  name = %q
  host = %q
}
`, name, host)
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
				return fmt.Errorf("Role still exist (%s): %s", role.GetID(), count)
			}
		}
		return nil
	}
}
