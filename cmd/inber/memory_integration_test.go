package main

import (
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/memory"
	"github.com/kayushkin/inber/tools"
)

func TestMemoryToolsLoadedInDefaultToolSet(t *testing.T) {
	dir := setupTestRepo(t)

	store, err := memory.OpenOrCreate(dir)
	if err != nil {
		t.Fatalf("failed to open memory store: %v", err)
	}
	defer store.Close()

	// Simulate what chat.go and run.go do for default (no agent config) case
	agentTools := tools.All()
	agentTools = append(agentTools, memory.AllMemoryTools(store)...)

	// Verify memory tools are present
	memToolNames := map[string]bool{
		"memory_search": false,
		"memory_save":   false,
		"memory_expand": false,
		"memory_forget": false,
	}

	for _, t := range agentTools {
		if _, ok := memToolNames[t.Name]; ok {
			memToolNames[t.Name] = true
		}
	}

	for name, found := range memToolNames {
		if !found {
			t.Errorf("memory tool %q not found in tool set", name)
		}
	}
}

func TestMemoryToolsLoadedInScopedToolSet(t *testing.T) {
	dir := setupTestRepo(t)

	store, err := memory.OpenOrCreate(dir)
	if err != nil {
		t.Fatalf("failed to open memory store: %v", err)
	}
	defer store.Close()

	// Simulate scoped tool loading with memory tools in agent config
	configTools := []string{"shell", "memory_search", "memory_save"}
	var agentTools []struct{ Name string }

	for _, toolName := range configTools {
		for _, t := range tools.All() {
			if t.Name == toolName {
				agentTools = append(agentTools, struct{ Name string }{t.Name})
				break
			}
		}
	}
	// Memory tools from config
	for _, toolName := range configTools {
		if strings.HasPrefix(toolName, "memory_") {
			for _, t := range memory.AllMemoryTools(store) {
				if t.Name == toolName {
					agentTools = append(agentTools, struct{ Name string }{t.Name})
					break
				}
			}
		}
	}

	found := map[string]bool{}
	for _, t := range agentTools {
		found[t.Name] = true
	}

	if !found["shell"] {
		t.Error("expected shell tool")
	}
	if !found["memory_search"] {
		t.Error("expected memory_search tool")
	}
	if !found["memory_save"] {
		t.Error("expected memory_save tool")
	}
}

func TestSessionSummaryAutoSave(t *testing.T) {
	dir := setupTestRepo(t)

	store, err := memory.OpenOrCreate(dir)
	if err != nil {
		t.Fatalf("failed to open memory store: %v", err)
	}
	defer store.Close()

	// Create some fake messages
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("Help me refactor the database layer")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("Sure, I'll restructure the queries into a repository pattern.")),
		anthropic.NewUserMessage(anthropic.NewTextBlock("Great, also add connection pooling")),
	}

	saveSessionSummary(store, messages, "test-agent")

	// Verify it was saved
	results, err := store.Search("refactor database", 5)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected session summary to be saved")
	}

	found := false
	for _, m := range results {
		if strings.Contains(m.Content, "refactor") {
			found = true
			// Check tags
			hasSessionTag := false
			hasAgentTag := false
			for _, tag := range m.Tags {
				if tag == "session-summary" {
					hasSessionTag = true
				}
				if tag == "test-agent" {
					hasAgentTag = true
				}
			}
			if !hasSessionTag {
				t.Error("expected 'session-summary' tag")
			}
			if !hasAgentTag {
				t.Error("expected 'test-agent' tag")
			}
			if m.Importance != 0.4 {
				t.Errorf("expected importance 0.4, got %f", m.Importance)
			}
			break
		}
	}
	if !found {
		t.Error("session summary not found in search results")
	}
}

func TestSessionSummaryEmptyMessages(t *testing.T) {
	dir := setupTestRepo(t)

	store, err := memory.OpenOrCreate(dir)
	if err != nil {
		t.Fatalf("failed to open memory store: %v", err)
	}
	defer store.Close()

	// Should not panic with empty messages
	saveSessionSummary(store, nil, "test-agent")
	saveSessionSummary(store, []anthropic.MessageParam{}, "test-agent")
}

// TestMemoryInstructionsInContext removed — tested old context.AutoLoad path
