package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/kayushkin/inber/memory"
	"github.com/kayushkin/inber/engine"
	"github.com/spf13/cobra"
)

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

func runMemorySearch(cmd *cobra.Command, args []string) {
	query := strings.Join(args, " ")
	store := getMemoryStore()
	defer store.Close()

	results, err := store.Search(query, memorySearchLimit)
	if err != nil {
		engine.Log.Error("%v", err)
		os.Exit(1)
	}

	if len(results) == 0 {
		fmt.Println("No memories found.")
		return
	}

	fmt.Printf("Found %d memories:\n\n", len(results))
	for i, m := range results {
		tags := strings.Join(m.Tags, ", ")
		fmt.Printf("%s%d. [%s]%s\n", engine.Bold, i+1, m.ID[:8], engine.Reset)
		fmt.Printf("   Source: %s | Importance: %.2f | Accessed: %d times\n", m.Source, m.Importance, m.AccessCount)
		fmt.Printf("   Tags: %s\n", tags)
		fmt.Printf("   %s%s%s\n\n", engine.Dim, truncateText(m.Content, 200), engine.Reset)
	}
}

func runMemoryList(cmd *cobra.Command, args []string) {
	store := getMemoryStore()
	defer store.Close()

	results, err := store.ListRecent(memoryListLimit, memoryListMin)
	if err != nil {
		engine.Log.Error("%v", err)
		os.Exit(1)
	}

	if len(results) == 0 {
		fmt.Println("No memories found.")
		return
	}

	fmt.Printf("Recent memories (%d):\n\n", len(results))
	for i, m := range results {
		tags := strings.Join(m.Tags, ", ")
		fmt.Printf("%s%d. [%s]%s  %s\n", engine.Bold, i+1, m.ID[:8], engine.Reset, m.CreatedAt.Format("2006-01-02"))
		fmt.Printf("   Importance: %.2f | Tags: %s\n", m.Importance, tags)
		fmt.Printf("   %s%s%s\n\n", engine.Dim, truncateText(m.Content, 150), engine.Reset)
	}
}

func runMemoryShow(cmd *cobra.Command, args []string) {
	id := args[0]
	store := getMemoryStore()
	defer store.Close()

	m, err := store.Get(id)
	if err != nil {
		engine.Log.Error("%v", err)
		os.Exit(1)
	}

	tags := strings.Join(m.Tags, ", ")
	fmt.Printf("%sMemory: %s%s\n", engine.Bold+engine.Blue, m.ID, engine.Reset)
	fmt.Printf("Created: %s\n", m.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Last accessed: %s\n", m.LastAccessed.Format("2006-01-02 15:04:05"))
	fmt.Printf("Access count: %d\n", m.AccessCount)
	fmt.Printf("Importance: %.2f\n", m.Importance)
	fmt.Printf("Source: %s\n", m.Source)
	fmt.Printf("Tags: %s\n", tags)
	
	if m.OriginalID != "" {
		fmt.Printf("Original memory: %s\n", m.OriginalID)
	}
	
	fmt.Printf("\nContent:\n%s\n", engine.Dim+"---"+engine.Reset)
	fmt.Println(m.Content)
	fmt.Printf("%s\n", engine.Dim+"---"+engine.Reset)
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
		engine.Log.Error("%v", err)
		os.Exit(1)
	}

	fmt.Printf("Memory saved: %s\n", m.ID)
}

func runMemoryForget(cmd *cobra.Command, args []string) {
	id := args[0]
	store := getMemoryStore()
	defer store.Close()

	if err := store.Forget(id); err != nil {
		engine.Log.Error("%v", err)
		os.Exit(1)
	}

	fmt.Printf("Memory forgotten: %s\n", id)
}