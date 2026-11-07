package server

import (
	"testing"

	"github.com/kayushkin/inber/agent/registry"
)

// The map is keyed on the slug and carries the display name and the stored
// workspace unchanged; an agent with no stored workspace gets an empty one.
func TestAgentConfigsFromRegistryKeysOnSlugAndKeepsStoredValues(t *testing.T) {
	registryConfig := &registry.RegistryConfig{Agents: map[string]*registry.AgentConfig{
		"claxon": {Name: "Claxon", Project: "orchestrator", Workspace: "/repos/orchestrator", Model: "model-a", Thinking: 2048, Tools: []string{"shell"}},
		"bran":   {Name: "Bran", Model: "model-b"},
	}}

	agents := AgentConfigsFromRegistry(registryConfig)

	if len(agents) != 2 {
		t.Fatalf("got %d agents, want 2: %#v", len(agents), agents)
	}
	claxon := agents["claxon"]
	if claxon.Name != "Claxon" || claxon.Project != "orchestrator" || claxon.Workspace != "/repos/orchestrator" ||
		claxon.Model != "model-a" || claxon.Thinking != 2048 || len(claxon.Tools) != 1 || claxon.Tools[0] != "shell" {
		t.Fatalf("claxon = %#v", claxon)
	}
	bran := agents["bran"]
	if bran.Name != "Bran" || bran.Workspace != "" {
		t.Fatalf("bran = %#v, want display name Bran and no workspace", bran)
	}
}
