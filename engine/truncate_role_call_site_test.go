package engine

import (
	"strings"
	"testing"

	sessionMod "github.com/kayushkin/inber/session"
)

// Which truncation budget a session gets is decided at exactly one line of
// initWorkflow -- session.ConfigForRole(e.AgentName) -- and until this
// file existed, nothing in the repository held that line.
//
// session's own suite is thorough about the helper (TestConfigForRole
// and ExampleConfigForRole both walk every role) and that is the trap:
// it proves the helper answers correctly, never that the engine asks it.
// Reverting the call site to session.DefaultTruncateConfig(), the role-blind
// behaviour the helper replaced, left `go test ./...` fully green -- measured
// 2026-08-29 on both call sites.
//
// So these tests observe the budget the session actually ends up with, through
// the exported Session.TruncateToolResult, and never name the helper. A rename
// keeps them passing; the engine forgetting to ask reddens them.

// roleBudgets is the size of tool result each role may keep, in tokens.
//
// The numbers are written out here on purpose. Reading them back from
// session.ConfigForRole would make this suite agree with the helper by
// construction -- the derived-fixture defect -- and a fixture derived from the
// thing under test cannot disagree with it.
var roleBudgets = []struct {
	role      string
	threshold int
	// discriminating reports whether this row can tell a role-aware call site
	// from a role-blind one. The role-blind default is 1000, so the rows that
	// happen to want 1000 pin the default branch and nothing more; they are
	// kept because they are still the contract, and flagged so no one counts
	// them as evidence the call site is held.
	discriminating bool
}{
	{role: "main", threshold: 1000, discriminating: false},
	{role: "agent", threshold: 1000, discriminating: false},
	{role: "MAIN", threshold: 1000, discriminating: false}, // the switch lowercases
	{role: "project", threshold: 3000, discriminating: true},
	{role: "run", threshold: 5000, discriminating: true},
	{role: "unrecognised-role", threshold: 1000, discriminating: false},
	{role: "", threshold: 1000, discriminating: false},
}

// toolResultOf returns a tool result that estimates to exactly tokens tokens.
// session.EstimateTokens is len/4, and this deliberately reproduces that rather
// than calling it, for the same reason roleBudgets holds literals.
func toolResultOf(tokens int) string { return strings.Repeat("x", tokens*4) }

// engineForRole builds the smallest Engine initWorkflow will act on: a session
// to configure, and the role to configure it for.
func engineForRole(t *testing.T, role string) (*Engine, *sessionMod.Session) {
	t.Helper()
	sess, err := sessionMod.New(t.TempDir(), "test-model", role, "", nil)
	if err != nil {
		t.Fatalf("session.New(%q): %v", role, err)
	}
	t.Cleanup(sess.Close)
	e := &Engine{Session: sess, AgentName: role, repoRoot: t.TempDir()}
	return e, sess
}

// truncates reports whether the session cut a tool result of the given size.
// Session.TruncateToolResult answers "" when it left the output alone.
func truncates(sess *sessionMod.Session, tokens int) bool {
	return sess.TruncateToolResult("Bash", toolResultOf(tokens), false) != ""
}

// TestInitWorkflowGivesTheSessionTheBudgetForItsRole pins the call site at its
// boundary: a result at the role's threshold survives, one token more is cut.
// Two rows here fail against a role-blind call site -- project keeping 3000 and
// run keeping 5000 -- which is what makes this a pin and not a restatement of
// the helper's own suite.
func TestInitWorkflowGivesTheSessionTheBudgetForItsRole(t *testing.T) {
	for _, b := range roleBudgets {
		t.Run(b.role, func(t *testing.T) {
			e, sess := engineForRole(t, b.role)
			e.initWorkflow(EngineConfig{})

			if truncates(sess, b.threshold) {
				t.Errorf("role %q cut a %d-token tool result; its budget is %d, so this one fits",
					b.role, b.threshold, b.threshold)
			}
			if !truncates(sess, b.threshold+1) {
				t.Errorf("role %q kept a %d-token tool result whole; its budget is %d, so this one is over",
					b.role, b.threshold+1, b.threshold)
			}
		})
	}
}

// TestInitWorkflowIsWhatSetsTheBudget separates "the engine asked for the role's
// budget" from "the session happened to start with it". A fresh session starts
// on session.DefaultTruncateConfig(), whose threshold is 1000 and therefore
// equal to main's -- so for main the call site is invisible by construction, and
// only a role whose budget differs from the default shows the call running.
func TestInitWorkflowIsWhatSetsTheBudget(t *testing.T) {
	const overTheDefaultUnderRun = 4000

	e, sess := engineForRole(t, "run")
	if !truncates(sess, overTheDefaultUnderRun) {
		t.Fatalf("a brand-new session kept a %d-token result whole; it is supposed to start on the "+
			"role-blind default, so this test can no longer tell whether initWorkflow ran",
			overTheDefaultUnderRun)
	}

	e.initWorkflow(EngineConfig{})

	if truncates(sess, overTheDefaultUnderRun) {
		t.Errorf("after initWorkflow a run session still cut a %d-token result: the session kept the "+
			"role-blind default, so the role was never asked for", overTheDefaultUnderRun)
	}
}

// TestInitWorkflowLeavesSmallToolResultsAlone is the cry-wolf control. Every
// assertion above is ultimately "was this cut?", and an error surface that fires
// on the quiet case would satisfy half of them while meaning nothing. No role
// truncates a result this small.
func TestInitWorkflowLeavesSmallToolResultsAlone(t *testing.T) {
	for _, b := range roleBudgets {
		t.Run(b.role, func(t *testing.T) {
			e, sess := engineForRole(t, b.role)
			e.initWorkflow(EngineConfig{})
			if truncates(sess, 1) {
				t.Errorf("role %q cut a 1-token tool result", b.role)
			}
		})
	}
}
