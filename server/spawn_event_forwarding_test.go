package server

import "testing"

// collectEvents returns a sink and the slice it appends to, so a test can read
// what a forwarder chain actually put on the top-level stream.
func collectEvents() (func(StreamEvent), *[]StreamEvent) {
	var got []StreamEvent
	return func(ev StreamEvent) { got = append(got, ev) }, &got
}

// alwaysStream adapts a fixed sink to the resolver the forwarder takes. The
// tests below are about *what* a forwarder forwards; where the parent's writer
// is at the moment it forwards is spawn_event_writer_test.go's subject.
func alwaysStream(onEvent func(StreamEvent)) func() func(StreamEvent) {
	return func() func(StreamEvent) { return onEvent }
}

func provenance(t *testing.T, ev StreamEvent) map[string]any {
	t.Helper()
	data, ok := ev.Data.(map[string]any)
	if !ok {
		t.Fatalf("event %q carries no provenance map, got %T", ev.Kind, ev.Data)
	}
	return data
}

// TestForwarderWrapsChildProgressForTheParentStream pins the behaviour the
// forwarder existed for: a sub-agent's own progress reaches the parent's stream
// as an agent_update that says which session produced it.
func TestForwarderWrapsChildProgressForTheParentStream(t *testing.T) {
	parentStream, got := collectEvents()
	child := newSubagentEventForwarder(alwaysStream(parentStream), "brigid", "child-key", "parent-key", 1)

	child(StreamEvent{Kind: "status", Text: "reading the repo", Turn: 3})
	child(StreamEvent{Kind: "thinking", Text: "weighing two options", Turn: 4})

	if len(*got) != 2 {
		t.Fatalf("parent stream got %d events, want 2: %+v", len(*got), *got)
	}
	for _, ev := range *got {
		if ev.Kind != eventKindAgentUpdate {
			t.Errorf("kind = %q, want %q", ev.Kind, eventKindAgentUpdate)
		}
		data := provenance(t, ev)
		if data["session_key"] != "child-key" || data["parent_key"] != "parent-key" ||
			data["agent"] != "brigid" || data["depth"] != 1 {
			t.Errorf("provenance = %+v, want the child's", data)
		}
	}
	if (*got)[0].Text != "reading the repo" || (*got)[0].Turn != 3 {
		t.Errorf("first event = %+v, want the status text and turn unchanged", (*got)[0])
	}
	if (*got)[1].Text != "weighing two options" || (*got)[1].Turn != 4 {
		t.Errorf("second event = %+v, want the thinking text and turn unchanged", (*got)[1])
	}
}

// TestForwarderPassesAGrandchildsProgressThrough is the defect this test file
// was written for. A grandchild's sink is the same forwarder, so its progress
// arrives at the child's forwarder already wrapped as agent_update. A forwarder
// that only matches status and thinking drops it — no log, no counter — and
// with MaxSpawnDepth defaulting to 2 that is the whole event stream of an
// ordinary sub-agent of a sub-agent.
func TestForwarderPassesAGrandchildsProgressThrough(t *testing.T) {
	topStream, got := collectEvents()
	child := newSubagentEventForwarder(alwaysStream(topStream), "brigid", "child-key", "top-key", 1)
	grandchild := newSubagentEventForwarder(alwaysStream(child), "lugh", "grandchild-key", "child-key", 2)

	grandchild(StreamEvent{Kind: "status", Text: "running the tests", Turn: 7})

	if len(*got) != 1 {
		t.Fatalf("top stream got %d events, want 1: %+v", len(*got), *got)
	}
	ev := (*got)[0]
	if ev.Kind != eventKindAgentUpdate {
		t.Fatalf("kind = %q, want %q", ev.Kind, eventKindAgentUpdate)
	}
	if ev.Text != "running the tests" || ev.Turn != 7 {
		t.Errorf("event = %+v, want the grandchild's text and turn", ev)
	}
	// The intermediate hop adds nothing a reader cannot already derive from
	// parent_key, so re-wrapping would only overwrite the true origin.
	data := provenance(t, ev)
	if data["session_key"] != "grandchild-key" || data["parent_key"] != "child-key" ||
		data["agent"] != "lugh" || data["depth"] != 2 {
		t.Errorf("provenance = %+v, want the grandchild's, not the child's", data)
	}
}

// TestForwarderPassesADescendantsSpawnAndDoneThrough covers the other two
// envelopes. Spawn fires them directly onto the parent's sink, so for a
// grandchild that sink is the child's forwarder — the same path the progress
// events take.
func TestForwarderPassesADescendantsSpawnAndDoneThrough(t *testing.T) {
	topStream, got := collectEvents()
	child := newSubagentEventForwarder(alwaysStream(topStream), "brigid", "child-key", "top-key", 1)

	spawned := StreamEvent{
		Kind: eventKindAgentSpawned,
		Text: "audit the tool registry",
		Data: map[string]any{"agent": "lugh", "session_key": "grandchild-key", "depth": 2},
	}
	done := StreamEvent{
		Kind: eventKindAgentDone,
		Text: "found two duplicate names",
		Data: map[string]any{"agent": "lugh", "session_key": "grandchild-key", "depth": 2, "status": "success"},
	}
	child(spawned)
	child(done)

	if len(*got) != 2 {
		t.Fatalf("top stream got %d events, want 2: %+v", len(*got), *got)
	}
	for i, want := range []StreamEvent{spawned, done} {
		if (*got)[i].Kind != want.Kind || (*got)[i].Text != want.Text {
			t.Errorf("event %d = %+v, want %+v unchanged", i, (*got)[i], want)
		}
		data := provenance(t, (*got)[i])
		if data["session_key"] != "grandchild-key" || data["agent"] != "lugh" || data["depth"] != 2 {
			t.Errorf("event %d provenance = %+v, want the grandchild's", i, data)
		}
	}
	if status := provenance(t, (*got)[1])["status"]; status != "success" {
		t.Errorf("agent_done lost its status field: got %v", status)
	}
}

// TestForwarderKeepsTheChildsTokenStreamOffTheParentStream pins the filtering
// that is deliberate, so that widening it is a decision someone makes on
// purpose rather than a side effect. A parent stream shows what its sub-agents
// are doing, not every token they emit.
func TestForwarderKeepsTheChildsTokenStreamOffTheParentStream(t *testing.T) {
	parentStream, got := collectEvents()
	child := newSubagentEventForwarder(alwaysStream(parentStream), "brigid", "child-key", "parent-key", 1)

	child(StreamEvent{Kind: "delta", Text: "half a sentence"})
	child(StreamEvent{Kind: "tool_call", Tool: "shell", Text: "ls"})
	child(StreamEvent{Kind: "tool_result", Tool: "shell", Text: "a directory listing"})
	child(StreamEvent{Kind: "done"})

	if len(*got) != 0 {
		t.Fatalf("parent stream got %d events, want none: %+v", len(*got), *got)
	}
}
