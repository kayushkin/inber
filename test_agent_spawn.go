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

func prettyPrint(data interface{}) {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Printf("%+v\n", data)
		return
	}
	fmt.Println(string(b))
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	url := "ws://localhost:18789/ws"
	token := "8a8b770d8433b3cd93b8c2cc9263a79a9eac17800ab5c92c"

	// Connect
	origin := strings.Replace(url, "ws://", "http://", 1)
	origin = strings.TrimSuffix(origin, "/ws")

	header := http.Header{}
	header.Set("Origin", origin)

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	fmt.Printf("🔌 Connecting to %s...\n", url)
	conn, _, err := dialer.DialContext(ctx, url, header)
	if err != nil {
		log.Fatalf("❌ Dial failed: %v", err)
	}
	defer conn.Close()
	fmt.Println("✅ WebSocket connected")

	// Read challenge
	fmt.Println("\n📥 Waiting for challenge...")
	var msg gwMessage
	if err := conn.ReadJSON(&msg); err != nil {
		log.Fatalf("❌ Read challenge failed: %v", err)
	}
	fmt.Printf("Received message:\n")
	prettyPrint(msg)

	if msg.Type != "event" || msg.Event != "connect.challenge" {
		log.Fatalf("❌ Expected connect.challenge, got type=%s event=%s", msg.Type, msg.Event)
	}
	fmt.Println("✅ Got challenge")

	// Send connect
	connectReq := map[string]interface{}{
		"type":   "req",
		"id":     uuid.New().String(),
		"method": "connect",
		"params": map[string]interface{}{
			"minProtocol": 3,
			"maxProtocol": 3,
			"client": map[string]interface{}{
				"id":       "openclaw-android",
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
			"userAgent": "inber-test/1.0.0",
		},
	}

	fmt.Println("\n📤 Sending connect request...")
	if err := conn.WriteJSON(connectReq); err != nil {
		log.Fatalf("❌ Send connect failed: %v", err)
	}
	fmt.Println("✅ Sent connect")

	// Read response
	fmt.Println("\n📥 Waiting for response...")
	if err := conn.ReadJSON(&msg); err != nil {
		log.Fatalf("❌ Read response failed: %v", err)
	}
	fmt.Printf("Received response:\n")
	prettyPrint(msg)

	if !msg.OK {
		if msg.Error != nil {
			log.Fatalf("❌ Connection rejected: %s", string(msg.Error))
		}
		log.Fatalf("❌ Connection rejected (no error details)")
	}

	fmt.Println("\n✅ Connected successfully!")

	// Check payload
	if msg.Payload != nil {
		fmt.Println("\n📦 Payload:")
		prettyPrint(json.RawMessage(msg.Payload))
	}

	// Try to spawn an agent
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("Testing Agent Spawn")
	fmt.Println(strings.Repeat("=", 60))

	spawnReq := map[string]interface{}{
		"type":   "req",
		"id":     uuid.New().String(),
		"method": "agent",
		"params": map[string]interface{}{
			"message":        "Say hello",
			"agentId":        "kayushkin",
			"channel":        "test",
			"idempotencyKey": uuid.New().String(),
		},
	}

	fmt.Println("\n📤 Sending agent request...")
	if err := conn.WriteJSON(spawnReq); err != nil {
		log.Fatalf("❌ Send agent request failed: %v", err)
	}
	fmt.Println("✅ Sent agent request")

	// Read agent response
	fmt.Println("\n📥 Waiting for agent response...")
	for i := 0; i < 20; i++ {
		if err := conn.ReadJSON(&msg); err != nil {
			log.Printf("❌ Read failed: %v", err)
			break
		}

		fmt.Printf("\n--- Message %d ---\n", i+1)
		prettyPrint(msg)

		// Check if this is an agent stream event
		if msg.Type == "event" && msg.Event == "agent" {
			var payload map[string]interface{}
			if err := json.Unmarshal(msg.Payload, &payload); err == nil {
				if stream, ok := payload["stream"].(string); ok {
					fmt.Printf("Stream: %s\n", stream)
					if stream == "lifecycle" {
						if data, ok := payload["data"].(map[string]interface{}); ok {
							if phase, ok := data["phase"].(string); ok {
								fmt.Printf("Phase: %s\n", phase)
								if phase == "end" {
									fmt.Println("\n✅ Agent completed!")
									break
								}
								if phase == "error" {
									if errMsg, ok := data["error"].(string); ok {
										fmt.Printf("❌ Error: %s\n", errMsg)
									}
									break
								}
							}
						}
					}
				}
			}
		}
	}
}
