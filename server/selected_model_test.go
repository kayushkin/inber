package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kayushkin/inber/engine"
)

// llm-bridge chooses a model for every session it starts and sends it as
// model. inber's wrapper used to drop it, and inber had nowhere to keep it, so
// a bridge session ran on its agent's own model whatever the bridge chose.

func TestABridgeSessionKeepsTheModelItWasCreatedWith(t *testing.T) {
	server := &Server{store: tempStore(t), config: Config{DefaultAgent: "claxon", Agents: map[string]AgentConfig{"claxon": {}}}}

	recorder := httptest.NewRecorder()
	server.handleBridgeSessions(recorder, httptest.NewRequest(http.MethodPost, "/sessions",
		strings.NewReader(`{"harness":"inber","agent_id":"claxon","model":"gpt-5.6-terra"}`)))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", recorder.Code, recorder.Body.String())
	}
	var created bridgeSession
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if got, err := server.store.SessionSelectedModel(created.SessionID); err != nil || got != "gpt-5.6-terra" {
		t.Fatalf("recorded model = %q, %v", got, err)
	}
}

func TestResumingALiveSessionWithAModelReachesItsEngineAndItsRecord(t *testing.T) {
	server := &Server{store: tempStore(t)}
	const key = "agent:claxon:bridge-1"
	if err := server.store.UpsertSession(key, "claxon", "main", SessionLineage{}, nil); err != nil {
		t.Fatal(err)
	}
	live := &engine.Engine{Model: "claude-sonnet-4-6"}
	server.sessions.Store(key, &Session{Key: key, AgentName: "claxon", Engine: live})

	recorder := httptest.NewRecorder()
	server.handleBridgeResume(recorder, httptest.NewRequest(http.MethodPost, "/sessions/"+key+"/resume",
		strings.NewReader(`{"model":"gpt-5.6-terra"}`)), key)
	if recorder.Code != http.StatusOK {
		t.Fatalf("resume: %d %s", recorder.Code, recorder.Body.String())
	}
	if live.Model != "gpt-5.6-terra" {
		t.Errorf("live engine model = %q", live.Model)
	}
	if got, _ := server.store.SessionSelectedModel(key); got != "gpt-5.6-terra" {
		t.Errorf("recorded model = %q", got)
	}
}

// The model is read when the engine is built, so a session rebuilt after a
// restart runs on it too. Building an engine needs the host's agent
// configuration, so this skips where it cannot run.
func TestASessionIsBuiltOnTheModelRecordedForIt(t *testing.T) {
	server := &Server{store: tempStore(t), config: Config{DataDir: t.TempDir()}}
	const key = "agent:claxon:bridge-2"
	if err := server.store.UpsertSession(key, "claxon", "main", SessionLineage{}, nil); err != nil {
		t.Fatal(err)
	}
	if err := server.store.SetSessionSelectedModel(key, "gpt-5.6-terra"); err != nil {
		t.Fatal(err)
	}
	created, err := server.createSession(context.Background(), key, "claxon",
		AgentConfig{Workspace: t.TempDir(), Model: "claude-sonnet-4-6"}, RunRequest{}, nil)
	if err != nil {
		t.Skipf("no engine can be built here (%v)", err)
	}
	if created.Engine.Model != "gpt-5.6-terra" {
		t.Errorf("engine model = %q, want the recorded gpt-5.6-terra over the agent's own", created.Engine.Model)
	}
}
