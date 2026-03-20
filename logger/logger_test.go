package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestLogger_BasicLogging(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithWriter(&buf, InfoLevel)

	logger.Info("test message")

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse log entry: %v", err)
	}

	if entry.Level != "info" {
		t.Errorf("Expected level 'info', got '%s'", entry.Level)
	}

	if entry.Message != "test message" {
		t.Errorf("Expected message 'test message', got '%s'", entry.Message)
	}

	if entry.Timestamp == "" {
		t.Error("Expected non-empty timestamp")
	}
}

func TestLogger_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithWriter(&buf, WarnLevel)

	// Debug and info should be filtered out
	logger.Debug("debug message")
	logger.Info("info message")
	
	if buf.Len() > 0 {
		t.Error("Expected no output for debug/info when level is warn")
	}

	// Warn should pass through
	logger.Warn("warn message")
	
	if buf.Len() == 0 {
		t.Error("Expected output for warn message")
	}
}

func TestLogger_WithFields(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithWriter(&buf, InfoLevel)

	logger.Info("test message", map[string]interface{}{
		"key1": "value1",
		"key2": 42,
	})

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse log entry: %v", err)
	}

	if entry.Fields["key1"] != "value1" {
		t.Errorf("Expected field key1='value1', got '%v'", entry.Fields["key1"])
	}

	if entry.Fields["key2"] != float64(42) {
		t.Errorf("Expected field key2=42, got '%v'", entry.Fields["key2"])
	}
}

func TestLogger_WithError(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithWriter(&buf, InfoLevel)

	err := errors.New("test error")
	logger.Error("error occurred", map[string]interface{}{
		"error": err,
	})

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse log entry: %v", err)
	}

	if entry.Error != "test error" {
		t.Errorf("Expected error 'test error', got '%s'", entry.Error)
	}
}

func TestLogger_WithComponent(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithWriter(&buf, InfoLevel)

	componentLogger := logger.WithComponent("server")
	componentLogger.Info("test message")

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse log entry: %v", err)
	}

	if entry.Component != "server" {
		t.Errorf("Expected component 'server', got '%s'", entry.Component)
	}
}

func TestLogger_WithContext(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithWriter(&buf, InfoLevel)

	ctx := context.WithValue(context.Background(), "session_id", "test-session")
	ctx = context.WithValue(ctx, "component", "api")

	contextLogger := logger.WithContext(ctx)
	contextLogger.Info("test message")

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse log entry: %v", err)
	}

	if entry.SessionID != "test-session" {
		t.Errorf("Expected session_id 'test-session', got '%s'", entry.SessionID)
	}

	if entry.Component != "api" {
		t.Errorf("Expected component 'api', got '%s'", entry.Component)
	}
}

func TestGlobalLogger(t *testing.T) {
	var buf bytes.Buffer
	
	// Save original stdout and replace temporarily
	originalLogger := defaultLogger
	defer func() { defaultLogger = originalLogger }()
	
	defaultLogger = NewWithWriter(&buf, InfoLevel)

	Info("global test message", map[string]interface{}{
		"global": true,
	})

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse log entry: %v", err)
	}

	if entry.Message != "global test message" {
		t.Errorf("Expected message 'global test message', got '%s'", entry.Message)
	}

	if entry.Fields["global"] != true {
		t.Errorf("Expected field global=true, got '%v'", entry.Fields["global"])
	}
}

func TestLogger_MultipleFields(t *testing.T) {
	var buf bytes.Buffer
	logger := NewWithWriter(&buf, InfoLevel)

	logger.Info("test message", 
		map[string]interface{}{"key1": "value1"},
		map[string]interface{}{"key2": "value2"},
	)

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse log entry: %v", err)
	}

	if entry.Fields["key1"] != "value1" {
		t.Errorf("Expected field key1='value1', got '%v'", entry.Fields["key1"])
	}

	if entry.Fields["key2"] != "value2" {
		t.Errorf("Expected field key2='value2', got '%v'", entry.Fields["key2"])
	}
}

func TestLogLevelString(t *testing.T) {
	tests := []struct {
		level    Level
		expected string
	}{
		{DebugLevel, "debug"},
		{InfoLevel, "info"},
		{WarnLevel, "warn"},
		{ErrorLevel, "error"},
	}

	for _, test := range tests {
		if test.level.String() != test.expected {
			t.Errorf("Expected %s, got %s", test.expected, test.level.String())
		}
	}
}

func TestSetLevel(t *testing.T) {
	var buf bytes.Buffer
	originalLogger := defaultLogger
	defer func() { defaultLogger = originalLogger }()
	
	defaultLogger = NewWithWriter(&buf, InfoLevel)

	// Set level to warn
	SetLevel(WarnLevel)

	// Info should be filtered out
	Info("info message")
	if strings.Contains(buf.String(), "info message") {
		t.Error("Expected info message to be filtered out when level is warn")
	}

	// Warn should pass through
	Warn("warn message")
	if !strings.Contains(buf.String(), "warn message") {
		t.Error("Expected warn message to be logged")
	}
}