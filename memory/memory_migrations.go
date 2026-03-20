package memory

import (
	"database/sql"
	"fmt"
)

// createSchema creates the initial database schema for memories, sessions, and related tables.
func createSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS memories (
		id TEXT PRIMARY KEY,
		content TEXT NOT NULL,
		summary TEXT,
		original_id TEXT,
		importance REAL NOT NULL DEFAULT 0.5,
		access_count INTEGER NOT NULL DEFAULT 0,
		last_accessed INTEGER NOT NULL,
		created_at INTEGER NOT NULL,
		source TEXT NOT NULL,
		embedding BLOB
	);
	CREATE INDEX IF NOT EXISTS idx_importance ON memories(importance);
	CREATE INDEX IF NOT EXISTS idx_last_accessed ON memories(last_accessed);
	CREATE INDEX IF NOT EXISTS idx_created_at ON memories(created_at);
	CREATE INDEX IF NOT EXISTS idx_source ON memories(source);

	CREATE TABLE IF NOT EXISTS memory_tags (
		memory_id TEXT NOT NULL,
		tag TEXT NOT NULL,
		FOREIGN KEY (memory_id) REFERENCES memories(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_memory_tags_memory_id ON memory_tags(memory_id);
	CREATE INDEX IF NOT EXISTS idx_memory_tags_tag ON memory_tags(tag);

	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		agent_name TEXT,
		model TEXT,
		started_at INTEGER NOT NULL,
		ended_at INTEGER,
		input_tokens INTEGER DEFAULT 0,
		output_tokens INTEGER DEFAULT 0,
		cost REAL DEFAULT 0,
		summary TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_sessions_started_at ON sessions(started_at);
	CREATE INDEX IF NOT EXISTS idx_sessions_agent_name ON sessions(agent_name);

	CREATE TABLE IF NOT EXISTS session_tags (
		session_id TEXT NOT NULL,
		tag TEXT NOT NULL,
		FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_session_tags_session_id ON session_tags(session_id);
	CREATE INDEX IF NOT EXISTS idx_session_tags_tag ON session_tags(tag);

	CREATE TABLE IF NOT EXISTS memory_usage (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		memory_id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		turn_number INTEGER NOT NULL,
		usage_type TEXT NOT NULL,
		accessed_at INTEGER NOT NULL,
		FOREIGN KEY (memory_id) REFERENCES memories(id) ON DELETE CASCADE,
		FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_memory_usage_memory_id ON memory_usage(memory_id);
	CREATE INDEX IF NOT EXISTS idx_memory_usage_session_id ON memory_usage(session_id);
	`
	
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	
	return nil
}

// runMigrations applies schema migrations to existing databases
func runMigrations(db *sql.DB) error {
	// Get list of existing columns
	existingCols := make(map[string]bool)
	rows, err := db.Query("PRAGMA table_info(memories)")
	if err != nil {
		return err
	}
	defer rows.Close()
	
	var cid int
	var name, typ string
	var notnull, pk int
	var dflt sql.NullString
	
	for rows.Next() {
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			continue
		}
		existingCols[name] = true
	}
	
	// List of all migrations (idempotent - run them all, sqlite will ignore errors for existing columns)
	migrations := []string{
		// Context unification fields (2026-03-01)
		"ALTER TABLE memories ADD COLUMN always_load INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE memories ADD COLUMN expires_at INTEGER",
		"ALTER TABLE memories ADD COLUMN tokens INTEGER NOT NULL DEFAULT 0",
		"CREATE INDEX IF NOT EXISTS idx_always_load ON memories(always_load)",
		"CREATE INDEX IF NOT EXISTS idx_expires_at ON memories(expires_at)",
		
		// Reference fields for lazy loading (2026-03-01)
		"ALTER TABLE memories ADD COLUMN ref_type TEXT NOT NULL DEFAULT 'memory'",
		"ALTER TABLE memories ADD COLUMN ref_target TEXT",
		"ALTER TABLE memories ADD COLUMN is_lazy INTEGER NOT NULL DEFAULT 0",
		"CREATE INDEX IF NOT EXISTS idx_ref_type ON memories(ref_type)",
	}
	
	for _, migration := range migrations {
		if _, err := db.Exec(migration); err != nil {
			// Ignore errors for already-existing columns/indexes
			continue
		}
	}
	
	return nil
}