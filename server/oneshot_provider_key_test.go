package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kayushkin/inber/agent"
)

// POST /api/oneshot sends with the configured Anthropic key, not with whatever
// ANTHROPIC_API_KEY holds: inber-server resolves its key from auth-store and no
// longer puts it in the environment.
func TestOneShotSendsWithTheConfiguredAnthropicKey(t *testing.T) {
	var sentKey string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sentKey = r.Header.Get("X-Api-Key")
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"msg_1","type":"message","role":"assistant","model":"claude-test","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer provider.Close()
	t.Setenv("ANTHROPIC_BASE_URL", provider.URL)
	t.Setenv("ANTHROPIC_API_KEY", "ambient-key-from-the-environment")

	g := &Server{config: Config{ProviderAPIKeys: agent.ProviderAPIKeys{Anthropic: "configured-key"}}}
	recorder := httptest.NewRecorder()
	g.handleOneShot(recorder, httptest.NewRequest(http.MethodPost, "/api/oneshot", strings.NewReader(`{"prompt":"hi"}`)))

	if recorder.Code != http.StatusOK {
		t.Fatalf("POST /api/oneshot = %d: %s", recorder.Code, recorder.Body)
	}
	if sentKey != "configured-key" {
		t.Fatalf("sent with %q, want the configured key", sentKey)
	}
}

// With no configured key it says so, instead of sending with none.
func TestOneShotWithNoConfiguredKeyAnswers503(t *testing.T) {
	g := &Server{}
	recorder := httptest.NewRecorder()
	g.handleOneShot(recorder, httptest.NewRequest(http.MethodPost, "/api/oneshot", strings.NewReader(`{"prompt":"hi"}`)))
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), "no Anthropic API key") {
		t.Fatalf("POST /api/oneshot with no key = %d: %s", recorder.Code, recorder.Body)
	}
}
