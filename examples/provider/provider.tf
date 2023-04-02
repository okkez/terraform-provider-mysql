terraform {
  required_providers {
    mysql = {
      source = "registry.terraform.io/okkez/mysql"
    }
  }
}

provider "mysql" {
  # example configuration here
}

data "mysql_tables" "example" {}
