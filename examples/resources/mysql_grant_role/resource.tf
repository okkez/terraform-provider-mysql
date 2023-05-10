resource "mysql_user" "app-user" {
  name = "app_user"
  auth_option {
    auth_string = "app-password"
  }
}

resource "mysql_role" "writer-role-a" {
  name = "writer_role_a"
}
resource "mysql_role" "writer-role-b" {
  name = "writer_role_b"
}
resource "mysql_role" "reader-role-c" {
  name = "reader_role_c"
}

resource "mysql_grant_role" "app-user" {
  to {
    name = mysql_user.app-user.name
  }
  roles = [
    mysql_role.writer-role-a.name,
    mysql_role.writer-role-b.name,
    mysql_role.reader-role-c.name,
  ]
}
