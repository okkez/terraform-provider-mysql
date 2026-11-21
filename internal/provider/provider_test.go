package provider

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"

	"github.com/okkez/terraform-provider-mysql/internal/utils"
)

// testAccStepExpectOutOfBandChange returns a step which makes a change outside of Terraform
// with preConfig and requires the refreshed plan to show it. `ExpectNonEmptyPlan` alone only
// allows a non-empty plan, so the step also requires one with a plan check.
func testAccStepExpectOutOfBandChange(preConfig func()) resource.TestStep {
	return resource.TestStep{
		PreConfig:          preConfig,
		RefreshState:       true,
		ExpectNonEmptyPlan: true,
		RefreshPlanChecks: resource.RefreshPlanChecks{
			PostRefresh: []plancheck.PlanCheck{plancheck.ExpectNonEmptyPlan()},
		},
	}
}

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

func TestMysqlErrorNumber(t *testing.T) {
	t.Parallel()
	if got := mysqlErrorNumber(&mysql.MySQLError{Number: 1193}); got != unknownSystemVariableErrorNumber {
		t.Errorf("mysqlErrorNumber(MySQLError 1193): got %d, want %d", got, unknownSystemVariableErrorNumber)
	}
	if got := mysqlErrorNumber(fmt.Errorf("not a mysql error")); got != 0 {
		t.Errorf("mysqlErrorNumber(non-MySQL error): got %d, want 0", got)
	}
	if got := mysqlErrorNumber(nil); got != 0 {
		t.Errorf("mysqlErrorNumber(nil): got %d, want 0", got)
	}
}
