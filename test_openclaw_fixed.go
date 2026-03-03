package main

import (
	"context"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent/registry"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Create registry
	client := anthropic.NewClient()
	reg, err := registry.New(&client, "/home/slava/life/repos/inber/agents", "/tmp/inber-test")
	if err != nil {
		fmt.Printf("❌ Failed to create registry: %v\n", err)
		return
	}

	// Try to spawn kayushkin agent (should route to OpenClaw)
	fmt.Println("Spawning kayushkin agent via OpenClaw...")
	result, err := reg.SpawnAndRun(ctx, "kayushkin", "Say hello")
	if err != nil {
		fmt.Printf("❌ Spawn failed: %v\n", err)
		return
	}

	fmt.Printf("✅ Result: %s\n", result.Text)
	fmt.Printf("Tokens: in=%d out=%d tools=%d\n", result.InputTokens, result.OutputTokens, result.ToolCalls)
}
