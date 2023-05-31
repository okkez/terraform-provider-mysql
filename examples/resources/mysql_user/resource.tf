# use random password
resource "mysql_user" "test" {
  name = "app-user"
  host = "app.example.com"
  auth_option {
    random_password = true
  }
}

# use RDS IAM DB Auth
# see https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/UsingWithRDS.IAMDBAuth.html
resource "mysql_user" "rds-user" {
  name = "app-user"
  auth_option {
    plugin = "AWSAuthenticationPlugin"
  }
}
