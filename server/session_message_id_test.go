package server

import (
	"testing"

	"github.com/kayushkin/inber/engine"
	sessionMod "github.com/kayushkin/inber/session"
)

// TestDisplayEventsNameTheMessageTheyBelongTo walks the whole chain the fix
// exists for: the agent tells the engine which message the provider is
// producing, and every event the session hands to a bus or bridge consumer
// carries that name. Testing the mapping functions alone would not catch the
// session forgetting to ask the engine.
func TestDisplayEventsNameTheMessageTheyBelongTo(t *testing.T) {
	engineSession, err := sessionMod.New(t.TempDir(), "claude-sonnet-4-5-20250929", "test", "", nil)
	if err != nil {
		t.Fatalf("building the engine's session: %v", err)
	}

	eng := &engine.Engine{Session: engineSession}
	s := &Session{Key: "test", Engine: eng}

	var events []StreamEvent
	s.setOnEvent(func(event StreamEvent) { events = append(events, event) })
	s.mu.Lock()
	s.updateHooks()
	s.mu.Unlock()

	hooks := eng.GetDisplayHooks()
	if hooks == nil {
		t.Fatal("updateHooks installed nothing — the test's premise is wrong")
	}

	// What the agent's OnMessageID hook does when the provider names the
	// message it is producing.
	eng.SetCurrentMessageID("msg_01XYZ")

	hooks.OnTextDelta("hello")
	hooks.OnThinking("hmm")
	hooks.OnToolCall("read_files", "{}")
	hooks.OnToolResult("read_files", "ok", false)
	hooks.OnStatus("Running agent...")

	if len(events) != 5 {
		t.Fatalf("got %d events, want one per display hook", len(events))
	}
	for _, event := range events[:4] {
		if event.MessageID != "msg_01XYZ" {
			t.Errorf("%s event message id = %q, want the message being produced", event.Kind, event.MessageID)
		}
	}
	if status := events[4]; status.MessageID != "" {
		t.Errorf("status event message id = %q, want empty: a status belongs to no message", status.MessageID)
	}
}
