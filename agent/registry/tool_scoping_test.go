package registry

import (
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/session"
)

func TestToolScoping(t *testing.T) {
	client := &anthropic.Client{}
	
	// Create a registry with mock configurations that have different tool sets
	r := &Registry{
		client: client,
		configs: map[string]*AgentConfig{
			"minimal-agent": {
				Name:   "minimal-agent",
				Model:  "claude-3-haiku-20240307",
				System: "You are a minimal agent",
				Tools:  []string{"read_file"},  // Only one tool
			},
			"power-agent": {
				Name:   "power-agent",
				Model:  "claude-3-sonnet-20240229",
				System: "You are a powerful agent",
				Tools:  []string{"shell", "read_file", "write_file", "edit_file"}, // Multiple tools
			},
			"read-only-agent": {
				Name:   "read-only-agent",
				Model:  "claude-3-haiku-20240307",
				System: "You are a read-only agent",
				Tools:  []string{"read_file", "list_files"}, // Read-only tools
			},
		},
		agents:   make(map[string]*agent.Agent),
		sessions: make(map[string]*session.Session),
		tools:    NewToolRegistry(),
	}

	// Test minimal agent only gets the tools it's configured for
	minimalAgent, err := r.createAgent(r.configs["minimal-agent"])
	if err != nil {
		t.Fatalf("Failed to create minimal agent: %v", err)
	}
	
	// Check that the agent has exactly 1 tool (read_file)
	minimalTools := getAgentToolNames(minimalAgent)
	expectedMinimal := []string{"read_file"}
	if !equalStringSlices(minimalTools, expectedMinimal) {
		t.Errorf("Minimal agent tools mismatch. Expected: %v, Got: %v", expectedMinimal, minimalTools)
	}

	// Test power agent gets multiple tools
	powerAgent, err := r.createAgent(r.configs["power-agent"])
	if err != nil {
		t.Fatalf("Failed to create power agent: %v", err)
	}
	
	powerTools := getAgentToolNames(powerAgent)
	expectedPower := []string{"shell", "read_file", "write_file", "edit_file"}
	if !equalStringSlices(powerTools, expectedPower) {
		t.Errorf("Power agent tools mismatch. Expected: %v, Got: %v", expectedPower, powerTools)
	}

	// Test read-only agent gets only read tools
	readOnlyAgent, err := r.createAgent(r.configs["read-only-agent"])
	if err != nil {
		t.Fatalf("Failed to create read-only agent: %v", err)
	}
	
	readOnlyTools := getAgentToolNames(readOnlyAgent)
	expectedReadOnly := []string{"read_file", "list_files"}
	if !equalStringSlices(readOnlyTools, expectedReadOnly) {
		t.Errorf("Read-only agent tools mismatch. Expected: %v, Got: %v", expectedReadOnly, readOnlyTools)
	}

	// Test that agents don't have tools they weren't configured for
	if containsString(minimalTools, "shell") {
		t.Error("Minimal agent should not have shell tool")
	}
	
	if containsString(readOnlyTools, "shell") {
		t.Error("Read-only agent should not have shell tool")
	}
	
	if containsString(readOnlyTools, "write_file") {
		t.Error("Read-only agent should not have write_file tool")
	}
}

func TestInvalidToolHandling(t *testing.T) {
	client := &anthropic.Client{}
	
	// Create a registry with an agent that has an invalid tool
	r := &Registry{
		client: client,
		configs: map[string]*AgentConfig{
			"invalid-tool-agent": {
				Name:   "invalid-tool-agent",
				Model:  "claude-3-haiku-20240307",
				System: "You are an agent with invalid tools",
				Tools:  []string{"read_file", "nonexistent_tool", "write_file"}, // Invalid tool in the middle
			},
		},
		agents:   make(map[string]*agent.Agent),
		sessions: make(map[string]*session.Session),
		tools:    NewToolRegistry(),
	}

	// Attempt to create agent should fail due to invalid tool
	_, err := r.createAgent(r.configs["invalid-tool-agent"])
	if err == nil {
		t.Error("Expected error when creating agent with invalid tool, but got none")
	}
	
	if !strings.Contains(err.Error(), "nonexistent_tool") {
		t.Errorf("Error message should mention the invalid tool 'nonexistent_tool', got: %v", err)
	}
}

func TestEmptyToolsList(t *testing.T) {
	client := &anthropic.Client{}
	
	// Create a registry with an agent that has no tools
	r := &Registry{
		client: client,
		configs: map[string]*AgentConfig{
			"no-tools-agent": {
				Name:   "no-tools-agent",
				Model:  "claude-3-haiku-20240307",
				System: "You are an agent with no tools",
				Tools:  []string{}, // Empty tools list
			},
		},
		agents:   make(map[string]*agent.Agent),
		sessions: make(map[string]*session.Session),
		tools:    NewToolRegistry(),
	}

	// Should be able to create agent with no tools
	noToolsAgent, err := r.createAgent(r.configs["no-tools-agent"])
	if err != nil {
		t.Fatalf("Failed to create no-tools agent: %v", err)
	}
	
	// Agent should have no tools
	noTools := getAgentToolNames(noToolsAgent)
	if len(noTools) != 0 {
		t.Errorf("No-tools agent should have 0 tools, got: %v", noTools)
	}
}

// Helper function to extract tool names from an agent
func getAgentToolNames(agent *agent.Agent) []string {
	tools := agent.GetTools()
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name
	}
	return names
}

// Helper function to check if two string slices are equal (ignoring order)
func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	
	countA := make(map[string]int)
	countB := make(map[string]int)
	
	for _, s := range a {
		countA[s]++
	}
	for _, s := range b {
		countB[s]++
	}
	
	for k, v := range countA {
		if countB[k] != v {
			return false
		}
	}
	return true
}

// Helper function to check if a slice contains a string
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}