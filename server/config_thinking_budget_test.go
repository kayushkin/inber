package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/engine"
)

// config_thinking_budget_test.go — POST /sessions/{id}/config refuses an
// effort whose thinking budget no request could be sent with (todo 79b8f9e5).
//
// The budget lasts for the session, and Anthropic rejects a request whose
// thinking budget is not below max_tokens. So a config that accepted such a
// budget answered 200 here and then made every later turn fail.

func sessionForConfig(t *testing.T) (*Server, *Session) {
	t.Helper()
	g := &Server{}
	g.config.DataDir = t.TempDir()
	eng := &engine.Engine{Model: "model-before"}
	eng.SetThinkingBudget(5000)
	s := &Session{
		Key:        "agent:test:bridge-config",
		AgentName:  "test",
		Engine:     eng,
		injections: make(chan string, 10),
	}
	g.sessions.Store(s.Key, s)
	return g, s
}

func postConfig(g *Server, key, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/sessions/"+key+"/config", strings.NewReader(body))
	rec := httptest.NewRecorder()
	g.handleBridgeConfig(rec, req, key)
	return rec
}

func TestConfigRefusesAnEffortNoRequestCouldCarry(t *testing.T) {
	for _, effort := range []string{
		"high",  // 32000, above the 16384 every request carries
		"16384", // equal is refused too: the budget must be below max_tokens
		"50000",
		"-1",
		"lots",   // used to become a budget of 0 with a 200
		"123abc", // fmt.Sscanf read this as 123
	} {
		t.Run(effort, func(t *testing.T) {
			g, s := sessionForConfig(t)
			rec := postConfig(g, s.Key, `{"model":"model-after","effort":"`+effort+`"}`)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("effort %q answered %d, want %d: %s", effort, rec.Code, http.StatusBadRequest, rec.Body)
			}
			if !strings.Contains(rec.Body.String(), effort) {
				t.Errorf("refusal does not name the effort it refused: %s", rec.Body)
			}
			if got := s.Engine.ThinkingBudget(); got != 5000 {
				t.Errorf("refused config changed the thinking budget to %d, want it left at 5000", got)
			}
			if s.Engine.Model != "model-before" {
				t.Errorf("refused config still changed the model to %q; a refusal must leave the session as it was", s.Engine.Model)
			}
		})
	}
}

func TestConfigAcceptsAnEffortBelowMaxTokens(t *testing.T) {
	for effort, want := range map[string]int64{
		"medium": 10000,
		"low":    0,
		"0":      0,
		"16383":  agent.RequestMaxTokens - 1,
	} {
		t.Run(effort, func(t *testing.T) {
			g, s := sessionForConfig(t)
			rec := postConfig(g, s.Key, `{"effort":"`+effort+`"}`)
			if rec.Code != http.StatusOK {
				t.Fatalf("effort %q answered %d, want 200: %s", effort, rec.Code, rec.Body)
			}
			if got := s.Engine.ThinkingBudget(); got != want {
				t.Errorf("effort %q set a budget of %d, want %d", effort, got, want)
			}
		})
	}
}
