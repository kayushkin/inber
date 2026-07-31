package server

import (
	"testing"

	"github.com/kayushkin/inber/engine"
)

// TestTheExecutionModeLeavesTheRequest pins the first link in the chain that
// carries a trust level from an API caller to the guard that enforces it.
//
// RunRequest declared Mode as `mode`, documented as "execution mode: observe,
// assist, autonomous", and no code anywhere in this repository ever read it.
// A caller who asked to observe got a 200 and a session that could run
// `bash -c` with the server's full environment.
func TestTheExecutionModeLeavesTheRequest(t *testing.T) {
	for _, mode := range []string{"observe", "assist", "autonomous"} {
		var cfg engine.EngineConfig

		applyRequestOverrides(&cfg, RunRequest{Mode: mode})

		if cfg.Mode != mode {
			t.Errorf("a request asking for %q produced an engine config with Mode %q — "+
				"mode is accepted over the API and dropped before the engine sees it", mode, cfg.Mode)
		}
	}
}

// TestARequestWithNoModeAsksForNoMode is the complement, and it is the one that
// keeps every existing session working. Nothing on this host sends a mode, so
// an empty request must leave the field empty — which the engine reads as the
// full access every session here has always run with — rather than write some
// stricter default over it.
func TestARequestWithNoModeAsksForNoMode(t *testing.T) {
	var cfg engine.EngineConfig

	applyRequestOverrides(&cfg, RunRequest{})

	if cfg.Mode != "" {
		t.Errorf("an empty request produced Mode %q, want \"\"", cfg.Mode)
	}
}
