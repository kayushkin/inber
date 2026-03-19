package session

import (
	"database/sql"
	"time"
)

// InsertTurn creates a new turn record (called when an API request starts).
func (d *SQLiteStore) InsertTurn(t *TurnRow) error {
	_, err := d.db.Exec(`
		INSERT INTO turns (session_id, turn, started_at)
		VALUES (?, ?, ?)`,
		t.SessionID, t.Turn, t.StartedAt,
	)
	return err
}

// EndTurn updates a turn with response data.
func (d *SQLiteStore) EndTurn(sessionID string, turn int, inTokens, outTokens, toolCalls int, cost float64, stopReason, errMsg string) error {
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
func (d *SQLiteStore) IncrementToolCalls(sessionID string, turn int) error {
	_, err := d.db.Exec(`
		UPDATE turns SET tool_calls = tool_calls + 1
		WHERE session_id = ? AND turn = ?`,
		sessionID, turn,
	)
	return err
}

// GetTurns returns all turns for a session.
func (d *SQLiteStore) GetTurns(sessionID string) ([]TurnRow, error) {
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