package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/kayushkin/inber/agent/registry"
)

func main() {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		log.Fatal("ANTHROPIC_API_KEY not set")
	}

	// Create registry
	client := anthropic.NewClient(option.WithAPIKey(apiKey))
	reg, err := registry.New(&client, "../../agents", "../../logs")
	if err != nil {
		log.Fatal(err)
	}
	defer reg.CloseAll()

	// List available agents
	agents := reg.List()
	fmt.Println("Available agents:", agents)

	// Use the coder agent
	fmt.Println("\n--- Using coder agent ---")
	runAgent(reg, "coder", "List all .go files in the current directory")

	// Use the researcher agent
	fmt.Println("\n--- Using researcher agent ---")
	runAgent(reg, "researcher", "Read the README.md file and summarize it")
}

func runAgent(reg *registry.Registry, agentName, task string) {
	// Get agent and config
	agent, err := reg.Get(agentName)
	if err != nil {
		log.Printf("Error getting agent %s: %v", agentName, err)
		return
	}

	cfg, err := reg.GetConfig(agentName)
	if err != nil {
		log.Printf("Error getting config: %v", err)
		return
	}

	// Get session for logging
	sess, err := reg.GetSession(agentName)
	if err != nil {
		log.Printf("Warning: logging disabled: %v", err)
	} else {
		agent.SetHooks(sess.Hooks())
		sess.LogUser(task)
	}

	// Build messages
	var messages []anthropic.MessageParam
	messages = append(messages,
		anthropic.NewUserMessage(anthropic.NewTextBlock(task)))

	// Run the agent
	fmt.Printf("[%s] Running task: %s\n", agentName, task)
	result, err := agent.Run(context.Background(), cfg.Model, &messages)
	if err != nil {
		log.Printf("Error: %v", err)
		return
	}

	// Log result
	if sess != nil {
		sess.LogAssistant(result.Text, result.InputTokens, result.OutputTokens, result.ToolCalls)
	}

	// Display results
	fmt.Printf("\n[%s] Response:\n%s\n", agentName, result.Text)
	fmt.Printf("\n[%s] Stats: in=%d out=%d tools=%d\n",
		agentName, result.InputTokens, result.OutputTokens, result.ToolCalls)
}
