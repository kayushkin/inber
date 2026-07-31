package server

import "testing"

// A sub-agent outlives the turn that spawned it. Spawn is asynchronous — the
// tool returns a SpawnResponse immediately and the work runs on the queue — and
// withoutCallerCancellation deliberately drops the caller's cancellation so that
// a browser tab closing cannot abort it. So by the time a child emits anything,
// the parent's request is routinely over and the parent has moved on to a writer
// belonging to a later turn, or to no writer at all.
//
// Session.onEvent is documented as "current request's event callback (updated
// per-turn)", and getOrCreateSession calls setOnEvent on every single run. These
// tests pin that the forwarder honours that: it must resolve the parent's writer
// when it emits, not keep the one that happened to be installed at spawn time.

// TestForwarderFollowsTheParentToItsCurrentWriter is the defect. Snapshotting
// the parent's callback at spawn time sends a sub-agent's whole event stream to
// the writer of the turn it was spawned in — a writer that has since been
// replaced and, on the HTTP path, belongs to a request that has returned.
func TestForwarderFollowsTheParentToItsCurrentWriter(t *testing.T) {
	parent := &Session{}

	spawningTurnStream, spawningTurnGot := collectEvents()
	parent.setOnEvent(spawningTurnStream)

	// The child is wired up here, during the turn that spawned it.
	child := newSubagentEventForwarder(parent.getOnEvent, "brigid", "child-key", "parent-key", 1)

	// That turn ends and the next one installs its own writer.
	laterTurnStream, laterTurnGot := collectEvents()
	parent.setOnEvent(laterTurnStream)

	// Only now does the child report anything — the ordinary case, not a corner one.
	child(StreamEvent{Kind: "status", Text: "still working", Turn: 9})

	if len(*spawningTurnGot) != 0 {
		t.Errorf("wrote to the spawning turn's writer, which is gone: %+v", *spawningTurnGot)
	}
	if len(*laterTurnGot) != 1 {
		t.Fatalf("the parent's current writer got %d events, want 1: %+v",
			len(*laterTurnGot), *laterTurnGot)
	}
	if ev := (*laterTurnGot)[0]; ev.Kind != eventKindAgentUpdate || ev.Text != "still working" {
		t.Errorf("event = %+v, want the child's progress as an agent_update", ev)
	}
}

// A descendant's envelope takes the same path, so it must follow the parent too.
// A grandchild is an ordinary case: MaxSpawnDepth defaults to 2.
func TestForwarderFollowsTheParentForADescendantsEnvelope(t *testing.T) {
	parent := &Session{}
	spawningTurnStream, spawningTurnGot := collectEvents()
	parent.setOnEvent(spawningTurnStream)

	child := newSubagentEventForwarder(parent.getOnEvent, "brigid", "child-key", "parent-key", 1)
	grandchild := newSubagentEventForwarder(alwaysStream(child), "lugh", "grandchild-key", "child-key", 2)

	laterTurnStream, laterTurnGot := collectEvents()
	parent.setOnEvent(laterTurnStream)

	grandchild(StreamEvent{Kind: "status", Text: "running the tests", Turn: 11})

	if len(*spawningTurnGot) != 0 {
		t.Errorf("wrote to the spawning turn's writer, which is gone: %+v", *spawningTurnGot)
	}
	if len(*laterTurnGot) != 1 {
		t.Fatalf("the parent's current writer got %d events, want 1: %+v",
			len(*laterTurnGot), *laterTurnGot)
	}
	// Passing the envelope through unchanged still has to hold on the new path.
	data := provenance(t, (*laterTurnGot)[0])
	if data["session_key"] != "grandchild-key" || data["agent"] != "lugh" || data["depth"] != 2 {
		t.Errorf("provenance = %+v, want the grandchild's", data)
	}
}

// A parent between turns has no writer, and Session.getOnEvent says so by
// returning nil. Forwarding into a stale callback instead is how an event
// reaches a request that has already returned — which for the HTTP path
// (api_run.go closes the sink over the http.ResponseWriter and its Flusher) is a
// write to a ResponseWriter net/http says may not be used once its handler has
// returned. Dropping is the only thing that can be delivered to nobody.
func TestForwarderDropsWhileTheParentHasNoWriter(t *testing.T) {
	parent := &Session{}
	finishedRequestStream, finishedRequestGot := collectEvents()
	parent.setOnEvent(finishedRequestStream)

	child := newSubagentEventForwarder(parent.getOnEvent, "brigid", "child-key", "parent-key", 1)

	// The request ends and nothing has replaced it.
	parent.setOnEvent(nil)

	child(StreamEvent{Kind: "status", Text: "still working"})
	child(StreamEvent{Kind: eventKindAgentDone, Text: "done", Data: map[string]any{"agent": "lugh"}})

	if len(*finishedRequestGot) != 0 {
		t.Errorf("wrote %d events into a finished request's writer: %+v",
			len(*finishedRequestGot), *finishedRequestGot)
	}
}

// A parent that had no writer when it spawned a child, and acquires one later,
// must still be able to see that child. The old wiring decided this once, at
// spawn time, and a child wired up in a silent moment stayed invisible for its
// whole life.
func TestForwarderReachesAWriterInstalledAfterTheSpawn(t *testing.T) {
	parent := &Session{}

	child := newSubagentEventForwarder(parent.getOnEvent, "brigid", "child-key", "parent-key", 1)

	laterTurnStream, laterTurnGot := collectEvents()
	parent.setOnEvent(laterTurnStream)

	child(StreamEvent{Kind: "status", Text: "still working"})

	if len(*laterTurnGot) != 1 {
		t.Fatalf("the parent's writer got %d events, want 1: %+v", len(*laterTurnGot), *laterTurnGot)
	}
}
