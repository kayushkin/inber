package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kayushkin/forge"
	"github.com/kayushkin/inber/agent"
)

// The defect these tests pin: `forgeDB` is a WorkspaceManager (an interface), and the
// server used to fill it from a `*forge.Forge` variable. A nil pointer stored in an
// interface still carries its type, so the interface is not nil — `forgeDB != nil`
// passed on a box with no forge database and the next call dereferenced a nil receiver.
// inber-server segfaulted on SIGTERM because of it (noteboard 507a7d57), and the same
// lie sits in front of every workspace tool's "forge not available" refusal.

func TestAMissingForgeDatabaseYieldsANilWorkspaceManager(t *testing.T) {
	manager := openWorkspaceManager(filepath.Join(t.TempDir(), "no-such-forge.db"))
	if manager != nil {
		t.Fatalf("a missing forge database must produce an untyped nil, got %#v", manager)
	}
}

func TestAnUnopenableForgeDatabaseYieldsANilWorkspaceManager(t *testing.T) {
	// A directory where the database should be: os.Stat succeeds, forge.Open fails.
	unopenable := filepath.Join(t.TempDir(), "forge.db")
	if err := os.Mkdir(unopenable, 0o755); err != nil {
		t.Fatal(err)
	}
	manager := openWorkspaceManager(unopenable)
	if manager != nil {
		t.Fatalf("an unopenable forge database must produce an untyped nil, got %#v", manager)
	}
}

func TestClosingAServerBuiltWithoutAForgeDatabaseDoesNotPanic(t *testing.T) {
	// This is the reported crash, end to end: HOME with no ~/.config/forge/forge.db,
	// New(), then the Close() that main defers on shutdown.
	home := t.TempDir()
	t.Setenv("HOME", home)

	server, err := New(Config{
		DataDir:      filepath.Join(home, "server"),
		ListenAddr:   ":0",
		DefaultAgent: "test",
		Agents:       map[string]AgentConfig{"test": {Name: "test", Model: "test-model"}},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if server.forgeDB != nil {
		t.Fatalf("no forge database exists under HOME, so forgeDB must be nil, got %#v", server.forgeDB)
	}

	// Before the fix this panicked with a nil pointer dereference inside forge.(*Forge).Close,
	// and everything Close does after the forge was skipped.
	if err := server.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestAWorkspaceToolRefusesInsteadOfCrashingWhenThereIsNoForge(t *testing.T) {
	// The refusal path is the reason the guards exist. A typed nil turned each of these
	// "forge not available" errors into a segfault instead.
	server := newTestServer(t, &mockWorkspaceManager{})
	server.forgeDB = openWorkspaceManager(filepath.Join(t.TempDir(), "no-such-forge.db"))

	server.mu.Lock()
	server.workspaces["brigid-123"] = &forge.Workspace{
		ID:      "brigid-123",
		Repos:   map[string]string{"kayushkin": "/tmp/ws/kayushkin"},
		Primary: "kayushkin",
		Branch:  "spawn/brigid-123",
		Status:  "done",
	}
	server.mu.Unlock()

	input, err := json.Marshal(map[string]any{
		"workspace_id": "brigid-123",
		"reason":       "test",
		"agent":        "claxon",
		"instructions": "test",
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, workspaceTool := range []agent.Tool{
		server.MergeWorkspaceTool(),
		server.RejectWorkspaceTool(),
		server.FixWorkspaceTool(),
	} {
		_, err := workspaceTool.Run(context.Background(), string(input))
		if err == nil {
			t.Errorf("%s: expected a refusal when no forge is available, got success", workspaceTool.Name)
			continue
		}
		if !strings.Contains(err.Error(), "forge not available") {
			t.Errorf("%s: expected \"forge not available\", got %v", workspaceTool.Name, err)
		}
	}
}
