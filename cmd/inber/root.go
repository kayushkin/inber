package main

import (
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/engine"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "inber [prompt]",
	Short: "inber - Go-based agent orchestration framework",
	Long: `inber is a Claude-powered agent framework with persistent memory,
context management, and multi-agent orchestration.

When called without a subcommand, acts as 'inber run' - sends a single message 
and prints the response.

Examples:
  inber "explain this error"
  inber run "one-off query"`,
	// Disable flag parsing errors for unknown subcommands
	// This allows "inber hello world" to be treated as args, not subcommands
	DisableFlagParsing: false,
	// Accept any number of args
	Args: cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		// Default to run command if no subcommand given
		runRun(cmd, args)
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		engine.Log.Error("%v", err)
		os.Exit(1)
	}
}

func init() {
	// Load .env files in priority order:
	// 1. Current directory .env (project-specific)
	// 2. ~/.config/inber/.env (user-specific)
	// 3. System environment variables (already available)
	
	homeDir, err := os.UserHomeDir()
	if err == nil {
		userConfigPath := filepath.Join(homeDir, ".config", "inber", ".env")
		_ = godotenv.Load(userConfigPath) // Ignore error if file doesn't exist
	}
	
	// Load local .env (overrides user config if present)
	_ = godotenv.Load()

	// Add run command flags to root command so `inber <prompt>` works
	rootCmd.Flags().StringVarP(&runModel, "model", "m", agent.DefaultModel, "Claude model to use")
	rootCmd.Flags().Int64VarP(&runThinking, "thinking", "t", 0, "Enable extended thinking with token budget (0=disabled)")
	rootCmd.Flags().StringVarP(&runAgent, "agent", "a", "", "Agent name to load from registry")
	rootCmd.Flags().BoolVar(&runRaw, "raw", false, "Skip context and memory loading")
	rootCmd.Flags().BoolVar(&runNoTools, "no-tools", false, "Disable all tools")
	rootCmd.Flags().StringVar(&runSystem, "system", "", "Override system prompt")
	rootCmd.Flags().BoolVarP(&runNew, "new", "n", false, "Start a new session instead of continuing the default")
	rootCmd.Flags().BoolVarP(&runDetach, "detach", "d", false, "Run in a one-off session without affecting the main session")
	// Add subcommands
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(agentsCmd)
	rootCmd.AddCommand(sessionsCmd)
	rootCmd.AddCommand(memoryCmd)
	rootCmd.AddCommand(modelsCmd)
	rootCmd.AddCommand(configCmd)
}
