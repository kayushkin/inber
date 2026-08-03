package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kayushkin/inber/memory"
)

// memoryServerOverTempWorkspace builds the smallest Server that handleMemorySave
// will accept: one named agent whose workspace is a throwaway directory.
func memoryServerOverTempWorkspace(t *testing.T) (*Server, string) {
	t.Helper()

	workspace := t.TempDir()
	srv := &Server{config: Config{Agents: map[string]AgentConfig{
		"tester": {Workspace: workspace},
	}}}
	return srv, workspace
}

// saveMemoryOverHTTP posts body to handleMemorySave and returns the new id.
func saveMemoryOverHTTP(t *testing.T, srv *Server, body map[string]any) string {
	t.Helper()

	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	rec := httptest.NewRecorder()
	srv.handleMemorySave(rec, httptest.NewRequest(http.MethodPost, "/api/memory", bytes.NewReader(raw)))
	if rec.Code != http.StatusOK {
		t.Fatalf("save returned %d: %s", rec.Code, rec.Body.String())
	}

	var saved struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil {
		t.Fatalf("decode save response: %v", err)
	}
	return saved.ID
}

// readBackSource opens the workspace store directly and returns the stored
// source. Reading the row rather than a response body is deliberate: the
// question is what was persisted, not what the handler echoed.
func readBackSource(t *testing.T, workspace, id string) string {
	t.Helper()

	store, err := memory.OpenOrCreate(workspace)
	if err != nil {
		t.Fatalf("open workspace store: %v", err)
	}
	defer store.Close()

	m, err := store.Get(id)
	if err != nil {
		t.Fatalf("get %s: %v", id, err)
	}
	return m.Source
}

// A memory saved over HTTP with no stated provenance must not be recorded as
// the user's. "user" is the top of the trust order the field exists to express,
// and an omitted field is not a claim — it is the absence of one.
func TestMemorySaveWithNoSourceDoesNotClaimTheUserSaidIt(t *testing.T) {
	srv, workspace := memoryServerOverTempWorkspace(t)

	id := saveMemoryOverHTTP(t, srv, map[string]any{
		"agent":   "tester",
		"content": "a memory whose provenance nobody stated",
	})

	got := readBackSource(t, workspace, id)
	if got == "user" {
		t.Fatal("an omitted source was recorded as \"user\": unknown provenance must not be promoted to the most trusted value")
	}
	if got != "" {
		t.Fatalf("an omitted source should stay empty, got %q", got)
	}
}

// The complement: a caller that does state provenance keeps it. Without this,
// discarding the field outright would satisfy the test above.
func TestMemorySaveKeepsTheSourceACallerActuallyGave(t *testing.T) {
	srv, workspace := memoryServerOverTempWorkspace(t)

	for _, source := range []string{"user", "agent", "system", "extraction"} {
		id := saveMemoryOverHTTP(t, srv, map[string]any{
			"agent":   "tester",
			"content": "a memory from " + source,
			"source":  source,
		})

		if got := readBackSource(t, workspace, id); got != source {
			t.Errorf("source %q was stored as %q", source, got)
		}
	}
}
