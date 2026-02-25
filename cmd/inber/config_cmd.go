package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kayushkin/inber/agent"
	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage configuration",
	Long:  `Show and initialize configuration.`,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current configuration",
	Run:   runConfigShow,
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Create default config file",
	Run:   runConfigInit,
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configInitCmd)
}

func runConfigShow(cmd *cobra.Command, args []string) {
	fmt.Printf("%sConfiguration:%s\n\n", bold+blue, reset)
	
	key := agent.APIKey()
	if key != "" {
		fmt.Printf("ANTHROPIC_API_KEY: %s...%s\n", key[:8], key[len(key)-4:])
	} else {
		fmt.Printf("%sANTHROPIC_API_KEY: not set%s\n", red, reset)
	}
	
	repoRoot, err := findRepoRoot()
	if err != nil {
		fmt.Printf("\nRepo root: %s(not in a git repository)%s\n", dim, reset)
	} else {
		fmt.Printf("\nRepo root: %s\n", repoRoot)
		
		// Check for agents.json
		agentsPath := filepath.Join(repoRoot, "agents.json")
		if _, err := os.Stat(agentsPath); err == nil {
			fmt.Printf("Agents config: %s\n", agentsPath)
		} else {
			fmt.Printf("Agents config: %snot found%s\n", dim, reset)
		}
		
		// Check for .inber directory
		inberDir := filepath.Join(repoRoot, ".inber")
		if info, err := os.Stat(inberDir); err == nil && info.IsDir() {
			fmt.Printf("Data directory: %s\n", inberDir)
		} else {
			fmt.Printf("Data directory: %snot initialized%s\n", dim, reset)
		}
	}
	
	fmt.Printf("\nDefault model: %s\n", agent.DefaultModel)
}

func runConfigInit(cmd *cobra.Command, args []string) {
	repoRoot, err := findRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: not in a git repository\n")
		os.Exit(1)
	}

	// Create .inber directory
	inberDir := filepath.Join(repoRoot, ".inber")
	if err := os.MkdirAll(inberDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating .inber: %v\n", err)
		os.Exit(1)
	}

	// Create agents.json if it doesn't exist
	agentsPath := filepath.Join(repoRoot, "agents.json")
	if _, err := os.Stat(agentsPath); os.IsNotExist(err) {
		example := `{
  "agents": [
    {
      "name": "default",
      "model": "claude-sonnet-4",
      "tools": [],
      "tags": ["general"]
    }
  ]
}
`
		if err := os.WriteFile(agentsPath, []byte(example), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "error creating agents.json: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Created %s\n", agentsPath)
	}

	// Create agents directory
	agentsDir := filepath.Join(repoRoot, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating agents/: %v\n", err)
		os.Exit(1)
	}

	// Create default agent identity
	defaultIdentity := filepath.Join(agentsDir, "default.md")
	if _, err := os.Stat(defaultIdentity); os.IsNotExist(err) {
		identity := `# Default Agent

You are a helpful coding assistant with access to tools for:
- Shell command execution
- File reading, writing, and editing
- Directory listing

Use these tools to help the user accomplish their tasks.
`
		if err := os.WriteFile(defaultIdentity, []byte(identity), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "error creating default.md: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Created %s\n", defaultIdentity)
	}

	// Create .env if it doesn't exist
	envPath := filepath.Join(repoRoot, ".env")
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		env := `# Anthropic API key
ANTHROPIC_API_KEY=your-key-here
`
		if err := os.WriteFile(envPath, []byte(env), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "error creating .env: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Created %s (remember to add your API key)\n", envPath)
	}

	fmt.Println("\nConfiguration initialized!")
}
