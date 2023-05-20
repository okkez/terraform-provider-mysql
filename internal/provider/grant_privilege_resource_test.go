package provider

import (
	"bytes"
	"fmt"
	"math/rand"
	"regexp"
	"testing"
	"text/template"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccGrantPrivilegeResource(t *testing.T) {
	database := fmt.Sprintf("test_database_%04d", rand.Intn(1000))
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

func TestAccGrantPrivilegeResource_LowerCase(t *testing.T) {
	database := fmt.Sprintf("test_database_%04d", rand.Intn(1000))
	user := NewRandomUser("test-user", "%")
	t.Logf("database: %s user: %s", database, user.GetID())
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config:      testAccGrantPrivilegeResource_Config(database, user.GetName(), "select"),
				ExpectError: regexp.MustCompile(`must be upper cases`),
			},
		},
	})
}

func TestAccGrantPrivilegeResource_Table(t *testing.T) {
	database := fmt.Sprintf("test_database_%04d", rand.Intn(1000))
	table := fmt.Sprintf("test_table_%04d", rand.Intn(1000))
	testAccGrantPrivilegeResource_PrepareTable(t, database, table)
	t.Cleanup(testAccGrantPrivilegeResource_Cleanup(t, database))
	user := NewRandomUser("test-user", "%")
	t.Logf("database: %s table: %s, user: %s", database, table, user.GetID())
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccGrantPrivilegeResource_ConfigWithTable(database, table, user.GetName(), "SELECT"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.#", "1"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.priv_type", "SELECT"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.columns.#", "0"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "on.database", database),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "on.table", table),
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
				Config: testAccGrantPrivilegeResource_ConfigWithTable(database, table, user.GetName(), "ALL PRIVILEGES"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.#", "1"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.priv_type", "ALL PRIVILEGES"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.columns.#", "0"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "on.database", database),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "on.table", table),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "to.name", user.GetName()),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "to.host", user.GetHost()),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "grant_option", "false"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func TestAccGrantPrivilegeResource_Columns(t *testing.T) {
	database := fmt.Sprintf("test_database_%04d", rand.Intn(1000))
	table := fmt.Sprintf("test_table_%04d", rand.Intn(1000))
	testAccGrantPrivilegeResource_PrepareTable(t, database, table)
	t.Cleanup(testAccGrantPrivilegeResource_Cleanup(t, database))
	user := NewRandomUser("test-user", "%")
	t.Logf("database: %s table: %s user: %s", database, table, user.GetID())
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccGrantPrivilegeResource_ConfigWithColumns(database, table, user.GetName(), "SELECT", `["name", "email"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.#", "1"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.priv_type", "SELECT"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.columns.#", "2"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.columns.0", "email"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.columns.1", "name"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "on.database", database),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "on.table", table),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "to.name", user.GetName()),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "to.host", user.GetHost()),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "grant_option", "true"),
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
				Config: testAccGrantPrivilegeResource_ConfigWithColumns(database, table, user.GetName(), "SELECT", `["name", "email", "address"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.#", "1"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.priv_type", "SELECT"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.columns.#", "3"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.columns.0", "address"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.columns.1", "email"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.columns.2", "name"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "on.database", database),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "on.table", table),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "to.name", user.GetName()),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "to.host", user.GetHost()),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "grant_option", "true"),
				),
			},
			{
				Config: testAccGrantPrivilegeResource_ConfigWithColumns(database, table, user.GetName(), "SELECT", `["name", "email"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.#", "1"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.priv_type", "SELECT"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.columns.#", "2"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.columns.0", "email"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.columns.1", "name"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "on.database", database),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "on.table", table),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "to.name", user.GetName()),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "to.host", user.GetHost()),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "grant_option", "true"),
				),
			},
			{
				Config: testAccGrantPrivilegeResource_ConfigWithColumns(database, table, user.GetName(), "SELECT", `["name", "address"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.#", "1"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.priv_type", "SELECT"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.columns.#", "2"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.columns.0", "address"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.columns.1", "name"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "on.database", database),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "on.table", table),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "to.name", user.GetName()),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "to.host", user.GetHost()),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "grant_option", "true"),
				),
			},
			{
				Config: testAccGrantPrivilegeResource_ConfigWithPrivilegesDatabase(database, table, user.GetName(), `["name", "address"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.#", "2"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.priv_type", "INSERT"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.columns.#", "2"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.columns.0", "address"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.columns.1", "name"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.1.priv_type", "SELECT"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.1.columns.#", "2"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.1.columns.0", "address"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.1.columns.1", "name"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "on.database", database),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "on.table", table),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "to.name", user.GetName()),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "to.host", user.GetHost()),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "grant_option", "true"),
				),
			},
			{
				Config: testAccGrantPrivilegeResource_ConfigWithColumns(database, table, user.GetName(), "INSERT", `["name", "email"]`),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.#", "1"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.priv_type", "INSERT"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.columns.#", "2"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.columns.0", "email"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.columns.1", "name"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "on.database", database),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "on.table", table),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "to.name", user.GetName()),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "to.host", user.GetHost()),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "grant_option", "true"),
				),
			},
			// Delete testing automatically occurs in TestCase
		},
	})
}

func TestAccGrantPrivilegeResource_DynamicPrivileges(t *testing.T) {
	database := fmt.Sprintf("test_database_%04d", rand.Intn(1000))
	user := NewRandomUser("test-user", "%")
	t.Logf("database: %s user: %s", database, user.GetID())
	dynamicPrivileges := []string{
		"SELECT", // to avoid USAGE privilege.
		"APPLICATION_PASSWORD_ADMIN",
		// AUDIT* is available for MySQL Enterprise Edition.
		// "AUDIT_ABORT_EXEMPT",
		// "AUDIT_ADMIN",
		"AUTHENTICATION_POLICY_ADMIN",
		"BACKUP_ADMIN",
		"BINLOG_ADMIN",
		"BINLOG_ENCRYPTION_ADMIN",
		"CLONE_ADMIN",
		"CONNECTION_ADMIN",
		"ENCRYPTION_KEY_ADMIN",
		// FIREWALL* is available for MySQL Enterprise Edition.
		// "FIREWALL_ADMIN",
		// "FIREWALL_EXEMPT",
		// "FIREWALL_USER",
		"FLUSH_OPTIMIZER_COSTS",
		"FLUSH_STATUS",
		"FLUSH_TABLES",
		"FLUSH_USER_RESOURCES",
		"GROUP_REPLICATION_ADMIN",
		"INNODB_REDO_LOG_ARCHIVE",
		"INNODB_REDO_LOG_ENABLE",
		// NDB_STORED_USER is available for NDB cluster.
		// "NDB_STORED_USER",
		"PASSWORDLESS_USER_ADMIN",
		"PERSIST_RO_VARIABLES_ADMIN",
		"REPLICATION_APPLIER",
		"REPLICATION_SLAVE_ADMIN",
		"RESOURCE_GROUP_ADMIN",
		"RESOURCE_GROUP_USER",
		"ROLE_ADMIN",
		"SESSION_VARIABLES_ADMIN",
		"SET_USER_ID",
		"SHOW_ROUTINE",
		// SKIP_QUERY_REWRITE is available if Query Rewrite Plugins are installed.
		// "SKIP_QUERY_REWRITE",
		"SYSTEM_USER",
		"SYSTEM_VARIABLES_ADMIN",
		"TABLE_ENCRYPTION_ADMIN",
		// "TP_CONNECTION_ADMIN",
		// VERSION_TOKEN_ADMIN is available if version_tokens plugin is installed.
		// "VERSION_TOKEN_ADMIN",
		"XA_RECOVER_ADMIN",
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccGrantPrivilegeResource_ConfigWithPrivileges(t, user.GetName(), []string{"SELECT"}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.#", "1"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.priv_type", "SELECT"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.columns.#", "0"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "on.database", "*"),
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
				Config: testAccGrantPrivilegeResource_ConfigWithPrivileges(t, user.GetName(), dynamicPrivileges),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.#", fmt.Sprintf("%d", len(dynamicPrivileges))),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.priv_type", "APPLICATION_PASSWORD_ADMIN"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "on.database", "*"),
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

func TestAccGrantPrivilegeResource_StaticPrivileges(t *testing.T) {
	database := fmt.Sprintf("test_database_%04d", rand.Intn(1000))
	user := NewRandomUser("test-user", "%")
	t.Logf("database: %s user: %s", database, user.GetID())
	staticPrivileges := []string{
		"ALTER",
		"ALTER ROUTINE",
		"CREATE",
		"CREATE ROLE",
		"CREATE ROUTINE",
		"CREATE TABLESPACE",
		"CREATE TEMPORARY TABLES",
		"CREATE USER",
		"CREATE VIEW",
		"DELETE",
		"DROP",
		"DROP ROLE",
		"EVENT",
		"EXECUTE",
		"FILE",
		"INDEX",
		"INSERT",
		"LOCK TABLES",
		"PROCESS",
		"REFERENCES",
		"RELOAD",
		"REPLICATION CLIENT",
		"REPLICATION SLAVE",
		"SELECT",
		"SHOW DATABASES",
		"SHOW VIEW",
		"SHUTDOWN",
		"SUPER",
		"TRIGGER",
		"UPDATE",
	}

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// Create and Read testing
			{
				Config: testAccGrantPrivilegeResource_ConfigWithPrivileges(t, user.GetName(), []string{"SELECT"}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.#", "1"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.priv_type", "SELECT"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.columns.#", "0"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "on.database", "*"),
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
				Config: testAccGrantPrivilegeResource_ConfigWithPrivileges(t, user.GetName(), staticPrivileges),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.#", fmt.Sprintf("%d", len(staticPrivileges))),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "privilege.0.priv_type", "ALTER ROUTINE"),
					resource.TestCheckResourceAttr("mysql_grant_privilege.test", "on.database", "*"),
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
	return fmt.Sprintf(`
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
}

func testAccGrantPrivilegeResource_ConfigWithTable(database, table, user, privilege string) string {
	return fmt.Sprintf(`
data "mysql_database" "test" {
  database = %q
}
resource "mysql_user" "test" {
  name = %q
}
resource "mysql_grant_privilege" "test" {
  privilege {
    priv_type = %q
  }
  on {
    database = data.mysql_database.test.database
    table = %q
  }
  to {
    name = mysql_user.test.name
    host = mysql_user.test.host
  }
}
`, database, user, privilege, table)
}

func testAccGrantPrivilegeResource_ConfigWithColumns(database, table, user, privilege, columns string) string {
	return fmt.Sprintf(`
data "mysql_database" "test" {
  database = %q
}
resource "mysql_user" "test" {
  name = %q
}
resource "mysql_grant_privilege" "test" {
  privilege {
    priv_type = %q
    columns = %s
  }
  on {
    database = data.mysql_database.test.database
    table = %q
  }
  to {
    name = mysql_user.test.name
    host = mysql_user.test.host
  }
  grant_option = true
}
`, database, user, privilege, columns, table)
}

func testAccGrantPrivilegeResource_ConfigWithPrivilegesDatabase(database, table, user, columns string) string {
	return fmt.Sprintf(`
data "mysql_database" "test" {
  database = %q
}
resource "mysql_user" "test" {
  name = %q
}
resource "mysql_grant_privilege" "test" {
  privilege {
    priv_type = "SELECT"
    columns = %s
  }
  privilege {
    priv_type = "INSERT"
    columns = %s
  }
  on {
    database = data.mysql_database.test.database
    table = %q
  }
  to {
    name = mysql_user.test.name
    host = mysql_user.test.host
  }
  grant_option = true
}
`, database, user, columns, columns, table)
}

func testAccGrantPrivilegeResource_ConfigWithPrivileges(t *testing.T, user string, privileges []string) string {
	configTemplate := `
resource "mysql_user" "test" {
  name = "{{ .User }}"
}
resource "mysql_grant_privilege" "test" {
{{- range $i, $p := .Privileges }}
  privilege {
    priv_type = "{{ $p }}"
  }
{{- end }}
  on {
    database = "*"
    table = "*"
  }
  to {
    name = mysql_user.test.name
    host = mysql_user.test.host
  }
}
`
	tpl, err := template.New("main.tf").Parse(configTemplate)
	if err != nil {
		t.Fatal(err)
		t.Fail()
	}
	w := new(bytes.Buffer)
	err = tpl.Execute(w, struct {
		User       string
		Privileges []string
	}{
		User:       user,
		Privileges: privileges,
	})
	if err != nil {
		t.Fatal(err)
		t.Fail()
	}
	return string(w.Bytes())
}

func testAccGrantPrivilegeResource_PrepareTable(t *testing.T, database, table string) {
	db := testDatabase()

	if _, err := db.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", database)); err != nil {
		t.Error(err.Error())
		t.FailNow()
	}

	if _, err := db.Exec(fmt.Sprintf("CREATE TABLE %s.%s (name text, email text, address text)", database, table)); err != nil {
		t.Error(err.Error())
		t.FailNow()
	}
}

func testAccGrantPrivilegeResource_Cleanup(t *testing.T, database string) func() {
	return func() {
		db := testDatabase()

		if _, err := db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", database)); err != nil {
			t.Error(err.Error())
			t.FailNow()
		}
	}
}
