package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kayushkin/inber/engine"
)

// llm-bridge sends its host and project prompt as system_prompt when it starts
// a session. inber used to have no field for it, so every inber session the
// bridge started ran without the prompt every other harness gets, and nothing
// said so.

func TestABridgeSessionKeepsTheSystemPromptItWasCreatedWith(t *testing.T) {
	server := &Server{store: tempStore(t), config: Config{DefaultAgent: "claxon", Agents: map[string]AgentConfig{"claxon": {}}}}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/sessions",
		strings.NewReader(`{"harness":"inber","agent_id":"claxon","system_prompt":"# Host prompt\nBe brief."}`))
	server.handleBridgeSessions(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", recorder.Code, recorder.Body.String())
	}
	var created bridgeSession
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	// createSession reads this when it builds the engine, on the first message
	// and again after a restart.
	text, err := server.store.SessionInjectedSystemPrompt(created.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if text != "# Host prompt\nBe brief." {
		t.Errorf("recorded prompt = %q", text)
	}
}

func TestResumingALiveSessionWithANewSystemPromptReachesItsEngineAndItsRecord(t *testing.T) {
	server := &Server{store: tempStore(t)}
	const key = "agent:claxon:bridge-1"
	if err := server.store.UpsertSession(key, "claxon", "main", SessionLineage{}, nil); err != nil {
		t.Fatal(err)
	}
	if err := server.store.SetSessionInjectedSystemPrompt(key, "old"); err != nil {
		t.Fatal(err)
	}
	live := &engine.Engine{}
	live.SetInjectedSystemPrompt("old")
	server.sessions.Store(key, &Session{Key: key, AgentName: "claxon", Engine: live})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/sessions/"+key+"/resume", strings.NewReader(`{"system_prompt":"new"}`))
	server.handleBridgeResume(recorder, request, key)
	if recorder.Code != http.StatusOK {
		t.Fatalf("resume: %d %s", recorder.Code, recorder.Body.String())
	}
	if got := live.InjectedSystemPrompt(); got != "new" {
		t.Errorf("live engine prompt = %q, want new", got)
	}
	if got, _ := server.store.SessionInjectedSystemPrompt(key); got != "new" {
		t.Errorf("recorded prompt = %q, want new", got)
	}

	// A resume with no body, as an older wrapper sends, leaves it alone.
	recorder = httptest.NewRecorder()
	server.handleBridgeResume(recorder, httptest.NewRequest(http.MethodPost, "/sessions/"+key+"/resume", nil), key)
	if got, _ := server.store.SessionInjectedSystemPrompt(key); got != "new" || live.InjectedSystemPrompt() != "new" {
		t.Errorf("a resume without system_prompt changed it: record %q, engine %q", got, live.InjectedSystemPrompt())
	}
}

func TestRecordingASystemPromptForASessionWithNoRowFails(t *testing.T) {
	store := tempStore(t)
	if err := store.SetSessionInjectedSystemPrompt("agent:claxon:nobody", "text"); err == nil {
		t.Fatal("recorded a prompt for a session with no row")
	}
}

func TestAStoreFromBeforeTheColumnGainsIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE sessions (key TEXT PRIMARY KEY, agent TEXT NOT NULL, kind TEXT NOT NULL DEFAULT 'main',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP, last_active TIMESTAMP DEFAULT CURRENT_TIMESTAMP, message_count INTEGER DEFAULT 0);
		INSERT INTO sessions (key, agent) VALUES ('agent:claxon:old', 'claxon');`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	store, err := NewStore(path)
	if err != nil {
		t.Fatalf("NewStore on an old database: %v", err)
	}
	defer store.Close()
	if text, err := store.SessionInjectedSystemPrompt("agent:claxon:old"); err != nil || text != "" {
		t.Fatalf("old session: %q, %v", text, err)
	}
	if err := store.SetSessionInjectedSystemPrompt("agent:claxon:old", "text"); err != nil {
		t.Fatal(err)
	}
}
