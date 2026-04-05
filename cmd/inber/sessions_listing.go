package main

import (
	"github.com/kayushkin/inber/internal/fsutil"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kayushkin/inber/engine"
	"github.com/kayushkin/inber/session"
	"github.com/spf13/cobra"
)

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

func runSessionsList(cmd *cobra.Command, args []string) {
	repoRoot, _ := fsutil.FindRepoRoot()
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
		if err != nil || d.IsDir() {
			return nil
		}
		if d.Name() == "session.jsonl" {
			// New format: logs/{agent}/{session_id}/session.jsonl
			sessionDir := filepath.Dir(path)
			sessionID := filepath.Base(sessionDir)
			agentDir := filepath.Dir(sessionDir)
			relAgent, _ := filepath.Rel(logsDir, agentDir)
			agentName := ""
			if relAgent != "." {
				agentName = relAgent
			}
			timePart := sessionID
			if len(timePart) > 17 {
				timePart = timePart[:17]
			}
			if t, err := time.Parse("2006-01-02_150405", timePart); err == nil {
				sessions = append(sessions, sessionInfo{ID: sessionID, Agent: agentName, StartTime: t, FilePath: path})
			}
		} else if strings.HasSuffix(d.Name(), ".jsonl") {
			// Legacy flat format: logs/{agent}/{session_id}.jsonl
			relDir, _ := filepath.Rel(logsDir, filepath.Dir(path))
			agentName := ""
			if relDir != "." {
				agentName = relDir
			}
			name := strings.TrimSuffix(d.Name(), ".jsonl")
			if t, err := time.Parse("2006-01-02_150405", name); err == nil {
				sessions = append(sessions, sessionInfo{ID: name, Agent: agentName, StartTime: t, FilePath: path})
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
			engine.Dim, s.ID, engine.Reset, agentLabel)
	}
}

func findSessionFile(logsDir, sessionID string) string {
	var logFile string
	filepath.WalkDir(logsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		// New format: logs/{agent}/{session_id}/session.jsonl
		if d.Name() == "session.jsonl" && strings.Contains(filepath.Dir(path), sessionID) {
			logFile = path
			return filepath.SkipAll
		}
		// Legacy format: logs/{agent}/{session_id}.jsonl
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

	// Read and display
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

		switch entry.Role {
		case "user":
			fmt.Printf("\n%s> %s%s\n", engine.Bold+engine.Green, engine.Reset, entry.Content)
		
		case "assistant":
			fmt.Printf("\n%s%s\n", engine.Dim, entry.Content)
		
		case "tool_call":
			fmt.Printf("\n%s[tool: %s]%s\n", engine.Yellow, entry.ToolName, engine.Reset)
			fmt.Printf("%s%s%s\n", engine.Dim, string(entry.ToolInput), engine.Reset)
		
		case "tool_result":
			fmt.Printf("%s→ %s%s\n", engine.Dim, entry.Content, engine.Reset)
		
		case "thinking":
			fmt.Printf("\n%s[thinking]%s\n%s\n", engine.Magenta, engine.Reset, entry.Content)
		}
	}
	fmt.Println()
}