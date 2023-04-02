package provider

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

func getDatabase(ctx context.Context, mysqlConf *MySQLConfiguration) (*sql.DB, error) {
	oneConnection, err := connectToMySQLInternal(ctx, mysqlConf)

	if err != nil {
		return nil, fmt.Errorf("failed to connect to MySQL: %v", err)
	}

	return oneConnection.Db, nil
}

func getDatabaseVersion(ctx context.Context, mysqlConf *MySQLConfiguration) *version.Version {
	oneConnection, err := connectToMySQLInternal(ctx, mysqlConf)
	
	if err != nil {
		tflog.Info(ctx, fmt.Sprintf("getting DB got us error: %v", err))
	}

	return oneConnection.Version
}

func quoteIdentifier(ctx context.Context, db *sql.DB, identifier string) (string, error) {
	var quotedIdentifier string
	if err := db.QueryRowContext(ctx, "SELECT sys.quote_identifier(?)", identifier).Scan(&quotedIdentifier); err != nil {
		return "", err
	}
	return quotedIdentifier, nil
}
