package agent

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/kayushkin/aiauth"
	modelstore "github.com/kayushkin/model-store"
)

// ModelClient wraps different provider clients with a unified interface.
type ModelClient struct {
	Provider        string
	Model           *modelstore.Model
	AnthropicClient *anthropic.Client
	OpenAIClient    *OpenAIClient
	IsOAuth         bool // true when using Claude Max OAuth token (needs Claude Code system prompt)
}

// NewModelClient creates a client for any provider using model-store for metadata
// and aiauth for credentials. Either store can be nil for fallback behavior.
func NewModelClient(modelIDOrAlias string, ms *modelstore.Store, auth *aiauth.Store) (*ModelClient, error) {
	// Resolve model metadata
	var model *modelstore.Model
	if ms != nil {
		m, err := ms.ResolveModel(modelIDOrAlias)
		if err == nil {
			model = m
		}
	}

	// Build minimal model info if not found in store
	if model == nil {
		model = &modelstore.Model{
			ID:       modelIDOrAlias,
			Provider: guessProvider(modelIDOrAlias),
			Name:     modelIDOrAlias,
		}
	}

	// Resolve credentials via aiauth
	var apiKey string
	if auth != nil {
		key, err := auth.ResolveKey(model.Provider)
		if err == nil {
			apiKey = key
		}
	}

	// Fallback to env var if aiauth didn't resolve
	if apiKey == "" {
		apiKey = envKeyForProvider(model.Provider)
	}

	if apiKey == "" {
		return nil, fmt.Errorf("no credentials found for provider %q (model %s)", model.Provider, model.ID)
	}

	return newClientFromKey(apiKey, model)
}

// newClientFromKey creates a client for the given provider using an API key/token.
func newClientFromKey(apiKey string, model *modelstore.Model) (*ModelClient, error) {
	mc := &ModelClient{
		Provider: model.Provider,
		Model:    model,
	}

	switch model.Provider {
	case "anthropic":
		client, err := newAnthropicClient(apiKey)
		if err != nil {
			return nil, fmt.Errorf("failed to create Anthropic client: %w", err)
		}
		mc.AnthropicClient = client
		mc.IsOAuth = strings.HasPrefix(apiKey, "sk-ant-oat01-")
		return mc, nil

	case "openai", "google", "openrouter", "ollama":
		baseURL := defaultBaseURL(model.Provider)
		mc.OpenAIClient = NewOpenAIClient(baseURL, apiKey, model.ID)
		return mc, nil

	default:
		// Catch-all: assume OpenAI-compatible
		mc.OpenAIClient = NewOpenAIClient("", apiKey, model.ID)
		return mc, nil
	}
}

// newAnthropicClient creates an Anthropic client from an API key or OAuth token.
func newAnthropicClient(key string) (*anthropic.Client, error) {
	log.Printf("[auth] creating Anthropic client: key_prefix=%s", key[:min(20, len(key))])

	if strings.HasPrefix(key, "sk-ant-oat01-") {
		c := anthropic.NewClient(
			option.WithAuthToken(key),
			option.WithHeaderDel("X-Api-Key"),
			option.WithHeader("anthropic-beta", "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,prompt-caching-2024-07-31"),
			option.WithHeader("user-agent", "claude-cli/2.1.44 (external, cli)"),
			option.WithHeader("x-app", "cli"),
		)
		return &c, nil
	}

	c := anthropic.NewClient(
		option.WithAPIKey(key),
		option.WithHeader("anthropic-beta", "prompt-caching-2024-07-31"),
	)
	return &c, nil
}

// guessProvider infers provider from model ID prefix when model-store is unavailable.
func guessProvider(modelID string) string {
	switch {
	case strings.HasPrefix(modelID, "claude-"):
		return "anthropic"
	case strings.HasPrefix(modelID, "gpt-") || strings.HasPrefix(modelID, "o1-") || strings.HasPrefix(modelID, "o3-"):
		return "openai"
	case strings.HasPrefix(modelID, "gemini-"):
		return "google"
	case strings.HasPrefix(modelID, "glm-") || strings.HasPrefix(modelID, "zai/"):
		return "zhipu"
	default:
		return "anthropic"
	}
}

// envKeyForProvider returns the API key from the canonical env var for a provider.
func envKeyForProvider(provider string) string {
	envVars := map[string]string{
		"anthropic":  "ANTHROPIC_API_KEY",
		"openai":     "OPENAI_API_KEY",
		"google":     "GOOGLE_API_KEY",
		"openrouter": "OPENROUTER_API_KEY",
	}
	if name, ok := envVars[provider]; ok {
		return os.Getenv(name)
	}
	return ""
}

// defaultBaseURL returns the default API base URL for known providers.
func defaultBaseURL(provider string) string {
	switch provider {
	case "openai":
		return "https://api.openai.com/v1"
	case "google":
		return "https://generativelanguage.googleapis.com/v1beta"
	case "openrouter":
		return "https://openrouter.ai/api/v1"
	default:
		return ""
	}
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
