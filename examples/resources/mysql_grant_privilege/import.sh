# Grant privileges can be imported by specifying `database@table@name@host`
# All parts are required.
terraform import mysql_grant_privilege.my-database-app-user db@*@app-user@app.example.com
