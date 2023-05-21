package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/okkez/terraform-provider-mysql/internal/utils"
)

func TestAccDefaultRoleResource(t *testing.T) {
	user := NewRandomUser("test-user", "%")
	role1 := NewRandomRole("test-role", "%")
	role2 := NewRandomRole("test-role", "%")
	t.Logf("user: %s, role1: %s, role2: %s", user.GetID(), role1.GetID(), role2.GetID())
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		CheckDestroy:             testAccTestAccDefaultRoleResource_CheckDestroy(user),
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccDefaultRoleResource_Config(t, user.GetName(), role1.GetName(), role2.GetName(), []string{"role1"}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_default_roles.test", "id", user.GetID()),
					resource.TestCheckResourceAttr("mysql_default_roles.test", "user", user.GetName()),
					resource.TestCheckResourceAttr("mysql_default_roles.test", "host", user.GetHost()),
					resource.TestCheckResourceAttr("mysql_default_roles.test", "default_role.#", "1"),
					resource.TestCheckResourceAttr("mysql_default_roles.test", "default_role.0.name", role1.GetName()),
					resource.TestCheckResourceAttr("mysql_default_roles.test", "default_role.0.host", "%"),
				),
			},
			// ImportState testing
			{
				ResourceName:      "mysql_default_roles.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
			// Update and Read testing
			{
				Config: testAccDefaultRoleResource_Config(t, user.GetName(), role1.GetName(), role2.GetName(), []string{"role2"}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_default_roles.test", "id", user.GetID()),
					resource.TestCheckResourceAttr("mysql_default_roles.test", "user", user.GetName()),
					resource.TestCheckResourceAttr("mysql_default_roles.test", "host", user.GetHost()),
					resource.TestCheckResourceAttr("mysql_default_roles.test", "default_role.#", "1"),
					resource.TestCheckResourceAttr("mysql_default_roles.test", "default_role.0.name", role2.GetName()),
					resource.TestCheckResourceAttr("mysql_default_roles.test", "default_role.0.host", "%"),
				),
			},
			{
				Config: testAccDefaultRoleResource_Config(t, user.GetName(), role1.GetName(), role2.GetName(), []string{"role1", "role2"}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_default_roles.test", "id", user.GetID()),
					resource.TestCheckResourceAttr("mysql_default_roles.test", "user", user.GetName()),
					resource.TestCheckResourceAttr("mysql_default_roles.test", "host", user.GetHost()),
					resource.TestCheckResourceAttr("mysql_default_roles.test", "default_role.#", "2"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func testAccDefaultRoleResource_Config(t *testing.T, user, role1, role2 string, defaultRoles []string) string {
	source := `
resource "mysql_user" "test" {
  name = "{{ .User }}"
}
resource "mysql_role" "role1" {
  name = "{{ .Role1 }}"
}
resource "mysql_role" "role2" {
  name = "{{ .Role2 }}"
}
resource "mysql_grant_role" "test" {
  to {
    name = mysql_user.test.name
  }
  role {
    name = mysql_role.role1.name
  }
  role {
    name = mysql_role.role2.name
  }
}
resource "mysql_default_roles" "test" {
  user = mysql_user.test.name
  {{- range $i, $role := .DefaultRoles }}
  default_role {
    name = mysql_role.{{ $role }}.name
  }
  {{- end }}
  depends_on = [mysql_grant_role.test]
}
`
	data := struct {
		User         string
		Role1        string
		Role2        string
		DefaultRoles []string
	}{
		User:         user,
		Role1:        role1,
		Role2:        role2,
		DefaultRoles: defaultRoles,
	}
	config, err := utils.Render(source, data)
	if err != nil {
		t.Fatal(err)
		t.Fail()
	}
	return config
}

func testAccTestAccDefaultRoleResource_CheckDestroy(user UserModel) resource.TestCheckFunc {
	return func(t *terraform.State) error {
		db := testDatabase()
		sql := `SELECT COUNT(*) FROM mysql.default_roles WHERE USER = ? AND HOST = ?`
		var count string
		if err := db.QueryRow(sql, user.GetName(), user.GetHost()).Scan(&count); err != nil {
			return err
		}
		if count != "0" {
			return fmt.Errorf("Default roles still exist (%s)", user.GetID())
		}
		return nil
	}
}
