// Package agent - lifecycle hooks and event system
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
)

// EventType identifies the lifecycle hook point
type EventType string

const (
	EventSessionStart  EventType = "on_session_start"
	EventBeforeRequest EventType = "on_before_request"
	EventAfterResponse EventType = "on_after_response"
	EventToolCall      EventType = "on_tool_call"
	EventToolResult    EventType = "on_tool_result"
	EventBeforeSpawn   EventType = "on_before_spawn"
	EventAfterSpawn    EventType = "on_after_spawn"
	EventSessionEnd    EventType = "on_session_end"
)

// Action defines what the hook handler wants to do
type Action string

const (
	ActionProceed Action = "proceed" // continue normally
	ActionAbort   Action = "abort"   // stop the operation
	ActionModify  Action = "modify"  // modify the event data and proceed
)

// Event is the base event data passed to all hooks
type Event struct {
	Type      EventType              `json:"type"`
	Timestamp time.Time              `json:"timestamp"`
	AgentName string                 `json:"agent_name"`
	SessionID string                 `json:"session_id"`
	Data      map[string]interface{} `json:"data"`
}

// HookResult is returned by hook handlers
type HookResult struct {
	Action  Action                 `json:"action"`
	Message string                 `json:"message,omitempty"` // optional explanation
	Data    map[string]interface{} `json:"data,omitempty"`    // modified data (for ActionModify)
}

// HookHandler processes an event and returns an action
type HookHandler func(ctx context.Context, event *Event) (*HookResult, error)

// HookConfig defines how an agent subscribes to hooks
type HookConfig struct {
	Events       []EventType `json:"hooks"`         // events this agent subscribes to
	GatekeeperFor []string    `json:"gatekeeper_for"` // agents this agent gates for
	Schedule     *Schedule   `json:"schedule,omitempty"`
}

// Schedule defines timed/cron-like triggers
type Schedule struct {
	Interval string `json:"interval,omitempty"` // e.g., "30s", "5m", "1h"
	Cron     string `json:"cron,omitempty"`     // cron expression (not implemented yet)
}

// HookRegistry manages hook subscriptions and dispatches events
type HookRegistry struct {
	mu           sync.RWMutex
	subscriptions map[EventType][]subscription
	gatekeepers  map[string][]string // agent -> list of gatekeeper agents
	handlers     map[string]HookHandler
}

type subscription struct {
	agentName string
	handler   HookHandler
}

// NewHookRegistry creates a new hook registry
func NewHookRegistry() *HookRegistry {
	return &HookRegistry{
		subscriptions: make(map[EventType][]subscription),
		gatekeepers:  make(map[string][]string),
		handlers:     make(map[string]HookHandler),
	}
}

// Register subscribes an agent to specific events
func (hr *HookRegistry) Register(agentName string, config *HookConfig, handler HookHandler) error {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	if handler == nil {
		return fmt.Errorf("handler cannot be nil")
	}

	hr.handlers[agentName] = handler

	// Subscribe to events
	for _, eventType := range config.Events {
		hr.subscriptions[eventType] = append(hr.subscriptions[eventType], subscription{
			agentName: agentName,
			handler:   handler,
		})
	}

	// Register as gatekeeper
	for _, targetAgent := range config.GatekeeperFor {
		hr.gatekeepers[targetAgent] = append(hr.gatekeepers[targetAgent], agentName)
	}

	return nil
}

// Unregister removes an agent from all subscriptions
func (hr *HookRegistry) Unregister(agentName string) {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	// Remove from subscriptions
	for eventType := range hr.subscriptions {
		subs := hr.subscriptions[eventType]
		filtered := subs[:0]
		for _, sub := range subs {
			if sub.agentName != agentName {
				filtered = append(filtered, sub)
			}
		}
		hr.subscriptions[eventType] = filtered
	}

	// Remove from gatekeepers
	for targetAgent := range hr.gatekeepers {
		gates := hr.gatekeepers[targetAgent]
		filtered := gates[:0]
		for _, gate := range gates {
			if gate != agentName {
				filtered = append(filtered, gate)
			}
		}
		if len(filtered) > 0 {
			hr.gatekeepers[targetAgent] = filtered
		} else {
			delete(hr.gatekeepers, targetAgent)
		}
	}

	delete(hr.handlers, agentName)
}

// Dispatch sends an event to all subscribers and gatekeepers
// Returns the final action after consulting all handlers
func (hr *HookRegistry) Dispatch(ctx context.Context, event *Event) (*HookResult, error) {
	hr.mu.RLock()
	
	// Get gatekeepers for this agent (if any)
	gatekeepers := hr.gatekeepers[event.AgentName]
	
	// Get direct subscribers
	subs := hr.subscriptions[event.Type]
	
	hr.mu.RUnlock()

	// First, check gatekeepers - they have veto power
	for _, gatekeeperName := range gatekeepers {
		hr.mu.RLock()
		handler := hr.handlers[gatekeeperName]
		hr.mu.RUnlock()

		if handler == nil {
			continue
		}

		// Create a copy of the event with gatekeeper context
		gateEvent := &Event{
			Type:      event.Type,
			Timestamp: event.Timestamp,
			AgentName: event.AgentName,
			SessionID: event.SessionID,
			Data:      make(map[string]interface{}),
		}
		for k, v := range event.Data {
			gateEvent.Data[k] = v
		}
		gateEvent.Data["_gatekeeper"] = gatekeeperName
		gateEvent.Data["_target_agent"] = event.AgentName

		result, err := handler(ctx, gateEvent)
		if err != nil {
			return nil, fmt.Errorf("gatekeeper %s error: %w", gatekeeperName, err)
		}

		// Gatekeepers can abort or modify
		if result.Action == ActionAbort {
			return result, nil
		}

		// If gatekeeper modified, update event data
		if result.Action == ActionModify && result.Data != nil {
			for k, v := range result.Data {
				event.Data[k] = v
			}
		}
	}

	// Then, run regular subscribers
	for _, sub := range subs {
		result, err := sub.handler(ctx, event)
		if err != nil {
			// Log error but don't fail the entire dispatch
			// In production, you'd want proper error handling here
			continue
		}

		// Subscribers can't abort (only gatekeepers can), but they can modify
		if result.Action == ActionModify && result.Data != nil {
			for k, v := range result.Data {
				event.Data[k] = v
			}
		}
	}

	// Default: proceed
	return &HookResult{Action: ActionProceed}, nil
}

// NewEvent creates a new event with the given type and data
func NewEvent(eventType EventType, agentName, sessionID string, data map[string]interface{}) *Event {
	return &Event{
		Type:      eventType,
		Timestamp: time.Now(),
		AgentName: agentName,
		SessionID: sessionID,
		Data:      data,
	}
}

// Helper functions to create specific event types

// SessionStartEvent creates an on_session_start event
func SessionStartEvent(agentName, sessionID string) *Event {
	return NewEvent(EventSessionStart, agentName, sessionID, map[string]interface{}{})
}

// BeforeRequestEvent creates an on_before_request event
func BeforeRequestEvent(agentName, sessionID string, params *anthropic.MessageNewParams) *Event {
	paramsJSON, _ := json.Marshal(params)
	return NewEvent(EventBeforeRequest, agentName, sessionID, map[string]interface{}{
		"model":       params.Model,
		"params_json": string(paramsJSON),
	})
}

// AfterResponseEvent creates an on_after_response event
func AfterResponseEvent(agentName, sessionID string, response *anthropic.Message) *Event {
	return NewEvent(EventAfterResponse, agentName, sessionID, map[string]interface{}{
		"stop_reason":   response.StopReason,
		"input_tokens":  response.Usage.InputTokens,
		"output_tokens": response.Usage.OutputTokens,
	})
}

// ToolCallEvent creates an on_tool_call event
func ToolCallEvent(agentName, sessionID, toolID, toolName string, input []byte) *Event {
	return NewEvent(EventToolCall, agentName, sessionID, map[string]interface{}{
		"tool_id":   toolID,
		"tool_name": toolName,
		"input":     string(input),
	})
}

// ToolResultEvent creates an on_tool_result event
func ToolResultEvent(agentName, sessionID, toolID, toolName, output string, isError bool) *Event {
	return NewEvent(EventToolResult, agentName, sessionID, map[string]interface{}{
		"tool_id":   toolID,
		"tool_name": toolName,
		"output":    output,
		"is_error":  isError,
	})
}

// BeforeSpawnEvent creates an on_before_spawn event
func BeforeSpawnEvent(agentName, sessionID, childAgentName string) *Event {
	return NewEvent(EventBeforeSpawn, agentName, sessionID, map[string]interface{}{
		"child_agent": childAgentName,
	})
}

// AfterSpawnEvent creates an on_after_spawn event
func AfterSpawnEvent(agentName, sessionID, childAgentName string, success bool, result interface{}) *Event {
	return NewEvent(EventAfterSpawn, agentName, sessionID, map[string]interface{}{
		"child_agent": childAgentName,
		"success":     success,
		"result":      result,
	})
}

// SessionEndEvent creates an on_session_end event
func SessionEndEvent(agentName, sessionID string, totalCost float64, totalTokens int) *Event {
	return NewEvent(EventSessionEnd, agentName, sessionID, map[string]interface{}{
		"total_cost":   totalCost,
		"total_tokens": totalTokens,
	})
}
