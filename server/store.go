package server

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kayushkin/inber/internal/sqlitewal"
	_ "modernc.org/sqlite"
)

// Store persists session and request metadata in SQLite.
type Store struct {
	db *sql.DB
}

// NewStore opens or creates the server database.
func NewStore(dbPath string) (*Store, error) {
	os.MkdirAll(filepath.Dir(dbPath), 0755)
	db, err := sql.Open("sqlite", sqlitewal.ConnectionDSN(dbPath))
	if err != nil {
		return nil, fmt.Errorf("open server db: %w", err)
	}
	// busy_timeout alone does not stop modernc surfacing SQLITE_BUSY to concurrent
	// writers, and this DB is written from concurrent HTTP handlers. A single
	// connection serialises them. Safe here: this package uses no transactions, so
	// nothing can hold the connection while waiting for another.
	db.SetMaxOpenConns(1)

	// See sqlitewal for why this is a statement with its own retry rather than a
	// DSN pragma: the conversion is the one lock busy_timeout will not wait for.
	if err := sqlitewal.SwitchToWAL(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("open server db: %w", err)
	}

	if err := migrateGatewayDB(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate server db: %w", err)
	}
	return &Store{db: db}, nil
}

func migrateGatewayDB(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			key TEXT PRIMARY KEY,
			agent TEXT NOT NULL,
			kind TEXT NOT NULL DEFAULT 'main',  -- 'main' | 'spawn'
			parent_key TEXT NOT NULL DEFAULT '',
			spawn_depth INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			last_active TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			message_count INTEGER DEFAULT 0
		);

		CREATE TABLE IF NOT EXISTS requests (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_key TEXT NOT NULL REFERENCES sessions(key),
			status TEXT NOT NULL DEFAULT 'pending',  -- pending, running, completed, error, timeout, interrupted
			input_text TEXT,
			output_text TEXT,
			turns INTEGER DEFAULT 0,
			input_tokens INTEGER DEFAULT 0,
			output_tokens INTEGER DEFAULT 0,
			cache_read_tokens INTEGER DEFAULT 0,
			cache_write_tokens INTEGER DEFAULT 0,
			cost REAL DEFAULT 0,
			started_at TIMESTAMP,
			completed_at TIMESTAMP,
			error_text TEXT,
			parent_request_id INTEGER REFERENCES requests(id)
		);

		CREATE INDEX IF NOT EXISTS idx_requests_session ON requests(session_key);
		CREATE INDEX IF NOT EXISTS idx_requests_status ON requests(status);
		CREATE INDEX IF NOT EXISTS idx_requests_parent ON requests(parent_request_id);
	`)
	if err != nil {
		return err
	}
	if err := addSessionLineageColumns(db); err != nil {
		return err
	}
	return backfillSessionLineageFromChildKeys(db)
}

// addSessionLineageColumns brings a sessions table created before lineage was
// recorded up to the schema above. CREATE TABLE IF NOT EXISTS leaves an
// existing table exactly as it found it, so on every host that ran an earlier
// build the two columns are missing and every read of them is an error.
func addSessionLineageColumns(db *sql.DB) error {
	existing, err := columnNames(db, "sessions")
	if err != nil {
		return err
	}
	additions := []struct {
		name       string
		definition string
	}{
		{"parent_key", `ALTER TABLE sessions ADD COLUMN parent_key TEXT NOT NULL DEFAULT ''`},
		{"spawn_depth", `ALTER TABLE sessions ADD COLUMN spawn_depth INTEGER NOT NULL DEFAULT 0`},
	}
	for _, addition := range additions {
		if existing[addition.name] {
			continue
		}
		if _, err := db.Exec(addition.definition); err != nil {
			return fmt.Errorf("add sessions.%s: %w", addition.name, err)
		}
	}
	return nil
}

func columnNames(db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return nil, fmt.Errorf("read columns of %s: %w", table, err)
	}
	defer rows.Close()

	names := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("read columns of %s: %w", table, err)
		}
		names[name] = true
	}
	return names, rows.Err()
}

// backfillSessionLineageFromChildKeys fills in the lineage of children recorded
// before there were columns to record it in.
//
// sessionKeyForChild builds a child's key as its parent's key with
// ":sub:<suffix>" appended, so an existing child's key still carries its
// parent's *key* — an id this table assigned, not a name — and one ":sub:" per
// level of the tree. The repair is therefore exact. Without it every child
// already on disk stays at depth zero forever, and the depth cap goes on lying
// about exactly the sessions it was added for.
//
// ⛔ This is a migration and has to stay one. Reading lineage out of a key at
// run time would be the same defect as reading the agent out of it: a key is a
// string a caller chooses, and POST /api/run already takes session_key and
// agent as independent fields.
func backfillSessionLineageFromChildKeys(db *sql.DB) error {
	rows, err := db.Query(
		`SELECT key FROM sessions WHERE parent_key = '' AND spawn_depth = 0 AND key LIKE '%' || ? || '%'`,
		childKeySeparator)
	if err != nil {
		return fmt.Errorf("find children with no recorded lineage: %w", err)
	}
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			rows.Close()
			return fmt.Errorf("find children with no recorded lineage: %w", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("find children with no recorded lineage: %w", err)
	}
	rows.Close()

	for _, key := range keys {
		parentKey := key[:strings.LastIndex(key, childKeySeparator)]
		depth := strings.Count(key, childKeySeparator)
		if _, err := db.Exec(
			`UPDATE sessions SET parent_key = ?, spawn_depth = ? WHERE key = ?`,
			parentKey, depth, key); err != nil {
			return fmt.Errorf("backfill lineage for session %s: %w", key, err)
		}
	}
	return nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// DeleteSession removes a session and all its requests from the database.
func (s *Store) DeleteSession(key string) error {
	_, err := s.db.Exec(`DELETE FROM requests WHERE session_key = ?`, key)
	if err != nil {
		return fmt.Errorf("delete requests for session %s: %w", key, err)
	}
	_, err = s.db.Exec(`DELETE FROM sessions WHERE key = ?`, key)
	if err != nil {
		return fmt.Errorf("delete session %s: %w", key, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Sessions
// ---------------------------------------------------------------------------

// SessionRow is a session record.
type SessionRow struct {
	Key          string
	Agent        string
	Kind         string // "main" | "spawn"
	CreatedAt    time.Time
	LastActive   time.Time
	MessageCount int
}

// SessionLineage is where a session came from: the key of the session that
// asked for it, and how many spawns deep in that tree it sits. A top-level
// session has neither, which is the zero value.
//
// The two travel together because either one alone describes a session that
// cannot exist — a parent with no depth, or a depth with no parent.
type SessionLineage struct {
	ParentKey  string
	SpawnDepth int
}

// UpsertSession creates or updates a session.
//
// Lineage is written when the row is created and, like the agent name, left
// alone on conflict: where a session came from is settled at the moment it is
// spawned and never changes afterwards.
func (s *Store) UpsertSession(key, agent, kind string, lineage SessionLineage) error {
	_, err := s.db.Exec(`
		INSERT INTO sessions (key, agent, kind, parent_key, spawn_depth) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET last_active = CURRENT_TIMESTAMP
	`, key, agent, kind, lineage.ParentKey, lineage.SpawnDepth)
	return err
}

// SessionLineage returns the lineage recorded against a session key, and the
// zero lineage — a root — when the store has never seen that key.
//
// This table is the only durable record of it. Session.SpawnDepth and
// Session.ParentKey are set by Spawn and forkSession and live in memory, so
// before this column pair existed a restart rebuilt every child as a root: the
// cap in Spawn is checked against the parent's depth, and a revived depth-2
// child reading zero could spawn MaxSpawnDepth more levels, each of which
// could do the same after the next restart.
func (s *Store) SessionLineage(key string) (SessionLineage, error) {
	var lineage SessionLineage
	err := s.db.QueryRow(`SELECT parent_key, spawn_depth FROM sessions WHERE key = ?`, key).
		Scan(&lineage.ParentKey, &lineage.SpawnDepth)
	if errors.Is(err, sql.ErrNoRows) {
		return SessionLineage{}, nil
	}
	if err != nil {
		return SessionLineage{}, fmt.Errorf("read lineage for session %s: %w", key, err)
	}
	return lineage, nil
}

// SessionAgent returns the agent name recorded against a session key, and an
// empty name when the store has never seen that key.
//
// This table is where a session's agent is written when the session is created,
// and for a spawned child it is the only place that records the child's own
// agent: the child's key is its parent's key with a suffix, so the key names the
// parent. UpsertSession deliberately leaves agent alone on conflict, so the name
// written at creation is the name this returns for the life of the session.
func (s *Store) SessionAgent(key string) (string, error) {
	var agent string
	err := s.db.QueryRow(`SELECT agent FROM sessions WHERE key = ?`, key).Scan(&agent)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read agent for session %s: %w", key, err)
	}
	return agent, nil
}

// TouchSession updates last_active and message count.
func (s *Store) TouchSession(key string, messageCount int) error {
	_, err := s.db.Exec(`
		UPDATE sessions SET last_active = CURRENT_TIMESTAMP, message_count = ? WHERE key = ?
	`, messageCount, key)
	return err
}

// ListSessions returns all sessions, optionally filtered by kind.
func (s *Store) ListSessions(kind string) ([]SessionRow, error) {
	query := `SELECT key, agent, kind, created_at, last_active, message_count FROM sessions`
	args := []any{}
	if kind != "" {
		query += ` WHERE kind = ?`
		args = append(args, kind)
	}
	query += ` ORDER BY last_active DESC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []SessionRow
	for rows.Next() {
		var r SessionRow
		if err := rows.Scan(&r.Key, &r.Agent, &r.Kind, &r.CreatedAt, &r.LastActive, &r.MessageCount); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// ---------------------------------------------------------------------------
// Requests
// ---------------------------------------------------------------------------

// RequestRow is a request record.
type RequestRow struct {
	ID              int        `json:"id"`
	SessionKey      string     `json:"session_key"`
	Status          string     `json:"status"`
	InputText       *string    `json:"input_text"`
	OutputText      *string    `json:"output_text"`
	Turns           int        `json:"turns"`
	InputTokens     int        `json:"input_tokens"`
	OutputTokens    int        `json:"output_tokens"`
	CacheReadTokens int        `json:"cache_read_tokens"`
	CacheWriteTokens int       `json:"cache_write_tokens"`
	Cost            float64    `json:"cost"`
	StartedAt       *time.Time `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at"`
	ErrorText       *string    `json:"error_text"`
	ParentRequestID *int       `json:"parent_request_id"`
}

// CreateRequest inserts a new request and returns its ID.
func (s *Store) CreateRequest(sessionKey, inputText string, parentRequestID *int) (int, error) {
	now := time.Now()
	res, err := s.db.Exec(`
		INSERT INTO requests (session_key, status, input_text, started_at, parent_request_id)
		VALUES (?, 'running', ?, ?, ?)
	`, sessionKey, inputText, now, parentRequestID)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	return int(id), err
}

// CompleteRequest marks a request as completed with results.
func (s *Store) CompleteRequest(id int, status, outputText, errorText string, turns, inputTokens, outputTokens, cacheRead, cacheWrite int, cost float64) error {
	_, err := s.db.Exec(`
		UPDATE requests SET
			status = ?, output_text = ?, error_text = ?,
			turns = ?, input_tokens = ?, output_tokens = ?,
			cache_read_tokens = ?, cache_write_tokens = ?,
			cost = ?, completed_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, status, outputText, errorText, turns, inputTokens, outputTokens, cacheRead, cacheWrite, cost, id)
	return err
}

// InterruptRunning marks all 'running' requests as 'interrupted'. Call on startup.
func (s *Store) InterruptRunning() (int, error) {
	res, err := s.db.Exec(`
		UPDATE requests SET status = 'interrupted', completed_at = CURRENT_TIMESTAMP
		WHERE status = 'running'
	`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ActiveRequest returns the currently running request for a session, if any.
func (s *Store) ActiveRequest(sessionKey string) (*RequestRow, error) {
	row := s.db.QueryRow(`
		SELECT id, session_key, status, input_text, output_text, turns,
			input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
			cost, started_at, completed_at, error_text, parent_request_id
		FROM requests WHERE session_key = ? AND status = 'running'
		ORDER BY id DESC LIMIT 1
	`, sessionKey)

	var r RequestRow
	err := row.Scan(&r.ID, &r.SessionKey, &r.Status, &r.InputText, &r.OutputText,
		&r.Turns, &r.InputTokens, &r.OutputTokens, &r.CacheReadTokens, &r.CacheWriteTokens,
		&r.Cost, &r.StartedAt, &r.CompletedAt, &r.ErrorText, &r.ParentRequestID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// RecentRequests returns the N most recent requests for a session.
func (s *Store) RecentRequests(sessionKey string, limit int) ([]RequestRow, error) {
	rows, err := s.db.Query(`
		SELECT id, session_key, status, input_text, output_text, turns,
			input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
			cost, started_at, completed_at, error_text, parent_request_id
		FROM requests WHERE session_key = ?
		ORDER BY id DESC LIMIT ?
	`, sessionKey, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []RequestRow
	for rows.Next() {
		var r RequestRow
		if err := rows.Scan(&r.ID, &r.SessionKey, &r.Status, &r.InputText, &r.OutputText,
			&r.Turns, &r.InputTokens, &r.OutputTokens, &r.CacheReadTokens, &r.CacheWriteTokens,
			&r.Cost, &r.StartedAt, &r.CompletedAt, &r.ErrorText, &r.ParentRequestID); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// SpawnChildren returns all requests spawned from a parent request.
func (s *Store) SpawnChildren(parentRequestID int) ([]RequestRow, error) {
	rows, err := s.db.Query(`
		SELECT id, session_key, status, input_text, output_text, turns,
			input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
			cost, started_at, completed_at, error_text, parent_request_id
		FROM requests WHERE parent_request_id = ?
		ORDER BY id
	`, parentRequestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []RequestRow
	for rows.Next() {
		var r RequestRow
		if err := rows.Scan(&r.ID, &r.SessionKey, &r.Status, &r.InputText, &r.OutputText,
			&r.Turns, &r.InputTokens, &r.OutputTokens, &r.CacheReadTokens, &r.CacheWriteTokens,
			&r.Cost, &r.StartedAt, &r.CompletedAt, &r.ErrorText, &r.ParentRequestID); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}
