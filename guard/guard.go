// Package guard provides safety controls for agent execution: execution modes,
// cost/token/turn limits, tool repetition detection, and anti-pattern monitoring.
//
// Execution modes control what an agent is allowed to do per-session:
//
//	Observe    — read-only tools (file reads, search, memory recall)
//	Assist     — reads + writes, with approval routing for dangerous ops
//	Autonomous — full tool access, no confirmation gates (current default)
//
// Limits enforce hard stops when agents exceed budgets:
//
//	MaxTurns        — API round-trip cap
//	MaxInputTokens  — cumulative input token cap
//	MaxCost         — cumulative dollar cost cap
//	MaxDuration     — wall-clock time cap
//
// Detectors watch for stuck or harmful agent behavior:
//
//	RepetitionDetector  — same tool + same input N times in a row
//	DelegationDetector  — spawn depth or delegation chain too deep
//	RubberStampDetector — review agent approving everything without substance
//
// Usage:
//
//	g := guard.New(guard.Config{Mode: guard.Assist, MaxCost: 5.00})
//	g.CheckTool("shell_commands", input)  // → allowed, denied, or needs_approval
//	g.RecordToolCall("shell_commands", input, output)
//	g.RecordCost(0.02)
//	if exceeded, reason := g.CheckLimits(); exceeded { ... }
package guard

// Mode controls the trust level for a session.
type Mode int

const (
	// Observe allows only read-only tools. No file writes, no shell execution.
	Observe Mode = iota
	// Assist allows reads and writes, but dangerous operations route for approval.
	Assist
	// Autonomous allows all tools without confirmation. Current default.
	Autonomous
)

func (m Mode) String() string {
	switch m {
	case Observe:
		return "observe"
	case Assist:
		return "assist"
	case Autonomous:
		return "autonomous"
	default:
		return "unknown"
	}
}

// Config configures the guard for a session.
type Config struct {
	Mode           Mode
	MaxTurns       int
	MaxInputTokens int
	MaxCost        float64 // dollars — 0 = unlimited
	MaxDuration    int     // seconds — 0 = unlimited

	// RepetitionThreshold is how many identical tool calls before triggering.
	// Default: 3.
	RepetitionThreshold int

	// ApprovalFunc is called when Mode is Assist and a tool needs confirmation.
	// Returns true if approved. Nil = deny all.
	ApprovalFunc func(tool, input string) bool
}

// ToolVerdict is the result of checking whether a tool call is allowed.
type ToolVerdict int

const (
	Allowed ToolVerdict = iota
	NeedsApproval
	Denied
)

// Guard enforces safety controls for a single agent session.
type Guard struct {
	cfg Config

	// Tracking
	turns      int
	inputToks  int
	cost       float64
	startTime  int64 // unix seconds

	// Repetition detection
	lastTool   string
	lastInput  string
	repeatCount int
}

// New creates a Guard with the given config.
func New(cfg Config) *Guard {
	if cfg.RepetitionThreshold == 0 {
		cfg.RepetitionThreshold = 3
	}
	return &Guard{cfg: cfg}
}

// CheckTool returns whether a tool call is allowed under the current mode.
func (g *Guard) CheckTool(tool, input string) ToolVerdict {
	// TODO: implement mode-based tool classification
	// Observe: only read_file, search, memory_load, list_files, etc.
	// Assist: all tools, but shell/write/delete route through ApprovalFunc
	// Autonomous: everything allowed
	switch g.cfg.Mode {
	case Observe:
		if isReadOnly(tool) {
			return Allowed
		}
		return Denied
	case Assist:
		if isDangerous(tool) {
			if g.cfg.ApprovalFunc != nil && g.cfg.ApprovalFunc(tool, input) {
				return Allowed
			}
			return NeedsApproval
		}
		return Allowed
	default:
		return Allowed
	}
}

// RecordToolCall tracks a tool invocation for repetition detection.
func (g *Guard) RecordToolCall(tool, input, output string) {
	if tool == g.lastTool && input == g.lastInput {
		g.repeatCount++
	} else {
		g.lastTool = tool
		g.lastInput = input
		g.repeatCount = 1
	}
}

// RecordTurn increments the turn counter and adds token usage.
func (g *Guard) RecordTurn(inputTokens int) {
	g.turns++
	g.inputToks += inputTokens
}

// RecordCost adds to the cumulative cost.
func (g *Guard) RecordCost(dollars float64) {
	g.cost += dollars
}

// CheckLimits returns whether any limit has been exceeded and why.
func (g *Guard) CheckLimits() (exceeded bool, reason string) {
	if g.cfg.MaxTurns > 0 && g.turns >= g.cfg.MaxTurns {
		return true, "max turns exceeded"
	}
	if g.cfg.MaxInputTokens > 0 && g.inputToks >= g.cfg.MaxInputTokens {
		return true, "max input tokens exceeded"
	}
	if g.cfg.MaxCost > 0 && g.cost >= g.cfg.MaxCost {
		return true, "max cost exceeded"
	}
	// TODO: check MaxDuration against startTime
	return false, ""
}

// IsRepeating returns true if the agent is stuck calling the same tool.
func (g *Guard) IsRepeating() bool {
	return g.repeatCount >= g.cfg.RepetitionThreshold
}

// --- Tool classification (stubs) ---

func isReadOnly(tool string) bool {
	// TODO: classify tools by read/write/execute
	switch tool {
	case "read_file", "list_files", "search_files", "memory_load", "memory_search",
		"repo_map", "recent_files", "web_search":
		return true
	}
	return false
}

func isDangerous(tool string) bool {
	// TODO: classify tools that need approval in Assist mode
	switch tool {
	case "shell_commands", "write_file", "edit_file", "delete_file", "deploy":
		return true
	}
	return false
}
