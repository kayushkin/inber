package session

import (
	"database/sql"
	"time"
)

// SQLiteStore wraps a SQLite database for session and turn tracking.
// Implements the Store interface.
type SQLiteStore struct {
	db *sql.DB
}

// DB is an alias for SQLiteStore to maintain backwards compatibility.
// Deprecated: Use SQLiteStore directly.
type DB = SQLiteStore

// SessionRow represents a session record.
type SessionRow struct {
	ID        string
	Agent     string
	Model     string
	Command   string
	ParentID  string
	PID       int
	StartedAt time.Time
	EndedAt   *time.Time
	Status    string // "running", "completed", "error", "interrupted"
	Error     string
	LogFile   string
	Task      string // first user message, truncated
}

// TurnRow represents a single API round-trip.
type TurnRow struct {
	SessionID  string
	Turn       int
	StartedAt  time.Time
	EndedAt    *time.Time
	InTokens   int
	OutTokens  int
	Cost       float64
	ToolCalls  int
	StopReason string
	Error      string
}

// SessionSummary is a session with aggregated turn data.
type SessionSummary struct {
	SessionRow
	Turns    int
	TotalIn  int
	TotalOut int
	TotalCost float64
	Duration  time.Duration
}

// ActiveAgentStatus represents a running agent for status display.
type ActiveAgentStatus struct {
	Agent     string
	Model     string
	Task      string
	Turns     int
	StartedAt time.Time
	LastTurn  time.Time
	Duration  time.Duration
	SessionID string
}

// Close closes the database.
func (d *SQLiteStore) Close() error {
	return d.db.Close()
}

// nullStr converts a string to sql.NullString for database operations.
func nullStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}