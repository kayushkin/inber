package session

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// DB wraps a SQLite database for session and turn tracking.
type DB struct {
	db *sql.DB
}

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

// OpenDB opens or creates the sessions database at .inber/sessions.db.
func OpenDB(repoRoot string) (*DB, error) {
	dir := filepath.Join(repoRoot, ".inber")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create .inber dir: %w", err)
	}

	dbPath := filepath.Join(dir, "sessions.db")
	db, err := sql.Open("sqlite3", dbPath+"?_journal=wal&_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sessions db: %w", err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate sessions db: %w", err)
	}

	return &DB{db: db}, nil
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

// InsertSession creates a new session record.
func (d *DB) InsertSession(s *SessionRow) error {
	_, err := d.db.Exec(`
		INSERT INTO sessions (id, agent, model, command, parent_id, pid, started_at, status, log_file)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'running', ?)`,
		s.ID, s.Agent, s.Model, s.Command, nullStr(s.ParentID), s.PID, s.StartedAt, s.LogFile,
	)
	return err
}

// EndSession marks a session as completed or errored.
func (d *DB) EndSession(id, status, errMsg string) error {
	_, err := d.db.Exec(`
		UPDATE sessions SET ended_at = ?, status = ?, error = ?
		WHERE id = ?`,
		time.Now(), status, nullStr(errMsg), id,
	)
	return err
}

// SetTask sets the task description for a session (first user message, truncated).
func (d *DB) SetTask(sessionID, task string) {
	if len(task) > 200 {
		task = task[:200] + "…"
	}
	d.db.Exec(`UPDATE sessions SET task = ? WHERE id = ?`, task, sessionID)
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

// ListActiveStatus returns running agents with their latest turn time and task.
// Cleans up stale sessions (dead PIDs) as a side effect.
func (d *DB) ListActiveStatus() ([]ActiveAgentStatus, error) {
	rows, err := d.db.Query(`
		SELECT s.id, s.agent, s.model, COALESCE(s.task,''), s.pid, s.started_at,
			COUNT(t.turn),
			COALESCE(MAX(t.started_at), s.started_at) as last_turn
		FROM sessions s
		LEFT JOIN turns t ON s.id = t.session_id
		WHERE s.status = 'running'
		GROUP BY s.id
		ORDER BY s.started_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ActiveAgentStatus
	var stale []string
	for rows.Next() {
		var a ActiveAgentStatus
		var pid int
		err := rows.Scan(&a.SessionID, &a.Agent, &a.Model, &a.Task, &pid, &a.StartedAt, &a.Turns, &a.LastTurn)
		if err != nil {
			return nil, err
		}
		if !isProcessAlive(pid) {
			stale = append(stale, a.SessionID)
			continue
		}
		a.Duration = time.Since(a.StartedAt)
		result = append(result, a)
	}

	// Clean up stale sessions
	for _, id := range stale {
		d.EndSession(id, "interrupted", "process exited unexpectedly")
	}

	return result, nil
}

// InsertTurn creates a new turn record (called when an API request starts).
func (d *DB) InsertTurn(t *TurnRow) error {
	_, err := d.db.Exec(`
		INSERT INTO turns (session_id, turn, started_at)
		VALUES (?, ?, ?)`,
		t.SessionID, t.Turn, t.StartedAt,
	)
	return err
}

// EndTurn updates a turn with response data.
func (d *DB) EndTurn(sessionID string, turn int, inTokens, outTokens, toolCalls int, cost float64, stopReason, errMsg string) error {
	_, err := d.db.Exec(`
		UPDATE turns SET ended_at = ?, in_tokens = ?, out_tokens = ?, cost = ?,
			tool_calls = ?, stop_reason = ?, error = ?
		WHERE session_id = ? AND turn = ?`,
		time.Now(), inTokens, outTokens, cost, toolCalls, nullStr(stopReason), nullStr(errMsg),
		sessionID, turn,
	)
	return err
}

// IncrementToolCalls bumps the tool_calls counter for the current turn.
func (d *DB) IncrementToolCalls(sessionID string, turn int) error {
	_, err := d.db.Exec(`
		UPDATE turns SET tool_calls = tool_calls + 1
		WHERE session_id = ? AND turn = ?`,
		sessionID, turn,
	)
	return err
}

// ListSessions returns sessions with aggregated turn data, newest first.
func (d *DB) ListSessions(limit int) ([]SessionSummary, error) {
	rows, err := d.db.Query(`
		SELECT s.id, s.agent, s.model, s.command, COALESCE(s.parent_id,''), s.pid,
			s.started_at, s.ended_at, s.status, COALESCE(s.error,''), COALESCE(s.log_file,''),
			COUNT(t.turn), COALESCE(SUM(t.in_tokens),0), COALESCE(SUM(t.out_tokens),0),
			COALESCE(SUM(t.cost),0)
		FROM sessions s
		LEFT JOIN turns t ON s.id = t.session_id
		GROUP BY s.id
		ORDER BY s.started_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []SessionSummary
	for rows.Next() {
		var ss SessionSummary
		var endedAt sql.NullTime
		err := rows.Scan(
			&ss.ID, &ss.Agent, &ss.Model, &ss.Command, &ss.ParentID, &ss.PID,
			&ss.StartedAt, &endedAt, &ss.Status, &ss.Error, &ss.LogFile,
			&ss.Turns, &ss.TotalIn, &ss.TotalOut, &ss.TotalCost,
		)
		if err != nil {
			return nil, err
		}
		if endedAt.Valid {
			ss.EndedAt = &endedAt.Time
			ss.Duration = endedAt.Time.Sub(ss.StartedAt)
		} else {
			ss.Duration = time.Since(ss.StartedAt)
		}
		result = append(result, ss)
	}
	return result, nil
}

// GetTurns returns all turns for a session.
func (d *DB) GetTurns(sessionID string) ([]TurnRow, error) {
	rows, err := d.db.Query(`
		SELECT session_id, turn, started_at, ended_at, in_tokens, out_tokens,
			cost, tool_calls, COALESCE(stop_reason,''), COALESCE(error,'')
		FROM turns WHERE session_id = ?
		ORDER BY turn`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []TurnRow
	for rows.Next() {
		var t TurnRow
		var endedAt sql.NullTime
		err := rows.Scan(&t.SessionID, &t.Turn, &t.StartedAt, &endedAt,
			&t.InTokens, &t.OutTokens, &t.Cost, &t.ToolCalls,
			&t.StopReason, &t.Error)
		if err != nil {
			return nil, err
		}
		if endedAt.Valid {
			t.EndedAt = &endedAt.Time
		}
		result = append(result, t)
	}
	return result, nil
}

// ListActive returns sessions with status 'running' whose PID is still alive.
// Stale sessions are marked as 'interrupted'.
func (d *DB) ListActive() ([]SessionSummary, error) {
	all, err := d.listByStatus("running")
	if err != nil {
		return nil, err
	}

	var alive []SessionSummary
	for _, s := range all {
		if isProcessAlive(s.PID) {
			alive = append(alive, s)
		} else {
			d.EndSession(s.ID, "interrupted", "process exited unexpectedly")
		}
	}
	return alive, nil
}

// DetectInterrupted marks any 'running' sessions with dead PIDs as 'interrupted'.
func (d *DB) DetectInterrupted() (int, error) {
	all, err := d.listByStatus("running")
	if err != nil {
		return 0, err
	}

	count := 0
	for _, s := range all {
		if !isProcessAlive(s.PID) {
			d.EndSession(s.ID, "interrupted", "process exited unexpectedly")
			count++
		}
	}
	return count, nil
}

func (d *DB) listByStatus(status string) ([]SessionSummary, error) {
	rows, err := d.db.Query(`
		SELECT s.id, s.agent, s.model, s.command, COALESCE(s.parent_id,''), s.pid,
			s.started_at, s.ended_at, s.status, COALESCE(s.error,''), COALESCE(s.log_file,''),
			COUNT(t.turn), COALESCE(SUM(t.in_tokens),0), COALESCE(SUM(t.out_tokens),0),
			COALESCE(SUM(t.cost),0)
		FROM sessions s
		LEFT JOIN turns t ON s.id = t.session_id
		WHERE s.status = ?
		GROUP BY s.id
		ORDER BY s.started_at DESC`, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []SessionSummary
	for rows.Next() {
		var ss SessionSummary
		var endedAt sql.NullTime
		err := rows.Scan(
			&ss.ID, &ss.Agent, &ss.Model, &ss.Command, &ss.ParentID, &ss.PID,
			&ss.StartedAt, &endedAt, &ss.Status, &ss.Error, &ss.LogFile,
			&ss.Turns, &ss.TotalIn, &ss.TotalOut, &ss.TotalCost,
		)
		if err != nil {
			return nil, err
		}
		if endedAt.Valid {
			ss.EndedAt = &endedAt.Time
			ss.Duration = endedAt.Time.Sub(ss.StartedAt)
		} else {
			ss.Duration = time.Since(ss.StartedAt)
		}
		result = append(result, ss)
	}
	return result, nil
}

// Close closes the database.
func (d *DB) Close() error {
	return d.db.Close()
}

func nullStr(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// isProcessAlive is defined in active.go
