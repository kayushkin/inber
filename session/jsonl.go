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

// closingTotals reads everything the closing entry reports in one critical
// section: the four cumulative counts and the cost worked out from them.
//
// It is one call rather than five reads at the entry because the counts are
// written by LogAssistant under the mutex and the closing entry used to read
// them outside it — the same race SaveCheckpoint already fixed for the same
// four fields. Taking them together is also what keeps the reported tokens and
// the reported cost describing the same turns; read apart, a turn logging
// concurrently lands in one and not the other.
func (s *Session) closingTotals() (TurnTokens, float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return TurnTokens{
		Input:      s.totalIn,
		Output:     s.totalOut,
		CacheRead:  s.totalCacheRead,
		CacheWrite: s.totalCacheWrite,
	}, s.cost()
}

// describeTotals renders the four counts as the closing line reports them.
//
// It names the whole prompt rather than `in=` alone. The four counts are
// disjoint and `Input` is only the part of the prompt the cache did not cover,
// so a session that spent 781k fresh input against 18.5M of cache traffic used
// to close with "in=781355 … cost: $57" — a figure priced from the whole prompt
// sitting beside a figure describing a twentieth of it. Anyone dividing the two
// to get a rate got one twenty-fifth of the real one.
func describeTotals(tokens TurnTokens, cost float64) string {
	return fmt.Sprintf("total tokens: prompt=%d (fresh=%d cache_read=%d cache_write=%d) out=%d — cost: $%.4f",
		tokens.Input+tokens.CacheRead+tokens.CacheWrite,
		tokens.Input, tokens.CacheRead, tokens.CacheWrite,
		tokens.Output, cost)
}

// closeJSONLWithCompletion closes the JSONL file for successful completion.
func (s *Session) closeJSONLWithCompletion() {
	tokens, cost := s.closingTotals()

	s.writeEntry(Entry{
		Timestamp:    time.Now(),
		Role:         "system",
		Content:      fmt.Sprintf("session complete — %s — %s", s.sessionID, describeTotals(tokens, cost)),
		InputTokens:  tokens.Input,
		OutputTokens: tokens.Output,
		CacheRead:    tokens.CacheRead,
		CacheWrite:   tokens.CacheWrite,
		TotalCost:    cost,
	})
	s.file.Close()
}

// closeJSONLWithError closes the JSONL file for error termination.
func (s *Session) closeJSONLWithError(err error) {
	tokens, cost := s.closingTotals()

	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}

	s.writeEntry(Entry{
		Timestamp:    time.Now(),
		Role:         "system",
		Content:      fmt.Sprintf("session error — %s — %s", errMsg, describeTotals(tokens, cost)),
		InputTokens:  tokens.Input,
		OutputTokens: tokens.Output,
		CacheRead:    tokens.CacheRead,
		CacheWrite:   tokens.CacheWrite,
		TotalCost:    cost,
	})
	s.file.Close()
}