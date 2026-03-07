package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/engine"
	modelstore "github.com/kayushkin/model-store"
	"github.com/spf13/cobra"
)

var modelsJSON bool

var modelsCmd = &cobra.Command{
	Use:   "models",
	Short: "Manage models",
	Long:  `List and manage available models.`,
}

var modelsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available models",
	Run:   runModelsList,
}

var modelsStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show model status with health data",
	Run:   runModelsStatus,
}

func init() {
	modelsCmd.AddCommand(modelsListCmd)
	modelsCmd.AddCommand(modelsStatusCmd)
	modelsStatusCmd.Flags().BoolVar(&modelsJSON, "json", false, "Output as JSON")

	// Make "models" alone run list
	modelsCmd.Run = runModelsList
}

func runModelsList(cmd *cobra.Command, args []string) {
	fmt.Printf("%sAvailable Claude models:%s\n\n", engine.Bold+engine.Blue, engine.Reset)

	for id, info := range agent.Models {
		fmt.Printf("%s%-35s%s\n", engine.Bold, id, engine.Reset)
		fmt.Printf("  Context window: %dk tokens\n", info.ContextWindow/1000)
		fmt.Printf("  Input cost:     $%.2f per 1M tokens\n", info.InputCostPer1M)
		fmt.Printf("  Output cost:    $%.2f per 1M tokens\n", info.OutputCostPer1M)
		fmt.Println()
	}
}

func runModelsStatus(cmd *cobra.Command, args []string) {
	store, err := modelstore.Open("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	statuses, err := store.AllModelsWithStatus()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if modelsJSON {
		data, _ := json.MarshalIndent(statuses, "", "  ")
		fmt.Println(string(data))
		return
	}

	// Human-readable output
	fmt.Printf("%sModel Status%s\n\n", engine.Bold+engine.Blue, engine.Reset)
	for _, ms := range statuses {
		enabled := "✅"
		if !ms.Enabled {
			enabled = "⬚ "
		}

		healthIcon := "❓"
		healthDetail := "never tried"
		if ms.Health != nil {
			if ms.Health.IsHealthy(30 * time.Minute) {
				healthIcon = "🟢"
				healthDetail = fmt.Sprintf("%.1fs avg, last %s ago",
					float64(ms.Health.AvgResponseMs)/1000,
					time.Since(ms.Health.LastSuccessAt).Truncate(time.Second))
			} else if !ms.Health.LastErrorAt.IsZero() && ms.Health.LastErrorAt.After(ms.Health.LastSuccessAt) {
				healthIcon = "🔴"
				healthDetail = fmt.Sprintf("error: %s (%s ago)",
					truncateStr(ms.Health.LastError, 40),
					time.Since(ms.Health.LastErrorAt).Truncate(time.Second))
			} else if !ms.Health.LastSuccessAt.IsZero() {
				healthIcon = "🟡"
				healthDetail = fmt.Sprintf("stale (last success %s ago)",
					time.Since(ms.Health.LastSuccessAt).Truncate(time.Minute))
			}
		}

		fmt.Printf("%s %s %s%-30s%s  p=%d  $%.2f/$%.2f  %s\n",
			enabled, healthIcon, engine.Bold, ms.Name, engine.Reset,
			ms.Priority, ms.InputCost, ms.OutputCost, healthDetail)
	}
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
