package server

import (
	"fmt"
	"os"

	"github.com/kayushkin/inber/logger"
)

// SelfTest runs startup checks and returns an error if critical systems are unavailable.
func (g *Server) SelfTest() error {
	log := logger.WithComponent("selftest")
	failed := 0

	// 1. NATS connectivity
	if g.bus == nil {
		log.Error("FAIL: NATS not connected", nil)
		failed++
	} else if err := g.bus.Publish("health.inber.startup", map[string]string{"status": "selftest"}); err != nil {
		log.Error("FAIL: NATS publish test", map[string]interface{}{"error": err})
		failed++
	} else {
		log.Info("PASS: NATS connected", nil)
	}

	// 2. Agent store readable
	if g.agentStore == nil {
		log.Error("FAIL: agent-store not available", nil)
		failed++
	} else {
		log.Info("PASS: agent-store available", nil)
	}

	// 3. At least one agent has a valid workspace
	workspaceOK := false
	for name, ac := range g.config.Agents {
		if ac.Workspace != "" {
			if _, err := os.Stat(ac.Workspace); err == nil {
				log.Info("PASS: workspace exists", map[string]interface{}{
					"agent":     name,
					"workspace": ac.Workspace,
				})
				workspaceOK = true
				break
			}
		}
	}
	if !workspaceOK {
		log.Warn("WARN: no agent has a valid workspace directory", nil)
	}

	// 4. Anthropic API key
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		log.Info("PASS: ANTHROPIC_API_KEY set", nil)
	} else {
		log.Warn("WARN: ANTHROPIC_API_KEY not set", nil)
	}

	if failed > 0 {
		return fmt.Errorf("selftest: %d critical check(s) failed", failed)
	}
	return nil
}
