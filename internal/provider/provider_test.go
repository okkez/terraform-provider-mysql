package provider

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	"github.com/okkez/terraform-provider-mysql/internal/utils"
)

// testAccProtoV6ProviderFactories are used to instantiate a provider during
// acceptance testing. The factory function will be invoked for every Terraform
// CLI command executed to create a provider server to which the CLI can
// reattach.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"mysql": providerserver.NewProtocol6WithError(New("test")()),
}

func testAccPreCheck(t *testing.T) {
	// You can add code here to run prior to any test case execution, for example assertions
	// about the appropriate environment variables being set are common to see in a pre-check
	// function.
}

func testDatabase() *sql.DB {
	ctx := context.Background()
	db, err := getDatabase(ctx, testMySQLConfig())
	if err != nil {
		panic(err.Error())
	}
	return db
}

func testMySQLConfig() *MySQLConfiguration {
	endpoint := utils.GetenvWithDefault("MYSQL_ENDPOINT", "localhost:3306")
	username := utils.GetenvWithDefault("MYSQL_USERNAME", "root")
	password := utils.GetenvWithDefault("MYSQL_PASSWORD", "password")
	conf := mysql.Config{
		User:      username,
		Passwd:    password,
		Net:       "tcp",
		Addr:      endpoint,
		TLSConfig: "false",
	}
	return &MySQLConfiguration{
		Config:              &conf,
		MaxConnLifetime:     time.Duration(8*60*60) * time.Second,
		MaxOpenConns:        5,
		ConnectRetryTimeout: time.Duration(300) * time.Second,
	}
}
