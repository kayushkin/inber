package server

import "github.com/kayushkin/bus/messages"

// busDeltaFor builds the bus delta that carries one stream event to
// chat.stream. The second return value reports whether the event belongs on
// the bus at all: "done" is published by the caller once the run returns, and
// an unrecognised kind carries nothing a consumer can render.
//
// This is a seam and it exists for one line. StreamEvent.MessageID names the
// assistant message an event came from, and that is how a consumer joins a run
// of deltas onto a single bubble. The copy used to live inside a closure that
// also held a live *bus.Client, so no test could reach it. It lives here now.
//
// The other seam — turning Server.bus into an interface so a fake could record
// what was published — was rejected. bus.NewClient returns a nil *bus.Client
// when NATS is absent, and both ListenBus and the spawn-delivery path branch on
// `bus == nil`. Stored in an interface field that nil is no longer nil, so
// every one of those guards would silently start taking the wrong branch.
func busDeltaFor(agent, sessionID string, ev StreamEvent) (messages.ChatDelta, bool) {
	// NewChatDelta panics on an empty type, and this function is called from
	// inside a streaming callback, so the panic would surface as a failed turn.
	// Kept rather than guarded: every producer of a StreamEvent names its kind,
	// and a silent drop here would hide the day one stops.
	delta := messages.NewChatDelta(agent, "inber", sessionID, ev.Kind)
	delta.MessageID = ev.MessageID

	switch ev.Kind {
	case "delta":
		delta.Type = "text"
		delta.Text = ev.Text
	case "thinking":
		delta.Text = ev.Text
	case "tool_call":
		delta.Type = "tool"
		delta.Tool = ev.Tool
		delta.ToolInput = ev.Text
	case "tool_result":
		delta.Tool = ev.Tool
		delta.ToolOutput = ev.Text
	default:
		return messages.ChatDelta{}, false
	}

	return delta, true
}
