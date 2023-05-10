data "mysql_tables" "mysql" {
  database = "mysql"
  pattern  = "help%"
}
