package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kayushkin/inber/memory"
	"github.com/kayushkin/inber/engine"
	"github.com/spf13/cobra"
)

var memoryStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show memory statistics",
	Run:   runMemoryStats,
}

var memoryCompactCmd = &cobra.Command{
	Use:   "compact",
	Short: "Compact old low-access memories",
	Run:   runMemoryCompact,
}

var memoryPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Show/run conversation pruning",
	Run:   runMemoryPrune,
}

var memoryDecayCmd = &cobra.Command{
	Use:   "decay",
	Short: "Run importance decay on all memories",
	Run:   runMemoryDecay,
}

func runMemoryStats(cmd *cobra.Command, args []string) {
	store := getMemoryStore()
	defer store.Close()

	// Get all memories
	all, err := store.ListRecent(10000, 0)
	if err != nil {
		engine.Log.Error("%v", err)
		os.Exit(1)
	}

	if len(all) == 0 {
		fmt.Println("No memories stored.")
		return
	}

	// Count tags
	tagCounts := make(map[string]int)
	var totalImportance float64
	importanceBuckets := make(map[string]int)
	
	for _, m := range all {
		for _, tag := range m.Tags {
			tagCounts[tag]++
		}
		totalImportance += m.Importance
		
		// Bucket by importance
		bucket := fmt.Sprintf("%.1f-%.1f", float64(int(m.Importance*10))/10, float64(int(m.Importance*10)+1)/10)
		importanceBuckets[bucket]++
	}

	avgImportance := totalImportance / float64(len(all))

	fmt.Printf("%sMemory Statistics%s\n\n", engine.Bold+engine.Blue, engine.Reset)
	fmt.Printf("Total memories: %d\n", len(all))
	fmt.Printf("Average importance: %.2f\n\n", avgImportance)

	// Top tags
	type tagCount struct {
		tag   string
		count int
	}
	var tags []tagCount
	for tag, count := range tagCounts {
		tags = append(tags, tagCount{tag, count})
	}
	
	// Sort by count
	for i := 0; i < len(tags); i++ {
		for j := i + 1; j < len(tags); j++ {
			if tags[j].count > tags[i].count {
				tags[i], tags[j] = tags[j], tags[i]
			}
		}
	}

	fmt.Printf("Top tags:\n")
	limit := 10
	if len(tags) < limit {
		limit = len(tags)
	}
	for i := 0; i < limit; i++ {
		fmt.Printf("  %-20s %d\n", tags[i].tag, tags[i].count)
	}

	fmt.Printf("\nImportance distribution:\n")
	bucketKeys := []string{"0.0-0.1", "0.1-0.2", "0.2-0.3", "0.3-0.4", "0.4-0.5", 
		"0.5-0.6", "0.6-0.7", "0.7-0.8", "0.8-0.9", "0.9-1.0"}
	for _, bucket := range bucketKeys {
		count := importanceBuckets[bucket]
		if count > 0 {
			bar := strings.Repeat("█", count*50/len(all))
			fmt.Printf("  %s  %s %d\n", bucket, bar, count)
		}
	}
}

func runMemoryCompact(cmd *cobra.Command, args []string) {
	store := getMemoryStore()
	defer store.Close()

	age, err := time.ParseDuration(memoryCompactAge)
	if err != nil {
		engine.Log.Error("invalid age duration: %v", err)
		os.Exit(1)
	}

	results, err := store.Compact(age, memoryCompactMinAccess)
	if err != nil {
		engine.Log.Error("%v", err)
		os.Exit(1)
	}

	if len(results) == 0 {
		fmt.Println("Nothing to compact.")
		return
	}

	fmt.Printf("Compacted %d groups:\n\n", len(results))
	for _, r := range results {
		fmt.Printf("  %s%s%s ← %d memories (tags: %s)\n",
			engine.Bold, r.NewID, engine.Reset, r.Count, strings.Join(r.Tags, ", "))
	}
}

func runMemoryPrune(cmd *cobra.Command, args []string) {
	store := getMemoryStore()
	defer store.Close()

	// Show memories that would be affected by compaction (low importance)
	all, err := store.ListRecent(1000, 0)
	if err != nil {
		engine.Log.Error("%v", err)
		os.Exit(1)
	}

	var prunable []memory.Memory
	for _, m := range all {
		if m.Importance < 0.3 && m.AccessCount < 2 {
			prunable = append(prunable, m)
		}
	}

	if len(prunable) == 0 {
		fmt.Println("No memories eligible for pruning.")
		return
	}

	fmt.Printf("Found %d prunable memories:\n\n", len(prunable))
	for _, m := range prunable {
		fmt.Printf("  [%s] imp=%.2f access=%d %s%s%s\n",
			m.ID[:8], m.Importance, m.AccessCount, engine.Dim, truncateText(m.Content, 60), engine.Reset)
	}

	if memoryPruneDryRun {
		fmt.Println("\n(dry run — no changes made)")
		return
	}

	// Actually forget them
	for _, m := range prunable {
		store.Forget(m.ID)
	}
	fmt.Printf("\nPruned %d memories.\n", len(prunable))
}

func runMemoryDecay(cmd *cobra.Command, args []string) {
	store := getMemoryStore()
	defer store.Close()

	if err := store.DecayImportance(); err != nil {
		engine.Log.Error("%v", err)
		os.Exit(1)
	}

	fmt.Println("Importance decay applied.")
}