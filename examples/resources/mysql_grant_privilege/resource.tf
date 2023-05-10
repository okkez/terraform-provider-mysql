resource "mysql_role" "writer-role" {
  name = "writer-role"
}

resource "mysql_grant_privilege" "writer-role" {
  privilege {
    priv_type = "SELECT"
  }
  privilege {
    priv_type = "INSERT"
  }
  priv_type {
    priv_type = "UPDATE"
  }
  priv_type {
    priv_type = "DELETE"
  }
  on {
    database = "app"
    table    = "users"
  }
  to {
    name = mysql_user.app-user.name
    host = "app.example.com"
  }
}
