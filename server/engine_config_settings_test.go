package server

import "testing"

// The three settings the command reads for the engine reach every session's
// engine configuration. Before 2026-09-24 the engine read them from the
// environment itself, so a server that forgot to pass them on would have
// dropped logstack and the agent-store path without a sound.
func TestEverySessionsEngineGetsTheServersAgentStoreLogstackAndBlueprint(t *testing.T) {
	g := &Server{config: Config{AgentStorePath: "/tmp/agents.db", LogstackURL: "http://localhost:8088", Blueprint: true}}

	cfg := g.engineConfigFor("bridge-test", "claxon", t.TempDir(), nil, AgentConfig{Name: "claxon"}, make(chan string))

	if cfg.AgentStorePath != "/tmp/agents.db" || cfg.LogstackURL != "http://localhost:8088" || !cfg.Blueprint {
		t.Fatalf("engine got agent-store %q, logstack %q, blueprint %v", cfg.AgentStorePath, cfg.LogstackURL, cfg.Blueprint)
	}
}
