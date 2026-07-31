package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The CLI resumes a session by reading .inber/workspace/<agent>/messages.json,
// and saveResumableState rewrites that same file at the end of every turn. The
// read used to be `if msgs, err := ws.LoadMessages(); err == nil && len(msgs) > 0`,
// which discarded the error: a transcript that could not be parsed started a
// brand new conversation, and the first turn of that conversation overwrote the
// transcript it had failed to read. The file was the only resumable copy — the
// per-invocation session log dir is never read back into an engine, which is
// what saveResumableState's own doc comment says.

func workspaceTranscriptPath(repoRoot, agentName string) string {
	return filepath.Join(repoRoot, ".inber", "workspace", agentName, "messages.json")
}

func TestSetupSessionRefusesAnUnreadableTranscript(t *testing.T) {
	repoRoot := t.TempDir()
	const agentName = "brigid"

	path := workspaceTranscriptPath(repoRoot, agentName)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("make workspace: %v", err)
	}
	if err := os.WriteFile(path, []byte(`[{"role":"user"`), 0644); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	_, _, _, _, _, err := setupSession(repoRoot, agentName, "chat", false, false)

	if err == nil {
		t.Fatal("a corrupt transcript started a fresh session; the next turn would overwrite it")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("the error should name the file to move aside, got: %v", err)
	}

	// And it must not have been touched on the way out.
	after, readErr := os.ReadFile(path)
	if readErr != nil || string(after) != `[{"role":"user"` {
		t.Errorf("the transcript was modified by a run that failed to read it: %q (%v)", after, readErr)
	}
}

func TestSetupSessionTreatsAMissingTranscriptAsAFreshStart(t *testing.T) {
	repoRoot := t.TempDir()

	_, sessionDB, _, messages, turnCounter, err := setupSession(repoRoot, "brigid", "chat", false, false)
	if sessionDB != nil {
		defer sessionDB.Close()
	}

	if err != nil {
		t.Fatalf("a workspace with no transcript is every session's first run: %v", err)
	}
	if len(messages) != 0 || turnCounter != 0 {
		t.Fatalf("want an empty conversation, got %d messages at turn %d", len(messages), turnCounter)
	}
}
