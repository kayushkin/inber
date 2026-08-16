package engine

import (
	"testing"

	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/guard"
)

// limit_check_test.go scores the limit callback itself: given a LimitConfig and
// a TurnResult, does buildLimitCheck answer "exceeded" at the right moment. It
// is thorough and it is a test of a function that nothing was shown to install.
//
// configureAgent is what installs it, behind a three-armed disjunction:
//
//	if e.Limits.MaxTurns > 0 || e.Limits.MaxInputTokens > 0 || e.Limits.MaxResponseTime > 0 {
//		a.SetLimitCheck(e.buildLimitCheck())
//	}
//
// Nothing ran that line. Measured on main by putting `panic()` on the first
// line of agent.SetLimitCheck: `go test ./...` stayed green, because every
// existing test either calls buildLimitCheck directly or assigns
// agent.LimitCheck by hand. So the callback is pinned and its installation is
// not, and the two failures that hides are the expensive ones — an arm dropped
// from the disjunction, or the condition inverted, leaves a session that asked
// for a cap running with no cap at all. Every limit_check_test.go case would
// still pass, because none of them goes through configureAgent.
//
// Each arm gets its own fixture. A fixture that sets all three limits at once
// is the realistic one and it is the one that cannot see a dropped arm: with
// MaxTurns already non-zero the disjunction is satisfied whatever the other two
// say. Impoverish the fixture until only the arm under test can carry it.

// installsLimitCheck runs the real wiring path and reports whether a limit
// check came out of it. It asserts the effect on the agent, not the value
// configureAgent was called with.
func installsLimitCheck(t *testing.T, limits LimitConfig) bool {
	t.Helper()
	a := agent.New(nil, "system")
	if a.LimitCheck != nil {
		t.Fatal("a fresh agent already carries a limit check, so this test cannot tell who installed it")
	}
	e := &Engine{Limits: limits}
	e.configureAgent(a)
	return a.LimitCheck != nil
}

func TestEachLimitOnItsOwnInstallsTheLimitCheck(t *testing.T) {
	for _, tc := range []struct {
		name   string
		limits LimitConfig
	}{
		{"only MaxTurns", LimitConfig{MaxTurns: 10}},
		{"only MaxInputTokens", LimitConfig{MaxInputTokens: 100000}},
		{"only MaxResponseTime", LimitConfig{MaxResponseTime: 30}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !installsLimitCheck(t, tc.limits) {
				t.Errorf("%s did not install a limit check, so a session that asked for this cap would run uncapped", tc.name)
			}
		})
	}
}

// The negative half. Without it the wiring test passes for a configureAgent
// that installs a limit check unconditionally, which would make every uncapped
// session pay for a callback that can only ever answer false.
func TestNoLimitsInstallsNoLimitCheck(t *testing.T) {
	if installsLimitCheck(t, LimitConfig{}) {
		t.Error("an engine with no limits installed a limit check anyway; the guard in configureAgent is not doing anything")
	}
}

// The installed callback has to be the engine's own, carrying the engine's
// limits — not merely non-nil. A wiring that installed some other callback, or
// one built from a zero LimitConfig, would satisfy the tests above and still
// never fire.
func TestTheInstalledCheckCarriesTheEnginesLimits(t *testing.T) {
	a := agent.New(nil, "system")
	e := &Engine{Limits: LimitConfig{MaxTurns: 10}}
	e.configureAgent(a)

	if a.LimitCheck == nil {
		t.Fatal("no limit check was installed")
	}

	under := &agent.TurnResult{ToolCalls: 9}
	if exceeded, _ := a.LimitCheck(under); exceeded {
		t.Error("the installed check refused a turn under the engine's own MaxTurns of 10")
	}

	over := &agent.TurnResult{ToolCalls: 10}
	exceeded, reason := a.LimitCheck(over)
	if !exceeded {
		t.Error("the installed check allowed a turn at the engine's MaxTurns of 10, so it is not carrying these limits")
	}
	if reason == "" {
		t.Error("the installed check refused without a reason, and the reason is what the final summary call is told")
	}
}

// The tool gate sits four lines above the limit check in configureAgent and is
// deliberately NOT behind a condition there — the comment says so, and the
// reason it gives is that deciding in configureAgent whether a session needs a
// gate would put that answer in two places. The guard answers instead, by
// buildToolRefusal returning nil when there is no guard to ask.
//
// That contrast is the design, so pin both halves against one fixture: with a
// guard present and no limits set, the gate is installed and the limit check is
// not. A test that only checked the limit check could not tell "configureAgent
// wires nothing here" from "configureAgent wires the gate and skips the check".
func TestTheToolGateIsWiredEvenWhenNoLimitsAre(t *testing.T) {
	a := agent.New(nil, "system")
	e := &Engine{Guard: guard.New(guard.Config{Mode: guard.Observe})}
	e.configureAgent(a)

	if a.ToolRefusal == nil {
		t.Error("no tool gate was wired for a guarded engine; the gate is not meant to depend on the limits")
	}
	if a.LimitCheck != nil {
		t.Error("a limit check was wired for an engine with no limits, so this fixture is not the no-limits case it claims to be")
	}
}

// The other half of the guard's answer: with no guard there is nothing to ask,
// so the gate must come out nil rather than as a closure that always allows.
// Those two look identical to the turn loop and differ entirely in what a later
// reader believes about whether the session was gated.
func TestAnUnguardedEngineWiresNoToolGate(t *testing.T) {
	a := agent.New(nil, "system")
	e := &Engine{}
	e.configureAgent(a)

	if a.ToolRefusal != nil {
		t.Error("an engine with no guard wired a tool gate anyway, which reads as a gated session that refuses nothing")
	}
}
