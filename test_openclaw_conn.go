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

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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

	fmt.Printf("Connecting to %s...\n", url)
	conn, _, err := dialer.DialContext(ctx, url, header)
	if err != nil {
		log.Fatalf("Dial failed: %v", err)
	}
	defer conn.Close()
	fmt.Println("✓ WebSocket connected")

	// Read challenge
	fmt.Println("Waiting for challenge...")
	var msg gwMessage
	if err := conn.ReadJSON(&msg); err != nil {
		log.Fatalf("Read challenge failed: %v", err)
	}
	fmt.Printf("Received: %+v\n", msg)

	if msg.Type != "event" || msg.Event != "connect.challenge" {
		log.Fatalf("Expected connect.challenge, got: %+v", msg)
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
				"id":       "test",
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
			"userAgent": "test/1.0.0",
		},
	}

	fmt.Println("Sending connect request...")
	if err := conn.WriteJSON(connectReq); err != nil {
		log.Fatalf("Send connect failed: %v", err)
	}
	fmt.Println("✓ Sent connect")

	// Read response
	fmt.Println("Waiting for response...")
	if err := conn.ReadJSON(&msg); err != nil {
		log.Fatalf("Read response failed: %v", err)
	}
	fmt.Printf("Received: %+v\n", msg)

	if msg.Type == "res" && msg.OK {
		fmt.Println("✓ Connected successfully!")
		
		// Try to spawn an agent
		fmt.Println("\n--- Testing Agent Spawn ---")
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
		
		fmt.Println("Sending agent request...")
		if err := conn.WriteJSON(spawnReq); err != nil {
			log.Fatalf("Send agent request failed: %v", err)
		}
		fmt.Println("✓ Sent agent request, waiting for response...")
		
		// Read agent response
		for i := 0; i < 10; i++ {
			if err := conn.ReadJSON(&msg); err != nil {
				log.Printf("Read agent response failed: %v", err)
				break
			}
			fmt.Printf("Agent event: %+v\n", msg)
		}
	} else {
		log.Printf("Connection rejected: %+v", msg)
	}
}
