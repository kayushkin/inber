package bus

import (
	"testing"
)

func TestNewClient(t *testing.T) {
	// Test with empty busURL
	client := NewClient("", "token", "consumer")
	if client != nil {
		t.Error("Expected nil client for empty busURL")
	}

	// Test with valid busURL
	client = NewClient("http://localhost:8080", "token", "consumer")
	if client == nil {
		t.Error("Expected non-nil client for valid busURL")
	}

	// Test URL conversions
	if client.busURL != "http://localhost:8080" {
		t.Errorf("Expected HTTP URL, got %s", client.busURL)
	}

	// Test WebSocket URL conversion
	client = NewClient("https://example.com", "token", "consumer")
	if client.wsURL != "wss://example.com" {
		t.Errorf("Expected wss URL, got %s", client.wsURL)
	}

	// Test default consumer
	client = NewClient("http://localhost:8080", "token", "")
	if client.consumer != "inber-server" {
		t.Errorf("Expected default consumer 'inber-server', got %s", client.consumer)
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