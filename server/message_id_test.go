package server

import "testing"

// TestStreamEventCarriesMessageIDOntoTheBridgeWire covers the reason the id is
// collected at all: a consumer of the bridge stream joins deltas, tool calls
// and tool results back onto the assistant message they came from, and can only
// do that if each event names it.
func TestStreamEventCarriesMessageIDOntoTheBridgeWire(t *testing.T) {
	const messageID = "msg_01ABC"

	deltaEvent := streamEventToBridge(StreamEvent{Kind: "delta", Text: "hello", MessageID: messageID}, "sess")
	if deltaEvent.Stream == nil {
		t.Fatal("a delta must map to a stream event — the test's premise is wrong")
	}
	if deltaEvent.Stream.MessageID != messageID {
		t.Errorf("stream event message id = %q, want %q", deltaEvent.Stream.MessageID, messageID)
	}

	callEvent := streamEventToBridge(StreamEvent{Kind: "tool_call", Tool: "read_files", Text: "{}", MessageID: messageID}, "sess")
	if callEvent.ToolCall == nil {
		t.Fatal("a tool_call must map to a tool call event — the test's premise is wrong")
	}
	if callEvent.ToolCall.MessageID != messageID {
		t.Errorf("tool call message id = %q, want %q", callEvent.ToolCall.MessageID, messageID)
	}

	resultEvent := streamEventToBridge(StreamEvent{Kind: "tool_result", Tool: "read_files", Text: "ok", MessageID: messageID}, "sess")
	if resultEvent.ToolResult == nil {
		t.Fatal("a tool_result must map to a tool result event — the test's premise is wrong")
	}
	if resultEvent.ToolResult.MessageID != messageID {
		t.Errorf("tool result message id = %q, want %q", resultEvent.ToolResult.MessageID, messageID)
	}
}

// TestUnnamedStreamEventLeavesTheBridgeFieldEmpty keeps the absent case
// distinguishable: a provider that names no message must leave the field off
// the wire rather than putting an empty string on it.
func TestUnnamedStreamEventLeavesTheBridgeFieldEmpty(t *testing.T) {
	event := streamEventToBridge(StreamEvent{Kind: "delta", Text: "hello"}, "sess")
	if event.Stream == nil {
		t.Fatal("a delta must map to a stream event — the test's premise is wrong")
	}
	if event.Stream.MessageID != "" {
		t.Errorf("stream event message id = %q, want empty", event.Stream.MessageID)
	}
}
