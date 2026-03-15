package server

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Store persists session and request metadata in SQLite.
type Store struct {
	db *sql.DB
}

// NewStore opens or creates the server database.
func NewStore(dbPath string) (*Store, error) {
	os.MkdirAll(filepath.Dir(dbPath), 0755)
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
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
	return err
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
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

// UpsertSession creates or updates a session.
func (s *Store) UpsertSession(key, agent, kind string) error {
	_, err := s.db.Exec(`
		INSERT INTO sessions (key, agent, kind) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET last_active = CURRENT_TIMESTAMP
	`, key, agent, kind)
	return err
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
