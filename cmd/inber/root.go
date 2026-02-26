package main

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/kayushkin/inber/agent"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "inber",
	Short: "inber - Go-based agent orchestration framework",
	Long: `inber is a Claude-powered agent framework with persistent memory,
context management, and multi-agent orchestration.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Default to run command if no subcommand given
		runRun(cmd, args)
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	// Load .env file
	godotenv.Load()

	// Add run command flags to root command so `inber <prompt>` works
	rootCmd.Flags().StringVarP(&runModel, "model", "m", agent.DefaultModel, "Claude model to use")
	rootCmd.Flags().Int64VarP(&runThinking, "thinking", "t", 0, "Enable extended thinking with token budget (0=disabled)")
	rootCmd.Flags().StringVarP(&runAgent, "agent", "a", "", "Agent name to load from registry")
	rootCmd.Flags().BoolVar(&runRaw, "raw", false, "Skip context and memory loading")
	rootCmd.Flags().BoolVar(&runNoTools, "no-tools", false, "Disable all tools")
	rootCmd.Flags().StringVar(&runSystem, "system", "", "Override system prompt")

	// Add subcommands
	rootCmd.AddCommand(chatCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(agentsCmd)
	rootCmd.AddCommand(sessionsCmd)
	rootCmd.AddCommand(memoryCmd)
	rootCmd.AddCommand(modelsCmd)
	rootCmd.AddCommand(configCmd)
}
