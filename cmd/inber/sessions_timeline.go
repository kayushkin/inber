package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kayushkin/inber/engine"
	"github.com/kayushkin/inber/session"
	"github.com/spf13/cobra"
)

var sessionsTimelineCmd = &cobra.Command{
	Use:   "timeline <id>",
	Short: "Show the session timeline",
	Args:  cobra.ExactArgs(1),
	Run:   runSessionsTimeline,
}

func runSessionsTimeline(cmd *cobra.Command, args []string) {
	sessionID := args[0]

	repoRoot, _ := engine.FindRepoRoot()
	if repoRoot == "" {
		repoRoot, _ = os.Getwd()
	}

	logsDir := filepath.Join(repoRoot, "logs")
	content, err := session.ReadTimelineFromJSONL(logsDir, sessionID)
	if err != nil {
		engine.Log.Error("%v", err)
		os.Exit(1)
	}

	fmt.Print(content)
}