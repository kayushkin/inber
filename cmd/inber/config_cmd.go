package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kayushkin/aiauth"
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

var configUserCmd = &cobra.Command{
	Use:   "user",
	Short: "Initialize user-level config at ~/.config/inber",
	Long: `Creates ~/.config/inber/.env with your API key.
This allows inber to work from any directory.`,
	Run: runConfigUser,
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configUserCmd)
}

func runConfigShow(cmd *cobra.Command, args []string) {
	fmt.Printf("%sConfiguration:%s\n\n", bold+blue, reset)
	
	store := aiauth.DefaultStore()
	key, keyErr := store.AnthropicKey()
	if keyErr == nil && key != "" {
		fmt.Printf("ANTHROPIC_API_KEY: %s\n", aiauth.MaskKey(key))
	} else {
		fmt.Printf("%sANTHROPIC_API_KEY: not set%s\n", red, reset)
	}
	
	// Show config file locations
	homeDir, err := os.UserHomeDir()
	if err == nil {
		userConfigPath := filepath.Join(homeDir, ".config", "inber", ".env")
		if _, err := os.Stat(userConfigPath); err == nil {
			fmt.Printf("User config: %s\n", userConfigPath)
		} else {
			fmt.Printf("User config: %snot found%s (run 'inber config user' to create)\n", dim, reset)
		}
	}
	
	repoRoot, err := FindRepoRoot()
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
	repoRoot, err := FindRepoRoot()
	if err != nil {
		Log.Error("not in a git repository")
		os.Exit(1)
	}

	// Create .inber directory
	inberDir := filepath.Join(repoRoot, ".inber")
	if err := os.MkdirAll(inberDir, 0755); err != nil {
		Log.Error("creating .inber: %v", err)
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
			Log.Error("creating agents.json: %v", err)
			os.Exit(1)
		}
		fmt.Printf("Created %s\n", agentsPath)
	}

	// Create agents directory
	agentsDir := filepath.Join(repoRoot, "agents")
	if err := os.MkdirAll(agentsDir, 0755); err != nil {
		Log.Error("creating agents/: %v", err)
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
			Log.Error("creating default.md: %v", err)
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
			Log.Error("creating .env: %v", err)
			os.Exit(1)
		}
		fmt.Printf("Created %s (remember to add your API key)\n", envPath)
	}

	fmt.Println("\nConfiguration initialized!")
}

func runConfigUser(cmd *cobra.Command, args []string) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		Log.Error("could not determine home directory: %v", err)
		os.Exit(1)
	}

	configDir := filepath.Join(homeDir, ".config", "inber")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		Log.Error("creating config directory: %v", err)
		os.Exit(1)
	}

	envPath := filepath.Join(configDir, ".env")
	
	// Check if file already exists
	if _, err := os.Stat(envPath); err == nil {
		fmt.Printf("Config file already exists at %s\n", envPath)
		fmt.Println("Edit it manually to update your API key.")
		return
	}

	// Prompt for API key
	fmt.Println("Enter your Anthropic API key (or press Enter to create empty file):")
	var apiKey string
	fmt.Scanln(&apiKey)

	var content string
	if apiKey != "" {
		content = fmt.Sprintf("# Anthropic API key for inber\nANTHROPIC_API_KEY=%s\n", apiKey)
	} else {
		content = "# Anthropic API key for inber\n# Get your key from: https://console.anthropic.com/\nANTHROPIC_API_KEY=your-key-here\n"
	}

	if err := os.WriteFile(envPath, []byte(content), 0600); err != nil { // 0600 = user read/write only
		Log.Error("creating config file: %v", err)
		os.Exit(1)
	}

	fmt.Printf("\n%sUser config created at:%s %s\n", green, reset, envPath)
	if apiKey == "" {
		fmt.Printf("\n%sRemember to edit the file and add your API key!%s\n", yellow, reset)
	} else {
		fmt.Printf("\n%sAPI key saved. You can now use inber from any directory.%s\n", green, reset)
	}
}
