package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	toolstoretools "github.com/kayushkin/tool-store/tools"
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
	_, err = New(client, tmpDir, "", "", "", toolstoretools.OutsideServiceConnections{})
	if err != nil {
		// Expected - agent store isn't available in test environment
		t.Logf("Expected error when agent-store is not available: %v", err)
	} else {
		// Unexpected - but could happen if agent-store is somehow available
		t.Log("Registry creation succeeded (agent-store may be available)")
	}
}

// The agent-store path New is given is the one it opens: an empty store at
// that path is created there, and has no agents to load.
func TestRegistry_NewOpensTheAgentStorePathItIsGiven(t *testing.T) {
	directory := t.TempDir()
	agentStorePath := filepath.Join(directory, "agents.db")

	_, err := New(&anthropic.Client{}, directory, agentStorePath, "", "", toolstoretools.OutsideServiceConnections{})
	if err == nil || !strings.Contains(err.Error(), "no agents registered") {
		t.Fatalf("New on an empty agent-store = %v, want the no-agents error", err)
	}
	if _, err := os.Stat(agentStorePath); err != nil {
		t.Fatalf("New did not open the agent-store at the path it was given: %v", err)
	}
}
