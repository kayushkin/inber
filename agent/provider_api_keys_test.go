package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kayushkin/aiauth"
)

// aiauthStoreWithOpenAIProfile is an aiauth store holding one OpenAI api_key
// profile, as ~/.openclaw/agents/main/agent/auth-profiles.json holds old
// profiles on this host.
func aiauthStoreWithOpenAIProfile(t *testing.T, profileKey string) *aiauth.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "auth-profiles.json")
	content := `{"version":1,"profiles":{"openai:default":{"type":"api_key","provider":"openai","key":"` + profileKey + `"}}}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := aiauth.NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

// The configured key wins over an aiauth profile. inber-server used to put
// auth-store's credential in the environment, where aiauth looks before its
// profiles; asking aiauth first now would send with an old profile instead.
func TestAConfiguredKeyWinsOverAnAiauthProfile(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "") // aiauth reads it before its profiles
	store := aiauthStoreWithOpenAIProfile(t, "sk-profile-on-disk")

	client, err := NewModelClient("gpt-4o", nil, store, ProviderAPIKeys{OpenAI: "sk-configured"})
	if err != nil {
		t.Fatal(err)
	}
	if client.OpenAIClient == nil || client.OpenAIClient.APIKey != "sk-configured" {
		t.Fatalf("sent with %+v, want the configured key", client.OpenAIClient)
	}

	client, err = NewModelClient("gpt-4o", nil, store, ProviderAPIKeys{})
	if err != nil {
		t.Fatal(err)
	}
	if client.OpenAIClient.APIKey != "sk-profile-on-disk" {
		t.Fatalf("with no configured key, sent with %q, want aiauth's profile", client.OpenAIClient.APIKey)
	}
}

func TestForProviderNamesEachConfiguredKeyAndNothingElse(t *testing.T) {
	keys := ProviderAPIKeys{Anthropic: "a", OpenAI: "o", Google: "g", OpenRouter: "r"}
	for provider, want := range map[string]string{"anthropic": "a", "openai": "o", "google": "g", "openrouter": "r", "ollama": "", "zhipu": ""} {
		if got := keys.ForProvider(provider); got != want {
			t.Errorf("ForProvider(%q) = %q, want %q", provider, got, want)
		}
	}
}
