// Package guard provides safety controls for agent execution: execution modes
// and cost/token/turn/duration limits.
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
//	MaxDuration     — wall-clock cap on how long the session may go on running
//
// This package once advertised three detectors — repetition, delegation depth
// and rubber-stamping. Only the first was ever written, and it does not run:
// RecordToolCall has no caller outside this package's own tests, so repeatCount
// never leaves zero, IsRepeating always answers false, and RepetitionThreshold
// is a knob no non-test caller can set feeding a counter nothing increments.
// See RecordToolCall for what would have to change. Delegation depth is bounded,
// but in the server (MaxSpawnDepth), not here. Nothing checks for rubber-stamping.
//
// Usage:
//
//	g := guard.New(guard.Config{Mode: guard.Assist, MaxCost: 5.00})
//	g.CheckTool("shell_commands", input)  // → allowed, denied, or needs_approval
//	g.RecordTurn(result.InputTokens)
//	g.RecordCost(0.02)
//	if exceeded, reason := g.CheckLimits(); exceeded { ... }
package guard

import "time"

// Mode controls the trust level for a session.
type Mode int

const (
	// Unset is the mode nobody named, and it is the zero value on purpose: a
	// Config built without a mode belongs to a caller who said nothing about
	// trust, and a caller who says nothing gets what every session on this
	// server got before modes were settable — full access. Making Observe the
	// zero value would read a silent config as the strictest mode and refuse
	// every write in the process.
	Unset Mode = iota
	// Observe allows only read-only tools. No file writes, no shell execution.
	Observe
	// Assist allows reads and writes, but dangerous operations route for approval.
	Assist
	// Autonomous allows all tools without confirmation. Current default.
	Autonomous
)

// String names the mode. Unset has no name — it is the absence of one — and
// renders as the empty string so that a recorded mode can be told apart from a
// mode nobody set, the same way a zero cap is told apart from a cap of zero.
func (m Mode) String() string {
	switch m {
	case Unset:
		return ""
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
	turns     int
	inputToks int
	cost      float64

	// The wall clock MaxDuration is measured against. It is two fields because
	// a session outlives the process that runs it: startedAt is when this guard
	// began running, and secondsBeforeThisRun is what earlier runs of the same
	// session already counted. See ElapsedSeconds.
	startedAt            time.Time
	secondsBeforeThisRun int

	// Repetition detection
	lastTool    string
	lastInput   string
	repeatCount int
}

// New creates a Guard with the given config.
//
// The clock MaxDuration is measured against starts here. Nothing else starts
// it: the guard is built once per session, and a cap that only began counting
// at the first turn would hand back whatever the session spent getting there.
func New(cfg Config) *Guard {
	if cfg.RepetitionThreshold == 0 {
		cfg.RepetitionThreshold = 3
	}
	return &Guard{cfg: cfg, startedAt: time.Now()}
}

// CheckTool returns whether a tool call is allowed under the current mode.
//
// Observe answers Allowed for the read-only tools and Denied for everything
// else, so a tool this package has never classified is refused rather than
// waved through. Assist routes the dangerous tools through ApprovalFunc and
// answers NeedsApproval when there is no approver to ask. Autonomous — and
// Unset, the mode nobody named — allow everything, which is what every session
// here did before this check had a caller.
func (g *Guard) CheckTool(tool, input string) ToolVerdict {
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

// Mode reports the trust level this guard is enforcing. Whoever refuses a tool
// call has to say under which mode it was refused, and the mode is otherwise
// unreadable from outside the package.
func (g *Guard) Mode() Mode {
	return g.cfg.Mode
}

// RecordToolCall tracks a tool invocation for repetition detection.
//
// Nothing calls it. The engine dispatches every tool through agent.Hooks
// (engine/build_hooks.go), and no hook reaches this method, so the counter it
// feeds has stayed at zero for the life of every session — which is why
// IsRepeating has never once answered true and RepetitionThreshold has never
// been compared against anything. Wiring it is one line in the OnToolResult
// hook; what a caller should then DO when IsRepeating goes true is the part
// nobody has decided, and building the write before naming that reader is how
// this pair got here.
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

// CostSoFar returns the cumulative cost recorded for this session, in dollars.
// This is the figure CheckLimits compares against MaxCost, and the only way to
// read it from outside the package: a running total nothing can see is a total
// nothing can check.
func (g *Guard) CostSoFar() float64 {
	return g.cost
}

// ElapsedSeconds returns the wall-clock seconds counted against MaxDuration:
// how long this guard has been running, plus what earlier runs of the same
// session recorded before it was rebuilt.
//
// The two are added rather than one replacing the other for the reason the
// dollar total is restored at all — a cap whose clock restarts every time the
// session is rebuilt is a cap per rebuild, not per session. Time the session
// spent not loaded is not counted, because nothing was running to count it.
func (g *Guard) ElapsedSeconds() int {
	return g.secondsBeforeThisRun + int(time.Since(g.startedAt).Seconds())
}

// CheckLimits returns whether any limit has been exceeded and why.
//
// Every cap here is checked between turns, so a limit is reached by refusing to
// start the next one rather than by stopping work already in flight. That is
// what MaxDuration means too: it bounds how long a session may go on running,
// not how long a single turn may take. MaxResponseTime is the per-turn bound
// and lives in the engine's build hooks.
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
	if g.cfg.MaxDuration > 0 && g.ElapsedSeconds() >= g.cfg.MaxDuration {
		return true, "max duration exceeded"
	}
	return false, ""
}

// SetMaxInputTokens updates the input token limit.
func (g *Guard) SetMaxInputTokens(max int) {
	g.cfg.MaxInputTokens = max
}

// IsRepeating returns true if the agent is stuck calling the same tool. It has
// no caller either, and cannot be true while RecordToolCall has none. See there.
func (g *Guard) IsRepeating() bool {
	return g.repeatCount >= g.cfg.RepetitionThreshold
}

// --- Tool classification (stubs) ---

// Every name below must be one a tool actually answers to — a name no tool has
// is not a safe default, it is a hole: isDangerous silently stopped matching the
// write tools when tool-store renamed write_file -> write_files, so Assist mode
// would have waved writes through without approval. TestClassifiedToolsExist
// pins each name against the real tool set.

func isReadOnly(tool string) bool {
	switch tool {
	case "read_files", "list_files", "ripgrep", "memory_expand", "memory_search",
		"repo_map", "recent_files", "web_search":
		return true
	}
	return false
}

func isDangerous(tool string) bool {
	switch tool {
	case "shell_commands", "write_files", "edit_files", "deploy":
		return true
	}
	return false
}
