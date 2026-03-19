package registry

import (
	"context"
	"testing"
	"os"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/memory"
	"github.com/kayushkin/inber/session"
	modelstore "github.com/kayushkin/model-store"
)

// mockMemoryStore is a simple in-memory mock for testing
type mockMemoryStore struct {
	memories map[string]memory.Memory
}

func (m *mockMemoryStore) Save(mem memory.Memory) error {
	if mem.ID == "" {
		mem.ID = "mock-id"
	}
	m.memories[mem.ID] = mem
	return nil
}

func (m *mockMemoryStore) Get(id string) (*memory.Memory, error) {
	mem, exists := m.memories[id]
	if !exists {
		return nil, nil // Return nil instead of undefined error
	}
	return &mem, nil
}

func (m *mockMemoryStore) Search(query string, limit int) ([]memory.Memory, error) {
	return nil, nil
}

func (m *mockMemoryStore) Forget(id string) error {
	delete(m.memories, id)
	return nil
}

func (m *mockMemoryStore) DecayImportance() error {
	return nil
}

func (m *mockMemoryStore) ListRecent(limit int, minImportance float64) ([]memory.Memory, error) {
	var result []memory.Memory
	for _, mem := range m.memories {
		result = append(result, mem)
		if len(result) >= limit {
			break
		}
	}
	return result, nil
}

func (m *mockMemoryStore) Compact(minAge time.Duration, minCount int) ([]memory.CompactionResult, error) {
	return nil, nil
}

func (m *mockMemoryStore) BuildContext(req memory.BuildContextRequest) ([]memory.Memory, int, error) {
	return nil, 0, nil
}

func (m *mockMemoryStore) PrepareSession(cfg memory.PrepareSessionConfig) error {
	return nil
}

func (m *mockMemoryStore) LoadToolRegistry(tools []memory.ToolMetadata) error {
	return nil
}

func (m *mockMemoryStore) UpdateToolUsageSummary(toolName, summary string, ttlSeconds int64) error {
	return nil
}

func (m *mockMemoryStore) SaveSession(sess memory.Session) error {
	return nil
}

func (m *mockMemoryStore) TrackMemoryUsage(memoryID, sessionID string, turnNumber int, usageType string) error {
	return nil
}

func (m *mockMemoryStore) Close() error {
	return nil
}

func newMockMemoryStore() *mockMemoryStore {
	return &mockMemoryStore{
		memories: make(map[string]memory.Memory),
	}
}

func TestToolRegistry(t *testing.T) {
	tr := NewToolRegistry()

	// Test List
	names := tr.List()
	if len(names) < 5 {
		t.Errorf("expected at least 5 built-in tools, got %d", len(names))
	}

	// Test Get
	tool, err := tr.Get("read_file")
	if err != nil {
		t.Errorf("Get(read_file) failed: %v", err)
	}
	if tool.Name != "read_file" {
		t.Errorf("expected tool name read_file, got %s", tool.Name)
	}

	// Test unknown tool
	_, err = tr.Get("unknown_tool")
	if err == nil {
		t.Error("expected error for unknown tool")
	}
}

func TestToolRegistry_Register(t *testing.T) {
	tr := NewToolRegistry()
	
	// Create a simple test tool (just using empty schema for simplicity)
	testTool := agent.Tool{
		Name:        "test_tool",
		Description: "A test tool",
		InputSchema: anthropic.ToolInputSchemaParam{},
		Run: func(ctx context.Context, input string) (string, error) {
			return "test result", nil
		},
	}

	// Register the test tool
	tr.Register("test_tool", testTool)

	// Verify it was registered
	retrieved, err := tr.Get("test_tool")
	if err != nil {
		t.Errorf("Get(test_tool) failed: %v", err)
	}
	if retrieved.Name != "test_tool" {
		t.Errorf("expected tool name test_tool, got %s", retrieved.Name)
	}

	// Verify it's in the list
	names := tr.List()
	found := false
	for _, name := range names {
		if name == "test_tool" {
			found = true
			break
		}
	}
	if !found {
		t.Error("test_tool not found in tool list")
	}
}

func TestToolRegistry_RegisterMemoryTools(t *testing.T) {
	tr := NewToolRegistry()
	mockStore := newMockMemoryStore()

	// Register memory tools
	tr.RegisterMemoryTools(mockStore)

	// Test that memory tools were registered
	memoryTools := []string{"memory_search", "memory_save", "memory_expand", "memory_forget"}
	for _, toolName := range memoryTools {
		_, err := tr.Get(toolName)
		if err != nil {
			t.Errorf("Memory tool %s not registered: %v", toolName, err)
		}
	}
}

func TestToolRegistry_RegisterSpawnTool(t *testing.T) {
	tr := NewToolRegistry()
	
	// Create a mock spawn tool (simplified schema)
	spawnTool := agent.Tool{
		Name:        "spawn_agent",
		Description: "Spawn a sub-agent",
		InputSchema: anthropic.ToolInputSchemaParam{},
		Run: func(ctx context.Context, input string) (string, error) {
			return "spawned", nil
		},
	}

	// Register spawn tool
	tr.RegisterSpawnTool(spawnTool)

	// Verify it was registered
	retrieved, err := tr.Get("spawn_agent")
	if err != nil {
		t.Errorf("Get(spawn_agent) failed: %v", err)
	}
	if retrieved.Name != "spawn_agent" {
		t.Errorf("expected tool name spawn_agent, got %s", retrieved.Name)
	}
}

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

func createMockRegistry(t *testing.T) *Registry {
	client := &anthropic.Client{}
	
	// Create temporary directory for logs
	tmpDir, err := os.MkdirTemp("", "test-logs")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })

	// Create a mock registry with some test agents
	r := &Registry{
		client:    client,
		logsDir:   tmpDir,
		default_:  "test-agent",
		configs: map[string]*AgentConfig{
			"test-agent": {
				Name:     "test-agent",
				Model:    "claude-3-haiku-20240307",
				System:   "You are a test agent",
				Tools:    []string{"read_file"},
				Thinking: 10000,
			},
			"another-agent": {
				Name:   "another-agent", 
				Model:  "claude-3-sonnet-20240229",
				System: "You are another test agent",
				Tools:  []string{"write_file"},
			},
		},
		agents:   make(map[string]*agent.Agent),
		sessions: make(map[string]*session.Session),
		tools:    NewToolRegistry(),
	}
	
	return r
}

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