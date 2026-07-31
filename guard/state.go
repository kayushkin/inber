package guard

import "fmt"

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
// Mode travels with the caps for the same reason they travel with the totals.
// It is not a budget but it is a limit, and it is the only one whose loss opens
// the session up rather than merely uncapping it: a session created to observe
// and rebuilt without its mode comes back able to run `bash -c`.
type State struct {
	Mode           string  `json:"mode"`
	MaxTurns       int     `json:"max_turns"`
	MaxInputTokens int     `json:"max_input_tokens"`
	MaxCost        float64 `json:"max_cost"`

	Turns       int     `json:"turns"`
	InputTokens int     `json:"input_tokens"`
	Cost        float64 `json:"cost"`
}

// State reports the caps this guard enforces and what has been recorded against
// them. It is the read half of RestoreState, and it omits the fields of Config
// that describe how the guard was wired rather than what it is holding the
// session to: RepetitionThreshold and ApprovalFunc, one of which is a function
// and neither of which is a limit.
//
// Mode is not among them. It reads like wiring, and it was left out on the
// grounds that a rebuild reconfigures it — but nothing here configures a mode
// except the request that started the session, and a rebuild has no request.
func (g *Guard) State() State {
	return State{
		Mode:           g.cfg.Mode.String(),
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
//
// A record with no mode leaves the guard in the mode it was built with, the
// same way a zero cap leaves the configured cap alone. Records written before
// the mode was persisted have none, and a blank one must not be able to lower
// a gate that this rebuild put up.
//
// A mode that is present and cannot be parsed is reported and the guard is left
// in Observe. The record says the session was running under some mode; the only
// thing in doubt is which, and the read-only one is the only answer that cannot
// hand back more access than the session had.
func (g *Guard) RestoreState(s State) error {
	var err error
	if s.Mode != "" {
		g.cfg.Mode, err = ParseMode(s.Mode)
	}
	g.cfg.MaxTurns = s.MaxTurns
	g.cfg.MaxInputTokens = s.MaxInputTokens
	g.cfg.MaxCost = s.MaxCost
	g.turns = s.Turns
	g.inputToks = s.InputTokens
	g.cost = s.Cost
	if err != nil {
		return fmt.Errorf("recorded execution mode unreadable, session restored in observe mode: %w", err)
	}
	return nil
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
//
// The mode follows the same rule with the empty string in place of zero, which
// is what Unset renders as. It matters more here than it does for the caps: a
// rebuild that named no mode would otherwise reconfigure an observe session as
// the autonomous one it is being rebuilt as, and open it up.
func ResumeState(recorded, configured State) State {
	resumed := State{
		Mode:           recorded.Mode,
		MaxTurns:       recorded.MaxTurns,
		MaxInputTokens: recorded.MaxInputTokens,
		MaxCost:        recorded.MaxCost,
		Turns:          recorded.Turns,
		InputTokens:    recorded.InputTokens,
		Cost:           recorded.Cost,
	}
	if configured.Mode != "" {
		resumed.Mode = configured.Mode
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
