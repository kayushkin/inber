// Command inber-server starts the inber agent server daemon.
//
// Usage:
//
//	inber-server                  # default port :8200
//	inber-server --addr :9000     # custom port
//	inber-server --config gw.json # custom config file
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/kayushkin/inber/agent/registry"
	"github.com/kayushkin/inber/logger"
	"github.com/kayushkin/inber/server"
)

func main() {
	addr := flag.String("addr", ":8200", "API listen address")
	configFile := flag.String("config", "", "Config file (JSON)")
	requireChecks := flag.String("require", "", "Comma-separated allowlist of selftest checks that are fatal at startup (nats,agent-store,workspace,anthropic-key). Unlisted checks are demoted to WARN. Empty = legacy default (agent-store only).")
	apiKeyFromAuthStore := flag.String("api-key-from-auth-store", "", "If non-empty, resolve ANTHROPIC_API_KEY from auth-store (http://localhost:8303) at startup using this value as X-Auth-App. Requires AUTH_STORE_TOKEN env var.")
	flag.Parse()

	if err := run(*addr, *configFile, *requireChecks, *apiKeyFromAuthStore); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(addr, configFile, requireChecks, apiKeyApp string) error {
	if apiKeyApp != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := resolveAnthropicFromAuthStore(ctx, apiKeyApp); err != nil {
			return fmt.Errorf("resolve anthropic credential: %w", err)
		}
	}

	var cfg server.Config

	if configFile != "" {
		loaded, err := server.LoadConfig(configFile)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		cfg = loaded
	} else {
		cfg = buildConfigFromRegistry()
	}

	cfg.ListenAddr = addr

	if requireChecks != "" {
		var checks []string
		for _, name := range strings.Split(requireChecks, ",") {
			name = strings.TrimSpace(name)
			if name != "" {
				checks = append(checks, name)
			}
		}
		cfg.RequireChecks = checks
	}

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

	// Ensure single instance.
	pidFile := server.NewPIDFile(g.DataDir())
	if err := pidFile.Acquire(); err != nil {
		return fmt.Errorf("pid file: %w", err)
	}
	defer pidFile.Release()

	if err := g.SelfTest(); err != nil {
		return fmt.Errorf("selftest: %w", err)
	}

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

	go func() {
		if err := g.ListenBus(ctx); err != nil && ctx.Err() == nil {
			logger.WithComponent("server").Error("bus listener stopped", map[string]interface{}{
				"error": err,
			})
		}
	}()

	return g.Serve(ctx)
}

// buildConfigFromRegistry builds server config from agent-store.
func buildConfigFromRegistry() server.Config {
	regCfg, err := registry.LoadConfig()

	agents := make(map[string]server.AgentConfig)

	if err == nil && regCfg != nil && regCfg.Agents != nil {
		for name, ac := range regCfg.Agents {
			workspace := ac.Workspace
			if workspace == "" {
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

			agents[name] = server.AgentConfig{
				Name:      name,
				Project:   ac.Project,
				Projects:  ac.Projects,
				Workspace: workspace,
				Model:     ac.Model,
				Thinking:  ac.Thinking,
				Tools:     ac.Tools,
			}
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
		"count":   len(agents),
		"agents":  strings.Join(agentNames, ", "),
		"default": defaultAgent,
	})

	return server.Config{
		Agents:       agents,
		DefaultAgent: defaultAgent,
	}
}
