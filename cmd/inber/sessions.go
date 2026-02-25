package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kayushkin/inber/session"
	"github.com/spf13/cobra"
)

var sessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "View session logs",
	Long:  `List and view session logs.`,
}

var sessionsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List recent sessions",
	Run:   runSessionsList,
}

var sessionsShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show a session's messages",
	Args:  cobra.ExactArgs(1),
	Run:   runSessionsShow,
}

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

var sessionsLimit int

func init() {
	sessionsCmd.AddCommand(sessionsListCmd)
	sessionsCmd.AddCommand(sessionsShowCmd)
	sessionsCmd.AddCommand(sessionsContextCmd)
	sessionsCmd.AddCommand(sessionsPromptsCmd)
	sessionsCmd.AddCommand(sessionsPromptCmd)
	
	sessionsListCmd.Flags().IntVarP(&sessionsLimit, "limit", "n", 10, "Number of sessions to show")
}

func runSessionsList(cmd *cobra.Command, args []string) {
	repoRoot, _ := findRepoRoot()
	if repoRoot == "" {
		repoRoot, _ = os.Getwd()
	}

	logsDir := filepath.Join(repoRoot, "logs")
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading logs: %v\n", err)
		os.Exit(1)
	}

	type sessionInfo struct {
		ID        string
		StartTime time.Time
		FilePath  string
	}

	var sessions []sessionInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}
		
		// Parse timestamp from filename (format: YYYYMMDD-HHMMSS-<id>.jsonl)
		parts := strings.Split(entry.Name(), "-")
		if len(parts) < 3 {
			continue
		}
		
		timeStr := parts[0] + parts[1]
		t, err := time.Parse("20060102150405", timeStr)
		if err != nil {
			continue
		}
		
		id := strings.TrimSuffix(parts[2], ".jsonl")
		sessions = append(sessions, sessionInfo{
			ID:        id,
			StartTime: t,
			FilePath:  filepath.Join(logsDir, entry.Name()),
		})
	}

	// Sort by time (newest first)
	for i := 0; i < len(sessions); i++ {
		for j := i + 1; j < len(sessions); j++ {
			if sessions[j].StartTime.After(sessions[i].StartTime) {
				sessions[i], sessions[j] = sessions[j], sessions[i]
			}
		}
	}

	// Limit
	if sessionsLimit > 0 && len(sessions) > sessionsLimit {
		sessions = sessions[:sessionsLimit]
	}

	if len(sessions) == 0 {
		fmt.Println("No sessions found.")
		return
	}

	fmt.Printf("Recent sessions (%d):\n\n", len(sessions))
	for _, s := range sessions {
		fmt.Printf("  %s  %s%-8s%s\n",
			s.StartTime.Format("2006-01-02 15:04:05"),
			dim, s.ID, reset)
	}
}

func runSessionsShow(cmd *cobra.Command, args []string) {
	sessionID := args[0]
	
	repoRoot, _ := findRepoRoot()
	if repoRoot == "" {
		repoRoot, _ = os.Getwd()
	}

	logsDir := filepath.Join(repoRoot, "logs")
	
	// Find the session file
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading logs: %v\n", err)
		os.Exit(1)
	}

	var logFile string
	for _, entry := range entries {
		if strings.Contains(entry.Name(), sessionID) && strings.HasSuffix(entry.Name(), ".jsonl") {
			logFile = filepath.Join(logsDir, entry.Name())
			break
		}
	}

	if logFile == "" {
		fmt.Fprintf(os.Stderr, "session not found: %s\n", sessionID)
		os.Exit(1)
	}

	// Read and display
	data, err := os.ReadFile(logFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading session: %v\n", err)
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

		switch entry.Role {
		case "user":
			fmt.Printf("\n%s> %s%s\n", bold+green, reset, entry.Content)
		
		case "assistant":
			fmt.Printf("\n%s%s\n", dim, entry.Content)
		
		case "tool_call":
			fmt.Printf("\n%s[tool: %s]%s\n", yellow, entry.ToolName, reset)
			fmt.Printf("%s%s%s\n", dim, string(entry.ToolInput), reset)
		
		case "tool_result":
			fmt.Printf("%s→ %s%s\n", dim, entry.Content, reset)
		
		case "thinking":
			fmt.Printf("\n%s[thinking]%s\n%s\n", magenta, reset, entry.Content)
		}
	}
	fmt.Println()
}

func runSessionsContext(cmd *cobra.Command, args []string) {
	sessionID := args[0]
	
	repoRoot, _ := findRepoRoot()
	if repoRoot == "" {
		repoRoot, _ = os.Getwd()
	}

	logsDir := filepath.Join(repoRoot, "logs")
	
	// Find the session file
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading logs: %v\n", err)
		os.Exit(1)
	}

	var logFile string
	for _, entry := range entries {
		if strings.Contains(entry.Name(), sessionID) && strings.HasSuffix(entry.Name(), ".jsonl") {
			logFile = filepath.Join(logsDir, entry.Name())
			break
		}
	}

	if logFile == "" {
		fmt.Fprintf(os.Stderr, "session not found: %s\n", sessionID)
		os.Exit(1)
	}

	// Read and find request with system prompt
	data, err := os.ReadFile(logFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading session: %v\n", err)
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

	repoRoot, _ := findRepoRoot()
	if repoRoot == "" {
		repoRoot, _ = os.Getwd()
	}

	logsDir := filepath.Join(repoRoot, "logs")
	files, err := session.ListPromptBreakdowns(logsDir, sessionID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
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

	repoRoot, _ := findRepoRoot()
	if repoRoot == "" {
		repoRoot, _ = os.Getwd()
	}

	logsDir := filepath.Join(repoRoot, "logs")
	content, err := session.ReadPromptBreakdown(logsDir, sessionID, turn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(content)
}
