package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/google/uuid"
)

type gwMessage struct {
	Type    string          `json:"type"`
	Event   string          `json:"event,omitempty"`
	ID      string          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	OK      bool            `json:"ok,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
}

type agentPayload struct {
	Stream string        `json:"stream"`
	Data   *agentDataMsg `json:"data,omitempty"`
}

type agentDataMsg struct {
	Delta        string `json:"delta,omitempty"`
	Phase        string `json:"phase,omitempty"`
	Error        string `json:"error,omitempty"`
	InputTokens  int    `json:"inputTokens,omitempty"`
	OutputTokens int    `json:"outputTokens,omitempty"`
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	url := "ws://localhost:18789/ws"
	token := "8a8b770d8433b3cd93b8c2cc9263a79a9eac17800ab5c92c"
	agentID := "kayushkin"
	task := "Say hello"

	// Create context with timeout
	ctx, cancel2 := context.WithTimeout(ctx, 10*time.Second)
	defer cancel2()

	// Connect to gateway
	fmt.Println("Connecting...")
	origin := strings.Replace(url, "ws://", "http://", 1)
	origin = strings.Replace(origin, "wss://", "https://", 1)
	origin = strings.TrimSuffix(origin, "/ws")
	origin = strings.TrimSuffix(origin, "/")

	header := http.Header{}
	header.Set("Origin", origin)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.DialContext(ctx, url, header)
	if err != nil {
		log.Fatalf("❌ Dial failed: %v", err)
	}
	defer conn.Close()
	fmt.Println("✓ WebSocket connected")

	// Wait for challenge
	var msg gwMessage
	if err := conn.ReadJSON(&msg); err != nil {
		log.Fatalf("❌ Read challenge failed: %v", err)
	}

	if msg.Type != "event" || msg.Event != "connect.challenge" {
		log.Fatalf("❌ Expected challenge, got: %+v", msg)
	}
	fmt.Println("✓ Got challenge")

	// Send connect
	connectReq := map[string]interface{}{
		"type":   "req",
		"id":     uuid.New().String(),
		"method": "connect",
		"params": map[string]interface{}{
			"minProtocol": 3,
			"maxProtocol": 3,
			"client": map[string]interface{}{
				"id":       "openclaw-control-ui",
				"version":  "1.0.0",
				"platform": "go",
				"mode":     "test",
			},
			"role":   "operator",
			"scopes": []string{"operator.admin", "operator.read", "operator.write"},
			"caps":   []string{},
			"commands": []string{},
			"permissions": map[string]interface{}{},
			"auth": map[string]interface{}{
				"token": token,
			},
			"locale":    "en-US",
			"userAgent": "inber/1.0.0",
		},
	}

	if err := conn.WriteJSON(connectReq); err != nil {
		log.Fatalf("❌ Send connect failed: %v", err)
	}
	fmt.Println("✓ Sent connect")

	// Wait for hello-ok
	if err := conn.ReadJSON(&msg); err != nil {
		log.Fatalf("❌ Read hello-ok failed: %v", err)
	}

	if !msg.OK {
		log.Fatalf("❌ Connection rejected: %+v", msg)
	}
	fmt.Println("✓ Connected")

	// Send agent request
	requestID := uuid.New().String()
	req := map[string]interface{}{
		"type":   "req",
		"id":     requestID,
		"method": "agent",
		"params": map[string]interface{}{
			"message":        task,
			"agentId":        agentID,
			"channel":        "webchat",
			"idempotencyKey": uuid.New().String(),
		},
	}

	if err := conn.WriteJSON(req); err != nil {
		log.Fatalf("❌ Send request failed: %v", err)
	}
	fmt.Printf("✓ Sent agent request for %s\n", agentID)

	// Read response
	var responseText strings.Builder
	var inputTokens, outputTokens int

	for i := 0; i < 50; i++ {
		if err := conn.ReadJSON(&msg); err != nil {
			log.Printf("❌ Read failed: %v", err)
			break
		}

		if msg.Type == "event" && msg.Event == "agent" {
			var payload agentPayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				continue
			}

			switch payload.Stream {
			case "assistant":
				if payload.Data != nil && payload.Data.Delta != "" {
					responseText.WriteString(payload.Data.Delta)
					fmt.Print(payload.Data.Delta)
				}

			case "lifecycle":
				if payload.Data != nil {
					phase := payload.Data.Phase
					if phase == "end" {
						fmt.Println("\n✓ Agent completed")
						goto done
					}
					if phase == "error" {
						errMsg := "agent error"
						if payload.Data.Error != "" {
							errMsg = payload.Data.Error
						}
						log.Fatalf("❌ Agent error: %s", errMsg)
					}
				}

			case "usage":
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

done:
	fmt.Printf("\n\n✅ Response: %s\n", strings.TrimSpace(responseText.String()))
	fmt.Printf("Tokens: in=%d out=%d\n", inputTokens, outputTokens)
}
