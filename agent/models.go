package agent

import "os"

// APIKey returns the Anthropic API key from the environment.
func APIKey() string {
	return os.Getenv("ANTHROPIC_API_KEY")
}

// ModelInfo describes a Claude model with metadata for tracking.
type ModelInfo struct {
	ID              string  // e.g., "claude-sonnet-4-5-20250929"
	ContextWindow   int     // max tokens
	InputCostPer1M  float64 // cost per 1M input tokens
	OutputCostPer1M float64 // cost per 1M output tokens
}

// Models is a registry of known Claude models.
var Models = map[string]ModelInfo{
	"claude-sonnet-4-5-20250929": {
		ID:              "claude-sonnet-4-5-20250929",
		ContextWindow:   200000,
		InputCostPer1M:  3.00,
		OutputCostPer1M: 15.00,
	},
	"claude-sonnet-4-6": {
		ID:              "claude-sonnet-4-6",
		ContextWindow:   200000,
		InputCostPer1M:  3.00,
		OutputCostPer1M: 15.00,
	},
	"claude-opus-4-6": {
		ID:              "claude-opus-4-6",
		ContextWindow:   200000,
		InputCostPer1M:  15.00,
		OutputCostPer1M: 75.00,
	},
}

// DefaultModel is the model used when none is specified.
const DefaultModel = "claude-sonnet-4-5-20250929"
