package main

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "inber",
	Short: "inber - Go-based agent orchestration framework",
	Long: `inber is a Claude-powered agent framework with persistent memory,
context management, and multi-agent orchestration.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Default to chat command if no subcommand given
		chatCmd.Run(cmd, args)
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

	// Add subcommands
	rootCmd.AddCommand(chatCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(agentsCmd)
	rootCmd.AddCommand(sessionsCmd)
	rootCmd.AddCommand(memoryCmd)
	rootCmd.AddCommand(modelsCmd)
	rootCmd.AddCommand(configCmd)
}
