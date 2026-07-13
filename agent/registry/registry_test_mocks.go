package registry

import (
	"os"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/memory"
	"github.com/kayushkin/inber/session"
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

func (m *mockMemoryStore) TrackMemoryUsage(memoryID, sessionID string, turnNumber int, usageType string) error {
	return nil
}

func (m *mockMemoryStore) SearchFiltered(query string, limit int, tag string) ([]memory.Memory, error) {
	return nil, nil
}

func (m *mockMemoryStore) Close() error {
	return nil
}

func newMockMemoryStore() *mockMemoryStore {
	return &mockMemoryStore{
		memories: make(map[string]memory.Memory),
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
				Tools:    []string{"read_files"},
				Thinking: 10000,
			},
			"another-agent": {
				Name:   "another-agent", 
				Model:  "claude-3-sonnet-20240229",
				System: "You are another test agent",
				Tools:  []string{"write_files"},
			},
		},
		agents:   make(map[string]*agent.Agent),
		sessions: make(map[string]*session.Session),
		tools:    NewToolRegistry(),
	}
	
	return r
}