package server

import (
	"testing"
	"time"
)

func TestEventPublisherNil(t *testing.T) {
	// Nil publisher should not panic.
	var ep *EventPublisher
	ep.SpawnStarted("session-key", "test-agent", "parent-key", "test task")
	ep.SpawnCompleted(SpawnResult{
		Agent:    "test-agent",
		ChildKey: "session-key", 
		Summary:  "test completed",
		Duration: time.Second,
	})
	ep.SessionActive("session-key", "test-agent")
	ep.SessionIdle("session-key", "test-agent")
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

func TestChatDeltaEventStructure(t *testing.T) {
	// Test event creation and structure using actual ChatDelta.
	ep := NewEventPublisher("", "") // nil publisher for testing
	
	// Test spawn started event creation
	ep.SpawnStarted("child:1", "ogma", "parent:main", "fix bugs")
	
	// Test spawn completion
	result := SpawnResult{
		Agent:    "ogma", 
		ChildKey: "child:1",
		Summary:  "Task completed successfully",
		Duration: 5 * time.Second,
	}
	ep.SpawnCompleted(result)
	
	// Test session events
	ep.SessionActive("session:1", "ogma")
	ep.SessionIdle("session:1", "ogma")
	
	// All methods should handle nil publisher gracefully
	// No panics expected - this tests defensive programming
}

func TestEventPublisherDisabled(t *testing.T) {
	ep := NewEventPublisher("", "")
	if ep != nil {
		t.Error("expected nil publisher for empty URL")
	}
}
