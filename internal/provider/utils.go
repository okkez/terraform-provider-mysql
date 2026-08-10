package provider

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/hashicorp/go-version"
)

func getDatabase(ctx context.Context, mysqlConf *MySQLConfiguration) (*sql.DB, error) {
	oneConnection, err := connectToMySQLInternal(ctx, mysqlConf)

	if err != nil {
		return nil, fmt.Errorf("failed to connect to MySQL: %v", err)
	}

	return oneConnection.Db, nil
}

// getDatabaseVersion returns the server version determined when connecting.
// The version is cached per DSN, so this does not query the server again.
func getDatabaseVersion(ctx context.Context, mysqlConf *MySQLConfiguration) (*version.Version, error) {
	oneConnection, err := connectToMySQLInternal(ctx, mysqlConf)

	if err != nil {
		return nil, fmt.Errorf("failed to connect to MySQL: %v", err)
	}

	return oneConnection.Version, nil
}

func quoteIdentifier(ctx context.Context, db *sql.DB, identifier string) (string, error) {
	var quotedIdentifier string
	stmt, err := db.PrepareContext(ctx, "SELECT sys.quote_identifier(?)")
	if err != nil {
		return "", err
	}
	defer func() { _ = stmt.Close() }()
	if err := stmt.QueryRowContext(ctx, identifier).Scan(&quotedIdentifier); err != nil {
		return "", err
	}
	return quotedIdentifier, nil
}

func quoteIdentifiers(ctx context.Context, db *sql.DB, identifiers ...string) ([]string, error) {
	quotedIdentifiers := make([]string, len(identifiers))
	var err error
	for i, identifier := range identifiers {
		quotedIdentifiers[i], err = quoteIdentifier(ctx, db, identifier)
		if err != nil {
			return quotedIdentifiers, err
		}
	}
	return quotedIdentifiers, nil
}
