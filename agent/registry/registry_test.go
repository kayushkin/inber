package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

func TestLoadConfig(t *testing.T) {
	// Create temp config files
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create agents.json
	configJSON := map[string]interface{}{
		"agents": map[string]interface{}{
			"test-agent": map[string]interface{}{
				"name":     "test-agent",
				"role":     "test role",
				"model":    "claude-sonnet-4-5",
				"thinking": 2048,
				"tools":    []string{"read_file", "write_file"},
				"context": map[string]interface{}{
					"tags":   []string{"test"},
					"budget": 10000,
				},
			},
		},
	}

	configData, err := json.MarshalIndent(configJSON, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(tmpDir, "agents.json")
	if err := os.WriteFile(configPath, configData, 0644); err != nil {
		t.Fatal(err)
	}

	// Create markdown file
	mdPath := filepath.Join(agentsDir, "test-agent.md")
	mdContent := "# Test Agent\n\nThis is a test system prompt."
	if err := os.WriteFile(mdPath, []byte(mdContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Load config
	registryCfg, err := LoadConfig(configPath, agentsDir)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	cfg, ok := registryCfg.Agents["test-agent"]
	if !ok {
		t.Fatal("test-agent not found in configs")
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
	if cfg.System != mdContent {
		t.Errorf("expected system %q, got %q", mdContent, cfg.System)
	}
}

func TestLoadConfigDir(t *testing.T) {
	tmpDir := t.TempDir()
	agentsDir := filepath.Join(tmpDir, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create agents.json with multiple agents
	configJSON := map[string]interface{}{
		"agents": map[string]interface{}{
			"agent1": map[string]interface{}{
				"name":  "agent1",
				"role":  "role1",
				"tools": []string{"read_file"},
			},
			"agent2": map[string]interface{}{
				"name":  "agent2",
				"role":  "role2",
				"tools": []string{"write_file"},
			},
		},
	}

	configData, err := json.MarshalIndent(configJSON, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(tmpDir, "agents.json")
	if err := os.WriteFile(configPath, configData, 0644); err != nil {
		t.Fatal(err)
	}

	// Create markdown files
	if err := os.WriteFile(filepath.Join(agentsDir, "agent1.md"), []byte("system1"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentsDir, "agent2.md"), []byte("system2"), 0644); err != nil {
		t.Fatal(err)
	}

	registryCfg, err := LoadConfigDir(agentsDir)
	if err != nil {
		t.Fatalf("LoadConfigDir failed: %v", err)
	}

	if len(registryCfg.Agents) != 2 {
		t.Errorf("expected 2 configs, got %d", len(registryCfg.Agents))
	}

	if _, ok := registryCfg.Agents["agent1"]; !ok {
		t.Error("agent1 not loaded")
	}
	if _, ok := registryCfg.Agents["agent2"]; !ok {
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

	// Create agents.json
	configJSON := map[string]interface{}{
		"agents": map[string]interface{}{
			"test": map[string]interface{}{
				"name":  "test",
				"role":  "test",
				"tools": []string{"read_file"},
			},
		},
	}

	configData, err := json.MarshalIndent(configJSON, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(tmpDir, "agents.json")
	if err := os.WriteFile(configPath, configData, 0644); err != nil {
		t.Fatal(err)
	}

	// Create markdown file
	mdPath := filepath.Join(configDir, "test.md")
	if err := os.WriteFile(mdPath, []byte("test system"), 0644); err != nil {
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
