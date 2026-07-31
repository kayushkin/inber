package engine

import (
	"testing"

	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/guard"
)

// The tests below pin the three links inside the engine that carried a cost cap
// from the caller to the thing that enforces it. Each was broken independently,
// and two of the three could be repaired on their own without any observable
// change, so each link is asserted separately: breaking one must fail one test
// and name the link that broke.

// TestSetupLimitsCarriesTheCostCap covers the link from EngineConfig to
// LimitConfig. setupLimits used to return three ints — turns, input tokens and
// response time — and MaxCost was declared on EngineConfig beside the other two
// limits but was never one of the values returned, so it stopped here.
func TestSetupLimitsCarriesTheCostCap(t *testing.T) {
	limits := setupLimits(EngineConfig{MaxCost: 2.50}, nil)

	if limits.MaxCost != 2.50 {
		t.Errorf("setupLimits produced MaxCost %v from a config asking for $2.50 — "+
			"the cap does not survive the trip from EngineConfig to LimitConfig", limits.MaxCost)
	}
}

// TestSetupLimitsLeavesAnUnaskedForCapUnlimited is the complement. A caller who
// names no cap must not acquire one: 0 means unlimited, and inventing a default
// dollar figure here would halt sessions nobody asked to halt.
func TestSetupLimitsLeavesAnUnaskedForCapUnlimited(t *testing.T) {
	if limits := setupLimits(EngineConfig{}, nil); limits.MaxCost != 0 {
		t.Errorf("a request with no cost cap produced MaxCost %v, want 0 (unlimited)", limits.MaxCost)
	}

	// Detached sessions get default turn and token limits. They must not also
	// acquire a default price.
	if limits := setupLimits(EngineConfig{Detach: true}, nil); limits.MaxCost != 0 {
		t.Errorf("a detached session was given a default cost cap of %v, want 0 (unlimited)", limits.MaxCost)
	}
}

// TestGuardIsGivenTheCostCap covers the link from LimitConfig to the guard.
// The guard's config used to be written out inline at construction, listing
// MaxTurns and MaxInputTokens only, so MaxCost arrived as its zero value —
// which guard.Config documents as unlimited. The omission was therefore
// indistinguishable from a caller who wanted no cap.
func TestGuardIsGivenTheCostCap(t *testing.T) {
	limits := LimitConfig{MaxTurns: 10, MaxInputTokens: 500, MaxCost: 2.50}

	cfg := limits.GuardConfig(guard.Autonomous)

	if cfg.MaxCost != 2.50 {
		t.Errorf("the guard was configured with MaxCost %v from limits asking for $2.50 — "+
			"an unset cap is read as unlimited, so the session would run uncapped", cfg.MaxCost)
	}
	// The limits that already worked must keep working.
	if cfg.MaxTurns != 10 || cfg.MaxInputTokens != 500 {
		t.Errorf("GuardConfig dropped a limit that used to arrive: turns=%d tokens=%d, want 10 and 500",
			cfg.MaxTurns, cfg.MaxInputTokens)
	}
}

// TestTurnCostReachesTheGuard covers the last link: the running total the cap is
// compared against. Guard.RecordCost had no callers anywhere in the repo, so
// g.cost stayed at zero for the life of every session and CheckLimits could
// never find it above any cap.
//
// The assertion is that the guard's total moved, not that it moved to a
// particular dollar figure: the price of a turn belongs to the model registry
// and is pinned by the cost tests in session/, not here.
func TestTurnCostReachesTheGuard(t *testing.T) {
	g := guard.New(guard.Config{MaxCost: 1000}) // high enough not to trip
	e := &Engine{Guard: g, Model: "claude-haiku-4-5-20251001"}

	if exceeded, _ := g.CheckLimits(); exceeded {
		t.Fatal("the guard reported a limit exceeded before any turn ran")
	}

	e.recordTurnUsage(&agent.TurnResult{InputTokens: 1_000_000, OutputTokens: 100_000})

	if e.Tokens.Cost <= 0 {
		t.Fatalf("the session's own cost total is %v after a million input tokens — "+
			"the turn was not priced at all, so this test cannot say anything about the guard", e.Tokens.Cost)
	}
	if g.CostSoFar() <= 0 {
		t.Errorf("the session was charged $%.4f and the guard's running total is still $%.4f — "+
			"nothing feeds Guard.RecordCost, so MaxCost can never be reached",
			e.Tokens.Cost, g.CostSoFar())
	}
	if got := g.CostSoFar(); got != e.Tokens.Cost {
		t.Errorf("the guard is enforcing against $%.6f while the session reports $%.6f — "+
			"the turn was priced twice and the two figures disagree", got, e.Tokens.Cost)
	}
}

// TestASessionIsRefusedOnceItPassesItsCostCap is the behaviour the whole chain
// exists to produce, asserted at the point of enforcement: once the spend
// recorded for a session reaches the cap it was given, the next turn is refused
// before any work is done, with the reason RunTurn surfaces as
// "guard: max cost exceeded".
func TestASessionIsRefusedOnceItPassesItsCostCap(t *testing.T) {
	g := guard.New(LimitConfig{MaxCost: 0.10}.GuardConfig(guard.Autonomous))
	e := &Engine{Guard: g, Model: "claude-haiku-4-5-20251001"}

	// A turn well above a ten-cent cap.
	e.recordTurnUsage(&agent.TurnResult{InputTokens: 1_000_000, OutputTokens: 100_000})

	exceeded, reason := g.CheckLimits()
	if !exceeded {
		t.Fatalf("a session capped at $0.10 spent $%.4f and was allowed to continue", e.Tokens.Cost)
	}
	if reason != "max cost exceeded" {
		t.Errorf("the turn was refused for %q, want %q — a caller reading the reason "+
			"would be told the wrong limit stopped it", reason, "max cost exceeded")
	}
}

// TestAnUncappedSessionIsNeverRefusedForCost is the complement, and it is the
// one that keeps the fix from being worse than the defect. Almost no session
// sets max_cost. If an unset cap were treated as zero dollars rather than as
// unlimited, every one of those sessions would be refused at its second turn.
func TestAnUncappedSessionIsNeverRefusedForCost(t *testing.T) {
	g := guard.New(LimitConfig{}.GuardConfig(guard.Autonomous))
	e := &Engine{Guard: g, Model: "claude-haiku-4-5-20251001"}

	for turn := 0; turn < 5; turn++ {
		e.recordTurnUsage(&agent.TurnResult{InputTokens: 1_000_000, OutputTokens: 100_000})
		if exceeded, reason := g.CheckLimits(); exceeded {
			t.Fatalf("a session that asked for no cost cap was refused at turn %d: %q", turn+1, reason)
		}
	}
}

// TestRecordTurnUsageWithoutAGuard pins the nil case. Not every Engine carries a
// guard — the benchmark engine does not — and accounting for a turn must not
// panic when there is nothing to enforce a cap.
func TestRecordTurnUsageWithoutAGuard(t *testing.T) {
	e := &Engine{Model: "claude-haiku-4-5-20251001"}

	e.recordTurnUsage(&agent.TurnResult{InputTokens: 1000, OutputTokens: 100})

	if e.Tokens.Input != 1000 {
		t.Errorf("input tokens = %d, want 1000 — a guardless engine stopped accounting", e.Tokens.Input)
	}
}
