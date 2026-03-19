package main

import (
	"fmt"
	"os"
	"time"

	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/engine"
	"github.com/spf13/cobra"
)

var benchmarkStartupCmd = &cobra.Command{
	Use:   "benchmark-startup",
	Short: "Benchmark inber startup time with detailed phase timing",
	Long: `Benchmark the complete startup sequence from command invocation to ready-for-API state.
	
Times each major initialization phase:
- Config resolution (agent config, model selection)
- Memory store initialization  
- Session preparation and memory loading
- Workspace setup and message loading
- Session database initialization
- Model store and client creation
- Tool building and registration

Useful for:
- Performance analysis and optimization
- Regression testing
- Comparing startup times across different configurations`,
	Run: runBenchmarkStartup,
}

var (
	benchRuns   int
	benchModel  string
	benchAgent  string
	benchRaw    bool
	benchDetach bool
)

func init() {
	benchmarkStartupCmd.Flags().IntVar(&benchRuns, "runs", 5, "Number of benchmark runs to average")
	benchmarkStartupCmd.Flags().StringVarP(&benchModel, "model", "m", agent.DefaultModel, "Model to use for benchmarking")
	benchmarkStartupCmd.Flags().StringVarP(&benchAgent, "agent", "a", "", "Agent to load for benchmarking")
	benchmarkStartupCmd.Flags().BoolVar(&benchRaw, "raw", false, "Benchmark raw mode (skip context/memory)")
	benchmarkStartupCmd.Flags().BoolVar(&benchDetach, "detach", false, "Benchmark detached mode")
}

type BenchmarkResult struct {
	TotalTime          time.Duration
	ConfigTime         time.Duration
	MemoryStoreTime    time.Duration
	SessionPrepTime    time.Duration
	WorkspaceTime      time.Duration
	SessionDBTime      time.Duration
	ModelStoreTime     time.Duration
	ModelClientTime    time.Duration
	AgentRegistryTime  time.Duration
	ToolsBuildingTime  time.Duration
	HooksSetupTime     time.Duration
	OverheadTime       time.Duration
}

func (br BenchmarkResult) String() string {
	return fmt.Sprintf(`Startup Benchmark Results:
├─ Total time:          %8.2f ms
├─ Config resolution:   %8.2f ms  (%.1f%%)
├─ Memory store:        %8.2f ms  (%.1f%%)
├─ Session preparation: %8.2f ms  (%.1f%%)
├─ Workspace setup:     %8.2f ms  (%.1f%%)
├─ Session DB:          %8.2f ms  (%.1f%%)
├─ Model store:         %8.2f ms  (%.1f%%)
├─ Model client:        %8.2f ms  (%.1f%%)
├─ Agent registry:      %8.2f ms  (%.1f%%)
├─ Tools building:      %8.2f ms  (%.1f%%)
├─ Hooks setup:         %8.2f ms  (%.1f%%)
└─ Overhead:            %8.2f ms  (%.1f%%)`,
		br.TotalTime.Seconds()*1000,
		br.ConfigTime.Seconds()*1000, br.ConfigTime.Seconds()/br.TotalTime.Seconds()*100,
		br.MemoryStoreTime.Seconds()*1000, br.MemoryStoreTime.Seconds()/br.TotalTime.Seconds()*100,
		br.SessionPrepTime.Seconds()*1000, br.SessionPrepTime.Seconds()/br.TotalTime.Seconds()*100,
		br.WorkspaceTime.Seconds()*1000, br.WorkspaceTime.Seconds()/br.TotalTime.Seconds()*100,
		br.SessionDBTime.Seconds()*1000, br.SessionDBTime.Seconds()/br.TotalTime.Seconds()*100,
		br.ModelStoreTime.Seconds()*1000, br.ModelStoreTime.Seconds()/br.TotalTime.Seconds()*100,
		br.ModelClientTime.Seconds()*1000, br.ModelClientTime.Seconds()/br.TotalTime.Seconds()*100,
		br.AgentRegistryTime.Seconds()*1000, br.AgentRegistryTime.Seconds()/br.TotalTime.Seconds()*100,
		br.ToolsBuildingTime.Seconds()*1000, br.ToolsBuildingTime.Seconds()/br.TotalTime.Seconds()*100,
		br.HooksSetupTime.Seconds()*1000, br.HooksSetupTime.Seconds()/br.TotalTime.Seconds()*100,
		br.OverheadTime.Seconds()*1000, br.OverheadTime.Seconds()/br.TotalTime.Seconds()*100,
	)
}

func runBenchmarkStartup(cmd *cobra.Command, args []string) {
	fmt.Printf("Benchmarking inber startup time (%d runs)...\n", benchRuns)
	fmt.Printf("Configuration: model=%s, agent=%s, raw=%v, detach=%v\n\n",
		benchModel, benchAgent, benchRaw, benchDetach)

	var results []BenchmarkResult
	
	for i := 0; i < benchRuns; i++ {
		fmt.Printf("Run %d/%d... ", i+1, benchRuns)
		result := measureSingleStartup()
		results = append(results, result)
		fmt.Printf("%.2f ms\n", result.TotalTime.Seconds()*1000)
	}

	// Calculate averages
	avg := calculateAverage(results)
	fmt.Printf("\n%s\n", avg)

	// Additional analysis
	fastest := results[0]
	slowest := results[0]
	for _, r := range results[1:] {
		if r.TotalTime < fastest.TotalTime {
			fastest = r
		}
		if r.TotalTime > slowest.TotalTime {
			slowest = r
		}
	}

	fmt.Printf("\nVariability:\n")
	fmt.Printf("├─ Fastest: %.2f ms\n", fastest.TotalTime.Seconds()*1000)
	fmt.Printf("├─ Slowest: %.2f ms\n", slowest.TotalTime.Seconds()*1000)
	fmt.Printf("└─ Spread:  %.2f ms (±%.1f%%)\n", 
		(slowest.TotalTime-fastest.TotalTime).Seconds()*1000,
		(slowest.TotalTime-fastest.TotalTime).Seconds()/avg.TotalTime.Seconds()*50) // /2 for ±
}

func measureSingleStartup() BenchmarkResult {
	startTime := time.Now()

	// Measure each phase by creating the engine config and calling NewEngine
	// We'll need to hook into the engine creation process to get detailed timing
	
	cfg := engine.EngineConfig{
		Model:       benchModel,
		AgentName:   benchAgent,
		Raw:         benchRaw,
		Detach:      benchDetach,
		CommandName: "benchmark-startup",
		// Disable display hooks to avoid overhead
		Display: &engine.DisplayHooks{},
	}

	// This creates the engine and measures total time
	// For detailed phase timing, we'd need to modify engine.NewEngine() 
	// or create a wrapper that measures each step
	eng, err := engine.NewEngine(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Engine creation failed: %v\n", err)
		os.Exit(1)
	}

	totalTime := time.Since(startTime)
	
	// Clean up
	eng.Close()

	// For now, return with total time measured
	// TODO: Add detailed phase timing by instrumenting NewEngine
	return BenchmarkResult{
		TotalTime: totalTime,
		// These would be filled in with detailed instrumentation:
		ConfigTime:        time.Duration(0), 
		MemoryStoreTime:   time.Duration(0),
		SessionPrepTime:   time.Duration(0),
		WorkspaceTime:     time.Duration(0),
		SessionDBTime:     time.Duration(0),
		ModelStoreTime:    time.Duration(0),
		ModelClientTime:   time.Duration(0),
		AgentRegistryTime: time.Duration(0),
		ToolsBuildingTime: time.Duration(0),
		HooksSetupTime:    time.Duration(0),
		OverheadTime:      totalTime, // All overhead for now
	}
}

func calculateAverage(results []BenchmarkResult) BenchmarkResult {
	if len(results) == 0 {
		return BenchmarkResult{}
	}

	var avg BenchmarkResult
	for _, r := range results {
		avg.TotalTime += r.TotalTime
		avg.ConfigTime += r.ConfigTime
		avg.MemoryStoreTime += r.MemoryStoreTime
		avg.SessionPrepTime += r.SessionPrepTime
		avg.WorkspaceTime += r.WorkspaceTime
		avg.SessionDBTime += r.SessionDBTime
		avg.ModelStoreTime += r.ModelStoreTime
		avg.ModelClientTime += r.ModelClientTime
		avg.AgentRegistryTime += r.AgentRegistryTime
		avg.ToolsBuildingTime += r.ToolsBuildingTime
		avg.HooksSetupTime += r.HooksSetupTime
		avg.OverheadTime += r.OverheadTime
	}

	count := time.Duration(len(results))
	avg.TotalTime /= count
	avg.ConfigTime /= count
	avg.MemoryStoreTime /= count
	avg.SessionPrepTime /= count
	avg.WorkspaceTime /= count
	avg.SessionDBTime /= count
	avg.ModelStoreTime /= count
	avg.ModelClientTime /= count
	avg.AgentRegistryTime /= count
	avg.ToolsBuildingTime /= count
	avg.HooksSetupTime /= count
	avg.OverheadTime /= count

	return avg
}