package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kayushkin/inber/agent/registry"
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
	repoRoot, _ := findRepoRoot()
	if repoRoot == "" {
		repoRoot, _ = os.Getwd()
	}

	configs, err := registry.LoadConfig(
		filepath.Join(repoRoot, "agents.json"),
		filepath.Join(repoRoot, "agents"),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading agents: %v\n", err)
		os.Exit(1)
	}

	if len(configs) == 0 {
		fmt.Println("No agents configured.")
		return
	}

	fmt.Printf("Configured agents (%d):\n\n", len(configs))
	for name, cfg := range configs {
		model := cfg.Model
		if model == "" {
			model = "(default)"
		}
		tools := fmt.Sprintf("%d tools", len(cfg.Tools))
		if len(cfg.Tools) == 0 {
			tools = "all tools"
		}
		fmt.Printf("  %s%-20s%s  model: %-25s  %s\n", bold, name, reset, model, tools)
	}
}

func runAgentsShow(cmd *cobra.Command, args []string) {
	agentName := args[0]
	
	repoRoot, _ := findRepoRoot()
	if repoRoot == "" {
		repoRoot, _ = os.Getwd()
	}

	configs, err := registry.LoadConfig(
		filepath.Join(repoRoot, "agents.json"),
		filepath.Join(repoRoot, "agents"),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading agents: %v\n", err)
		os.Exit(1)
	}

	cfg, ok := configs[agentName]
	if !ok {
		fmt.Fprintf(os.Stderr, "agent not found: %s\n", agentName)
		os.Exit(1)
	}

	fmt.Printf("%s%s%s\n", bold+blue, cfg.Name, reset)
	fmt.Printf("Model: %s\n", cfg.Model)
	
	if len(cfg.Tools) > 0 {
		fmt.Printf("Tools: %s\n", strings.Join(cfg.Tools, ", "))
	} else {
		fmt.Printf("Tools: all\n")
	}
	
	if cfg.Thinking > 0 {
		fmt.Printf("Thinking: %d tokens\n", cfg.Thinking)
	}
	
	fmt.Printf("\nIdentity:\n%s\n", dim+"---"+reset)
	fmt.Println(cfg.System)
	fmt.Printf("%s\n", dim+"---"+reset)
}
