package provider

import (
	"context"
	"database/sql"
	"fmt"
)

func getDatabase(ctx context.Context, mysqlConf *MySQLConfiguration) (*sql.DB, error) {
	oneConnection, err := connectToMySQLInternal(ctx, mysqlConf)

	if err != nil {
		return nil, fmt.Errorf("failed to connect to MySQL: %v", err)
	}

	return oneConnection.Db, nil
}

/*
	func getDatabaseVersion(ctx context.Context, mysqlConf *MySQLConfiguration) *version.Version {
		oneConnection, err := connectToMySQLInternal(ctx, mysqlConf)

		if err != nil {
			tflog.Info(ctx, fmt.Sprintf("getting DB got us error: %v", err))
		}

		return oneConnection.Version
	}
*/

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
