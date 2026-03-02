package agent

import (
	"fmt"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	modelstore "github.com/kayushkin/model-store"
	"github.com/kayushkin/aiauth"
)

// ModelClient wraps different provider clients with a unified interface.
type ModelClient struct {
	Provider       string
	Model          *modelstore.Model
	AnthropicClient *anthropic.Client
	// For OpenAI-compatible providers (OpenAI, Google, OpenRouter, Ollama)
	// We'll use anthropic-sdk-go's client with a different base URL
	// since the task says to use OpenAI-compatible API pattern
	BaseClient     interface{} // Will be extended later for non-Anthropic
}

// NewModelClient creates a client for any provider using model-store.
// Falls back to direct Anthropic client if model-store is not available.
func NewModelClient(modelIDOrAlias string) (*ModelClient, error) {
	// Try model-store first
	store, err := modelstore.Open("")
	if err == nil {
		defer store.Close()
		
		// Seed if empty (first run)
		providers, _ := store.Providers()
		if len(providers) == 0 {
			if err := store.Seed(); err != nil {
				// Failed to seed, fall through to fallback
				return newAnthropicFallbackClient(modelIDOrAlias)
			}
		}
		
		// Try to resolve model (to get model ID from alias)
		model, err := store.ResolveModel(modelIDOrAlias)
		if err == nil {
			// Model found - now try to get credentials
			creds, err := store.Resolve(model.Provider)
			if err == nil {
				// Have both model and credentials from store
				return newClientFromModelStore(creds, model)
			}
			// No credentials in store, but we have the resolved model ID
			// Fall back to aiauth/env with the resolved model ID
			return newAnthropicFallbackClientWithModel(model.ID, model)
		}
		// Model not found in store, fall through to fallback
		// (This is expected for direct model IDs that aren't in the catalog)
	}

	// Fallback: assume Anthropic and use existing auth methods
	return newAnthropicFallbackClient(modelIDOrAlias)
}

// newClientFromModelStore creates a client based on provider type.
func newClientFromModelStore(creds *modelstore.Credentials, model *modelstore.Model) (*ModelClient, error) {
	mc := &ModelClient{
		Provider: creds.Provider,
		Model:    model,
	}

	switch creds.Provider {
	case "anthropic":
		// Use aiauth for Anthropic (supports OAuth refresh)
		authStore := aiauth.DefaultStore()
		client, err := authStore.AnthropicClient()
		if err != nil {
			return nil, fmt.Errorf("failed to create Anthropic client: %w", err)
		}
		mc.AnthropicClient = client
		return mc, nil

	case "openai", "google", "openrouter", "ollama":
		// TODO: Implement OpenAI-compatible client
		// For now, return error indicating unsupported
		return nil, fmt.Errorf("provider %s not yet implemented (OpenAI-compatible coming soon)", creds.Provider)

	default:
		return nil, fmt.Errorf("unsupported provider: %s", creds.Provider)
	}
}

// newAnthropicFallbackClient creates an Anthropic client using the old auth methods.
func newAnthropicFallbackClient(modelID string) (*ModelClient, error) {
	// Find model info from built-in registry
	modelInfo, ok := Models[modelID]
	if !ok {
		modelInfo = ModelInfo{
			ID:              modelID,
			ContextWindow:   200000,
			InputCostPer1M:  3.00,
			OutputCostPer1M: 15.00,
		}
	}

	// Convert to model-store format for consistency
	model := &modelstore.Model{
		ID:         modelInfo.ID,
		Provider:   "anthropic",
		Name:       modelInfo.ID,
		MaxTokens:  modelInfo.ContextWindow,
		InputCost:  modelInfo.InputCostPer1M,
		OutputCost: modelInfo.OutputCostPer1M,
	}

	return newAnthropicFallbackClientWithModel(modelInfo.ID, model)
}

// newAnthropicFallbackClientWithModel creates an Anthropic client with a pre-resolved model.
func newAnthropicFallbackClientWithModel(modelID string, model *modelstore.Model) (*ModelClient, error) {
	// Use aiauth as primary method
	authStore := aiauth.DefaultStore()
	client, err := authStore.AnthropicClient()
	if err != nil {
		// Final fallback: environment variable
		apiKey := os.Getenv("ANTHROPIC_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("no Anthropic credentials found (tried aiauth, env)")
		}
		c := anthropic.NewClient(option.WithAPIKey(apiKey))
		client = &c
	}

	return &ModelClient{
		Provider:        "anthropic",
		Model:           model,
		AnthropicClient: client,
	}, nil
}

// GetAnthropicClient returns the Anthropic client or error if not Anthropic provider.
func (mc *ModelClient) GetAnthropicClient() (*anthropic.Client, error) {
	if mc.AnthropicClient == nil {
		return nil, fmt.Errorf("not an Anthropic client")
	}
	return mc.AnthropicClient, nil
}
