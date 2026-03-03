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
}

// NewModelClient creates a client for any provider using model-store.
// Falls back to direct Anthropic client if model-store is not available.
// store parameter can be nil, in which case it falls back to direct auth.
func NewModelClient(modelIDOrAlias string, store *modelstore.Store) (*ModelClient, error) {
	// Try model-store first (if provided and seeded)
	if store != nil {
		// Try to resolve model (to get model ID from alias)
		model, err := store.ResolveModel(modelIDOrAlias)
		if err == nil {
			// Model found - now try to get credentials
			creds, err := store.Resolve(model.Provider)
			if err == nil {
				// Have both model and credentials from store
				return newClientFromModelStore(creds, model)
			}
			// No credentials in store, fall back to aiauth/env
			// Use the resolved model ID from store
			return newAnthropicFallbackClient(model.ID)
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

	// Build minimal model info (no unnecessary conversion)
	model := &modelstore.Model{
		ID:       modelID,
		Provider: "anthropic",
		Name:     modelID,
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
