package server

import (
	"testing"

	"github.com/kayushkin/inber/engine"
)

// TestTheCostCapLeavesTheRequest pins the first link in the chain that carries a
// spending cap from an API caller to the guard that enforces it.
//
// RunRequest declared MaxCost as `max_cost`, documented as "safety limit: max
// dollar cost", and no code in this package ever read it. A caller who asked
// for a cap got a 200 and a session with no cap at all.
func TestTheCostCapLeavesTheRequest(t *testing.T) {
	var cfg engine.EngineConfig

	applyRequestOverrides(&cfg, RunRequest{MaxCost: 5.00})

	if cfg.MaxCost != 5.00 {
		t.Errorf("a request asking for a $5.00 cap produced an engine config with MaxCost %v — "+
			"max_cost is accepted over the API and dropped before the engine sees it", cfg.MaxCost)
	}
}

// TestTheOtherSafetyLimitsStillLeaveTheRequest guards the fields that already
// worked. max_cost was the one limit of the three that was dropped, which is
// what made the defect invisible: sessions with max_turns or max_input_tokens
// behaved exactly as documented.
func TestTheOtherSafetyLimitsStillLeaveTheRequest(t *testing.T) {
	var cfg engine.EngineConfig

	applyRequestOverrides(&cfg, RunRequest{MaxTurns: 7, MaxInputTokens: 90_000})

	if cfg.MaxTurns != 7 {
		t.Errorf("MaxTurns = %d, want 7", cfg.MaxTurns)
	}
	if cfg.MaxInputTokens != 90_000 {
		t.Errorf("MaxInputTokens = %d, want 90000", cfg.MaxInputTokens)
	}
}

// TestARequestWithNoCostCapAsksForNoCap is the complement. Zero dollars means
// unlimited in guard.Config, so a request that names no cap must leave the
// field alone rather than write a zero that reads as a cap of nothing.
func TestARequestWithNoCostCapAsksForNoCap(t *testing.T) {
	var cfg engine.EngineConfig

	applyRequestOverrides(&cfg, RunRequest{})

	if cfg.MaxCost != 0 {
		t.Errorf("an empty request produced MaxCost %v, want 0 (unlimited)", cfg.MaxCost)
	}
}
