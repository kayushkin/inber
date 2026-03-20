package main

import (
	"fmt"
	"os"
	"time"

	"github.com/kayushkin/inber/engine"
	"github.com/kayushkin/inber/session"
	"github.com/spf13/cobra"
)

var sessionsActiveCmd = &cobra.Command{
	Use:   "active",
	Short: "Show currently running sessions",
	Run:   runSessionsActive,
}

func runSessionsActive(cmd *cobra.Command, args []string) {
	repoRoot, _ := engine.FindRepoRoot()
	if repoRoot == "" {
		repoRoot, _ = os.Getwd()
	}

	sdb, err := session.OpenDB(repoRoot)
	if err != nil {
		engine.Log.Error("%v", err)
		os.Exit(1)
	}
	defer sdb.Close()

	active, err := sdb.ListActive()
	if err != nil {
		engine.Log.Error("%v", err)
		os.Exit(1)
	}

	if len(active) == 0 {
		fmt.Println("No active sessions.")
		return
	}

	fmt.Printf("Active sessions (%d):\n\n", len(active))
	for _, s := range active {
		duration := s.Duration.Truncate(time.Second)
		fmt.Printf("  %sPID %d%s  %s  %s%-8s%s  agent=%s  turns=%d  $%.4f  (%s)\n",
			engine.Bold, s.PID, engine.Reset,
			s.Command,
			engine.Dim, s.ID, engine.Reset,
			s.Agent,
			s.Turns,
			s.TotalCost,
			duration)
	}
}