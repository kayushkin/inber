package agent

import (
	modelstore "github.com/kayushkin/model-store"
)

// ModelInfo describes a model with metadata for tracking.
type ModelInfo struct {
	ID              string  // e.g., "claude-sonnet-4-5-20250929"
	ContextWindow   int     // max tokens
	InputCostPer1M  float64 // cost per 1M input tokens
	OutputCostPer1M float64 // cost per 1M output tokens
}

// GetModelInfo retrieves model information from the model-store.
// Falls back to reasonable defaults if the model is not found.
func GetModelInfo(modelID string, store *modelstore.Store) ModelInfo {
	if store != nil {
		models, err := store.AllModelsWithStatus()
		if err == nil {
			for _, m := range models {
				if m.Name == modelID {
					// Model-store has costs but may not have context window
					// Use reasonable default for context window
					return ModelInfo{
						ID:              modelID,
						ContextWindow:   200000,  // reasonable default since not in model-store
						InputCostPer1M:  m.InputCost,
						OutputCostPer1M: m.OutputCost,
					}
				}
			}
		}
	}

	// Fallback defaults for unknown models
	return ModelInfo{
		ID:              modelID,
		ContextWindow:   200000,  // reasonable default
		InputCostPer1M:  3.00,    // mid-range cost
		OutputCostPer1M: 15.00,   // mid-range cost
	}
}

// DefaultModel is the model used when none is specified.
const DefaultModel = "claude-sonnet-4-20250514"
