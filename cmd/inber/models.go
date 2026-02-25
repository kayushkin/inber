package main

import (
	"fmt"

	"github.com/kayushkin/inber/agent"
	"github.com/spf13/cobra"
)

var modelsCmd = &cobra.Command{
	Use:   "models",
	Short: "Manage models",
	Long:  `List available Claude models.`,
}

var modelsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available models",
	Run:   runModelsList,
}

func init() {
	modelsCmd.AddCommand(modelsListCmd)
	
	// Make "models" alone run list
	modelsCmd.Run = runModelsList
}

func runModelsList(cmd *cobra.Command, args []string) {
	fmt.Printf("%sAvailable Claude models:%s\n\n", bold+blue, reset)
	
	for id, info := range agent.Models {
		fmt.Printf("%s%-35s%s\n", bold, id, reset)
		fmt.Printf("  Context window: %dk tokens\n", info.ContextWindow/1000)
		fmt.Printf("  Input cost:     $%.2f per 1M tokens\n", info.InputCostPer1M)
		fmt.Printf("  Output cost:    $%.2f per 1M tokens\n", info.OutputCostPer1M)
		fmt.Println()
	}
}
