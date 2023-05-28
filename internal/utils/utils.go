package utils

import (
	"context"
	"database/sql"
	"os"
	"strconv"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

func GetenvWithDefault(key, defaultValue string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	} else {
		return defaultValue
	}
}

func UserExists(ctx context.Context, db *sql.DB, user, host string) bool {
	var count string
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM mysql.user WHERE User = ? AND Host = ?", user, host).Scan(&count); err != nil {
		tflog.Error(ctx, err.Error())
		return false
	}
	if c, err := strconv.ParseInt(count, 10, 64); err != nil {
		tflog.Error(ctx, err.Error())
		return false
	} else {
		return c > 0
	}
}
