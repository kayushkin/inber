package session

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kayushkin/inber/internal/sqlitewal"
	_ "modernc.org/sqlite"
)

// OpenDB opens or creates the sessions database at .inber/sessions.db.
func OpenDB(repoRoot string) (*SQLiteStore, error) {
	dir := filepath.Join(repoRoot, ".inber")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create .inber dir: %w", err)
	}

	dbPath := filepath.Join(dir, "sessions.db")
	db, err := sql.Open("sqlite", sqlitewal.ConnectionDSN(dbPath))
	if err != nil {
		return nil, fmt.Errorf("open sessions db: %w", err)
	}
	// busy_timeout alone does not stop modernc surfacing SQLITE_BUSY to concurrent
	// writers; a single connection serialises them. Safe here: this package uses no
	// transactions, so nothing can hold the connection while waiting for another.
	db.SetMaxOpenConns(1)

	// This database lives under the repo being worked on, so every session open
	// on that repo races to convert the same fresh file. Waiting is the point:
	// see sqlitewal for why busy_timeout cannot cover this one statement.
	if err := sqlitewal.SwitchToWAL(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("open sessions db: %w", err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate sessions db: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			id         TEXT PRIMARY KEY,
			agent      TEXT NOT NULL,
			model      TEXT NOT NULL,
			command    TEXT NOT NULL,
			parent_id  TEXT,
			pid        INTEGER NOT NULL,
			started_at DATETIME NOT NULL,
			ended_at   DATETIME,
			status     TEXT NOT NULL DEFAULT 'running',
			error      TEXT,
			log_file   TEXT
		);

		CREATE TABLE IF NOT EXISTS turns (
			session_id  TEXT NOT NULL REFERENCES sessions(id),
			turn        INTEGER NOT NULL,
			started_at  DATETIME NOT NULL,
			ended_at    DATETIME,
			in_tokens   INTEGER DEFAULT 0,
			out_tokens  INTEGER DEFAULT 0,
			cost        REAL DEFAULT 0,
			tool_calls  INTEGER DEFAULT 0,
			stop_reason TEXT,
			error       TEXT,
			PRIMARY KEY (session_id, turn)
		);

		CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status);
		CREATE INDEX IF NOT EXISTS idx_sessions_started ON sessions(started_at);
		CREATE INDEX IF NOT EXISTS idx_turns_session ON turns(session_id);
	`)
	if err != nil {
		return err
	}

	// Migration: add task column
	db.Exec(`ALTER TABLE sessions ADD COLUMN task TEXT DEFAULT ''`)
	return nil
}