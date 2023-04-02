terraform {
  required_providers {
    mysql = {
      source = "registry.terraform.io/okkez/mysql"
    }
  }
}

provider "mysql" {
  # example configuration here
  endpoint = "localhost:33306"
  username = "root"
  password = "password"
}

data "mysql_tables" "test" {
  database = "test"
  #pattern = "%2"
}

output "tables" {
  value = data.mysql_tables.test
}
