package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/kayushkin/inber/agent"
)

// OpenClawSubagent connects to an OpenClaw gateway and delegates tasks to specific agents.
type OpenClawSubagent struct {
	url      string
	token    string
	agentID  string
	timeout  time.Duration
}

// NewOpenClawSubagent creates a new OpenClaw subagent connector.
func NewOpenClawSubagent(url, token, agentID string, timeout time.Duration) *OpenClawSubagent {
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	return &OpenClawSubagent{
		url:     url,
		token:   token,
		agentID: agentID,
		timeout: timeout,
	}
}

// Run delegates a task to the OpenClaw agent and returns the result.
func (o *OpenClawSubagent) Run(ctx context.Context, task string) (*agent.TurnResult, error) {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, o.timeout)
	defer cancel()

	// Connect to gateway
	conn, err := o.connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("connect failed: %w", err)
	}
	defer conn.Close()

	// Send agent request
	requestID := uuid.New().String()
	req := map[string]interface{}{
		"type":   "req",
		"id":     requestID,
		"method": "agent",
		"params": map[string]interface{}{
			"message":        task,
			"agentId":        o.agentID,
			"channel":        "webchat",
			"idempotencyKey": uuid.New().String(),
		},
	}

	if err := conn.WriteJSON(req); err != nil {
		return nil, fmt.Errorf("send request failed: %w", err)
	}

	// Buffer streaming response
	var responseText strings.Builder
	var inputTokens, outputTokens int

	for {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("timeout waiting for response")
		default:
		}

		var msg GatewayMessage
		if err := conn.ReadJSON(&msg); err != nil {
			return nil, fmt.Errorf("read response failed: %w", err)
		}

		// Handle agent events
		if msg.Type == "event" && msg.Event == "agent" {
			if msg.Payload == nil {
				continue
			}

			var payload AgentEventPayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				continue
			}

			switch payload.Stream {
			case "assistant":
				if payload.Data != nil && payload.Data.Delta != "" {
					responseText.WriteString(payload.Data.Delta)
				}

			case "lifecycle":
				if payload.Data != nil {
					phase := payload.Data.Phase
					if phase == "end" {
						// Response complete
						result := &agent.TurnResult{
							Text:         strings.TrimSpace(responseText.String()),
							InputTokens:  inputTokens,
							OutputTokens: outputTokens,
						}
						return result, nil
					}
					if phase == "error" {
						errMsg := "agent error"
						if payload.Data.Error != "" {
							errMsg = payload.Data.Error
						}
						return nil, fmt.Errorf("agent error: %s", errMsg)
					}
				}

			case "usage":
				// Track token usage if provided
				if payload.Data != nil {
					if payload.Data.InputTokens > 0 {
						inputTokens = payload.Data.InputTokens
					}
					if payload.Data.OutputTokens > 0 {
						outputTokens = payload.Data.OutputTokens
					}
				}
			}
		}
	}
}

// connect establishes a WebSocket connection and performs the OpenClaw handshake.
func (o *OpenClawSubagent) connect(ctx context.Context) (*websocket.Conn, error) {
	// Derive Origin from WebSocket URL
	origin := strings.Replace(o.url, "ws://", "http://", 1)
	origin = strings.Replace(origin, "wss://", "https://", 1)
	origin = strings.TrimSuffix(origin, "/ws")
	origin = strings.TrimSuffix(origin, "/")

	header := http.Header{}
	header.Set("Origin", origin)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, o.url, header)
	if err != nil {
		return nil, fmt.Errorf("dial failed: %w", err)
	}

	// Wait for connect.challenge event
	var msg GatewayMessage
	if err := conn.ReadJSON(&msg); err != nil {
		conn.Close()
		return nil, fmt.Errorf("read challenge failed: %w", err)
	}

	if msg.Type != "event" || msg.Event != "connect.challenge" {
		conn.Close()
		return nil, fmt.Errorf("unexpected message, expected connect.challenge")
	}

	// Send connect request
	connectReq := map[string]interface{}{
		"type":   "req",
		"id":     uuid.New().String(),
		"method": "connect",
		"params": map[string]interface{}{
			"minProtocol": 3,
			"maxProtocol": 3,
			"client": map[string]interface{}{
				"id":       "inber-orchestrator",
				"version":  "1.0.0",
				"platform": "go",
				"mode":     "webchat",
			},
			"role":   "operator",
			"scopes": []string{"operator.admin", "operator.read", "operator.write"},
			"caps":   []string{},
			"commands": []string{},
			"permissions": map[string]interface{}{},
			"auth": map[string]interface{}{
				"token": o.token,
			},
			"locale":    "en-US",
			"userAgent": "inber/1.0.0",
		},
	}

	if err := conn.WriteJSON(connectReq); err != nil {
		conn.Close()
		return nil, fmt.Errorf("send connect failed: %w", err)
	}

	// Wait for hello-ok response
	if err := conn.ReadJSON(&msg); err != nil {
		conn.Close()
		return nil, fmt.Errorf("read hello-ok failed: %w", err)
	}

	if msg.Type != "res" || !msg.OK {
		errMsg := "connection rejected"
		if msg.Error != nil {
			var errData map[string]interface{}
			if err := json.Unmarshal(msg.Error, &errData); err == nil {
				if m, ok := errData["message"].(string); ok {
					errMsg = m
				}
			}
		}
		conn.Close()
		return nil, fmt.Errorf("connect failed: %s", errMsg)
	}

	// Verify hello-ok payload
	if msg.Payload != nil {
		var payload map[string]interface{}
		if err := json.Unmarshal(msg.Payload, &payload); err == nil {
			if payload["type"] != "hello-ok" {
				conn.Close()
				return nil, fmt.Errorf("unexpected payload type: %v", payload["type"])
			}
		}
	}

	return conn, nil
}

// GatewayMessage represents the base OpenClaw gateway message structure.
type GatewayMessage struct {
	Type    string          `json:"type"`
	Event   string          `json:"event,omitempty"`
	ID      string          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	OK      bool            `json:"ok,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
}

// AgentEventPayload represents agent event data.
type AgentEventPayload struct {
	Stream string          `json:"stream"`
	Data   *AgentEventData `json:"data,omitempty"`
}

// AgentEventData represents the data field in agent events.
type AgentEventData struct {
	Delta        string `json:"delta,omitempty"`
	Phase        string `json:"phase,omitempty"`
	Error        string `json:"error,omitempty"`
	InputTokens  int    `json:"inputTokens,omitempty"`
	OutputTokens int    `json:"outputTokens,omitempty"`
}
