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
  privilege {
    priv_type = "UPDATE"
  }
  privilege {
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
