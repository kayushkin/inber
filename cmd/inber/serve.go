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

	// Start bus listener in background (subscribes to inbound, routes to agents).
	go func() {
		if err := g.ListenBus(ctx); err != nil && ctx.Err() == nil {
			log.Printf("[server] bus listener stopped: %v", err)
		}
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
			info := projectMap[name]

			workspace := ""
			home, _ := os.UserHomeDir()
			lookupName := name
			if info.Project != "" {
				lookupName = info.Project
			}
			candidate := filepath.Join(home, "life", "repos", lookupName)
			if _, err := os.Stat(candidate); err == nil {
				workspace = candidate
			}

			gac := server.AgentConfig{
				Name:      name,
				Project:   info.Project,
				Projects:  info.Projects,
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

// agentProjectInfo holds project config from agents.json.
type agentProjectInfo struct {
	Project  string
	Projects []string
}

// loadAgentsJSONProjects reads agents.json and returns per-agent project info.
func loadAgentsJSONProjects() map[string]agentProjectInfo {
	result := make(map[string]agentProjectInfo)
	home, _ := os.UserHomeDir()

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
				Project  string   `json:"project"`
				Projects []string `json:"projects"`
			} `json:"agents"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			continue
		}
		for name, cfg := range raw.Agents {
			info := agentProjectInfo{
				Project:  cfg.Project,
				Projects: cfg.Projects,
			}
			// If projects list is empty, use project as single-item list.
			if len(info.Projects) == 0 && info.Project != "" {
				info.Projects = []string{info.Project}
			}
			result[name] = info
		}
		break
	}
	return result
}
