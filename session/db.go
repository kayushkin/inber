package session

import (
	"database/sql"
)

// Close closes the database.
func (d *SQLiteStore) Close() error {
	return d.db.Close()
}

func nullStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// isProcessAlive is defined in active.go