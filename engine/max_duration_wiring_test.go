package engine

import (
	"testing"

	"github.com/kayushkin/inber/guard"
)

// A cap is only a cap if it survives every hop between the caller who asked for
// it and the guard that enforces it. MaxCost was dropped at three of those hops
// and nobody noticed for months, because a dropped cap and an unset cap look
// the same from every side. MaxDuration had it worse: it never had a hop to be
// dropped at, because nothing outside the guard package had ever declared it.
//
// These tests pin the engine's hop. The API's is in
// server/max_duration_request_test.go, and the enforcement itself is in
// guard/duration_test.go.

func TestTheEngineHandsItsDurationCapToTheGuard(t *testing.T) {
	cfg := LimitConfig{MaxDuration: 900}.GuardConfig(guard.Autonomous)

	if cfg.MaxDuration != 900 {
		t.Errorf("an engine limited to 900s produced a guard config with MaxDuration %d — "+
			"the cap is dropped between the engine and the guard that enforces it", cfg.MaxDuration)
	}
}

func TestAnEngineWithNoDurationCapLeavesTheGuardUnlimited(t *testing.T) {
	cfg := LimitConfig{}.GuardConfig(guard.Autonomous)

	if cfg.MaxDuration != 0 {
		t.Errorf("an engine with no duration limit produced MaxDuration %d, want 0 (unlimited)", cfg.MaxDuration)
	}
}

// TestSetupLimitsCarriesTheDurationCapOffTheEngineConfig covers the hop before
// it: the field is read off EngineConfig, where the server writes it, rather
// than being left behind the way MaxCost was when setupLimits returned a tuple.
func TestSetupLimitsCarriesTheDurationCapOffTheEngineConfig(t *testing.T) {
	limits := setupLimits(EngineConfig{MaxDuration: 1800}, nil)

	if limits.MaxDuration != 1800 {
		t.Errorf("a session configured with a 1800s cap produced LimitConfig.MaxDuration %d — "+
			"the cap is dropped between the engine config and the limits the guard is built from",
			limits.MaxDuration)
	}
}

// TestADetachedSessionInventsNoDurationCap. Detached sessions get default turn
// and token bounds because they run unattended, but a wall-clock deadline
// invented here would be a policy nobody asked for — and unlike a turn cap it
// can stop a session that is doing exactly what it was told to do.
func TestADetachedSessionInventsNoDurationCap(t *testing.T) {
	limits := setupLimits(EngineConfig{Detach: true}, nil)

	if limits.MaxDuration != 0 {
		t.Errorf("a detached session was given a %ds duration cap nobody asked for", limits.MaxDuration)
	}
}
