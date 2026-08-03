package server

import (
	"context"
	"testing"

	"github.com/kayushkin/inber/engine"
	sessionMod "github.com/kayushkin/inber/session"
)

func equalToolNames(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// TestResumingASessionKeepsTheToolsItHadTakenAway is this fix stated as the
// defect it closes.
//
// `disabled_tools` reached an engine through one door — POST
// /sessions/{id}/config — and was stored in engine memory, on no config struct,
// serialized by nothing. It therefore had one session's lifetime in one
// process. handleBridgeResume rebuilds a session that is asked for and is no
// longer in memory, and the rebuild put every tool back on the wire: a caller
// who had taken `shell` away was told "updated", and the session came back with
// `shell` in its tool set, no log line and no error.
//
// It runs both real functions — persistSessionState writes and
// restoreDisabledTools reads — but stops short of the HTTP handler, because
// standing up an engine through NewEngine needs a workspace and a model client.
// What it does cover is the state that crosses the process boundary.
func TestResumingASessionKeepsTheToolsItHadTakenAway(t *testing.T) {
	server := &Server{config: Config{DataDir: t.TempDir()}}
	const key = "agent:brigid:main"

	original := &engine.Engine{}
	original.SetDisabledTools([]string{"shell", "write_files"})
	server.persistSessionState(&Session{Key: key, Engine: original})

	// The rebuild. Nothing on the resume path configures a disabled set, so this
	// is the engine handed back to the caller.
	rebuilt := &engine.Engine{}
	server.restoreDisabledTools(key, rebuilt)

	if got, want := rebuilt.DisabledToolNames(), []string{"shell", "write_files"}; !equalToolNames(got, want) {
		t.Errorf("the resumed session has %v off the wire, want %v — a tool a caller took away came back", got, want)
	}
}

// TestResumingASessionThatReEnabledEverythingDoesNotTakeToolsBackAway. The
// re-enable-everything request is an empty set, and a save that skipped it
// would leave the earlier record in place — so the rebuild would honour a
// request the caller had already withdrawn.
func TestResumingASessionThatReEnabledEverythingDoesNotTakeToolsBackAway(t *testing.T) {
	server := &Server{config: Config{DataDir: t.TempDir()}}
	const key = "agent:brigid:main"

	eng := &engine.Engine{}
	eng.SetDisabledTools([]string{"shell"})
	server.persistSessionState(&Session{Key: key, Engine: eng})

	eng.SetDisabledTools(nil)
	server.persistSessionState(&Session{Key: key, Engine: eng})

	rebuilt := &engine.Engine{}
	server.restoreDisabledTools(key, rebuilt)

	if got := rebuilt.DisabledToolNames(); len(got) != 0 {
		t.Errorf("the resumed session has %v off the wire, want nothing — the withdrawn request outlived its withdrawal", got)
	}
}

// TestCreatingASessionThatHasNoRecordGetsEveryToolItWasConfiguredWith.
// restoreDisabledTools runs on every createSession, not only the resume, so a
// first-time creation and a spawned or forked child — neither of which has a
// record under its own key — must come out of it exactly as they were
// configured. That is also what keeps this change silent on the open question
// of whether a child should inherit its parent's set (noteboard todo 65301d09).
func TestCreatingASessionThatHasNoRecordGetsEveryToolItWasConfiguredWith(t *testing.T) {
	server := &Server{config: Config{DataDir: t.TempDir()}}

	fresh := &engine.Engine{}
	server.restoreDisabledTools("agent:brigid:child-1", fresh)

	if got := fresh.DisabledToolNames(); len(got) != 0 {
		t.Errorf("a session with no record came back with %v off the wire", got)
	}
}

// The caller, not the reader. restoreDisabledTools answering correctly says
// nothing about whether createSession — the one place a session is rebuilt from
// what was persisted under its key — asks it. Deleting that one line leaves
// every test above this one green, which is the shape of the defect itself:
// the tool went back on the wire and nothing said so.
//
// Building an engine needs a workspace and the host's agent configuration, so
// this skips where it cannot run rather than failing; the assertions above it
// never skip.
func TestARebuiltSessionComesBackWithTheToolsItHadTakenAwayStillGone(t *testing.T) {
	workspace := t.TempDir()
	server := &Server{store: tempStore(t), config: Config{DataDir: t.TempDir()}}
	const key = "agent:claxon:main"
	if err := server.store.UpsertSession(key, "claxon", "main", SessionLineage{}, nil); err != nil {
		t.Fatalf("record the session: %v", err)
	}
	if err := sessionMod.SaveDisabledTools(dirForSessionKey(server, key), []string{"shell"}); err != nil {
		t.Fatalf("record the disabled tool: %v", err)
	}

	rebuilt, err := server.createSession(context.Background(), key, "claxon",
		AgentConfig{Workspace: workspace}, RunRequest{}, nil)
	if err != nil {
		t.Skipf("no engine can be built here (%v)", err)
	}

	if got, want := rebuilt.Engine.DisabledToolNames(), []string{"shell"}; !equalToolNames(got, want) {
		t.Fatalf("the rebuilt session has %v off the wire, want %v", got, want)
	}
	for _, name := range rebuilt.Engine.EnabledToolNames() {
		if name == "shell" {
			t.Errorf("shell is back on the wire after a rebuild, and the caller that took it away was told %q", "updated")
		}
	}
}
