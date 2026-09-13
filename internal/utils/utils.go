package utils

import (
	"context"
	"database/sql"
	"os"
)

func GetenvWithDefault(key, defaultValue string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	} else {
		return defaultValue
	}
}

// UserExists reports whether the user exists on the server. The error is returned as is,
// so that the caller can distinguish a missing user from a failed query.
func UserExists(ctx context.Context, db *sql.DB, user, host string) (bool, error) {
	var count int64
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM mysql.user WHERE User = ? AND Host = ?", user, host).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}
