package server

import (
	"testing"

	toolstoretools "github.com/kayushkin/tool-store/tools"
)

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

// The tool connections reach every session's engine too. Before 2026-09-25 the
// tools package read them from the environment when the tool was built.
func TestEverySessionsEngineGetsTheServersToolConnections(t *testing.T) {
	connections := toolstoretools.OutsideServiceConnections{
		Pinchtab:    toolstoretools.PinchtabConnection{BaseURL: "http://pinchtab:1", Token: "pinchtab-token"},
		BraveAPIKey: "brave-key",
		Scheduler:   toolstoretools.SchedulerConnection{BaseURL: "http://scheduler:2", Token: "scheduler-token"},
	}
	g := &Server{config: Config{ToolConnections: connections}}

	cfg := g.engineConfigFor("bridge-test", "claxon", t.TempDir(), nil, AgentConfig{Name: "claxon"}, make(chan string))

	if cfg.ToolConnections != connections {
		t.Fatalf("engine got tool connections %+v, want %+v", cfg.ToolConnections, connections)
	}
}

// The inber server URL reaches every session's engine, which hands it to the
// spawn tool. Before 2026-09-25 the spawn tool read INBER_SERVER_URL itself.
func TestEverySessionsEngineGetsTheServersInberServerURL(t *testing.T) {
	g := &Server{config: Config{InberServerURL: "http://inber:8200"}}

	cfg := g.engineConfigFor("bridge-test", "claxon", t.TempDir(), nil, AgentConfig{Name: "claxon"}, make(chan string))

	if cfg.InberServerURL != "http://inber:8200" {
		t.Fatalf("engine got inber server URL %q", cfg.InberServerURL)
	}
}
