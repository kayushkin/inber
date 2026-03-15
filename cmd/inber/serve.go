package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/kayushkin/inber/agent/registry"
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

The server loads agent configs from agents.json / agent-store
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
	if busURL := os.Getenv("BUS_URL"); busURL != "" {
		cfg.BusURL = busURL
	}
	if busToken := os.Getenv("BUS_TOKEN"); busToken != "" {
		cfg.BusToken = busToken
	}

	g, err := server.New(cfg)
	if err != nil {
		return fmt.Errorf("create server: %w", err)
	}
	defer g.Close()

	// Handle shutdown signals.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("[server] received %s, shutting down...", sig)
		cancel()
	}()

	return g.Serve(ctx)
}

// buildConfigFromRegistry builds gateway config from the existing
// agents.json / agent-store system.
func buildConfigFromRegistry() server.Config {
	regCfg, fromStore := registry.LoadConfigWithFallback("", "")

	agents := make(map[string]server.AgentConfig)

	if regCfg != nil && regCfg.Agents != nil {
		// Also load agents.json for project field (not in agent-store).
		projectMap := loadAgentsJSONProjects()

		for name, ac := range regCfg.Agents {
			// Get project from agents.json (agent-store doesn't have it).
			project := projectMap[name]

			workspace := ""
			// Resolve workspace from project field, falling back to agent name.
			// This is the SOURCE repo — slots are resolved at spawn time via forge.
			home, _ := os.UserHomeDir()
			lookupName := name
			if project != "" {
				lookupName = project
			}
			candidate := filepath.Join(home, "life", "repos", lookupName)
			if _, err := os.Stat(candidate); err == nil {
				workspace = candidate
			}

			gac := server.AgentConfig{
				Name:      name,
				Project:   project,
				Workspace: workspace, // source repo (slots override this for spawns)
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
		log.Printf("[server] no agents found in registry, using defaults")
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
	log.Printf("[server] loaded %d agents from registry: %s (default: %s)",
		len(agents), strings.Join(agentNames, ", "), defaultAgent)

	return server.Config{
		Agents:       agents,
		DefaultAgent: defaultAgent,
	}
}

// loadAgentsJSONProjects reads agents.json and returns a map of agent name → project.
func loadAgentsJSONProjects() map[string]string {
	result := make(map[string]string)
	home, _ := os.UserHomeDir()

	// Try multiple locations for agents.json.
	paths := []string{
		filepath.Join(home, "life", "repos", "inber", "agents.json"),
		"agents.json",
	}

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var raw struct {
			Agents map[string]struct {
				Project string `json:"project"`
			} `json:"agents"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			continue
		}
		for name, cfg := range raw.Agents {
			if cfg.Project != "" {
				result[name] = cfg.Project
			}
		}
		break
	}
	return result
}
