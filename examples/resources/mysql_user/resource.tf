# use random password
resource "mysql_user" "test" {
  name = "app-user"
  host = "app.example.com"
  auth_option {
    random_password = true
  }
}

# rotate the password without downtime using MySQL dual password support
# see https://dev.mysql.com/doc/refman/8.0/en/password-management.html#dual-passwords
#
# 1. change `auth_string` and apply with `retain_current_password = true`
#    to keep the old password usable as the secondary password
# 2. deploy the new password to your applications
# 3. apply with `discard_old_password = true` to drop the old password
#
# step 3 is mandatory. removing `retain_current_password` does not discard the old
# password, so skipping it leaves the old password valid forever
resource "mysql_user" "rotating-user" {
  name = "app-user"
  host = "app.example.com"
  auth_option {
    auth_string             = "new-password"
    retain_current_password = true
  }
}

# warn while an account still has a secondary password, so that a rotation which was
# never finished with `discard_old_password` does not go unnoticed
# `check` requires Terraform 1.5 or later. use an output instead on an earlier version
check "no_stale_secondary_password" {
  assert {
    condition     = !mysql_user.rotating-user.has_secondary_password
    error_message = "${mysql_user.rotating-user.id} still has a secondary password. Apply with discard_old_password = true once every consumer uses the new password."
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
