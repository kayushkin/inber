package main

import (
	"testing"
)

func TestTruncateText(t *testing.T) {
	tests := []struct {
		input    string
		max      int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello..."},
		{"", 5, ""},
	}

	for i, test := range tests {
		result := truncateText(test.input, test.max)
		if result != test.expected {
			t.Errorf("test %d: expected %q, got %q", i, test.expected, result)
		}
	}
}