package registry

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

func TestLoadConfig(t *testing.T) {
	// Create temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-agent.yaml")

	configYAML := `
name: test-agent
role: "test role"
system: "test system prompt"
model: claude-sonnet-4-5
thinking: 2048
tools:
  - read_file
  - write_file
context:
  tags:
    - test
  budget: 10000
`

	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Name != "test-agent" {
		t.Errorf("expected name test-agent, got %s", cfg.Name)
	}
	if cfg.Model != "claude-sonnet-4-5" {
		t.Errorf("expected model claude-sonnet-4-5, got %s", cfg.Model)
	}
	if cfg.Thinking != 2048 {
		t.Errorf("expected thinking 2048, got %d", cfg.Thinking)
	}
	if len(cfg.Tools) != 2 {
		t.Errorf("expected 2 tools, got %d", len(cfg.Tools))
	}
	if len(cfg.Context.Tags) != 1 || cfg.Context.Tags[0] != "test" {
		t.Errorf("expected tags [test], got %v", cfg.Context.Tags)
	}
}

func TestLoadConfigDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple config files
	configs := map[string]string{
		"agent1.yaml": `
name: agent1
role: "role1"
system: "system1"
tools:
  - read_file
`,
		"agent2.yaml": `
name: agent2
role: "role2"
system: "system2"
tools:
  - write_file
`,
	}

	for filename, content := range configs {
		path := filepath.Join(tmpDir, filename)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	loaded, err := LoadConfigDir(tmpDir)
	if err != nil {
		t.Fatalf("LoadConfigDir failed: %v", err)
	}

	if len(loaded) != 2 {
		t.Errorf("expected 2 configs, got %d", len(loaded))
	}

	if _, ok := loaded["agent1"]; !ok {
		t.Error("agent1 not loaded")
	}
	if _, ok := loaded["agent2"]; !ok {
		t.Error("agent2 not loaded")
	}
}

func TestRegistryBasics(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "agents")
	logsDir := filepath.Join(tmpDir, "logs")

	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a test config
	configPath := filepath.Join(configDir, "test.yaml")
	configYAML := `
name: test
role: "test"
system: "test system"
tools:
  - read_file
`
	if err := os.WriteFile(configPath, []byte(configYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Create registry (without real API key)
	client := anthropic.NewClient(option.WithAPIKey("test-key"))
	reg, err := New(&client, configDir, logsDir)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	// Test List
	names := reg.List()
	if len(names) != 1 || names[0] != "test" {
		t.Errorf("expected [test], got %v", names)
	}

	// Test GetConfig
	cfg, err := reg.GetConfig("test")
	if err != nil {
		t.Errorf("GetConfig failed: %v", err)
	}
	if cfg.Name != "test" {
		t.Errorf("expected name test, got %s", cfg.Name)
	}

	// Test Get (creates agent)
	agent, err := reg.Get("test")
	if err != nil {
		t.Errorf("Get failed: %v", err)
	}
	if agent == nil {
		t.Error("expected agent, got nil")
	}

	// Test GetContext
	ctx, err := reg.GetContext("test")
	if err != nil {
		t.Errorf("GetContext failed: %v", err)
	}
	if ctx == nil {
		t.Error("expected context, got nil")
	}

	// Test unknown agent
	_, err = reg.Get("unknown")
	if err == nil {
		t.Error("expected error for unknown agent")
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
