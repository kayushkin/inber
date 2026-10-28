package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// TestANewSessionDoesNotOpenWithAnotherSessionsConversation. NewEngine loads the
// agent's workspace transcript, one file per agent that whichever of its
// sessions ran last rewrote. createSession only replaced it when the new
// session had a transcript of its own, so a brand-new session opened holding
// the previous session's conversation and answered from it: a model asked to
// read a file replied from an earlier session's edit of that file instead.
//
// Building an engine needs the host's agent configuration, so this skips where
// it cannot run, as the other createSession tests do.
func TestANewSessionDoesNotOpenWithAnotherSessionsConversation(t *testing.T) {
	workspace := t.TempDir()
	previous := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("an earlier session's question")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("an earlier session's answer")),
	}
	encoded, err := json.Marshal(previous)
	if err != nil {
		t.Fatal(err)
	}
	agentWorkspace := filepath.Join(workspace, ".inber", "workspace", "claxon")
	if err := os.MkdirAll(agentWorkspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agentWorkspace, "messages.json"), encoded, 0o644); err != nil {
		t.Fatal(err)
	}

	server := &Server{store: tempStore(t), config: Config{DataDir: t.TempDir()}}
	const key = "agent:claxon:bridge-1"
	if err := server.store.UpsertSession(key, "claxon", "main", SessionLineage{}, nil); err != nil {
		t.Fatal(err)
	}

	created, err := server.createSession(context.Background(), key, "claxon", AgentConfig{Workspace: workspace}, RunRequest{}, nil)
	if err != nil {
		t.Skipf("no engine can be built here (%v)", err)
	}
	if n := len(created.Engine.Messages); n != 0 {
		t.Fatalf("a new session opened with %d messages from the agent's last session", n)
	}
	if _, err := os.Stat(filepath.Join(agentWorkspace, "messages.json")); err != nil {
		t.Errorf("the agent's workspace transcript is gone: %v", err)
	}
}
