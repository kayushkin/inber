package guard

// State is everything about a guard that has to outlive the process it ran in:
// the caps it is enforcing and the totals it has counted against them.
//
// A guard is built fresh every time a session is built, and a fresh guard is
// zero on both halves. Zero caps read as unlimited (see Config), so a session
// that was given limits and is later rebuilt does not merely come back without
// them — it comes back indistinguishable from a session nobody ever capped, with
// no log line and no error. Zero totals fail the other way: a $5 cap whose
// running total restarts at nothing every time the session is rebuilt is a $5
// cap per rebuild, not per session.
//
// The two halves travel together because either one alone restores a limit that
// does not hold. Caps without totals hand the whole budget back on every
// rebuild; totals without caps count against nothing.
type State struct {
	MaxTurns       int     `json:"max_turns"`
	MaxInputTokens int     `json:"max_input_tokens"`
	MaxCost        float64 `json:"max_cost"`

	Turns       int     `json:"turns"`
	InputTokens int     `json:"input_tokens"`
	Cost        float64 `json:"cost"`
}

// State reports the caps this guard enforces and what has been recorded against
// them. It is the read half of RestoreState, and it deliberately omits the
// fields of Config that describe how the guard was wired rather than how far
// the session has got: Mode, RepetitionThreshold and ApprovalFunc are rebuilt
// from the agent's configuration every time and one of them is a function.
func (g *Guard) State() State {
	return State{
		MaxTurns:       g.cfg.MaxTurns,
		MaxInputTokens: g.cfg.MaxInputTokens,
		MaxCost:        g.cfg.MaxCost,
		Turns:          g.turns,
		InputTokens:    g.inputToks,
		Cost:           g.cost,
	}
}

// RestoreState installs the caps and totals recorded for an earlier run of the
// same session. Call it on a guard that has not yet run a turn — it overwrites
// the totals rather than adding to them, because it is putting a session back
// where it was, not merging two sessions.
func (g *Guard) RestoreState(s State) {
	g.cfg.MaxTurns = s.MaxTurns
	g.cfg.MaxInputTokens = s.MaxInputTokens
	g.cfg.MaxCost = s.MaxCost
	g.turns = s.Turns
	g.inputToks = s.InputTokens
	g.cost = s.Cost
}

// ResumeState says what state a rebuilt session's guard should hold, given what
// was recorded for that session before (recorded) and what this rebuild
// configured it with (configured).
//
// The totals always come from the record. They are the session's own history
// and a rebuild has no other way to learn them.
//
// A cap takes the configured value wherever the rebuild has one, and falls back
// to the record where it has none. A configured cap is somebody asking now, and
// what is being asked for now outranks what was asked for last time — including
// when it is tighter.
//
// A configured zero is not somebody asking for unlimited. Nothing in this
// codebase can lower a cap to zero: over the HTTP API zero means "field not
// sent" and is skipped rather than copied, and the one mid-session setter,
// SetMaxInputTokens, is only ever called with a positive number. So zero here
// always means "nothing was said this time", and the record is what was said
// last.
func ResumeState(recorded, configured State) State {
	resumed := State{
		MaxTurns:       recorded.MaxTurns,
		MaxInputTokens: recorded.MaxInputTokens,
		MaxCost:        recorded.MaxCost,
		Turns:          recorded.Turns,
		InputTokens:    recorded.InputTokens,
		Cost:           recorded.Cost,
	}
	if configured.MaxTurns != 0 {
		resumed.MaxTurns = configured.MaxTurns
	}
	if configured.MaxInputTokens != 0 {
		resumed.MaxInputTokens = configured.MaxInputTokens
	}
	if configured.MaxCost != 0 {
		resumed.MaxCost = configured.MaxCost
	}
	return resumed
}
