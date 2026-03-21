package registry

import (
	"testing"

	"github.com/kayushkin/inber/agent"
	modelstore "github.com/kayushkin/model-store"
)

func TestRegistry_List(t *testing.T) {
	r := createMockRegistry(t)
	
	agents := r.List()
	if len(agents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(agents))
	}
	
	// Check that both agents are present
	agentMap := make(map[string]bool)
	for _, name := range agents {
		agentMap[name] = true
	}
	
	if !agentMap["test-agent"] {
		t.Error("test-agent not found in list")
	}
	if !agentMap["another-agent"] {
		t.Error("another-agent not found in list")
	}
}

func TestRegistry_Default(t *testing.T) {
	r := createMockRegistry(t)
	
	defaultAgent := r.Default()
	if defaultAgent != "test-agent" {
		t.Errorf("expected default agent 'test-agent', got '%s'", defaultAgent)
	}
}

func TestRegistry_GetConfig(t *testing.T) {
	r := createMockRegistry(t)
	
	// Test exact case match
	cfg, err := r.GetConfig("test-agent")
	if err != nil {
		t.Errorf("GetConfig(test-agent) failed: %v", err)
	}
	if cfg.Name != "test-agent" {
		t.Errorf("expected config name 'test-agent', got '%s'", cfg.Name)
	}
	
	// Test case-insensitive match
	cfg, err = r.GetConfig("TEST-AGENT")
	if err != nil {
		t.Errorf("GetConfig(TEST-AGENT) failed: %v", err)
	}
	if cfg.Name != "test-agent" {
		t.Errorf("expected config name 'test-agent', got '%s'", cfg.Name)
	}
	
	// Test non-existent agent
	_, err = r.GetConfig("non-existent")
	if err == nil {
		t.Error("expected error for non-existent agent")
	}
}

func TestRegistry_SetMemoryStore(t *testing.T) {
	r := createMockRegistry(t)
	mockStore := newMockMemoryStore()
	
	// Should not panic
	r.SetMemoryStore(mockStore)
	
	// Memory tools should now be available
	_, err := r.tools.Get("memory_search")
	if err != nil {
		t.Errorf("memory_search tool not registered after SetMemoryStore: %v", err)
	}
}

func TestRegistry_SetModelClient(t *testing.T) {
	r := createMockRegistry(t)
	
	// Create a mock model client
	mc := &agent.ModelClient{}
	
	// Should not panic
	r.SetModelClient(mc)
	
	// Check that it was set
	if r.modelClient != mc {
		t.Error("model client was not set correctly")
	}
}

func TestRegistry_SetModelStore(t *testing.T) {
	r := createMockRegistry(t)
	
	// Create a mock model store
	store := &modelstore.Store{}
	
	// Should not panic
	r.SetModelStore(store)
	
	// Check that it was set
	if r.modelStore != store {
		t.Error("model store was not set correctly")
	}
}

func TestRegistry_SetOpenClawConfig(t *testing.T) {
	r := createMockRegistry(t)
	
	url := "https://test.openclaw.com"
	token := "test-token"
	agents := []string{"agent1", "agent2"}
	
	r.SetOpenClawConfig(url, token, agents)
	
	if r.openclawURL != url {
		t.Errorf("expected OpenClaw URL '%s', got '%s'", url, r.openclawURL)
	}
	if r.openclawToken != token {
		t.Errorf("expected OpenClaw token '%s', got '%s'", token, r.openclawToken)
	}
	if len(r.openclawAgents) != len(agents) {
		t.Errorf("expected %d OpenClaw agents, got %d", len(agents), len(r.openclawAgents))
	}
}

func TestRegistry_Get(t *testing.T) {
	r := createMockRegistry(t)
	
	// First call should create the agent
	agent1, err := r.Get("test-agent")
	if err != nil {
		t.Errorf("Get(test-agent) failed: %v", err)
	}
	if agent1 == nil {
		t.Error("expected non-nil agent")
	}
	
	// Second call should return the same agent
	agent2, err := r.Get("test-agent")
	if err != nil {
		t.Errorf("Get(test-agent) second call failed: %v", err)
	}
	if agent1 != agent2 {
		t.Error("expected same agent instance on second call")
	}
	
	// Test non-existent agent
	_, err = r.Get("non-existent")
	if err == nil {
		t.Error("expected error for non-existent agent")
	}
}

// Note: Session tests would require proper session initialization
// which depends on model-store being set up, so we'll test the basic flow
func TestRegistry_SessionMethods(t *testing.T) {
	r := createMockRegistry(t)
	
	// Test GetSession - may or may not succeed depending on model store setup
	_, err := r.GetSession("test-agent")
	if err != nil {
		// Expected when model store isn't properly configured
		t.Logf("GetSession failed as expected without model store: %v", err)
	} else {
		t.Log("GetSession succeeded (model store may be available)")
	}
	
	// Test CloseSession - should not panic even if session doesn't exist
	r.CloseSession("test-agent")
	
	// Test CloseAll - should not panic
	r.CloseAll()
}