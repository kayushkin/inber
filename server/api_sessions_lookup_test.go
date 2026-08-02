package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kayushkin/inber/session"
)

// writeSessionArtifact puts one of the non-transcript files a real session
// directory holds next to the transcript.
func writeSessionArtifact(t *testing.T, workspace string, segments ...string) {
	t.Helper()
	path := filepath.Join(append([]string{workspace, "logs"}, segments...)...)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// The per-session endpoints — /context, /timeline, /prompts — all resolve an id
// through locateSession. It used to ask whether the id appeared anywhere in the
// path and take the first hit, which answered three different questions wrong
// and had no error path for any of them.

// A truncated id is the live case: session ids are <timestamp>_<4 hex>, so an
// id cut short in a URL is a prefix of a longer one. The old search served that
// longer session's transcript as the answer.
func TestAPrefixOfASessionIDIsNotFound(t *testing.T) {
	workspace := t.TempDir()
	writeSessionLog(t, workspace, "claxon", "2026-08-02_010000_cana")

	server := &Server{config: Config{Agents: map[string]AgentConfig{
		"claxon": {Workspace: workspace},
	}}}

	logFile, err := server.findSessionLogFile("2026-08-02_010000_ca")
	if err != nil {
		t.Fatalf("findSessionLogFile: %v", err)
	}
	if logFile != "" {
		t.Errorf("a prefix id resolved to %q; it names no session", logFile)
	}

	// The counterweight: the full id still resolves, so "find nothing" is not
	// how this passes.
	logFile, err = server.findSessionLogFile("2026-08-02_010000_cana")
	if err != nil {
		t.Fatalf("findSessionLogFile: %v", err)
	}
	if logFile == "" {
		t.Error("the full session id did not resolve")
	}
}

// The second way it was wrong: the id was matched against every directory above
// the session as well, so an agent directory named like a session id claimed
// every session under it.
func TestAnAgentDirectoryNamedLikeASessionIsNotASession(t *testing.T) {
	workspace := t.TempDir()
	writeSessionLog(t, workspace, "2026-08-02_010000_cana", "2026-08-02_020000_beef")

	server := &Server{config: Config{Agents: map[string]AgentConfig{
		"claxon": {Workspace: workspace},
	}}}

	logFile, err := server.findSessionLogFile("2026-08-02_010000_cana")
	if err != nil {
		t.Fatalf("findSessionLogFile: %v", err)
	}
	if logFile != "" {
		t.Errorf("the agent directory answered as a session: %q", logFile)
	}
}

// The third: the first hit in walk order won and nothing checked for a second.
// Two roots can hold the same id — ids are minted from a clock and a random
// suffix, per workspace, with nothing in them naming the root — and picking one
// serves the loser's transcript under the winner's id. The caller is told the
// question is broken instead.
func TestASessionIDInTwoLogsRootsIsRefusedRatherThanPicked(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()
	writeSessionLog(t, first, "claxon", "2026-08-02_010000_cana")
	writeSessionLog(t, second, "fionn", "2026-08-02_010000_cana")

	server := &Server{config: Config{Agents: map[string]AgentConfig{
		"claxon": {Workspace: first},
		"fionn":  {Workspace: second},
	}}}

	logFile, err := server.findSessionLogFile("2026-08-02_010000_cana")
	if err == nil {
		t.Fatalf("an id naming two sessions resolved to %q", logFile)
	}
	if !strings.Contains(err.Error(), "2026-08-02_010000_cana") {
		t.Errorf("the error does not name the ambiguous session: %v", err)
	}
	if !strings.Contains(err.Error(), session.LogsRoot(first)) || !strings.Contains(err.Error(), session.LogsRoot(second)) {
		t.Errorf("the error does not name both roots, so it cannot be acted on: %v", err)
	}

	// findLogsDir reads through the same resolution and must refuse the same
	// question; it is the one three of the four endpoints call.
	if _, err := server.findLogsDir("2026-08-02_010000_cana"); err == nil {
		t.Error("findLogsDir picked one of two sessions")
	}
}

// Two sessions of the same id inside ONE root is the same coin flip one level
// down, and walk order decides it. Refused for the same reason.
func TestASessionIDTwiceInOneRootIsRefused(t *testing.T) {
	workspace := t.TempDir()
	writeSessionLog(t, workspace, "claxon", "2026-08-02_010000_cana")
	writeSessionLog(t, workspace, "fionn", "2026-08-02_010000_cana")

	server := &Server{config: Config{Agents: map[string]AgentConfig{
		"claxon": {Workspace: workspace},
	}}}

	if logFile, err := server.findSessionLogFile("2026-08-02_010000_cana"); err == nil {
		t.Fatalf("an id naming two sessions in one root resolved to %q", logFile)
	}
}

// An empty id names no session. Every path that is not a transcript derives the
// empty id, so an unguarded equality match would answer the first file it saw.
//
// The fixture has to hold a file that is NOT a transcript for that to be
// checkable at all: a session directory holding only session.jsonl cannot tell
// the guard from its absence, because nothing in it derives the empty id.
// Real log directories are never that bare — messages.json, checkpoint.json and
// prompts/ all sit alongside the transcript.
func TestAnEmptySessionIDIsNotFound(t *testing.T) {
	workspace := t.TempDir()
	writeSessionLog(t, workspace, "claxon", "2026-08-02_010000_cana")
	writeSessionArtifact(t, workspace, "claxon", "2026-08-02_010000_cana", "messages.json")

	server := &Server{config: Config{Agents: map[string]AgentConfig{
		"claxon": {Workspace: workspace},
	}}}

	logFile, err := server.findSessionLogFile("")
	if err != nil {
		t.Fatalf("findSessionLogFile: %v", err)
	}
	if logFile != "" {
		t.Errorf("the empty id resolved to %q", logFile)
	}
}
