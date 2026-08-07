package server

import (
	"testing"

	"github.com/kayushkin/inber/engine"
	sessionMod "github.com/kayushkin/inber/session"
	"github.com/kayushkin/llm-bridge/msg"
)

// TestToolEventsCarryTheProvidersToolID walks the whole chain, because the two
// halves fail differently: the session can stamp an id the bridge then drops,
// and the bridge can copy a field the session never filled. Testing
// streamEventToBridge alone would not have caught the display hook that took no
// id at all, which is what was actually wrong.
func TestToolEventsCarryTheProvidersToolID(t *testing.T) {
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

	hooks.OnToolCall("toolu_01ABC", "read_files", `{"path":"a.go"}`)
	hooks.OnToolResult("toolu_01ABC", "read_files", "package main", false)

	if len(events) != 2 {
		t.Fatalf("got %d events, want the call and its result", len(events))
	}
	for _, event := range events {
		if event.ToolID != "toolu_01ABC" {
			t.Errorf("%s stream event tool id = %q, want the provider's block id", event.Kind, event.ToolID)
		}
	}

	call := streamEventToBridge(events[0], "sess-1")
	if call.ToolCall == nil {
		t.Fatal("tool_call did not map to a bridge ToolCall event")
	}
	if call.ToolCall.ToolID != "toolu_01ABC" {
		t.Errorf("bridge tool_call tool id = %q, want the provider's block id", call.ToolCall.ToolID)
	}

	result := streamEventToBridge(events[1], "sess-1")
	if result.ToolResult == nil {
		t.Fatal("tool_result did not map to a bridge ToolResult event")
	}
	if result.ToolResult.ToolID != "toolu_01ABC" {
		t.Errorf("bridge tool_result tool id = %q, want the provider's block id", result.ToolResult.ToolID)
	}
}

// TestToolIDPairsTwoCallsToTheSameToolInOneTurn is the case the id exists for.
// A consumer pairing a result to its call by tool NAME cannot tell these two
// apart — bridge-ui's fallback picks the last unfilled call of that name — so
// the second result would be attributed to the first read. The id is the only
// field that separates them.
func TestToolIDPairsTwoCallsToTheSameToolInOneTurn(t *testing.T) {
	calls := []StreamEvent{
		{Kind: "tool_call", ToolID: "toolu_01FIRST", Tool: "read_files", Text: `{"path":"a.go"}`},
		{Kind: "tool_call", ToolID: "toolu_02SECOND", Tool: "read_files", Text: `{"path":"b.go"}`},
	}
	results := []StreamEvent{
		{Kind: "tool_result", ToolID: "toolu_02SECOND", Tool: "read_files", Text: "contents of b"},
		{Kind: "tool_result", ToolID: "toolu_01FIRST", Tool: "read_files", Text: "contents of a"},
	}

	// Results deliberately arrive in the opposite order to the calls, which is
	// what makes name-matching wrong rather than merely fragile.
	pairedInput := map[string]string{}
	for _, call := range calls {
		bridged := streamEventToBridge(call, "sess-1")
		pairedInput[bridged.ToolCall.ToolID] = string(bridged.ToolCall.Input)
	}
	for _, result := range results {
		bridged := streamEventToBridge(result, "sess-1")
		input, ok := pairedInput[bridged.ToolResult.ToolID]
		if !ok {
			t.Fatalf("result %q matched no call — ids are not reaching the wire", bridged.ToolResult.ToolID)
		}
		switch bridged.ToolResult.ToolID {
		case "toolu_01FIRST":
			if input != `{"path":"a.go"}` {
				t.Errorf("first call paired with input %q, want a.go", input)
			}
		case "toolu_02SECOND":
			if input != `{"path":"b.go"}` {
				t.Errorf("second call paired with input %q, want b.go", input)
			}
		}
	}

	if len(pairedInput) != 2 {
		t.Fatalf("two calls to one tool collapsed to %d id(s): %v", len(pairedInput), pairedInput)
	}
}

// Guard against the field being dropped from the protocol type rather than from
// inber: msg.ToolCallEvent.ToolID has no omitempty, so an unset id ships as
// "tool_id":"" and reads downstream as "this harness has no ids" instead of as
// a bug.
func TestBridgeToolEventTypesStillCarryAnID(t *testing.T) {
	if (msg.ToolCallEvent{ToolID: "x"}).ToolID != "x" {
		t.Error("msg.ToolCallEvent no longer carries ToolID")
	}
	if (msg.ToolResultEvent{ToolID: "x"}).ToolID != "x" {
		t.Error("msg.ToolResultEvent no longer carries ToolID")
	}
}
