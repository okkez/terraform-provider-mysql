resource "mysql_user" "app" {
  name = "app-user"
}
resource "mysql_role" "writer-role" {
  name = "writer-role"
}
resource "mysql_grant_role" "app-user" {
  to {
    name = mysql_user.app.name
  }
  roles = [
    mysql_role.writer-role.name,
  ]
}

resource "mysql_default_roles" "test" {
  user = mysql_user.app.name

  default_role {
    name = mysql_role.writer-role.name
  }

  # Roles must be granted to users before default roles can be set for them.
  depends_on = [mysql_grant_role.app-user]
}
