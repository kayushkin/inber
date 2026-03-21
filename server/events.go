package server

import (
	"fmt"
	"log"
	"time"

	"github.com/kayushkin/inber/bus"
	natsbus "github.com/kayushkin/bus"
)

// EventPublisher sends server events to the bus so dashboards can display them.
type EventPublisher struct {
	nc *natsbus.Client
}

// GatewayEvent is published to the bus for dashboard consumption.
type GatewayEvent struct {
	Kind       string      `json:"kind"`        // "spawn_started", "spawn_progress", "spawn_completed", "session_active", "session_idle"
	SessionKey string      `json:"session_key"`
	Agent      string      `json:"agent"`
	ParentKey  string      `json:"parent_key,omitempty"`
	Task       string      `json:"task,omitempty"`
	Status     string      `json:"status,omitempty"`
	Summary    string      `json:"summary,omitempty"`
	Tokens     *TokenUsage `json:"tokens,omitempty"`
	DurationMs int64       `json:"duration_ms,omitempty"`
	Error      string      `json:"error,omitempty"`
	Timestamp  time.Time   `json:"timestamp"`
}

// NewEventPublisher creates a publisher. Pass empty natsURL to disable.
func NewEventPublisher(natsURL, _ string) *EventPublisher {
	if natsURL == "" {
		return nil
	}

	nc, err := natsbus.Connect(natsbus.Options{
		URL:  natsURL,
		Name: "inber-events",
	})
	if err != nil {
		log.Printf("[events] failed to connect to NATS: %v", err)
		return nil
	}

	return &EventPublisher{nc: nc}
}

// Publish sends an event to the bus on the "server" subject.
func (ep *EventPublisher) Publish(event GatewayEvent) {
	if ep == nil {
		return
	}

	event.Timestamp = time.Now()

	if err := ep.nc.Publish("server", event); err != nil {
		log.Printf("[events] publish error: %v", err)
	}
}

// SpawnStarted publishes a spawn start event.
func (ep *EventPublisher) SpawnStarted(sessionKey, agent, parentKey, task string) {
	ep.Publish(GatewayEvent{
		Kind:       "spawn_started",
		SessionKey: sessionKey,
		Agent:      agent,
		ParentKey:  parentKey,
		Task:       task,
	})
}

// SpawnCompleted publishes a spawn completion event.
func (ep *EventPublisher) SpawnCompleted(result SpawnResult) {
	ep.Publish(GatewayEvent{
		Kind:       "spawn_completed",
		SessionKey: result.ChildKey,
		Agent:      result.Agent,
		Task:       result.Task,
		Status:     result.Status,
		Summary:    truncate(result.Summary, 500),
		Tokens:     &result.Tokens,
		DurationMs: result.Duration.Milliseconds(),
		Error:      result.Error,
	})
}

// PublishOutbound sends a spawn result to the bus "chat.outbound" subject.
func (ep *EventPublisher) PublishOutbound(parentAgent string, result SpawnResult) {
	if ep == nil {
		return
	}

	m := bus.NewOutboundFull(parentAgent, "websocket", "done", "", fmt.Sprintf("🔔 **Sub-agent %s completed** (%s)\n%s", result.Agent, result.Status, result.Summary))
	if err := ep.nc.Publish("chat.outbound", m); err != nil {
		log.Printf("[events] outbound publish error: %v", err)
	}
}

// SessionActive publishes when a session starts running.
func (ep *EventPublisher) SessionActive(sessionKey, agent string) {
	ep.Publish(GatewayEvent{
		Kind:       "session_active",
		SessionKey: sessionKey,
		Agent:      agent,
	})
}

// SessionIdle publishes when a session finishes.
func (ep *EventPublisher) SessionIdle(sessionKey, agent string) {
	ep.Publish(GatewayEvent{
		Kind:       "session_idle",
		SessionKey: sessionKey,
		Agent:      agent,
	})
}

// Close closes the NATS connection.
func (ep *EventPublisher) Close() {
	if ep != nil && ep.nc != nil {
		ep.nc.Close()
	}
}
