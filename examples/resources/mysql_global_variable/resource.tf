resource "mysql_global_variable" "table_definition_cache" {
  name  = "table_definition_cache"
  value = "4000"
}
