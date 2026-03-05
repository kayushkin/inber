package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kayushkin/inber/agent/registry"
	"github.com/kayushkin/inber/engine"
	"github.com/spf13/cobra"
)

var agentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "Manage agent configurations",
	Long:  `List, show, and manage configured agents.`,
}

var agentsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configured agents",
	Run:   runAgentsList,
}

var agentsShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show agent configuration and identity",
	Args:  cobra.ExactArgs(1),
	Run:   runAgentsShow,
}

func init() {
	agentsCmd.AddCommand(agentsListCmd)
	agentsCmd.AddCommand(agentsShowCmd)
}

func runAgentsList(cmd *cobra.Command, args []string) {
	repoRoot, _ := engine.FindRepoRoot()
	if repoRoot == "" {
		repoRoot, _ = os.Getwd()
	}

	cfg, err := registry.LoadConfig(
		filepath.Join(repoRoot, "agents.json"),
		filepath.Join(repoRoot, "agents"),
	)
	if err != nil {
		engine.Log.Error("loading agents: %v", err)
		os.Exit(1)
	}

	if len(cfg.Agents) == 0 {
		fmt.Println("No agents configured.")
		return
	}

	fmt.Printf("Configured agents (%d):\n\n", len(cfg.Agents))
	if cfg.Default != "" {
		fmt.Printf("  Default: %s%s%s\n\n", engine.Bold+engine.Blue, cfg.Default, engine.Reset)
	}
	for name, agentCfg := range cfg.Agents {
		model := agentCfg.Model
		if model == "" {
			model = "(default)"
		}
		tools := fmt.Sprintf("%d tools", len(agentCfg.Tools))
		if len(agentCfg.Tools) == 0 {
			tools = "all tools"
		}
		defaultMarker := ""
		if name == cfg.Default {
			defaultMarker = " " + engine.Green + "*" + engine.Reset
		}
		fmt.Printf("  %s%-20s%s%s  model: %-25s  %s\n", engine.Bold, name, engine.Reset, defaultMarker, model, tools)
	}
}

func runAgentsShow(cmd *cobra.Command, args []string) {
	agentName := args[0]
	
	repoRoot, _ := engine.FindRepoRoot()
	if repoRoot == "" {
		repoRoot, _ = os.Getwd()
	}

	registryCfg, err := registry.LoadConfig(
		filepath.Join(repoRoot, "agents.json"),
		filepath.Join(repoRoot, "agents"),
	)
	if err != nil {
		engine.Log.Error("loading agents: %v", err)
		os.Exit(1)
	}

	agentCfg, ok := registryCfg.Agents[agentName]
	if !ok {
		engine.Log.Error("agent not found: %s", agentName)
		os.Exit(1)
	}

	fmt.Printf("%s%s%s", engine.Bold+engine.Blue, agentCfg.Name, engine.Reset)
	if agentName == registryCfg.Default {
		fmt.Printf(" %s(default)%s", engine.Green, engine.Reset)
	}
	fmt.Println()
	
	fmt.Printf("Model: %s\n", agentCfg.Model)
	
	if len(agentCfg.Tools) > 0 {
		fmt.Printf("Tools: %s\n", strings.Join(agentCfg.Tools, ", "))
	} else {
		fmt.Printf("Tools: all\n")
	}
	
	if agentCfg.Thinking > 0 {
		fmt.Printf("Thinking: %d tokens\n", agentCfg.Thinking)
	}
	
	fmt.Printf("\nIdentity:\n%s\n", engine.Dim+"---"+engine.Reset)
	fmt.Println(agentCfg.System)
	fmt.Printf("%s\n", engine.Dim+"---"+engine.Reset)
}
