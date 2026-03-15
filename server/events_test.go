package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestEventPublisherNil(t *testing.T) {
	// Nil publisher should not panic.
	var ep *EventPublisher
	ep.SpawnStarted("key", "agent", "parent", "task")
	ep.SpawnCompleted(SpawnResult{})
	ep.SessionActive("key", "agent")
	ep.SessionIdle("key", "agent")
}

func TestEventPublisherPublishes(t *testing.T) {
	var mu sync.Mutex
	var received []map[string]interface{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var msg map[string]interface{}
		json.Unmarshal(body, &msg)
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
		w.WriteHeader(200)
	}))
	defer srv.Close()

	ep := NewEventPublisher(srv.URL, "test-token")
	ep.SpawnStarted("child:1", "ogma", "parent:main", "fix bugs")

	// Give HTTP call time.
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}

	if received[0]["topic"] != "server" {
		t.Errorf("expected topic=server, got %v", received[0]["topic"])
	}
	if received[0]["source"] != "server" {
		t.Errorf("expected source=server, got %v", received[0]["source"])
	}

	// Parse the payload.
	payloadRaw, _ := json.Marshal(received[0]["payload"])
	var event GatewayEvent
	json.Unmarshal(payloadRaw, &event)

	if event.Kind != "spawn_started" {
		t.Errorf("expected kind=spawn_started, got %s", event.Kind)
	}
	if event.Agent != "ogma" {
		t.Errorf("expected agent=ogma, got %s", event.Agent)
	}
}

func TestEventPublisherDisabled(t *testing.T) {
	ep := NewEventPublisher("", "")
	if ep != nil {
		t.Error("expected nil publisher for empty URL")
	}
}
