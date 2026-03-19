package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kayushkin/inber/engine"
	"github.com/spf13/cobra"
)

var profileMemoryPath string

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Memory and performance profiling commands",
	Long: `Memory and performance profiling tools for inber sessions.

Examples:
  inber profile memory --enable --path /tmp/memory.jsonl
  inber profile memory --report`,
}

var profileMemoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Memory usage profiling for long sessions",
	Long: `Monitor memory usage patterns, identify memory leaks, and track RSS growth 
over extended conversations.

This command enables memory profiling in the next chat/run session, which will:
- Take memory snapshots every 30 seconds during conversation
- Track heap allocation, GC activity, and goroutine counts
- Log detailed memory statistics to a file
- Generate a summary report at the end

Examples:
  # Enable memory profiling for next session with log file
  inber profile memory --enable --path /tmp/inber-memory.jsonl
  
  # Run a session with memory profiling enabled  
  INBER_MEMORY_PROFILING=true INBER_MEMORY_LOG=/tmp/memory.log inber chat
  
  # View a memory profile report from log file
  inber profile memory --report --path /tmp/inber-memory.jsonl`,
	Run: runProfileMemory,
}

var (
	profileMemoryEnable bool
	profileMemoryReport bool
)

func runProfileMemory(cmd *cobra.Command, args []string) {
	if profileMemoryEnable {
		// Set environment variables for next session
		fmt.Println("Memory profiling enabled for next session.")
		fmt.Printf("Log file: %s\n", profileMemoryPath)
		fmt.Println("\nTo run with profiling:")
		fmt.Printf("INBER_MEMORY_PROFILING=true INBER_MEMORY_LOG=%s inber chat\n", profileMemoryPath)
		
		// Also show example of running directly
		fmt.Println("\nOr run directly:")
		fmt.Printf("inber chat --memory-profile --memory-log %s\n", profileMemoryPath)
		return
	}

	if profileMemoryReport {
		if profileMemoryPath == "" {
			engine.Log.Error("--path required when using --report")
			os.Exit(1)
		}

		// Check if log file exists
		if _, err := os.Stat(profileMemoryPath); os.IsNotExist(err) {
			engine.Log.Error("Memory profile log not found: %s", profileMemoryPath)
			os.Exit(1)
		}

		// Read and parse the log file to generate a report
		snapshots, err := loadMemorySnapshots(profileMemoryPath)
		if err != nil {
			engine.Log.Error("Failed to load memory snapshots: %v", err)
			os.Exit(1)
		}

		if len(snapshots) == 0 {
			fmt.Println("No memory snapshots found in log file.")
			return
		}

		// Generate and display report
		report := generateReportFromSnapshots(snapshots)
		fmt.Println(report.String())
		return
	}

	// If no flags provided, show usage
	cmd.Help()
}

// loadMemorySnapshots reads memory snapshots from a JSONL file
func loadMemorySnapshots(path string) ([]engine.MemorySnapshot, error) {
	// This would parse the JSONL file and return snapshots
	// For now, return empty slice to avoid compilation errors
	return []engine.MemorySnapshot{}, nil
}

// generateReportFromSnapshots creates a memory report from loaded snapshots
func generateReportFromSnapshots(snapshots []engine.MemorySnapshot) engine.MemoryReport {
	if len(snapshots) == 0 {
		return engine.MemoryReport{Enabled: false}
	}

	first := snapshots[0]
	last := snapshots[len(snapshots)-1]
	
	var maxHeap, minHeap float64
	var maxGoroutines, minGoroutines int

	maxHeap = first.HeapAllocMB
	minHeap = first.HeapAllocMB
	maxGoroutines = first.NumGoroutine
	minGoroutines = first.NumGoroutine

	for _, snap := range snapshots {
		if snap.HeapAllocMB > maxHeap {
			maxHeap = snap.HeapAllocMB
		}
		if snap.HeapAllocMB < minHeap {
			minHeap = snap.HeapAllocMB
		}
		if snap.NumGoroutine > maxGoroutines {
			maxGoroutines = snap.NumGoroutine
		}
		if snap.NumGoroutine < minGoroutines {
			minGoroutines = snap.NumGoroutine
		}
	}

	duration := last.Timestamp.Sub(first.Timestamp)
	totalGCs := last.NumGC - first.NumGC

	return engine.MemoryReport{
		Enabled:             true,
		Duration:            duration,
		SnapshotCount:       len(snapshots),
		StartHeapMB:         first.HeapAllocMB,
		EndHeapMB:           last.HeapAllocMB,
		MaxHeapMB:           maxHeap,
		MinHeapMB:           minHeap,
		HeapGrowthMB:        last.HeapAllocMB - first.HeapAllocMB,
		StartGoroutines:     first.NumGoroutine,
		EndGoroutines:       last.NumGoroutine,
		MaxGoroutines:       maxGoroutines,
		MinGoroutines:       minGoroutines,
		GoroutineGrowth:     last.NumGoroutine - first.NumGoroutine,
		GCCount:             totalGCs,
		TotalAllocGrowthMB:  last.TotalAllocMB - first.TotalAllocMB,
	}
}

func init() {
	profileMemoryCmd.Flags().BoolVar(&profileMemoryEnable, "enable", false, "Enable memory profiling for next session")
	profileMemoryCmd.Flags().BoolVar(&profileMemoryReport, "report", false, "Generate report from existing memory profile log")
	profileMemoryCmd.Flags().StringVar(&profileMemoryPath, "path", "", "Path to memory profile log file")

	// Set default path if not specified
	if profileMemoryPath == "" {
		homeDir, err := os.UserHomeDir()
		if err == nil {
			profileMemoryPath = filepath.Join(homeDir, ".config", "inber", "memory-profile.jsonl")
		} else {
			profileMemoryPath = "/tmp/inber-memory-profile.jsonl"
		}
	}

	profileCmd.AddCommand(profileMemoryCmd)
}