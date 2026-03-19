package session

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// createJSONLFile creates a new JSONL log file for the session.
func createJSONLFile(logsDir, agentName, sessionID string) (*os.File, error) {
	// Create logs directory if it doesn't exist
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create logs directory: %w", err)
	}

	// Generate filename with timestamp and agent name
	now := time.Now()
	filename := fmt.Sprintf("%s_%s_%s.jsonl", 
		now.Format("20060102_150405"), 
		agentName, 
		sessionID)
	
	path := filepath.Join(logsDir, filename)
	
	file, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("failed to create log file %s: %w", path, err)
	}
	
	return file, nil
}

// writeEntry writes a single log entry to the JSONL file.
func (s *Session) writeEntry(e Entry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enc.Encode(e) // best-effort; don't crash on log failure

	// Send copy to logstack if available (async, best-effort)
	s.logstack.Log(e)
}

// flushJSONL ensures all pending writes are flushed to disk.
func (s *Session) flushJSONL() {
	if s.file != nil {
		s.file.Sync()
	}
}

// closeJSONLWithCompletion closes the JSONL file for successful completion.
func (s *Session) closeJSONLWithCompletion() {
	s.mu.Lock()
	cost := s.cost()
	s.mu.Unlock()

	s.writeEntry(Entry{
		Timestamp:    time.Now(),
		Role:         "system",
		Content:      fmt.Sprintf("session complete — %s — total tokens: in=%d out=%d — cost: $%.4f", s.sessionID, s.totalIn, s.totalOut, cost),
		InputTokens:  s.totalIn,
		OutputTokens: s.totalOut,
		TotalCost:    cost,
	})
	s.file.Close()
}

// closeJSONLWithError closes the JSONL file for error termination.
func (s *Session) closeJSONLWithError(err error) {
	s.mu.Lock()
	cost := s.cost()
	s.mu.Unlock()

	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}

	s.writeEntry(Entry{
		Timestamp:    time.Now(),
		Role:         "system",
		Content:      fmt.Sprintf("session error — %s — total tokens: in=%d out=%d — cost: $%.4f", errMsg, s.totalIn, s.totalOut, cost),
		InputTokens:  s.totalIn,
		OutputTokens: s.totalOut,
		TotalCost:    cost,
	})
	s.file.Close()
}