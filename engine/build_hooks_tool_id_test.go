package engine

import (
	"testing"

	sessionMod "github.com/kayushkin/inber/session"
)

// buildHooks wires the display branch TWICE — once with session logging and
// once without (`e.Session != nil`) — and the two are separate literals, so a
// fix applied to one leaves the other dropping the id. Both are asserted here
// for that reason: an earlier version of this test exercised the display hooks
// directly and stayed green while this file was sabotaged, because calling
// DisplayHooks.OnToolCall by hand never runs the forwarding under test.
func TestBuildHooksForwardsTheToolIDToTheDisplayHooks(t *testing.T) {
	for _, tc := range []struct {
		name        string
		withSession bool
	}{
		{"session logging on", true},
		{"session logging off", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &Engine{}
			if tc.withSession {
				engineSession, err := sessionMod.New(t.TempDir(), "claude-sonnet-4-5-20250929", "test", "", nil)
				if err != nil {
					t.Fatalf("building the engine's session: %v", err)
				}
				e.Session = engineSession
			}

			var callID, resultID string
			e.SetDisplayHooks(&DisplayHooks{
				OnToolCall: func(toolID, name, input string) {
					callID = toolID
				},
				OnToolResult: func(toolID, name, output string, isError bool) {
					resultID = toolID
				},
			})

			hooks := e.buildHooks()
			if hooks.OnToolCall == nil || hooks.OnToolResult == nil {
				t.Fatal("buildHooks wired no tool hooks — the test's premise is wrong")
			}

			hooks.OnToolCall("toolu_01ABC", "read_files", []byte(`{"path":"a.go"}`))
			hooks.OnToolResult("toolu_01ABC", "read_files", "package main", false)

			if callID != "toolu_01ABC" {
				t.Errorf("display OnToolCall got tool id %q, want the provider's block id", callID)
			}
			if resultID != "toolu_01ABC" {
				t.Errorf("display OnToolResult got tool id %q, want the provider's block id", resultID)
			}
		})
	}
}
