package session

import (
	"database/sql"
	"time"

	"github.com/kayushkin/inber/internal/textutil"
)

// InsertSession creates a new session record.
func (d *SQLiteStore) InsertSession(s *SessionRow) error {
	_, err := d.db.Exec(`
		INSERT INTO sessions (id, agent, model, command, parent_id, pid, started_at, status, log_file)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'running', ?)`,
		s.ID, s.Agent, s.Model, s.Command, nullStr(s.ParentID), s.PID, s.StartedAt, s.LogFile,
	)
	return err
}

// EndSession marks a session as completed or errored.
func (d *SQLiteStore) EndSession(id, status, errMsg string) error {
	_, err := d.db.Exec(`
		UPDATE sessions SET ended_at = ?, status = ?, error = ?
		WHERE id = ?`,
		time.Now(), status, nullStr(errMsg), id,
	)
	return err
}

// SetTask sets the task description for a session (first user message, truncated).
func (d *SQLiteStore) SetTask(sessionID, task string) error {
	task = textutil.TruncateWith(task, 200, "…")
	_, err := d.db.Exec(`UPDATE sessions SET task = ? WHERE id = ?`, task, sessionID)
	return err
}

// ListActiveStatus returns running agents with their latest turn time and task.
// Cleans up stale sessions (dead PIDs) as a side effect.
//
// Turn count and latest-turn time are aggregated in Go rather than by SQL. The
// timestamp columns hold a Go time.Time as the driver encodes it, which SQLite's
// own date functions cannot parse (strftime returns NULL on it). The driver does
// decode the column back into a time.Time — but only for a *bare column*, whose
// declared type it can see. An expression such as COALESCE(MAX(t.started_at), ...)
// has no declared type, so the driver hands back a string and the scan fails. So
// every timestamp here is selected as a bare column, and the aggregation that
// would otherwise wrap one happens below.
func (d *SQLiteStore) ListActiveStatus() ([]ActiveAgentStatus, error) {
	rows, err := d.db.Query(`
		SELECT s.id, s.agent, s.model, COALESCE(s.task,''), s.pid, s.started_at, t.started_at
		FROM sessions s
		LEFT JOIN turns t ON s.id = t.session_id
		WHERE s.status = 'running'
		ORDER BY s.started_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// One row per (session, turn); a session with no turns yields a single row with
	// a NULL turn timestamp. Preserve the query's session ordering.
	var order []string
	byID := make(map[string]*ActiveAgentStatus)
	pids := make(map[string]int)
	for rows.Next() {
		var (
			sessionID, agent, model, task string
			pid                           int
			startedAt                     time.Time
			turnStartedAt                 sql.NullTime
		)
		if err := rows.Scan(&sessionID, &agent, &model, &task, &pid, &startedAt, &turnStartedAt); err != nil {
			return nil, err
		}

		a := byID[sessionID]
		if a == nil {
			a = &ActiveAgentStatus{
				SessionID: sessionID,
				Agent:     agent,
				Model:     model,
				Task:      task,
				StartedAt: startedAt,
				LastTurn:  startedAt, // no turns yet: the session start is the latest activity
			}
			byID[sessionID] = a
			pids[sessionID] = pid
			order = append(order, sessionID)
		}

		if turnStartedAt.Valid {
			a.Turns++
			if turnStartedAt.Time.After(a.LastTurn) {
				a.LastTurn = turnStartedAt.Time
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var result []ActiveAgentStatus
	var stale []string
	for _, id := range order {
		a := byID[id]
		if !isProcessAlive(pids[id]) {
			stale = append(stale, id)
			continue
		}
		a.Duration = time.Since(a.StartedAt)
		result = append(result, *a)
	}

	// Clean up stale sessions
	for _, id := range stale {
		d.EndSession(id, "interrupted", "process exited unexpectedly")
	}

	return result, nil
}

// ListSessions returns sessions with aggregated turn data, newest first.
func (d *SQLiteStore) ListSessions(limit int) ([]SessionSummary, error) {
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

// ListActive returns sessions with status 'running' whose PID is still alive.
// Stale sessions are marked as 'interrupted'.
func (d *SQLiteStore) ListActive() ([]SessionSummary, error) {
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
func (d *SQLiteStore) DetectInterrupted() (int, error) {
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

func (d *SQLiteStore) listByStatus(status string) ([]SessionSummary, error) {
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