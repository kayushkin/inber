package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/kayushkin/inber/agent/registry"
	"github.com/kayushkin/inber/logger"
	"github.com/kayushkin/inber/server"
	"github.com/spf13/cobra"
)

var (
	serveAddr string
	serveConfig string
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the inber server",
	Long: `Start the inber server daemon. Keeps agent sessions alive,
manages sub-agent spawning, and exposes an HTTP API.

The server loads agent configs from agent-store
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
	var cfg server.Config

	// Try loading from config file.
	if serveConfig != "" {
		loaded, err := server.LoadConfig(serveConfig)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		cfg = loaded
	} else {
		// Build config from existing agent registry.
		cfg = buildConfigFromRegistry()
	}

	cfg.ListenAddr = serveAddr

	// Wire bus integration from env vars.
	if natsURL := os.Getenv("NATS_URL"); natsURL != "" {
		cfg.NatsURL = natsURL
	} else if cfg.NatsURL == "" {
		cfg.NatsURL = "nats://localhost:4222"
	}
	if busToken := os.Getenv("BUS_TOKEN"); busToken != "" {
		cfg.BusToken = busToken
	}

	// Wire OpenClaw proxy from env vars.
	if ocURL := os.Getenv("OPENCLAW_URL"); ocURL != "" {
		cfg.OpenClawURL = ocURL
	}
	if ocToken := os.Getenv("OPENCLAW_TOKEN"); ocToken != "" {
		cfg.OpenClawToken = ocToken
	}

	g, err := server.New(cfg)
	if err != nil {
		return fmt.Errorf("create server: %w", err)
	}
	defer g.Close()

	if err := g.SelfTest(); err != nil {
		return fmt.Errorf("selftest: %w", err)
	}

	// Handle shutdown signals.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.WithComponent("server").Info("received shutdown signal", map[string]interface{}{
			"signal": sig.String(),
		})
		cancel()
	}()

	// Start bus listener in background (subscribes to inbound, routes to agents).
	go func() {
		if err := g.ListenBus(ctx); err != nil && ctx.Err() == nil {
			logger.WithComponent("server").Error("bus listener stopped", map[string]interface{}{
				"error": err,
			})
		}
	}()

	return g.Serve(ctx)
}

// buildConfigFromRegistry builds gateway config from agent-store.
func buildConfigFromRegistry() server.Config {
	regCfg, err := registry.LoadConfig()

	agents := make(map[string]server.AgentConfig)

	if err == nil && regCfg != nil && regCfg.Agents != nil {
		for name, ac := range regCfg.Agents {
			workspace := ac.Workspace
			if workspace == "" {
				// Try to resolve workspace from project name
				home, _ := os.UserHomeDir()
				lookupName := name
				if ac.Project != "" {
					lookupName = ac.Project
				}
				candidate := filepath.Join(home, "life", "repos", lookupName)
				if _, err := os.Stat(candidate); err == nil {
					workspace = candidate
				}
			}

			gac := server.AgentConfig{
				Name:      name,
				Project:   ac.Project,
				Projects:  ac.Projects,
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

	if len(agents) == 0 {
		logger.WithComponent("server").Warn("no agents found in registry, using defaults")
		agents["default"] = server.AgentConfig{
			Name:  "default",
			Model: "claude-sonnet-4-5-20250929",
		}
		defaultAgent = "default"
	}

	agentNames := make([]string, 0, len(agents))
	for name := range agents {
		agentNames = append(agentNames, name)
	}
	logger.WithComponent("server").Info("loaded agents from registry", map[string]interface{}{
		"count":        len(agents),
		"agents":       strings.Join(agentNames, ", "),
		"default":      defaultAgent,
	})

	return server.Config{
		Agents:       agents,
		DefaultAgent: defaultAgent,
	}
}

// removed: loadAgentsJSONProjects — agent-store is the source of truth
