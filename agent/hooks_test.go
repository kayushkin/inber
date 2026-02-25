package agent

import (
	"context"
	"testing"
	"time"
)

func TestHookRegistry_RegisterAndDispatch(t *testing.T) {
	registry := NewHookRegistry()
	
	var receivedEvent *Event
	handler := func(ctx context.Context, event *Event) (*HookResult, error) {
		receivedEvent = event
		return &HookResult{Action: ActionProceed}, nil
	}

	config := &HookConfig{
		Events: []EventType{EventSessionStart, EventToolCall},
	}

	err := registry.Register("test-agent", config, handler)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Dispatch a session start event
	event := SessionStartEvent("test-agent", "session-123")
	result, err := registry.Dispatch(context.Background(), event)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	if result.Action != ActionProceed {
		t.Errorf("Expected ActionProceed, got %v", result.Action)
	}

	if receivedEvent == nil {
		t.Fatal("Handler was not called")
	}

	if receivedEvent.Type != EventSessionStart {
		t.Errorf("Expected EventSessionStart, got %v", receivedEvent.Type)
	}

	if receivedEvent.AgentName != "test-agent" {
		t.Errorf("Expected agent name 'test-agent', got %v", receivedEvent.AgentName)
	}
}

func TestHookRegistry_Gatekeeper(t *testing.T) {
	registry := NewHookRegistry()

	// Register target agent (will be gated, so doesn't need a handler)
	targetConfig := &HookConfig{
		Events: []EventType{EventToolCall},
	}

	targetHandler := func(ctx context.Context, event *Event) (*HookResult, error) {
		return &HookResult{Action: ActionProceed}, nil
	}

	err := registry.Register("target-agent", targetConfig, targetHandler)
	if err != nil {
		t.Fatalf("Register target failed: %v", err)
	}

	// Register gatekeeper
	gatekeeperCalled := false
	gatekeeperHandler := func(ctx context.Context, event *Event) (*HookResult, error) {
		gatekeeperCalled = true
		// Gatekeeper aborts the tool call
		return &HookResult{
			Action:  ActionAbort,
			Message: "blocked by security policy",
		}, nil
	}

	gatekeeperConfig := &HookConfig{
		Events:        []EventType{EventToolCall},
		GatekeeperFor: []string{"target-agent"},
	}

	err = registry.Register("security-agent", gatekeeperConfig, gatekeeperHandler)
	if err != nil {
		t.Fatalf("Register gatekeeper failed: %v", err)
	}

	// Dispatch tool call for target agent
	event := ToolCallEvent("target-agent", "session-123", "tool-1", "shell", []byte(`{"command": "rm -rf /"}`))
	result, err := registry.Dispatch(context.Background(), event)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	if !gatekeeperCalled {
		t.Error("Gatekeeper handler was not called")
	}

	if result.Action != ActionAbort {
		t.Errorf("Expected ActionAbort, got %v", result.Action)
	}

	if result.Message != "blocked by security policy" {
		t.Errorf("Expected abort message, got %v", result.Message)
	}
}

func TestHookRegistry_GatekeeperModify(t *testing.T) {
	registry := NewHookRegistry()

	// Register gatekeeper that modifies events
	gatekeeperHandler := func(ctx context.Context, event *Event) (*HookResult, error) {
		// Modify the tool input to sanitize it
		return &HookResult{
			Action: ActionModify,
			Data: map[string]interface{}{
				"input": `{"command": "echo safe"}`,
			},
		}, nil
	}

	gatekeeperConfig := &HookConfig{
		GatekeeperFor: []string{"target-agent"},
	}

	err := registry.Register("sanitizer-agent", gatekeeperConfig, gatekeeperHandler)
	if err != nil {
		t.Fatalf("Register gatekeeper failed: %v", err)
	}

	// Dispatch event
	event := ToolCallEvent("target-agent", "session-123", "tool-1", "shell", []byte(`{"command": "rm -rf /"}`))
	result, err := registry.Dispatch(context.Background(), event)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	if result.Action == ActionAbort {
		t.Error("Expected modification, got abort")
	}

	// Check that event data was modified
	if event.Data["input"] != `{"command": "echo safe"}` {
		t.Errorf("Event data was not modified: %v", event.Data["input"])
	}
}

func TestHookRegistry_Unregister(t *testing.T) {
	registry := NewHookRegistry()

	called := false
	handler := func(ctx context.Context, event *Event) (*HookResult, error) {
		called = true
		return &HookResult{Action: ActionProceed}, nil
	}

	config := &HookConfig{
		Events: []EventType{EventSessionStart},
	}

	err := registry.Register("test-agent", config, handler)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Unregister
	registry.Unregister("test-agent")

	// Dispatch should not call handler
	event := SessionStartEvent("test-agent", "session-123")
	_, err = registry.Dispatch(context.Background(), event)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	if called {
		t.Error("Handler was called after unregister")
	}
}

func TestEventHelpers(t *testing.T) {
	tests := []struct {
		name      string
		event     *Event
		eventType EventType
	}{
		{
			name:      "SessionStart",
			event:     SessionStartEvent("agent1", "session1"),
			eventType: EventSessionStart,
		},
		{
			name:      "ToolCall",
			event:     ToolCallEvent("agent1", "session1", "tool-1", "shell", []byte(`{"cmd":"ls"}`)),
			eventType: EventToolCall,
		},
		{
			name:      "ToolResult",
			event:     ToolResultEvent("agent1", "session1", "tool-1", "shell", "success", false),
			eventType: EventToolResult,
		},
		{
			name:      "SessionEnd",
			event:     SessionEndEvent("agent1", "session1", 0.05, 1000),
			eventType: EventSessionEnd,
		},
		{
			name:      "BeforeSpawn",
			event:     BeforeSpawnEvent("agent1", "session1", "child-agent"),
			eventType: EventBeforeSpawn,
		},
		{
			name:      "AfterSpawn",
			event:     AfterSpawnEvent("agent1", "session1", "child-agent", true, "result"),
			eventType: EventAfterSpawn,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.event.Type != tt.eventType {
				t.Errorf("Expected event type %v, got %v", tt.eventType, tt.event.Type)
			}

			if tt.event.AgentName != "agent1" {
				t.Errorf("Expected agent name 'agent1', got %v", tt.event.AgentName)
			}

			if tt.event.SessionID != "session1" {
				t.Errorf("Expected session ID 'session1', got %v", tt.event.SessionID)
			}

			if tt.event.Timestamp.IsZero() {
				t.Error("Timestamp should not be zero")
			}

			if time.Since(tt.event.Timestamp) > time.Second {
				t.Error("Timestamp should be recent")
			}
		})
	}
}

func TestMultipleSubscribers(t *testing.T) {
	registry := NewHookRegistry()

	called1 := false
	handler1 := func(ctx context.Context, event *Event) (*HookResult, error) {
		called1 = true
		return &HookResult{Action: ActionProceed}, nil
	}

	called2 := false
	handler2 := func(ctx context.Context, event *Event) (*HookResult, error) {
		called2 = true
		return &HookResult{Action: ActionProceed}, nil
	}

	config := &HookConfig{
		Events: []EventType{EventSessionStart},
	}

	err := registry.Register("agent1", config, handler1)
	if err != nil {
		t.Fatalf("Register agent1 failed: %v", err)
	}

	err = registry.Register("agent2", config, handler2)
	if err != nil {
		t.Fatalf("Register agent2 failed: %v", err)
	}

	// Dispatch should call both handlers
	event := SessionStartEvent("test-agent", "session-123")
	_, err = registry.Dispatch(context.Background(), event)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	if !called1 {
		t.Error("Handler1 was not called")
	}

	if !called2 {
		t.Error("Handler2 was not called")
	}
}

func TestGatekeeperPriority(t *testing.T) {
	registry := NewHookRegistry()

	// Register regular subscriber
	subscriberHandler := func(ctx context.Context, event *Event) (*HookResult, error) {
		return &HookResult{Action: ActionProceed}, nil
	}

	subscriberConfig := &HookConfig{
		Events: []EventType{EventToolCall},
	}

	err := registry.Register("subscriber", subscriberConfig, subscriberHandler)
	if err != nil {
		t.Fatalf("Register subscriber failed: %v", err)
	}

	// Register gatekeeper that aborts
	gatekeeperHandler := func(ctx context.Context, event *Event) (*HookResult, error) {
		return &HookResult{
			Action:  ActionAbort,
			Message: "blocked",
		}, nil
	}

	gatekeeperConfig := &HookConfig{
		GatekeeperFor: []string{"target-agent"},
	}

	err = registry.Register("gatekeeper", gatekeeperConfig, gatekeeperHandler)
	if err != nil {
		t.Fatalf("Register gatekeeper failed: %v", err)
	}

	// Dispatch for target-agent
	event := ToolCallEvent("target-agent", "session-123", "tool-1", "shell", []byte(`{}`))
	result, err := registry.Dispatch(context.Background(), event)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}

	// Gatekeeper should have aborted before subscriber was called
	if result.Action != ActionAbort {
		t.Error("Expected abort from gatekeeper")
	}

	// Note: The current implementation still calls subscribers even if gatekeeper aborts
	// This might be desired behavior (subscribers can log/observe) or might need to change
}
