package server

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEventPublisherNil(t *testing.T) {
	// Nil publisher should not panic.
	var ep *EventPublisher
	ep.SpawnStarted("key", "agent", "parent", "task")
	ep.SpawnCompleted(SpawnResult{})
	ep.SessionActive("key", "agent")
	ep.SessionIdle("key", "agent")
}

func TestEventPublisherCreation(t *testing.T) {
	// Test that publisher creates properly with valid URL.
	ep := NewEventPublisher("nats://localhost:4222", "test-token")
	if ep != nil {
		// If NATS is available, we get a publisher.
		// If not available, we get nil (which is expected for tests).
		ep.Close()
	}

	// Test that empty URL returns nil.
	ep = NewEventPublisher("", "")
	if ep != nil {
		t.Error("expected nil publisher for empty URL")
	}
}

func TestGatewayEventStructure(t *testing.T) {
	// Test event creation and structure.
	event := GatewayEvent{
		Kind:       "spawn_started",
		SessionKey: "child:1",
		Agent:      "ogma",
		ParentKey:  "parent:main",
		Task:       "fix bugs",
		Timestamp:  time.Now(),
	}

	if event.Kind != "spawn_started" {
		t.Errorf("expected kind=spawn_started, got %s", event.Kind)
	}
	if event.Agent != "ogma" {
		t.Errorf("expected agent=ogma, got %s", event.Agent)
	}
	if event.SessionKey != "child:1" {
		t.Errorf("expected session_key=child:1, got %s", event.SessionKey)
	}
	if event.ParentKey != "parent:main" {
		t.Errorf("expected parent_key=parent:main, got %s", event.ParentKey)
	}
	if event.Task != "fix bugs" {
		t.Errorf("expected task='fix bugs', got %s", event.Task)
	}

	// Test JSON marshaling.
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}

	var decoded GatewayEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal event: %v", err)
	}

	if decoded.Kind != event.Kind {
		t.Errorf("JSON round-trip failed for kind: %s != %s", decoded.Kind, event.Kind)
	}
}

func TestEventPublisherDisabled(t *testing.T) {
	ep := NewEventPublisher("", "")
	if ep != nil {
		t.Error("expected nil publisher for empty URL")
	}
}
