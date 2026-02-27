package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kayushkin/inber/memory"
	"github.com/spf13/cobra"
)

var memoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Manage persistent memories",
	Long:  `Search, save, list, and manage persistent memories.`,
}

var memorySearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search memories",
	Args:  cobra.MinimumNArgs(1),
	Run:   runMemorySearch,
}

var memoryListCmd = &cobra.Command{
	Use:   "list",
	Short: "List recent memories",
	Run:   runMemoryList,
}

var memoryShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show a specific memory",
	Args:  cobra.ExactArgs(1),
	Run:   runMemoryShow,
}

var memorySaveCmd = &cobra.Command{
	Use:   "save <text>",
	Short: "Manually save a memory",
	Args:  cobra.MinimumNArgs(1),
	Run:   runMemorySave,
}

var memoryForgetCmd = &cobra.Command{
	Use:   "forget <id>",
	Short: "Forget a memory",
	Args:  cobra.ExactArgs(1),
	Run:   runMemoryForget,
}

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
	
	memorySearchCmd.Flags().IntVarP(&memorySearchLimit, "limit", "n", 10, "Maximum results")
	memoryListCmd.Flags().IntVarP(&memoryListLimit, "limit", "n", 20, "Maximum results")
	memoryListCmd.Flags().Float64VarP(&memoryListMin, "min-importance", "i", 0.5, "Minimum importance")
	memorySaveCmd.Flags().StringSliceVarP(&memorySaveTags, "tags", "t", []string{}, "Tags (comma-separated)")
	memorySaveCmd.Flags().Float64VarP(&memorySaveImportance, "importance", "i", 0.5, "Importance (0-1)")
	memoryCompactCmd.Flags().StringVar(&memoryCompactAge, "age", "168h", "Minimum age for compaction (e.g., 168h for 7 days)")
	memoryCompactCmd.Flags().IntVar(&memoryCompactMinAccess, "min-access", 3, "Minimum access count threshold")
	memoryPruneCmd.Flags().BoolVar(&memoryPruneDryRun, "dry-run", false, "Show what would be pruned without pruning")
}

func getMemoryStore() *memory.Store {
	repoRoot, _ := FindRepoRoot()
	if repoRoot == "" {
		repoRoot, _ = os.Getwd()
	}

	store, err := memory.OpenOrCreate(repoRoot)
	if err != nil {
		Log.Error("opening memory store: %v", err)
		os.Exit(1)
	}
	return store
}

func runMemorySearch(cmd *cobra.Command, args []string) {
	query := strings.Join(args, " ")
	store := getMemoryStore()
	defer store.Close()

	results, err := store.Search(query, memorySearchLimit)
	if err != nil {
		Log.Error("%v", err)
		os.Exit(1)
	}

	if len(results) == 0 {
		fmt.Println("No memories found.")
		return
	}

	fmt.Printf("Found %d memories:\n\n", len(results))
	for i, m := range results {
		tags := strings.Join(m.Tags, ", ")
		fmt.Printf("%s%d. [%s]%s\n", bold, i+1, m.ID[:8], reset)
		fmt.Printf("   Source: %s | Importance: %.2f | Accessed: %d times\n", m.Source, m.Importance, m.AccessCount)
		fmt.Printf("   Tags: %s\n", tags)
		fmt.Printf("   %s%s%s\n\n", dim, truncateText(m.Content, 200), reset)
	}
}

func runMemoryList(cmd *cobra.Command, args []string) {
	store := getMemoryStore()
	defer store.Close()

	results, err := store.ListRecent(memoryListLimit, memoryListMin)
	if err != nil {
		Log.Error("%v", err)
		os.Exit(1)
	}

	if len(results) == 0 {
		fmt.Println("No memories found.")
		return
	}

	fmt.Printf("Recent memories (%d):\n\n", len(results))
	for i, m := range results {
		tags := strings.Join(m.Tags, ", ")
		fmt.Printf("%s%d. [%s]%s  %s\n", bold, i+1, m.ID[:8], reset, m.CreatedAt.Format("2006-01-02"))
		fmt.Printf("   Importance: %.2f | Tags: %s\n", m.Importance, tags)
		fmt.Printf("   %s%s%s\n\n", dim, truncateText(m.Content, 150), reset)
	}
}

func runMemoryShow(cmd *cobra.Command, args []string) {
	id := args[0]
	store := getMemoryStore()
	defer store.Close()

	m, err := store.Get(id)
	if err != nil {
		Log.Error("%v", err)
		os.Exit(1)
	}

	tags := strings.Join(m.Tags, ", ")
	fmt.Printf("%sMemory: %s%s\n", bold+blue, m.ID, reset)
	fmt.Printf("Created: %s\n", m.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Last accessed: %s\n", m.LastAccessed.Format("2006-01-02 15:04:05"))
	fmt.Printf("Access count: %d\n", m.AccessCount)
	fmt.Printf("Importance: %.2f\n", m.Importance)
	fmt.Printf("Source: %s\n", m.Source)
	fmt.Printf("Tags: %s\n", tags)
	
	if m.OriginalID != "" {
		fmt.Printf("Original memory: %s\n", m.OriginalID)
	}
	
	fmt.Printf("\nContent:\n%s\n", dim+"---"+reset)
	fmt.Println(m.Content)
	fmt.Printf("%s\n", dim+"---"+reset)
}

func runMemorySave(cmd *cobra.Command, args []string) {
	content := strings.Join(args, " ")
	store := getMemoryStore()
	defer store.Close()

	m := memory.Memory{
		ID:         uuid.New().String(),
		Content:    content,
		Tags:       memorySaveTags,
		Importance: memorySaveImportance,
		Source:     "user",
	}

	if err := store.Save(m); err != nil {
		Log.Error("%v", err)
		os.Exit(1)
	}

	fmt.Printf("Memory saved: %s\n", m.ID)
}

func runMemoryForget(cmd *cobra.Command, args []string) {
	id := args[0]
	store := getMemoryStore()
	defer store.Close()

	if err := store.Forget(id); err != nil {
		Log.Error("%v", err)
		os.Exit(1)
	}

	fmt.Printf("Memory forgotten: %s\n", id)
}

func runMemoryStats(cmd *cobra.Command, args []string) {
	store := getMemoryStore()
	defer store.Close()

	// Get all memories
	all, err := store.ListRecent(10000, 0)
	if err != nil {
		Log.Error("%v", err)
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

	fmt.Printf("%sMemory Statistics%s\n\n", bold+blue, reset)
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
		Log.Error("invalid age duration: %v", err)
		os.Exit(1)
	}

	results, err := store.Compact(age, memoryCompactMinAccess)
	if err != nil {
		Log.Error("%v", err)
		os.Exit(1)
	}

	if len(results) == 0 {
		fmt.Println("Nothing to compact.")
		return
	}

	fmt.Printf("Compacted %d groups:\n\n", len(results))
	for _, r := range results {
		fmt.Printf("  %s%s%s ← %d memories (tags: %s)\n",
			bold, r.NewID, reset, r.Count, strings.Join(r.Tags, ", "))
	}
}

func runMemoryPrune(cmd *cobra.Command, args []string) {
	store := getMemoryStore()
	defer store.Close()

	// Show memories that would be affected by compaction (low importance)
	all, err := store.ListRecent(1000, 0)
	if err != nil {
		Log.Error("%v", err)
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
			m.ID[:8], m.Importance, m.AccessCount, dim, truncateText(m.Content, 60), reset)
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
		Log.Error("%v", err)
		os.Exit(1)
	}

	fmt.Println("Importance decay applied.")
}

func truncateText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
