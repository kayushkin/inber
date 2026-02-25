package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/kayushkin/inber/agent"
)

// Config holds parsed CLI flags and validated settings.
type Config struct {
	Model          string
	Key            string
	ThinkingBudget int64 // 0 = disabled
}

// ParseConfig parses CLI flags, loads .env, and validates.
// Returns nil if the program should exit (e.g. --list-models).
func ParseConfig() *Config {
	modelFlag := flag.String("model", agent.DefaultModel, "Claude model to use")
	thinkingFlag := flag.Int64("thinking", 0, "Enable extended thinking with token budget (min 1024, 0=disabled)")
	listModels := flag.Bool("list-models", false, "List available models and exit")
	flag.Parse()

	if *listModels {
		fmt.Println("Available models:")
		for id, info := range agent.Models {
			fmt.Printf("  %-30s  ctx=%dk  in=$%.2f/1M  out=$%.2f/1M\n",
				id, info.ContextWindow/1000, info.InputCostPer1M, info.OutputCostPer1M)
		}
		return nil
	}

	godotenv.Load()

	key := agent.APIKey()
	if key == "" {
		fmt.Fprintln(os.Stderr, "ANTHROPIC_API_KEY not set")
		os.Exit(1)
	}

	if _, ok := agent.Models[*modelFlag]; !ok {
		fmt.Fprintf(os.Stderr, "unknown model: %s\nUse --list-models to see available models.\n", *modelFlag)
		os.Exit(1)
	}

	return &Config{
		Model:          *modelFlag,
		Key:            key,
		ThinkingBudget: *thinkingFlag,
	}
}
