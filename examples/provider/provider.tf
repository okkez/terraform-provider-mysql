terraform {
  required_providers {
    mysql = {
      source = "okkez/mysql"
    }
  }
}

provider "mysql" {
  # example configuration here
}

data "mysql_tables" "example" {
  database = "mysql"
}
