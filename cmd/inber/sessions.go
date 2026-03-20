package main

import (
	"github.com/spf13/cobra"
)

var sessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "View session logs",
	Long:  `List and view session logs.`,
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