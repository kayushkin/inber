package server

import (
	"fmt"
	"os"

	"github.com/kayushkin/inber/logger"
)

// Selftest check names. Keep in sync with the --require flag in
// cmd/inber-server/main.go and docs/oneshot-deployment.md.
const (
	checkNATS         = "nats"
	checkAgentStore   = "agent-store"
	checkWorkspace    = "workspace"
	checkAnthropicKey = "anthropic-key"
)

// SelfTest runs startup checks and returns an error if critical systems are unavailable.
//
// A check is "critical" (FAIL on failure) iff its name appears in
// g.config.RequireChecks. If RequireChecks is nil/empty, the legacy behavior
// applies: agent-store is the only critical check.
func (g *Server) SelfTest() error {
	log := logger.WithComponent("selftest")
	critical := g.criticalCheckSet()
	failed := 0

	report := func(name, msg string, ok bool, fields map[string]interface{}) {
		if ok {
			log.Info("PASS: "+msg, fields)
			return
		}
		if critical[name] {
			log.Error("FAIL: "+msg, fields)
			failed++
		} else {
			log.Warn("WARN: "+msg, fields)
		}
	}

	// 1. NATS connectivity.
	switch {
	case g.bus == nil:
		report(checkNATS, "NATS not connected — bus event publishing disabled", false, nil)
	default:
		if err := g.bus.Publish("health.inber.startup", map[string]string{"status": "selftest"}); err != nil {
			report(checkNATS, "NATS publish failed — bus event publishing disabled", false, map[string]interface{}{"error": err})
		} else {
			report(checkNATS, "NATS connected", true, nil)
		}
	}

	// 2. Agent store readable.
	report(checkAgentStore, "agent-store available", g.agentStore != nil, nil)

	// 3. At least one agent has a valid workspace.
	workspaceOK := false
	for name, ac := range g.config.Agents {
		if ac.Workspace == "" {
			continue
		}
		if _, err := os.Stat(ac.Workspace); err != nil {
			continue
		}
		log.Info("PASS: workspace exists", map[string]interface{}{
			"agent":     name,
			"workspace": ac.Workspace,
		})
		workspaceOK = true
		break
	}
	if !workspaceOK {
		report(checkWorkspace, "no agent has a valid workspace directory", false, nil)
	}

	// 4. Anthropic API key.
	report(checkAnthropicKey, "ANTHROPIC_API_KEY set", os.Getenv("ANTHROPIC_API_KEY") != "", nil)

	if failed > 0 {
		return fmt.Errorf("selftest: %d critical check(s) failed", failed)
	}
	return nil
}

// criticalCheckSet returns the set of check names that are fatal at startup.
// If g.config.RequireChecks is nil/empty, the legacy default (agent-store) applies.
func (g *Server) criticalCheckSet() map[string]bool {
	if len(g.config.RequireChecks) == 0 {
		return map[string]bool{checkAgentStore: true}
	}
	out := make(map[string]bool, len(g.config.RequireChecks))
	for _, name := range g.config.RequireChecks {
		out[name] = true
	}
	return out
}
