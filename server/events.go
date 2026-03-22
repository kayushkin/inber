package server

import (
	"log"
	"time"

	natsbus "github.com/kayushkin/bus"
	"github.com/kayushkin/bus/messages"
)

// EventPublisher sends server events to the bus so dashboards can display them.
type EventPublisher struct {
	nc *natsbus.Client
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

// publishDelta sends a ChatDelta to chat.stream.
func (ep *EventPublisher) publishDelta(delta messages.ChatDelta) {
	if ep == nil || ep.nc == nil {
		return
	}
	if err := ep.nc.Publish("chat.stream", delta); err != nil {
		log.Printf("[events] publish error: %v", err)
	}
}

// SpawnStarted publishes a spawn_started event on chat.stream.
func (ep *EventPublisher) SpawnStarted(sessionKey, agent, parentKey, task string) {
	delta := messages.NewChatDelta(agent, "inber", sessionKey, "spawn_started")
	delta.Text = task
	ep.publishDelta(delta)
}

// SpawnCompleted publishes a spawn_completed event on chat.stream.
func (ep *EventPublisher) SpawnCompleted(result SpawnResult) {
	delta := messages.NewChatDelta(result.Agent, "inber", result.ChildKey, "spawn_completed")
	delta.Text = truncate(result.Summary, 500)
	if result.Duration > 0 {
		delta.Stats = &messages.TurnStats{
			DurationMs: int(result.Duration.Milliseconds()),
		}
	}
	ep.publishDelta(delta)
}

// SessionActive publishes when a session starts running.
func (ep *EventPublisher) SessionActive(sessionKey, agent string) {
	delta := messages.NewChatDelta(agent, "inber", sessionKey, "session_active")
	ep.publishDelta(delta)
}

// SessionIdle publishes when a session finishes.
func (ep *EventPublisher) SessionIdle(sessionKey, agent string) {
	delta := messages.NewChatDelta(agent, "inber", sessionKey, "session_idle")
	ep.publishDelta(delta)
}

// PublishOutbound sends a completed message to chat.outbound via JetStream.
func (ep *EventPublisher) PublishOutbound(agent, sessionID, text string) {
	if ep == nil || ep.nc == nil {
		return
	}
	msg := messages.ChatOutbound{
		Agent:        agent,
		Orchestrator: "inber",
		SessionID:    sessionID,
		Text:         text,
		Timestamp:    time.Now(),
	}
	if err := ep.nc.JetPublish("chat.outbound", msg); err != nil {
		log.Printf("[events] outbound publish error: %v", err)
	}
}

// Close closes the NATS connection.
func (ep *EventPublisher) Close() {
	if ep != nil && ep.nc != nil {
		ep.nc.Close()
	}
}
