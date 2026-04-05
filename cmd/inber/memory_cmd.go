package main

import (
	"github.com/kayushkin/inber/internal/fsutil"
	"os"

	"github.com/kayushkin/inber/memory"
	"github.com/kayushkin/inber/engine"
	"github.com/spf13/cobra"
)

var memoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Manage persistent memories",
	Long:  `Search, save, list, and manage persistent memories.`,
}

var (
	memorySearchLimit    int
	memoryListLimit      int
	memoryListMin        float64
	memorySaveTags       []string
	memorySaveImportance float64
	memoryCompactAge     string
	memoryCompactMinAccess int
	memoryPruneDryRun    bool
)

func init() {
	memoryCmd.AddCommand(memorySearchCmd)
	memoryCmd.AddCommand(memoryListCmd)
	memoryCmd.AddCommand(memoryShowCmd)
	memoryCmd.AddCommand(memorySaveCmd)
	memoryCmd.AddCommand(memoryForgetCmd)
	memoryCmd.AddCommand(memoryStatsCmd)
	memoryCmd.AddCommand(memoryCompactCmd)
	memoryCmd.AddCommand(memoryPruneCmd)
	memoryCmd.AddCommand(memoryDecayCmd)
	
	// Set up flags
	memorySearchCmd.Flags().IntVarP(&memorySearchLimit, "limit", "n", 10, "Maximum results")
	memoryListCmd.Flags().IntVarP(&memoryListLimit, "limit", "n", 20, "Maximum results")
	memoryListCmd.Flags().Float64VarP(&memoryListMin, "min-importance", "i", 0.5, "Minimum importance")
	memorySaveCmd.Flags().StringSliceVarP(&memorySaveTags, "tags", "t", []string{}, "Tags (comma-separated)")
	memorySaveCmd.Flags().Float64VarP(&memorySaveImportance, "importance", "i", 0.5, "Importance (0-1)")
	memoryCompactCmd.Flags().StringVar(&memoryCompactAge, "age", "168h", "Minimum age for compaction (e.g., 168h for 7 days)")
	memoryCompactCmd.Flags().IntVar(&memoryCompactMinAccess, "min-access", 3, "Minimum access count threshold")
	memoryPruneCmd.Flags().BoolVar(&memoryPruneDryRun, "dry-run", false, "Show what would be pruned without pruning")
}

// getMemoryStore opens a memory store for the current repository
func getMemoryStore() memory.MemoryStore {
	repoRoot, _ := fsutil.FindRepoRoot()
	if repoRoot == "" {
		repoRoot, _ = os.Getwd()
	}

	store, err := memory.OpenOrCreate(repoRoot)
	if err != nil {
		engine.Log.Error("opening memory store: %v", err)
		os.Exit(1)
	}
	return store
}

// truncateText truncates text to the specified length with ellipsis
func truncateText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}