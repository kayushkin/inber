package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// APIKey returns the Anthropic API key.
// Priority: ANTHROPIC_API_KEY env var → OpenClaw auth store (token or api_key).
func APIKey() string {
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		return key
	}
	return resolveOpenClawAuthKey()
}

// resolveOpenClawAuthKey reads Anthropic credentials from OpenClaw's auth store.
// Returns all found keys ordered by preference (token first, then api_key).
func resolveOpenClawAuthKey() string {
	keys := ResolveOpenClawAuthKeys()
	if len(keys) > 0 {
		return keys[0]
	}
	return ""
}

// ResolveOpenClawAuthKeys returns all Anthropic keys from OpenClaw's auth store,
// ordered by preference (token/oauth first, then api_key).
func ResolveOpenClawAuthKeys() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	authPath := filepath.Join(home, ".openclaw", "agents", "main", "agent", "auth-profiles.json")
	return readAuthProfileKeys(authPath)
}

// readAuthProfileKeys extracts all Anthropic keys from an auth-profiles.json file.
// Returns token types first (Max subscription), then api_key types.
func readAuthProfileKeys(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var store struct {
		Profiles map[string]struct {
			Type     string `json:"type"`
			Provider string `json:"provider"`
			Key      string `json:"key"`
			Token    string `json:"token"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal(data, &store); err != nil {
		return nil
	}

	var tokens, apiKeys []string
	for _, profile := range store.Profiles {
		if profile.Provider != "anthropic" {
			continue
		}
		if profile.Type == "token" && profile.Token != "" {
			tokens = append(tokens, profile.Token)
		}
		if profile.Type == "api_key" && profile.Key != "" {
			apiKeys = append(apiKeys, profile.Key)
		}
	}
	return append(tokens, apiKeys...)
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
