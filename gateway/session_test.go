package gateway

import (
	"testing"
)

func TestSessionPendingMessages(t *testing.T) {
	s := &Session{
		Key:        "test",
		injections: make(chan string, 10),
	}

	// Queue pending messages while idle.
	s.queuePending("msg1")
	s.queuePending("msg2")

	s.mu.Lock()
	if len(s.pendingMessages) != 2 {
		t.Fatalf("expected 2 pending, got %d", len(s.pendingMessages))
	}
	s.mu.Unlock()
}

func TestSessionInjectMidRun(t *testing.T) {
	s := &Session{
		Key:        "test",
		injections: make(chan string, 10),
	}

	s.inject("hello")

	select {
	case msg := <-s.injections:
		if msg != "hello" {
			t.Errorf("expected 'hello', got %q", msg)
		}
	default:
		t.Error("expected message on injections channel")
	}
}

func TestSessionInjectBufferFull(t *testing.T) {
	s := &Session{
		Key:        "test",
		injections: make(chan string, 2),
	}

	// Fill buffer.
	s.inject("a")
	s.inject("b")
	// Third should be dropped (not block).
	s.inject("c") // should not panic or block
}

func TestSessionKeyForChild(t *testing.T) {
	key1 := sessionKeyForChild("agent:claxon:main")
	key2 := sessionKeyForChild("agent:claxon:main")

	if key1 == key2 {
		t.Error("expected unique child keys")
	}

	if len(key1) < len("agent:claxon:main:sub:") {
		t.Errorf("key too short: %s", key1)
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input string
		n     int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello world", 8, "hello..."},
		{"a\nb\nc", 20, "a b c"},
		{"", 5, ""},
	}

	for _, tt := range tests {
		got := truncate(tt.input, tt.n)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.n, got, tt.want)
		}
	}
}
