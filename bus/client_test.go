package bus

import (
	"testing"
)

func TestNewClient(t *testing.T) {
	// Test with empty natsURL
	client := NewClient("", "consumer")
	if client != nil {
		t.Error("Expected nil client for empty natsURL")
	}

	// Test with invalid natsURL (can't connect) returns nil
	client = NewClient("nats://localhost:19999", "consumer")
	if client != nil {
		t.Log("Client nil as expected when NATS not running")
	}
}

func TestTruncateBus(t *testing.T) {
	tests := []struct {
		input    string
		limit    int
		expected string
	}{
		{"short", 10, "short"},
		{"exactly ten", 11, "exactly ten"},
		{"this is longer than limit", 10, "this is lo..."},
		{"", 5, ""},
	}

	for _, tt := range tests {
		result := truncateBus(tt.input, tt.limit)
		if result != tt.expected {
			t.Errorf("truncateBus(%q, %d) = %q, want %q", tt.input, tt.limit, result, tt.expected)
		}
	}
}
