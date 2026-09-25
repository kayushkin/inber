package agent

import (
	"fmt"
	"log"
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

// ProviderAPIKeys are the provider credentials the caller was configured with.
// inber-server sets them from ANTHROPIC_API_KEY, OPENAI_API_KEY, GOOGLE_API_KEY
// and OPENROUTER_API_KEY, and sets Anthropic from auth-store instead when it is
// started with --api-key-from-auth-store. An empty field means none.
type ProviderAPIKeys struct {
	Anthropic  string
	OpenAI     string
	Google     string
	OpenRouter string
}

// ForProvider returns the configured key for provider, or "" for a provider
// with no field here.
func (keys ProviderAPIKeys) ForProvider(provider string) string {
	switch provider {
	case "anthropic":
		return keys.Anthropic
	case "openai":
		return keys.OpenAI
	case "google":
		return keys.Google
	case "openrouter":
		return keys.OpenRouter
	default:
		return ""
	}
}

// NewModelClient creates a client for any provider using model-store for
// metadata. The credential is the configured key in keys when there is one,
// and otherwise what aiauth resolves. Either store can be nil.
//
// The configured key comes first because it is what aiauth used to find first:
// aiauth reads ANTHROPIC_API_KEY, OPENAI_API_KEY and GOOGLE_API_KEY before its
// profiles on disk, and inber-server used to put the auth-store credential in
// ANTHROPIC_API_KEY for it to find. Asking aiauth first would hand the server
// whichever old profile is on disk instead.
func NewModelClient(modelIDOrAlias string, ms *modelstore.Store, auth *aiauth.Store, keys ProviderAPIKeys) (*ModelClient, error) {
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

	apiKey := keys.ForProvider(model.Provider)
	if apiKey == "" && auth != nil {
		key, err := auth.ResolveKey(model.Provider)
		if err == nil {
			apiKey = key
		}
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

	// Teach the egress gate the credential this client is about to send with.
	// It is the one secret inber is certain to hold and the one a session can
	// most easily read back — it is resolved from auth-store or aiauth's own
	// config on disk, both readable by read_files — and on this host it is
	// nowhere in the environment, so the redactor cannot learn it any other
	// way. The key travels in a header, never in the body, so redacting the
	// body cannot break authentication.
	registerProviderCredentialWithEgressRedactor(model.Provider, apiKey)

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
//
// Both forms carry EgressRedactionRequestOption. It is passed at construction
// rather than at each call because a client is what a caller is handed: the
// summarizer, the conversation extractor and the turn loop all send through a
// client they were given, and a gate installed per call site would have to be
// remembered at every one of them.
func newAnthropicClient(key string) (*anthropic.Client, error) {
	log.Printf("[auth] creating Anthropic client: key_prefix=%s", key[:min(20, len(key))])

	if strings.HasPrefix(key, "sk-ant-oat01-") {
		c := anthropic.NewClient(
			EgressRedactionRequestOption(),
			option.WithAuthToken(key),
			option.WithHeaderDel("X-Api-Key"),
			option.WithHeader("anthropic-beta", "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,prompt-caching-2024-07-31"),
			option.WithHeader("user-agent", "claude-cli/2.1.44 (external, cli)"),
			option.WithHeader("x-app", "cli"),
		)
		return &c, nil
	}

	c := anthropic.NewClient(
		EgressRedactionRequestOption(),
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

// defaultBaseURL returns the default API base URL for known providers.
//
// The google entry has been read as a bug twice and filed as one once, so the
// measurement is written down here rather than re-derived a fourth time.
// agent/openai.go appends "/chat/completions", giving
// https://generativelanguage.googleapis.com/v1beta/chat/completions. Google's
// docs describe the OpenAI-compatible surface as /v1beta/openai/..., which
// reads like a missing path segment. It is not: Google serves both spellings.
//
// Probed 2026-08-07 without a key. An unknown path under this host answers 404
// with an empty body; a known one gets far enough to answer 400 with a
// structured google.rpc.BadRequest. Both /v1beta/chat/completions and
// /v1beta/openai/chat/completions answer 400 and reject the same unknown field
// with a byte-identical transcoding error, so both are registered routes onto
// the same request proto. It is not a wildcard segment matching anything:
// /v1beta/BOGUSSEG/chat/completions and /v1beta/openai/openai/chat/completions
// both 404. So this URL is reachable and nothing here 404s.
//
// Not proven: a completion with a valid key. No Google credential exists on
// this host, so the round trip is untested. What is ruled out is the 404 —
// which matters because engine/failover.go would have recorded it against the
// model in the host-shared model-store, blaming Google for inber's URL.
//
// Google documents only the /openai spelling, so moving to it is defensible
// hardening against the undocumented alias being withdrawn. That is a change
// to a working URL, not a fix, and it belongs with the open decision about
// whether provider transport lives here at all.
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
