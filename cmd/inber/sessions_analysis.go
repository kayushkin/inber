package main

import (
	"github.com/kayushkin/inber/internal/fsutil"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kayushkin/inber/engine"
	"github.com/kayushkin/inber/session"
	"github.com/spf13/cobra"
)

var sessionsContextCmd = &cobra.Command{
	Use:   "context <id>",
	Short: "Show context used in a session",
	Args:  cobra.ExactArgs(1),
	Run:   runSessionsContext,
}

var sessionsPromptsCmd = &cobra.Command{
	Use:   "prompts <id>",
	Short: "List prompt breakdowns for a session",
	Args:  cobra.ExactArgs(1),
	Run:   runSessionsPrompts,
}

var sessionsPromptCmd = &cobra.Command{
	Use:   "prompt <id> <turn>",
	Short: "Show a specific prompt breakdown",
	Args:  cobra.ExactArgs(2),
	Run:   runSessionsPrompt,
}

func runSessionsContext(cmd *cobra.Command, args []string) {
	sessionID := args[0]
	
	repoRoot, _ := fsutil.FindRepoRoot()
	if repoRoot == "" {
		repoRoot, _ = os.Getwd()
	}

	logsDir := filepath.Join(repoRoot, "logs")
	logFile := findSessionFile(logsDir, sessionID)

	if logFile == "" {
		engine.Log.Error("session not found: %s", sessionID)
		os.Exit(1)
	}

	// Read and find request with system prompt
	data, err := os.ReadFile(logFile)
	if err != nil {
		engine.Log.Error("reading session: %v", err)
		os.Exit(1)
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		var entry session.Entry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		if entry.Role == "request" && len(entry.Request) > 0 {
			// Parse the request to extract system prompt
			var req struct {
				System string `json:"system"`
			}
			if err := json.Unmarshal(entry.Request, &req); err == nil && req.System != "" {
				fmt.Println(req.System)
				return
			}
		}
	}

	fmt.Println("No context found in session.")
}

func runSessionsPrompts(cmd *cobra.Command, args []string) {
	sessionID := args[0]

	repoRoot, _ := fsutil.FindRepoRoot()
	if repoRoot == "" {
		repoRoot, _ = os.Getwd()
	}

	logsDir := filepath.Join(repoRoot, "logs")
	files, err := session.ListPromptBreakdowns(logsDir, sessionID)
	if err != nil {
		engine.Log.Error("%v", err)
		os.Exit(1)
	}

	if len(files) == 0 {
		fmt.Println("No prompt breakdowns found.")
		return
	}

	fmt.Printf("Prompt breakdowns for %s:\n\n", sessionID)
	for _, f := range files {
		fmt.Printf("  %s\n", filepath.Base(f))
	}
}

func runSessionsPrompt(cmd *cobra.Command, args []string) {
	sessionID := args[0]
	turn := 0
	fmt.Sscanf(args[1], "%d", &turn)

	repoRoot, _ := fsutil.FindRepoRoot()
	if repoRoot == "" {
		repoRoot, _ = os.Getwd()
	}

	logsDir := filepath.Join(repoRoot, "logs")
	content, err := session.ReadPromptBreakdown(logsDir, sessionID, turn)
	if err != nil {
		engine.Log.Error("%v", err)
		os.Exit(1)
	}

	fmt.Println(content)
}