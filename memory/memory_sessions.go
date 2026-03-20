package memory

import (
	"fmt"
	"time"
)

// SaveSession stores a session record.
func (s *Store) SaveSession(sess Session) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	query := `
	INSERT INTO sessions (id, agent_name, model, started_at, ended_at, input_tokens, output_tokens, cost, summary)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err = tx.Exec(query,
		sess.ID, sess.AgentName, sess.Model,
		sess.StartedAt.Unix(), nullInt64(sess.EndedAt),
		sess.InputTokens, sess.OutputTokens, sess.Cost, nullString(sess.Summary),
	)
	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}

	// Insert tags
	if len(sess.Tags) > 0 {
		tagStmt, err := tx.Prepare("INSERT INTO session_tags (session_id, tag) VALUES (?, ?)")
		if err != nil {
			return fmt.Errorf("prepare tag insert: %w", err)
		}
		defer tagStmt.Close()

		for _, tag := range sess.Tags {
			if _, err := tagStmt.Exec(sess.ID, tag); err != nil {
				return fmt.Errorf("insert tag: %w", err)
			}
		}
	}

	return tx.Commit()
}

// TrackMemoryUsage records when a memory was used in a session.
func (s *Store) TrackMemoryUsage(memoryID, sessionID string, turnNumber int, usageType string) error {
	query := `
	INSERT INTO memory_usage (memory_id, session_id, turn_number, usage_type, accessed_at)
	VALUES (?, ?, ?, ?, ?)
	`
	_, err := s.db.Exec(query, memoryID, sessionID, turnNumber, usageType, time.Now().Unix())
	return err
}