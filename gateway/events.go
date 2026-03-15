package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// EventPublisher sends gateway events to the bus so dashboards can display them.
type EventPublisher struct {
	busURL   string
	busToken string
	client   *http.Client
}

// GatewayEvent is published to the bus for dashboard consumption.
type GatewayEvent struct {
	Kind       string    `json:"kind"`        // "spawn_started", "spawn_progress", "spawn_completed", "session_active", "session_idle"
	SessionKey string    `json:"session_key"`
	Agent      string    `json:"agent"`
	ParentKey  string    `json:"parent_key,omitempty"`
	Task       string    `json:"task,omitempty"`
	Status     string    `json:"status,omitempty"`
	Summary    string    `json:"summary,omitempty"`
	Tokens     *TokenUsage `json:"tokens,omitempty"`
	DurationMs int64     `json:"duration_ms,omitempty"`
	Error      string    `json:"error,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
}

// NewEventPublisher creates a publisher. Pass empty busURL to disable.
func NewEventPublisher(busURL, busToken string) *EventPublisher {
	if busURL == "" {
		return nil
	}
	return &EventPublisher{
		busURL:   busURL,
		busToken: busToken,
		client:   &http.Client{Timeout: 5 * time.Second},
	}
}

// Publish sends an event to the bus on the "gateway" topic.
func (ep *EventPublisher) Publish(event GatewayEvent) {
	if ep == nil {
		return
	}

	event.Timestamp = time.Now()

	payload, err := json.Marshal(event)
	if err != nil {
		log.Printf("[events] marshal error: %v", err)
		return
	}

	body := map[string]interface{}{
		"topic":   "gateway",
		"payload": json.RawMessage(payload),
		"source":  "gateway",
	}
	data, _ := json.Marshal(body)

	url := fmt.Sprintf("%s/publish?token=%s", ep.busURL, ep.busToken)
	resp, err := ep.client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		log.Printf("[events] publish error: %v", err)
		return
	}
	resp.Body.Close()
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
