package agent

import (
	"testing"

	modelstore "github.com/kayushkin/model-store"
)

// An API key for provider "openai" is sent to OpenAI's Responses endpoint,
// because its newest models refuse tool requests on chat completions.
func TestAnOpenAIAPIKeyClientSendsToTheResponsesEndpoint(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	client, err := NewModelClient("gpt-5.6-terra", nil, nil, ProviderCredentials{OpenAI: "sk-configured"})
	if err != nil {
		t.Fatal(err)
	}
	openAIClient := client.OpenAIClient
	if openAIClient == nil || !openAIClient.UsesResponsesAPI ||
		openAIClient.BaseURL != OpenAIResponsesAPIEndpoint || openAIClient.APIKey != "sk-configured" ||
		openAIClient.CodexSubscriptionAuthFilePath != "" {
		t.Fatalf("client = %+v, want the API-key Responses transport on %s", openAIClient, OpenAIResponsesAPIEndpoint)
	}
}

// A key aiauth resolves takes the same path as a configured one: the
// transport follows the provider, not where the key came from.
func TestAnOpenAIKeyFromAiauthAlsoSendsToTheResponsesEndpoint(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	store := aiauthStoreWithOpenAIProfile(t, "sk-profile-on-disk")
	client, err := NewModelClient("gpt-4o", nil, store, ProviderCredentials{})
	if err != nil {
		t.Fatal(err)
	}
	if !client.OpenAIClient.UsesResponsesAPI || client.OpenAIClient.BaseURL != OpenAIResponsesAPIEndpoint {
		t.Fatalf("client = %+v, want the API-key Responses transport", client.OpenAIClient)
	}
}

// The transport follows the provider model-store names, not the model's name:
// an OpenAI-looking id served by another provider keeps chat completions.
func TestTheProviderNotTheModelNameChoosesTheTransport(t *testing.T) {
	client, err := newClientFromKey("sk-or-key", &modelstore.Model{ID: "gpt-5.6-terra", Provider: "openrouter"})
	if err != nil {
		t.Fatal(err)
	}
	if client.OpenAIClient.UsesResponsesAPI || client.OpenAIClient.BaseURL != "https://openrouter.ai/api/v1" {
		t.Fatalf("client = %+v, want OpenRouter's chat-completions surface", client.OpenAIClient)
	}
}

// Providers that copy OpenAI's chat-completions surface keep it, including the
// catch-all for providers inber does not name.
func TestOpenAICompatibleProvidersKeepChatCompletions(t *testing.T) {
	wantBaseURL := map[string]string{
		"google":     "https://generativelanguage.googleapis.com/v1beta",
		"openrouter": "https://openrouter.ai/api/v1",
		"ollama":     "",
		"zhipu":      "",
	}
	for provider, want := range wantBaseURL {
		client, err := newClientFromKey("key-"+provider, &modelstore.Model{ID: "some-model", Provider: provider})
		if err != nil {
			t.Fatalf("%s: %v", provider, err)
		}
		if client.OpenAIClient == nil || client.OpenAIClient.UsesResponsesAPI || client.OpenAIClient.BaseURL != want {
			t.Errorf("%s: client = %+v, want chat completions on %q", provider, client.OpenAIClient, want)
		}
	}
}

// A configured Codex subscription still serves every OpenAI model, ahead of
// any API key, and does not also turn on the API-key transport.
func TestTheCodexSubscriptionStillWinsOverAnOpenAIAPIKey(t *testing.T) {
	client, err := NewModelClient("gpt-5.6-terra", nil, nil, ProviderCredentials{
		OpenAI:                          "sk-configured",
		OpenAICodexSubscriptionAuthFile: "/nonexistent/auth.json",
	})
	if err != nil {
		t.Fatal(err)
	}
	openAIClient := client.OpenAIClient
	if openAIClient.UsesResponsesAPI || openAIClient.CodexSubscriptionAuthFilePath != "/nonexistent/auth.json" ||
		openAIClient.BaseURL != CodexSubscriptionResponsesEndpoint || openAIClient.APIKey != "" {
		t.Fatalf("client = %+v, want the Codex subscription transport", openAIClient)
	}
}
