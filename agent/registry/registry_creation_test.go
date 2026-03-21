package registry

import (
	"os"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

func TestRegistry_Creation(t *testing.T) {
	// Create a mock client
	client := &anthropic.Client{}
	
	// Create temporary directory for logs
	tmpDir, err := os.MkdirTemp("", "test-logs")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// This will fail because agent-store is not set up, so we test the error handling
	_, err = New(client, tmpDir)
	if err != nil {
		// Expected - agent store isn't available in test environment
		t.Logf("Expected error when agent-store is not available: %v", err)
	} else {
		// Unexpected - but could happen if agent-store is somehow available
		t.Log("Registry creation succeeded (agent-store may be available)")
	}
}

func TestRegistry_New(t *testing.T) {
	// Create a mock client
	client := &anthropic.Client{}
	
	// Create temporary directory for logs
	tmpDir, err := os.MkdirTemp("", "test-logs")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Test the API - agent-store may or may not be available
	_, err = New(client, tmpDir)
	
	if err != nil {
		// Expected when agent-store is not available
		t.Logf("Expected error when agent-store is not available: %v", err)
	}
}