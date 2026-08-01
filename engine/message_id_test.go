package engine

import "testing"

// TestBuildHooksRecordsTheMessageBeingProduced covers the seam between the
// agent, which learns the message id from the provider, and everything that
// stamps an event with it, which reads it back off the engine.
func TestBuildHooksRecordsTheMessageBeingProduced(t *testing.T) {
	e := &Engine{}

	if got := e.CurrentMessageID(); got != "" {
		t.Fatalf("a fresh engine reports message id %q, want empty", got)
	}

	hooks := e.buildHooks()
	if hooks.OnMessageID == nil {
		t.Fatal("buildHooks left OnMessageID unwired, so no event can name its message")
	}

	hooks.OnMessageID("msg_first")
	if got := e.CurrentMessageID(); got != "msg_first" {
		t.Errorf("CurrentMessageID = %q, want %q", got, "msg_first")
	}

	hooks.OnMessageID("msg_second")
	if got := e.CurrentMessageID(); got != "msg_second" {
		t.Errorf("CurrentMessageID = %q after a second message, want %q", got, "msg_second")
	}
}
