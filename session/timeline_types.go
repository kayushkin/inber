package session

import (
	"time"
)

// TimelineEvent represents a single event in the session timeline.
type TimelineEvent struct {
	Type      string    // "prompt", "thinking", "tool_call", "tool_result", "response", "stats"
	Timestamp time.Time

	// Prompt events
	UserMessage string // truncated
	PromptFile  string // path to full prompt breakdown file
	TurnNumber  int

	// Tool events
	ToolName    string
	ToolInput   string // summarized
	ToolOutput  string // summarized
	ToolIsError bool
	ToolCount   int // running count for this turn

	// Response events
	ResponseText string // truncated

	// Stats
	InputTokens  int
	OutputTokens int
	CacheRead    int
	CacheWrite   int
	ToolCalls    int
	Cost         float64
	Model        string
}