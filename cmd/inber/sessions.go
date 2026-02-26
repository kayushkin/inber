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

var sessionsTimelineCmd = &cobra.Command{
	Use:   "timeline <id>",
	Short: "Show the session timeline",
	Args:  cobra.ExactArgs(1),
	Run:   runSessionsTimeline,
}

var sessionsActiveCmd = &cobra.Command{
	Use:   "active",
	Short: "Show currently running sessions",
	Run:   runSessionsActive,
}

var sessionsLimit int

func init() {
	sessionsCmd.AddCommand(sessionsListCmd)
	sessionsCmd.AddCommand(sessionsShowCmd)
	sessionsCmd.AddCommand(sessionsContextCmd)
	sessionsCmd.AddCommand(sessionsPromptsCmd)
	sessionsCmd.AddCommand(sessionsPromptCmd)
	sessionsCmd.AddCommand(sessionsTimelineCmd)
	sessionsCmd.AddCommand(sessionsActiveCmd)
	
	sessionsListCmd.Flags().IntVarP(&sessionsLimit, "limit", "n", 10, "Number of sessions to show")
}

func runSessionsList(cmd *cobra.Command, args []string) {
	repoRoot, _ := FindRepoRoot()
	if repoRoot == "" {
		repoRoot, _ = os.Getwd()
	}

	logsDir := filepath.Join(repoRoot, "logs")

	type sessionInfo struct {
		ID        string
		Agent     string
		StartTime time.Time
		FilePath  string
	}

	var sessions []sessionInfo
	filepath.WalkDir(logsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		
		// Determine agent name from subdirectory
		relDir, _ := filepath.Rel(logsDir, filepath.Dir(path))
		agentName := ""
		if relDir != "." {
			agentName = relDir
		}
		
		name := strings.TrimSuffix(d.Name(), ".jsonl")
		
		// Try new format: YYYY-MM-DD_HHMMSS
		if t, err := time.Parse("2006-01-02_150405", name); err == nil {
			sessions = append(sessions, sessionInfo{
				ID:        name,
				Agent:     agentName,
				StartTime: t,
				FilePath:  path,
			})
			return nil
		}
		
		// Try old format: YYYYMMDD-HHMMSS-<id>
		parts := strings.Split(name, "-")
		if len(parts) >= 3 {
			timeStr := parts[0] + parts[1]
			if t, err := time.Parse("20060102150405", timeStr); err == nil {
				id := strings.Join(parts[2:], "-")
				sessions = append(sessions, sessionInfo{
					ID:        id,
					Agent:     agentName,
					StartTime: t,
					FilePath:  path,
				})
			}
		}
		return nil
	})

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
		agentLabel := ""
		if s.Agent != "" {
			agentLabel = fmt.Sprintf(" [%s]", s.Agent)
		}
		fmt.Printf("  %s  %s%-8s%s%s\n",
			s.StartTime.Format("2006-01-02 15:04:05"),
			dim, s.ID, reset, agentLabel)
	}
}

func findSessionFile(logsDir, sessionID string) string {
	var logFile string
	filepath.WalkDir(logsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.Contains(d.Name(), sessionID) && strings.HasSuffix(d.Name(), ".jsonl") {
			logFile = path
			return filepath.SkipAll
		}
		return nil
	})
	return logFile
}

func runSessionsShow(cmd *cobra.Command, args []string) {
	sessionID := args[0]
	
	repoRoot, _ := FindRepoRoot()
	if repoRoot == "" {
		repoRoot, _ = os.Getwd()
	}

	logsDir := filepath.Join(repoRoot, "logs")
	logFile := findSessionFile(logsDir, sessionID)

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
	
	repoRoot, _ := FindRepoRoot()
	if repoRoot == "" {
		repoRoot, _ = os.Getwd()
	}

	logsDir := filepath.Join(repoRoot, "logs")
	logFile := findSessionFile(logsDir, sessionID)

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

	repoRoot, _ := FindRepoRoot()
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

	repoRoot, _ := FindRepoRoot()
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

func runSessionsTimeline(cmd *cobra.Command, args []string) {
	sessionID := args[0]

	repoRoot, _ := FindRepoRoot()
	if repoRoot == "" {
		repoRoot, _ = os.Getwd()
	}

	logsDir := filepath.Join(repoRoot, "logs")
	content, err := session.ReadTimelineFile(logsDir, sessionID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Print(content)
}

func runSessionsActive(cmd *cobra.Command, args []string) {
	repoRoot, _ := FindRepoRoot()
	if repoRoot == "" {
		repoRoot, _ = os.Getwd()
	}

	active, err := session.ListActive(repoRoot)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if len(active) == 0 {
		fmt.Println("No active sessions.")
		return
	}

	fmt.Printf("Active sessions (%d):\n\n", len(active))
	for _, s := range active {
		duration := time.Since(s.StartTime).Truncate(time.Second)
		fmt.Printf("  %sPID %d%s  %s  %s%-8s%s  agent=%s  (%s)\n",
			bold, s.PID, reset,
			s.Command,
			dim, s.SessionID, reset,
			s.Agent,
			duration)
	}
}
