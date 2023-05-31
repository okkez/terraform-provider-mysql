package provider

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/okkez/terraform-provider-mysql/internal/utils"
)

func TestAccGrantRoleResource(t *testing.T) {
	user := NewRandomUser("test-user", "%")
	role0 := NewRandomRole("test-role0", "%")
	role1 := NewRandomRole("test-role1", "%")
	roles := []RoleModel{role0, role1}
	t.Logf("user: %s, role1: %s, role2: %s", user.GetName(), role0.GetName(), role1.GetName())
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccGrantRoleResource_Config(t, user.GetName(), roles, []string{"role0"}, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_grant_role.test", "to.name", user.GetName()),
					resource.TestCheckResourceAttr("mysql_grant_role.test", "to.host", user.GetHost()),
					resource.TestCheckResourceAttr("mysql_grant_role.test", "role.#", "1"),
					resource.TestCheckResourceAttr("mysql_grant_role.test", "role.0.name", role0.GetName()),
					resource.TestCheckResourceAttr("mysql_grant_role.test", "id", user.GetID()),
				),
			},
			// ImportState testing
			{
				ResourceName:            "mysql_grant_role.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"role.0.host"},
			},
			// Update and Read testing
			{
				Config: testAccGrantRoleResource_Config(t, user.GetName(), roles, []string{"role1"}, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_grant_role.test", "to.name", user.GetName()),
					resource.TestCheckResourceAttr("mysql_grant_role.test", "to.host", user.GetHost()),
					resource.TestCheckResourceAttr("mysql_grant_role.test", "role.#", "1"),
					resource.TestCheckResourceAttr("mysql_grant_role.test", "role.0.name", role1.GetName()),
					resource.TestCheckResourceAttr("mysql_grant_role.test", "id", user.GetID()),
				),
			},
			{
				Config: testAccGrantRoleResource_Config(t, user.GetName(), roles, []string{"role0", "role1"}, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_grant_role.test", "to.name", user.GetName()),
					resource.TestCheckResourceAttr("mysql_grant_role.test", "to.host", user.GetHost()),
					resource.TestCheckResourceAttr("mysql_grant_role.test", "role.#", "2"),
					resource.TestCheckResourceAttr("mysql_grant_role.test", "role.0.name", role0.GetName()),
					resource.TestCheckResourceAttr("mysql_grant_role.test", "role.1.name", role1.GetName()),
					resource.TestCheckResourceAttr("mysql_grant_role.test", "id", user.GetID()),
				),
			},
			{
				Config: testAccGrantRoleResource_Config(t, user.GetName(), roles, []string{"role1"}, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_grant_role.test", "to.name", user.GetName()),
					resource.TestCheckResourceAttr("mysql_grant_role.test", "to.host", user.GetHost()),
					resource.TestCheckResourceAttr("mysql_grant_role.test", "role.#", "1"),
					resource.TestCheckResourceAttr("mysql_grant_role.test", "role.0.name", role1.GetName()),
					resource.TestCheckResourceAttr("mysql_grant_role.test", "id", user.GetID()),
				),
			},
			{
				Config: testAccGrantRoleResource_Config(t, user.GetName(), roles, []string{"role1"}, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_grant_role.test", "to.name", user.GetName()),
					resource.TestCheckResourceAttr("mysql_grant_role.test", "to.host", user.GetHost()),
					resource.TestCheckResourceAttr("mysql_grant_role.test", "role.#", "1"),
					resource.TestCheckResourceAttr("mysql_grant_role.test", "role.0.name", role1.GetName()),
					resource.TestCheckResourceAttr("mysql_grant_role.test", "role.0.host", role1.GetHost()),
					resource.TestCheckResourceAttr("mysql_grant_role.test", "id", user.GetID()),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func TestAccGrantRoleResource_ImportNonExistentRemoteObject(t *testing.T) {
	role0 := NewRandomRole("test-role0", "%")
	role1 := NewRandomRole("test-role1", "%")
	roles := []RoleModel{role0, role1}
	t.Logf("role1: %s, role2: %s", role0.GetName(), role1.GetName())
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// ImportState testing
			{
				ResourceName:      "mysql_grant_role.test",
				ImportState:       true,
				ImportStateId:     "non-existent-user@%",
				ImportStateVerify: false,
				Config:            testAccGrantRoleResource_ConfigWithNonExistentUser(t, "non-existent-user", roles, []string{"role0"}),
				ExpectError:       regexp.MustCompile("Cannot import non-existent remote object"),
			},
		},
	})
}

func testAccGrantRoleResource_Config(t *testing.T, user string, roles []RoleModel, grantedRoles []string, withHost bool) string {
	source := `
resource "mysql_user" "test" {
  name = "{{ .User }}"
}
{{- range $i, $role := .Roles }}
resource "mysql_role" "role{{ $i }}" {
  name = "{{ $role.GetName }}"
  {{- if $.WithHost }}
  host = "%"
  {{- end }}
}
{{- end }}
resource "mysql_grant_role" "test" {
  to {
    name = mysql_user.test.name
  }
  {{- range $1, $role := .GrantedRoles }}
  role {
    name = mysql_role.{{ $role }}.name
    {{- if $.WithHost }}
    host = "%"
    {{- end }}
  }
  {{- end }}
}
`
	data := struct {
		User         string
		Roles        []RoleModel
		GrantedRoles []string
		WithHost     bool
	}{
		User:         user,
		Roles:        roles,
		GrantedRoles: grantedRoles,
		WithHost:     withHost,
	}
	config, err := utils.Render(source, data)
	if err != nil {
		t.Fatal(err)
		t.Fail()
	}
	return config
}

func testAccGrantRoleResource_ConfigWithNonExistentUser(t *testing.T, user string, roles []RoleModel, grantedRoles []string) string {
	source := `
{{- range $i, $role := .Roles }}
resource "mysql_role" "role{{ $i }}" {
  name = "{{ $role.GetName }}"
}
{{- end }}
resource "mysql_grant_role" "test" {
  to {
    name = "non-existent-user"
  }
  {{- range $1, $role := .GrantedRoles }}
  role {
    name = mysql_role.{{ $role }}.name
  }
  {{- end }}
}
`
	data := struct {
		User         string
		Roles        []RoleModel
		GrantedRoles []string
	}{
		User:         user,
		Roles:        roles,
		GrantedRoles: grantedRoles,
	}
	config, err := utils.Render(source, data)
	if err != nil {
		t.Fatal(err)
		t.Fail()
	}
	return config
}
