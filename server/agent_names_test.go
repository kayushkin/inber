package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// eighteenAgents is the size of the real registry on this host at the time this
// was written. It matters: the defect these tests pin is a map range, and a map
// range over three keys repeats itself often enough to pass by luck.
func eighteenAgents() map[string]AgentConfig {
	names := []string{
		"argraphments", "bench", "bile", "bran", "brigid", "claxon",
		"dagda", "etain", "fionn", "goibniu", "healthcheck", "inber-party",
		"keyboard", "lugh", "manannan", "ogma", "oisin", "scathach",
	}
	agents := make(map[string]AgentConfig, len(names))
	for _, n := range names {
		agents[n] = AgentConfig{Name: n, Model: "test-model"}
	}
	return agents
}

// The spawn_agent description is a tool description, and agent/agent_run.go
// puts the cache_control breakpoint on the last tool definition — so these bytes
// key the whole cached prefix. Two sessions of the same agent, a fork and its
// parent, and a session and its resume each build this string separately; if it
// is not the same string, none of them can share a cache entry.
func TestSpawnAgentToolDescriptionDoesNotChangeBetweenBuilds(t *testing.T) {
	srv := &Server{config: Config{Agents: eighteenAgents()}}

	first := srv.SpawnAgentTool("session:1").InputSchema.Properties.(map[string]any)["agent"].(map[string]any)["description"].(string)
	for i := 0; i < 200; i++ {
		got := srv.SpawnAgentTool("session:2").InputSchema.Properties.(map[string]any)["agent"].(map[string]any)["description"].(string)
		if got != first {
			t.Fatalf("spawn_agent's description changed between two builds of the same config.\n"+
				"That re-keys the tools block, and with it every cached segment after it.\nbuild 1: %s\nbuild %d: %s",
				first, i+2, got)
		}
	}

	// And it must name every agent, or a stable string could be stable because
	// it is empty.
	for name := range srv.config.Agents {
		if !strings.Contains(first, name) {
			t.Errorf("description omits agent %q: %s", name, first)
		}
	}
}

// GET /api/agents is not just a UI listing: agent/registry/spawn_tool.go fetches
// it and renders the names straight into the CLI's own spawn_agent description.
// A body whose order changes per request is the same cache defect, arriving over
// HTTP.
func TestAgentsListingDoesNotChangeBetweenRequests(t *testing.T) {
	srv := &Server{config: Config{Agents: eighteenAgents()}}

	names := func() []string {
		rec := httptest.NewRecorder()
		srv.handleAgents(rec, httptest.NewRequest(http.MethodGet, "/api/agents", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
		var got []registryAgent
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		out := make([]string, len(got))
		for i, a := range got {
			out[i] = a.Name
		}
		return out
	}

	first := strings.Join(names(), ",")
	for i := 0; i < 200; i++ {
		if got := strings.Join(names(), ","); got != first {
			t.Fatalf("/api/agents returned a different order on request %d.\nfirst: %s\nnow:   %s", i+2, first, got)
		}
	}
	if len(strings.Split(first, ",")) != len(srv.config.Agents) {
		t.Fatalf("listing dropped agents: %s", first)
	}
}

// The default agent is not a cosmetic field. toolsForAgent compares against it
// to decide which session is the orchestrator, and the orchestrator is the one
// that gets merge_workspace, reject_workspace and fix_workspace. Picking it out
// of a map range meant a different agent held those tools after every restart.
func TestDefaultAgentFallbackPicksTheSameAgentEveryTime(t *testing.T) {
	for i := 0; i < 200; i++ {
		cfg := Config{Agents: eighteenAgents()}
		if guessed := resolveDefaultAgent(&cfg); !guessed {
			t.Fatal("resolveDefaultAgent filled the field without reporting that it guessed")
		}
		if cfg.DefaultAgent != "argraphments" {
			t.Fatalf("fallback default agent changed on run %d: %s", i+1, cfg.DefaultAgent)
		}
	}
}

// A default the operator did configure must survive untouched — the fallback is
// a last resort, not a normaliser.
func TestConfiguredDefaultAgentIsNotOverridden(t *testing.T) {
	cfg := Config{DefaultAgent: "claxon", Agents: eighteenAgents()}
	if guessed := resolveDefaultAgent(&cfg); guessed {
		t.Error("reported a guess for a default that was configured")
	}
	srv := &Server{config: cfg}
	if srv.config.DefaultAgent != "claxon" {
		t.Fatalf("configured default was replaced: %s", srv.config.DefaultAgent)
	}
	tools := srv.toolsForAgent("session:1", "claxon")
	var hasMerge bool
	for _, tl := range tools {
		if tl.Name == "merge_workspace" {
			hasMerge = true
		}
	}
	if !hasMerge {
		t.Error("the configured default agent is not being treated as the orchestrator")
	}
}
