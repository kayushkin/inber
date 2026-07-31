package server

import (
	"encoding/json"
	"testing"

	"github.com/kayushkin/inber/engine"
	"github.com/kayushkin/inber/guard"
)

// TestTheDurationCapLeavesTheRequest pins the API's link in the chain that
// carries a wall-clock cap from a caller to the guard that enforces it, the
// same way max_cost is pinned next door.
func TestTheDurationCapLeavesTheRequest(t *testing.T) {
	var cfg engine.EngineConfig

	applyRequestOverrides(&cfg, RunRequest{MaxDuration: 1800})

	if cfg.MaxDuration != 1800 {
		t.Errorf("a request asking for a 1800s cap produced an engine config with MaxDuration %d — "+
			"max_duration is accepted over the API and dropped before the engine sees it", cfg.MaxDuration)
	}
}

// TestARequestWithNoDurationCapAsksForNoCap is the complement. Zero seconds
// means unlimited in guard.Config, so a request that names no cap must leave
// the field alone rather than write a zero that reads as a cap of nothing.
func TestARequestWithNoDurationCapAsksForNoCap(t *testing.T) {
	var cfg engine.EngineConfig

	applyRequestOverrides(&cfg, RunRequest{})

	if cfg.MaxDuration != 0 {
		t.Errorf("an empty request produced MaxDuration %d, want 0 (unlimited)", cfg.MaxDuration)
	}
}

// TestTheDurationCapIsSpelledTheWayTheOtherLimitsAre. The field is only
// reachable if a caller can name it, and the name is the API. This asserts the
// wire spelling rather than the Go one, because a caller sends the former.
func TestTheDurationCapIsSpelledTheWayTheOtherLimitsAre(t *testing.T) {
	var req RunRequest
	if err := json.Unmarshal([]byte(`{"max_duration": 300}`), &req); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if req.MaxDuration != 300 {
		t.Errorf("a request body carrying max_duration=300 decoded to MaxDuration %d — "+
			"the documented field name is not the one the server reads", req.MaxDuration)
	}
}

// TestARebuiltSessionKeepsItsDurationCapAndItsElapsedTime is the resume half.
// handleBridgeResume rebuilds a session with a zero RunRequest, so every cap it
// had has to come back off the record — and the elapsed total has to come with
// it, or a session rebuilt often enough never reaches any deadline at all.
func TestARebuiltSessionKeepsItsDurationCapAndItsElapsedTime(t *testing.T) {
	original := guard.New(guard.Config{Mode: guard.Autonomous, MaxDuration: 600})
	recorded := original.State()
	recorded.ElapsedSeconds = 590

	rebuilt := guard.New(guard.Config{Mode: guard.Autonomous})
	if err := rebuilt.RestoreState(guard.ResumeState(recorded, rebuilt.State())); err != nil {
		t.Fatalf("RestoreState: %v", err)
	}

	state := rebuilt.State()
	if state.MaxDuration != 600 {
		t.Errorf("the resumed session's duration cap is %ds, want 600s — a capped session came back uncapped",
			state.MaxDuration)
	}
	if state.ElapsedSeconds < 590 {
		t.Errorf("the resumed session reports %ds elapsed, want at least 590s — the rebuild handed the deadline back",
			state.ElapsedSeconds)
	}
}
