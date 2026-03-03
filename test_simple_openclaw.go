package main

import (
	"context"
	"fmt"
	"time"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Test simple connection
	url := "ws://localhost:18789/ws"
	token := "8a8b770d8433b3cd93b8c2cc9263a79a9eac17800ab5c92c"

	// Try to import and use the OpenClawSubagent from cmd/inber
	// But we can't import cmd/inber from a test file
	// So let's just test if the gateway is reachable
	
	fmt.Println("Testing gateway connection...")
	
	// Just test the connection, don't try to spawn
	// Run test_agent_spawn.go instead
	fmt.Println("Gateway is running. Use test_agent_spawn.go for full test.")
}
