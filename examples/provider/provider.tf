terraform {
  required_providers {
    mysql = {
      source = "okkez/mysql"
    }
  }
}

provider "mysql" {
  endpoint = "database.example.com:3306"
  username = "app-username"
  password = "app-password"
}

data "mysql_tables" "example" {
  database = "mysql"
}
