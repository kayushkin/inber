package agent

import (
	"fmt"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	modelstore "github.com/kayushkin/model-store"
)

// ModelClient wraps different provider clients with a unified interface.
type ModelClient struct {
	Provider        string
	Model           *modelstore.Model
	AnthropicClient *anthropic.Client
	OpenAIClient    *OpenAIClient
}

// NewModelClient creates a client for any provider using model-store.
// Falls back to direct Anthropic client if model-store is not available.
// store parameter can be nil, in which case it falls back to direct auth.
func NewModelClient(modelIDOrAlias string, store *modelstore.Store) (*ModelClient, error) {
	// Try model-store first (if provided)
	if store != nil {
		// Use ResolveForModel which handles model-specific credentials and falls back to provider-level
		creds, model, err := store.ResolveForModel(modelIDOrAlias)
		if err == nil {
			// Have both model and credentials from store
			return newClientFromCredentials(creds, model)
		}
		// Model not found in store, fall through to fallback
		// (This is expected for direct model IDs that aren't in the catalog)
	}

	// Fallback: assume Anthropic and use env var
	return newAnthropicFallbackClient(modelIDOrAlias)
}

// newClientFromCredentials creates a client based on credential type and provider.
func newClientFromCredentials(creds *modelstore.Credentials, model *modelstore.Model) (*ModelClient, error) {
	mc := &ModelClient{
		Provider: creds.Provider,
		Model:    model,
	}

	switch creds.Provider {
	case "anthropic":
		client, err := newAnthropicClientFromCreds(creds)
		if err != nil {
			return nil, fmt.Errorf("failed to create Anthropic client: %w", err)
		}
		mc.AnthropicClient = client
		return mc, nil

	case "openai", "google", "openrouter", "ollama":
		// OpenAI-compatible providers
		baseURL := creds.BaseURL
		if baseURL == "" {
			// Default base URLs for known providers
			switch creds.Provider {
			case "openai":
				baseURL = "https://api.openai.com/v1"
			case "google":
				baseURL = "https://generativelanguage.googleapis.com/v1beta"
			case "openrouter":
				baseURL = "https://openrouter.ai/api/v1"
			}
		}
		apiKey := modelstore.ActiveKey(creds)
		client := NewOpenAIClient(baseURL, apiKey, model.ID)
		mc.OpenAIClient = client
		return mc, nil

	default:
		// Catch-all: assume OpenAI-compatible for unknown providers
		apiKey := modelstore.ActiveKey(creds)
		client := NewOpenAIClient(creds.BaseURL, apiKey, model.ID)
		mc.OpenAIClient = client
		return mc, nil
	}
}

// newAnthropicClientFromCreds creates an Anthropic client from model-store credentials.
// Handles both OAuth tokens (sk-ant-oat01-*) and API keys (sk-ant-api03-*).
func newAnthropicClientFromCreds(creds *modelstore.Credentials) (*anthropic.Client, error) {
	key := modelstore.ActiveKey(creds)
	if key == "" {
		return nil, fmt.Errorf("no active key in credential %s", creds.ID)
	}

	if strings.HasPrefix(key, "sk-ant-oat01-") {
		// OAuth tokens require Bearer auth + beta headers.
		// Explicitly clear apiKey to prevent SDK's DefaultClientOptions from
		// also sending x-api-key (which the server would treat as a no-credits API key).
		c := anthropic.NewClient(
			option.WithAPIKey(""),
			option.WithAuthToken(key),
			option.WithHeader("anthropic-beta", "claude-code-20250219,oauth-2025-04-20,prompt-caching-2024-07-31"),
			option.WithHeader("user-agent", "claude-cli/2.1.44 (external, cli)"),
			option.WithHeader("x-app", "cli"),
		)
		return &c, nil
	}

	// API keys use x-api-key header
	c := anthropic.NewClient(
		option.WithAPIKey(key),
		option.WithHeader("anthropic-beta", "prompt-caching-2024-07-31"),
	)
	return &c, nil
}

// newAnthropicFallbackClient creates an Anthropic client using environment variable.
// Returns error if modelID appears to be for a different provider (e.g., glm-*).
func newAnthropicFallbackClient(modelID string) (*ModelClient, error) {
	// Detect non-Anthropic models and return clear error
	if strings.HasPrefix(modelID, "glm-") || strings.HasPrefix(modelID, "zai/") {
		return nil, fmt.Errorf("model %s requires zhipu/zai provider (not configured)", modelID)
	}
	if strings.HasPrefix(modelID, "gpt-") || strings.HasPrefix(modelID, "o1-") || strings.HasPrefix(modelID, "o3-") {
		return nil, fmt.Errorf("model %s requires openai provider (not configured)", modelID)
	}
	if strings.HasPrefix(modelID, "gemini-") {
		return nil, fmt.Errorf("model %s requires google provider (not configured)", modelID)
	}

	// Final fallback: environment variable
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("no Anthropic credentials found (tried model-store, env)")
	}

	c := anthropic.NewClient(
		option.WithAPIKey(apiKey),
		option.WithHeader("anthropic-beta", "prompt-caching-2024-07-31"),
	)

	// Build minimal model info
	model := &modelstore.Model{
		ID:       modelID,
		Provider: "anthropic",
		Name:     modelID,
	}

	return &ModelClient{
		Provider:        "anthropic",
		Model:           model,
		AnthropicClient: &c,
	}, nil
}

// GetAnthropicClient returns the Anthropic client or error if not Anthropic provider.
func (mc *ModelClient) GetAnthropicClient() (*anthropic.Client, error) {
	if mc.AnthropicClient == nil {
		return nil, fmt.Errorf("not an Anthropic client")
	}
	return mc.AnthropicClient, nil
}

// GetOpenAIClient returns the OpenAI client or error if not OpenAI-compatible provider.
func (mc *ModelClient) GetOpenAIClient() (*OpenAIClient, error) {
	if mc.OpenAIClient == nil {
		return nil, fmt.Errorf("not an OpenAI-compatible client")
	}
	return mc.OpenAIClient, nil
}

// IsOpenAI returns true if this client uses OpenAI-compatible API.
func (mc *ModelClient) IsOpenAI() bool {
	return mc.OpenAIClient != nil
}
