package session

import (
	"database/sql"
	"time"
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
	if len(task) > 200 {
		task = task[:200] + "…"
	}
	_, err := d.db.Exec(`UPDATE sessions SET task = ? WHERE id = ?`, task, sessionID)
	return err
}

// ListActiveStatus returns running agents with their latest turn time and task.
// Cleans up stale sessions (dead PIDs) as a side effect.
func (d *SQLiteStore) ListActiveStatus() ([]ActiveAgentStatus, error) {
	rows, err := d.db.Query(`
		SELECT s.id, s.agent, s.model, COALESCE(s.task,''), s.pid,
			COALESCE(strftime('%s', s.started_at), 0),
			COUNT(t.turn),
			COALESCE(strftime('%s', COALESCE(MAX(t.started_at), s.started_at)), 0)
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
		var startedAtUnix, lastTurnUnix int64
		err := rows.Scan(&a.SessionID, &a.Agent, &a.Model, &a.Task, &pid, &startedAtUnix, &a.Turns, &lastTurnUnix)
		if err != nil {
			return nil, err
		}
		a.StartedAt = time.Unix(startedAtUnix, 0)
		a.LastTurn = time.Unix(lastTurnUnix, 0)
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