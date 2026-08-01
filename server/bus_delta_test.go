package server

import "testing"

// TestBusDeltaCarriesTheMessageIDOfEveryPublishedKind is the curative test for
// the one line of the message-id work that shipped uncovered: server/bus.go
// copying StreamEvent.MessageID onto the delta it publishes to chat.stream.
// A consumer joins a run of deltas onto one assistant bubble by that id, so an
// event that loses it is an event no replay can place.
func TestBusDeltaCarriesTheMessageIDOfEveryPublishedKind(t *testing.T) {
	const messageID = "msg_01XYZ"

	for _, ev := range []StreamEvent{
		{Kind: "delta", Text: "hello", MessageID: messageID},
		{Kind: "thinking", Text: "hmm", MessageID: messageID},
		{Kind: "tool_call", Tool: "read_files", Text: "{}", MessageID: messageID},
		{Kind: "tool_result", Tool: "read_files", Text: "ok", MessageID: messageID},
	} {
		delta, ok := busDeltaFor("claxon", "main", ev)
		if !ok {
			t.Fatalf("%s event was not published to the bus at all", ev.Kind)
		}
		if delta.MessageID != messageID {
			t.Errorf("%s delta message id = %q, want %q", ev.Kind, delta.MessageID, messageID)
		}
	}
}

// TestBusDeltaLeavesAnUnnamedMessageUnnamed pins the other half: the id is
// copied, not invented. A provider that has not yet named the message must
// reach the bus with an empty field, so a consumer can tell "not named yet"
// from "belongs to some other bubble".
func TestBusDeltaLeavesAnUnnamedMessageUnnamed(t *testing.T) {
	delta, ok := busDeltaFor("claxon", "main", StreamEvent{Kind: "delta", Text: "hello"})
	if !ok {
		t.Fatal("delta event was not published to the bus at all")
	}
	if delta.MessageID != "" {
		t.Errorf("delta message id = %q, want empty", delta.MessageID)
	}
}

// TestBusDeltaKeepsDoneAndUnknownKindsOffTheStream covers the second return
// value. "done" is published by the caller once Stream returns, so publishing
// it here would deliver the end of a turn twice; an unrecognised kind carries
// nothing a consumer can render.
func TestBusDeltaKeepsDoneAndUnknownKindsOffTheStream(t *testing.T) {
	for _, kind := range []string{"done", "status", "wat"} {
		if _, ok := busDeltaFor("claxon", "main", StreamEvent{Kind: kind, Text: "x"}); ok {
			t.Errorf("%s event was published to chat.stream, want dropped", kind)
		}
	}
}

// TestBusDeltaMapsEachKindOntoItsOwnFields keeps the id assertions above
// honest: without it they are satisfied by a function that returns a delta
// carrying an id and nothing else.
func TestBusDeltaMapsEachKindOntoItsOwnFields(t *testing.T) {
	tests := []struct {
		event      StreamEvent
		wantType   string
		wantText   string
		wantTool   string
		wantInput  string
		wantOutput string
	}{
		{
			event:    StreamEvent{Kind: "delta", Text: "hello"},
			wantType: "text",
			wantText: "hello",
		},
		{
			event:    StreamEvent{Kind: "thinking", Text: "hmm"},
			wantType: "thinking",
			wantText: "hmm",
		},
		{
			event:     StreamEvent{Kind: "tool_call", Tool: "read_files", Text: `{"path":"x"}`},
			wantType:  "tool",
			wantTool:  "read_files",
			wantInput: `{"path":"x"}`,
		},
		{
			event:      StreamEvent{Kind: "tool_result", Tool: "read_files", Text: "ok"},
			wantType:   "tool_result",
			wantTool:   "read_files",
			wantOutput: "ok",
		},
	}

	for _, tt := range tests {
		delta, ok := busDeltaFor("claxon", "main", tt.event)
		if !ok {
			t.Fatalf("%s event was not published to the bus at all", tt.event.Kind)
		}
		if delta.Type != tt.wantType {
			t.Errorf("%s delta type = %q, want %q", tt.event.Kind, delta.Type, tt.wantType)
		}
		if delta.Text != tt.wantText {
			t.Errorf("%s delta text = %q, want %q", tt.event.Kind, delta.Text, tt.wantText)
		}
		if delta.Tool != tt.wantTool {
			t.Errorf("%s delta tool = %q, want %q", tt.event.Kind, delta.Tool, tt.wantTool)
		}
		if delta.ToolInput != tt.wantInput {
			t.Errorf("%s delta tool input = %q, want %q", tt.event.Kind, delta.ToolInput, tt.wantInput)
		}
		if delta.ToolOutput != tt.wantOutput {
			t.Errorf("%s delta tool output = %q, want %q", tt.event.Kind, delta.ToolOutput, tt.wantOutput)
		}
		if delta.Agent != "claxon" || delta.Orchestrator != "inber" || delta.SessionID != "main" {
			t.Errorf("%s delta addressed to %q/%q/%q, want claxon/inber/main",
				tt.event.Kind, delta.Agent, delta.Orchestrator, delta.SessionID)
		}
	}
}
