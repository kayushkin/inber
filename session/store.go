package session

// Store defines the interface for session and turn storage backends.
// This allows swapping between SQLite, Postgres, Redis, or other implementations.
type Store interface {
	// Session management
	InsertSession(s *SessionRow) error
	EndSession(id, status, errMsg string) error
	SetTask(sessionID, task string) error
	ListSessions(limit int) ([]SessionSummary, error)
	ListActive() ([]SessionSummary, error)
	ListActiveStatus() ([]ActiveAgentStatus, error)
	DetectInterrupted() (int, error)

	// Turn management
	InsertTurn(t *TurnRow) error
	EndTurn(sessionID string, turn int, inTokens, outTokens, toolCalls int, cost float64, stopReason, errMsg string) error
	IncrementToolCalls(sessionID string, turn int) error
	GetTurns(sessionID string) ([]TurnRow, error)

	// Cleanup
	Close() error
}

// Ensure SQLiteStore implements Store interface
var _ Store = (*SQLiteStore)(nil)