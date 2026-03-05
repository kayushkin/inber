package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

var testUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// TestOpenClawSubagent_SuccessfulDelegation tests a complete successful task delegation
func TestOpenClawSubagent_SuccessfulDelegation(t *testing.T) {
	// Mock OpenClaw gateway
	mockGateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Logf("upgrade error: %v", err)
			return
		}
		defer conn.Close()

		// 1. Send connect.challenge
		challenge := map[string]interface{}{
			"type":  "event",
			"event": "connect.challenge",
		}
		if err := conn.WriteJSON(challenge); err != nil {
			t.Logf("write challenge error: %v", err)
			return
		}

		// 2. Read connect request
		var connectReq map[string]interface{}
		if err := conn.ReadJSON(&connectReq); err != nil {
			t.Logf("read connect error: %v", err)
			return
		}

		// Verify connect request format
		if connectReq["type"] != "req" || connectReq["method"] != "connect" {
			t.Errorf("invalid connect request: %v", connectReq)
			return
		}

		// 3. Send hello-ok response
		helloOk := map[string]interface{}{
			"type": "res",
			"ok":   true,
			"payload": map[string]interface{}{
				"type":     "hello-ok",
				"protocol": 3,
			},
		}
		if err := conn.WriteJSON(helloOk); err != nil {
			t.Logf("write hello-ok error: %v", err)
			return
		}

		// 4. Read agent request
		var agentReq map[string]interface{}
		if err := conn.ReadJSON(&agentReq); err != nil {
			t.Logf("read agent request error: %v", err)
			return
		}

		// Verify agent request
		if agentReq["type"] != "req" || agentReq["method"] != "agent" {
			t.Errorf("invalid agent request: %v", agentReq)
			return
		}

		params := agentReq["params"].(map[string]interface{})
		if params["agentId"] != "test-agent" {
			t.Errorf("wrong agentId: %v", params["agentId"])
		}
		if params["message"] != "Test task" {
			t.Errorf("wrong message: %v", params["message"])
		}

		// 5. Stream assistant response
		deltas := []string{"Hello", " from ", "OpenClaw", "!"}
		for _, delta := range deltas {
			event := map[string]interface{}{
				"type":  "event",
				"event": "agent",
				"payload": map[string]interface{}{
					"stream": "assistant",
					"data": map[string]interface{}{
						"delta": delta,
					},
				},
			}
			if err := conn.WriteJSON(event); err != nil {
				t.Logf("write delta error: %v", err)
				return
			}
			time.Sleep(10 * time.Millisecond) // Simulate streaming
		}

		// 6. Send lifecycle end
		endEvent := map[string]interface{}{
			"type":  "event",
			"event": "agent",
			"payload": map[string]interface{}{
				"stream": "lifecycle",
				"data": map[string]interface{}{
					"phase": "end",
				},
			},
		}
		if err := conn.WriteJSON(endEvent); err != nil {
			t.Logf("write end error: %v", err)
			return
		}
	}))
	defer mockGateway.Close()

	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + mockGateway.URL[4:] + "/ws"

	// Create OpenClaw subagent
	subagent := NewOpenClawSubagent(wsURL, "test-token", "test-agent", 5*time.Second)

	// Run task
	ctx := context.Background()
	result, err := subagent.Run(ctx, "Test task")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Verify result
	expectedText := "Hello from OpenClaw!"
	if result.Text != expectedText {
		t.Errorf("expected text %q, got %q", expectedText, result.Text)
	}

	t.Logf("✓ Successful delegation: '%s'", result.Text)
}

// TestOpenClawSubagent_Timeout tests timeout handling
func TestOpenClawSubagent_Timeout(t *testing.T) {
	// Mock gateway that never completes
	mockGateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// Send challenge and hello-ok
		conn.WriteJSON(map[string]interface{}{"type": "event", "event": "connect.challenge"})
		conn.ReadJSON(&map[string]interface{}{})
		conn.WriteJSON(map[string]interface{}{
			"type": "res",
			"ok":   true,
			"payload": map[string]interface{}{
				"type":     "hello-ok",
				"protocol": 3,
			},
		})

		// Read agent request but never respond
		conn.ReadJSON(&map[string]interface{}{})

		// Block until connection closes
		time.Sleep(5 * time.Second)
	}))
	defer mockGateway.Close()

	wsURL := "ws" + mockGateway.URL[4:] + "/ws"

	subagent := NewOpenClawSubagent(wsURL, "test-token", "test-agent", 100*time.Millisecond)

	ctx := context.Background()
	_, err := subagent.Run(ctx, "Test task")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	// Accept either timeout or connection close error (both are valid timeout scenarios)
	errStr := err.Error()
	if !strings.Contains(errStr, "timeout") && !strings.Contains(errStr, "close") && !strings.Contains(errStr, "EOF") {
		t.Errorf("expected timeout/close error, got: %v", err)
	}

	t.Logf("✓ Timeout handled correctly")
}

// TestOpenClawSubagent_AgentError tests error response handling
func TestOpenClawSubagent_AgentError(t *testing.T) {
	mockGateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// Standard handshake
		conn.WriteJSON(map[string]interface{}{"type": "event", "event": "connect.challenge"})
		conn.ReadJSON(&map[string]interface{}{})
		conn.WriteJSON(map[string]interface{}{
			"type": "res",
			"ok":   true,
			"payload": map[string]interface{}{
				"type":     "hello-ok",
				"protocol": 3,
			},
		})

		// Read agent request
		conn.ReadJSON(&map[string]interface{}{})

		// Send error lifecycle event
		errorEvent := map[string]interface{}{
			"type":  "event",
			"event": "agent",
			"payload": map[string]interface{}{
				"stream": "lifecycle",
				"data": map[string]interface{}{
					"phase": "error",
					"error": "Agent execution failed",
				},
			},
		}
		conn.WriteJSON(errorEvent)
	}))
	defer mockGateway.Close()

	wsURL := "ws" + mockGateway.URL[4:] + "/ws"

	subagent := NewOpenClawSubagent(wsURL, "test-token", "test-agent", 5*time.Second)

	ctx := context.Background()
	_, err := subagent.Run(ctx, "Test task")
	if err == nil {
		t.Fatal("expected agent error, got nil")
	}

	expectedErr := "agent error: Agent execution failed"
	if err.Error() != expectedErr {
		t.Errorf("expected error %q, got %q", expectedErr, err.Error())
	}

	t.Logf("✓ Agent error handled correctly")
}

// TestOpenClawSubagent_ConnectionFailed tests connection failure handling
func TestOpenClawSubagent_ConnectionFailed(t *testing.T) {
	// Invalid URL
	subagent := NewOpenClawSubagent("ws://localhost:99999/ws", "test-token", "test-agent", 1*time.Second)

	ctx := context.Background()
	_, err := subagent.Run(ctx, "Test task")
	if err == nil {
		t.Fatal("expected connection error, got nil")
	}

	t.Logf("✓ Connection failure handled: %v", err)
}

// TestOpenClawSubagent_AuthRejection tests authentication rejection
func TestOpenClawSubagent_AuthRejection(t *testing.T) {
	mockGateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// Send challenge
		conn.WriteJSON(map[string]interface{}{"type": "event", "event": "connect.challenge"})

		// Read connect request
		conn.ReadJSON(&map[string]interface{}{})

		// Send rejection
		rejection := map[string]interface{}{
			"type": "res",
			"ok":   false,
			"error": map[string]interface{}{
				"message": "Invalid authentication token",
			},
		}
		conn.WriteJSON(rejection)
	}))
	defer mockGateway.Close()

	wsURL := "ws" + mockGateway.URL[4:] + "/ws"

	subagent := NewOpenClawSubagent(wsURL, "bad-token", "test-agent", 5*time.Second)

	ctx := context.Background()
	_, err := subagent.Run(ctx, "Test task")
	if err == nil {
		t.Fatal("expected auth error, got nil")
	}

	// Check that error contains the key message (allow for nested "connect failed:")
	if !strings.Contains(err.Error(), "Invalid authentication token") {
		t.Errorf("expected error to contain 'Invalid authentication token', got %q", err.Error())
	}

	t.Logf("✓ Auth rejection handled correctly")
}

// TestOpenClawSubagent_TokenUsageTracking tests that token usage is tracked
func TestOpenClawSubagent_TokenUsageTracking(t *testing.T) {
	mockGateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// Standard handshake
		conn.WriteJSON(map[string]interface{}{"type": "event", "event": "connect.challenge"})
		conn.ReadJSON(&map[string]interface{}{})
		conn.WriteJSON(map[string]interface{}{
			"type": "res",
			"ok":   true,
			"payload": map[string]interface{}{
				"type":     "hello-ok",
				"protocol": 3,
			},
		})

		// Read agent request
		conn.ReadJSON(&map[string]interface{}{})

		// Send usage event
		usageEvent := map[string]interface{}{
			"type":  "event",
			"event": "agent",
			"payload": map[string]interface{}{
				"stream": "usage",
				"data": map[string]interface{}{
					"inputTokens":  150,
					"outputTokens": 75,
				},
			},
		}
		conn.WriteJSON(usageEvent)

		// Send assistant response
		conn.WriteJSON(map[string]interface{}{
			"type":  "event",
			"event": "agent",
			"payload": map[string]interface{}{
				"stream": "assistant",
				"data":   map[string]interface{}{"delta": "Response"},
			},
		})

		// Send end
		conn.WriteJSON(map[string]interface{}{
			"type":  "event",
			"event": "agent",
			"payload": map[string]interface{}{
				"stream": "lifecycle",
				"data":   map[string]interface{}{"phase": "end"},
			},
		})
	}))
	defer mockGateway.Close()

	wsURL := "ws" + mockGateway.URL[4:] + "/ws"

	subagent := NewOpenClawSubagent(wsURL, "test-token", "test-agent", 5*time.Second)

	ctx := context.Background()
	result, err := subagent.Run(ctx, "Test task")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Verify token tracking
	if result.InputTokens != 150 {
		t.Errorf("expected inputTokens=150, got %d", result.InputTokens)
	}
	if result.OutputTokens != 75 {
		t.Errorf("expected outputTokens=75, got %d", result.OutputTokens)
	}

	t.Logf("✓ Token usage tracked correctly: in=%d, out=%d", result.InputTokens, result.OutputTokens)
}

// Mock JSON structures for testing
func TestGatewayMessageSerialization(t *testing.T) {
	// Test that our message structures serialize correctly
	msg := GatewayMessage{
		Type:  "event",
		Event: "agent",
		Payload: json.RawMessage(`{"stream":"assistant","data":{"delta":"test"}}`),
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded GatewayMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded.Type != "event" || decoded.Event != "agent" {
		t.Errorf("decode failed: %+v", decoded)
	}

	t.Logf("✓ Message serialization works correctly")
}
