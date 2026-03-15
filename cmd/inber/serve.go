package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/kayushkin/inber/agent/registry"
	"github.com/kayushkin/inber/gateway"
	"github.com/spf13/cobra"
)

var (
	serveAddr string
	serveConfig string
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the gateway daemon",
	Long: `Start the inber gateway daemon. Keeps agent sessions alive,
manages sub-agent spawning, and exposes an HTTP API.

The gateway loads agent configs from agents.json / agent-store
and creates engines on demand.

Example:
  inber serve                    # default port :8200
  inber serve --addr :9000       # custom port
  inber serve --config gw.json   # custom config file`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runServe()
	},
}

func init() {
	serveCmd.Flags().StringVar(&serveAddr, "addr", ":8200", "API listen address")
	serveCmd.Flags().StringVar(&serveConfig, "config", "", "Config file (JSON)")
}

func runServe() error {
	var cfg gateway.Config

	// Try loading from config file.
	if serveConfig != "" {
		loaded, err := gateway.LoadConfig(serveConfig)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		cfg = loaded
	} else {
		// Build config from existing agent registry.
		cfg = buildConfigFromRegistry()
	}

	cfg.ListenAddr = serveAddr

	g, err := gateway.New(cfg)
	if err != nil {
		return fmt.Errorf("create gateway: %w", err)
	}
	defer g.Close()

	// Handle shutdown signals.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("[gateway] received %s, shutting down...", sig)
		cancel()
	}()

	return g.Serve(ctx)
}

// buildConfigFromRegistry builds gateway config from the existing
// agents.json / agent-store system.
func buildConfigFromRegistry() gateway.Config {
	regCfg, fromStore := registry.LoadConfigWithFallback("", "")

	agents := make(map[string]gateway.AgentConfig)

	if regCfg != nil && regCfg.Agents != nil {
		for name, ac := range regCfg.Agents {
			workspace := ""
			// Try to resolve workspace from agent store project field.
			// For now, use a convention: ~/life/repos/<name>
			home, _ := os.UserHomeDir()
			candidate := filepath.Join(home, "life", "repos", name)
			if _, err := os.Stat(candidate); err == nil {
				workspace = candidate
			}

			gac := gateway.AgentConfig{
				Name:      name,
				Workspace: workspace,
				Model:     ac.Model,
				Thinking:  ac.Thinking,
				Tools:     ac.Tools,
			}
			agents[name] = gac
		}
	}

	defaultAgent := ""
	if regCfg != nil {
		defaultAgent = regCfg.Default
	}

	_ = fromStore // suppress unused

	if len(agents) == 0 {
		log.Printf("[gateway] no agents found in registry, using defaults")
		agents["default"] = gateway.AgentConfig{
			Name:  "default",
			Model: "claude-sonnet-4-5-20250929",
		}
		defaultAgent = "default"
	}

	agentNames := make([]string, 0, len(agents))
	for name := range agents {
		agentNames = append(agentNames, name)
	}
	log.Printf("[gateway] loaded %d agents from registry: %s (default: %s)",
		len(agents), strings.Join(agentNames, ", "), defaultAgent)

	return gateway.Config{
		Agents:       agents,
		DefaultAgent: defaultAgent,
	}
}
