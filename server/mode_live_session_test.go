package server

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"

	"github.com/kayushkin/inber/engine"
	"github.com/kayushkin/inber/guard"
)

// liveSession puts a session in the map the way the server holds one, so the
// tests below reach getOrCreateSession's existing-session branch and never
// touch createSession — which would want a workspace and a model client.
func liveSession(t *testing.T, key string, mode guard.Mode) *Server {
	t.Helper()
	server := &Server{config: Config{DataDir: t.TempDir()}}
	server.sessions.Store(key, &Session{
		Key:    key,
		Engine: &engine.Engine{Guard: guard.New(guard.Config{Mode: mode})},
	})
	return server
}

// captureLog collects what the package logs while fn runs. The diagnostics
// under test go to the standard logger, which is where the rest of this package
// reports a session-level fact.
func captureLog(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	previousOutput := log.Writer()
	previousFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(previousOutput)
		log.SetFlags(previousFlags)
	})
	fn()
	return buf.String()
}

// TestAModeChangeOnALiveSessionIsStillDroppedAndNowSaysSo pins the defect as
// PRESENT, deliberately.
//
// The existing coverage — mode_request_test.go — calls applyRequestOverrides
// directly, so it is green about a field that never reaches a live session:
// applyRequestOverrides is reached only from createSession, and
// getOrCreateSession returns an existing session before it reads req. This test
// is the same question asked one layer out, through the real
// getOrCreateSession, which is where the gap is.
//
// It asserts the drop, not a fix. Whether mode becomes per-turn on a live
// session is the open call on noteboard todo c14cd190, and whoever answers it
// has to change this assertion — which is the point of writing it this way. A
// test that only pinned the log line would go green under either answer.
func TestAModeChangeOnALiveSessionIsStillDroppedAndNowSaysSo(t *testing.T) {
	const key = "agent:brigid:main"
	server := liveSession(t, key, guard.Autonomous)

	var sess *Session
	output := captureLog(t, func() {
		var err error
		sess, err = server.getOrCreateSession(context.Background(), key, "brigid", AgentConfig{},
			RunRequest{Mode: "observe"}, nil)
		if err != nil {
			t.Fatalf("asking a live session for a readable mode failed the turn: %v", err)
		}
	})

	// The drop itself, unchanged. This is the assertion a fix must break.
	if got := sess.Engine.Guard.Mode(); got != guard.Autonomous {
		t.Errorf("the live session is now enforcing %q — mode became per-turn, which is the open "+
			"decision on todo c14cd190, not something this change was allowed to make", got)
	}

	// And it is no longer silent.
	if !strings.Contains(output, "observe") || !strings.Contains(output, "autonomous") {
		t.Errorf("a request that asked a live autonomous session to observe logged %q — "+
			"the line has to name both the mode asked for and the mode in force, or it cannot "+
			"tell an operator what was dropped", output)
	}
	if !strings.Contains(output, key) {
		t.Errorf("the diagnostic %q does not name the session it is about (%s)", output, key)
	}
}

// TestAnUnreadableModeIsRefusedOnALiveSessionToo closes the sharper half: the
// same typo answered two different ways by the same endpoint.
//
// NewEngine refuses to build a session whose mode it cannot parse, on the
// stated reasoning that the only default available is the one that trusts it
// with everything. One turn later, against the same key, that string was not
// parsed at all — so `mode: "obserev"` was a hard error on request 1 and a
// silent 200 on request 2. This needs no answer to the per-turn question:
// unreadable input is unreadable under every reading of it.
func TestAnUnreadableModeIsRefusedOnALiveSessionToo(t *testing.T) {
	const key = "agent:brigid:main"
	server := liveSession(t, key, guard.Autonomous)

	_, err := server.getOrCreateSession(context.Background(), key, "brigid", AgentConfig{},
		RunRequest{Mode: "obserev"}, nil)
	if err == nil {
		t.Fatal("a live session accepted the execution mode \"obserev\" — session creation refuses " +
			"that exact string, so the same request is an error on the first turn and a silent " +
			"success on the second")
	}
	if !strings.Contains(err.Error(), "obserev") {
		t.Errorf("the refusal %q does not quote the value it refused", err)
	}
	// The same wrap session creation uses, so the two paths read identically.
	if !strings.Contains(err.Error(), "execution mode") {
		t.Errorf("the refusal %q does not read like the one NewEngine gives for the same input", err)
	}
}

// TestASessionAskedForTheModeItIsAlreadyEnforcingSaysNothing keeps the
// diagnostic worth reading.
//
// A client that stamps its mode on every request is not asking for a change,
// and a line per turn saying so would bury the case that matters under the case
// that does not.
func TestASessionAskedForTheModeItIsAlreadyEnforcingSaysNothing(t *testing.T) {
	for _, mode := range []guard.Mode{guard.Observe, guard.Assist, guard.Autonomous} {
		const key = "agent:brigid:main"
		server := liveSession(t, key, mode)

		output := captureLog(t, func() {
			if _, err := server.getOrCreateSession(context.Background(), key, "brigid", AgentConfig{},
				RunRequest{Mode: mode.String()}, nil); err != nil {
				t.Fatalf("mode %q: %v", mode, err)
			}
		})

		if output != "" {
			t.Errorf("a request restating the mode already in force (%q) logged %q — nothing was "+
				"asked for and nothing was dropped", mode, output)
		}
	}
}

// TestARequestWithNoModeOnALiveSessionSaysNothing is the case every request on
// this host actually is. Nothing here sends a mode, so an empty field must stay
// silent — otherwise this diagnostic fires on every turn of every session and
// is worth nothing when it fires for a reason.
func TestARequestWithNoModeOnALiveSessionSaysNothing(t *testing.T) {
	const key = "agent:brigid:main"
	server := liveSession(t, key, guard.Autonomous)

	output := captureLog(t, func() {
		if _, err := server.getOrCreateSession(context.Background(), key, "brigid", AgentConfig{},
			RunRequest{}, nil); err != nil {
			t.Fatalf("a request with no mode failed the turn: %v", err)
		}
	})

	if output != "" {
		t.Errorf("a request naming no mode logged %q", output)
	}
}

// TestTheLiveSessionIsStillReturnedAndStillWired guards the thing a diagnostic
// inserted into the hot path is most likely to break. getOrCreateSession's
// existing-session branch has one job besides returning the session — pointing
// its event callback at this request's writer — and a check added in front of
// that has to leave both alone.
func TestTheLiveSessionIsStillReturnedAndStillWired(t *testing.T) {
	const key = "agent:brigid:main"
	server := liveSession(t, key, guard.Autonomous)
	existing, _ := server.sessions.Load(key)

	delivered := 0
	sess, err := server.getOrCreateSession(context.Background(), key, "brigid", AgentConfig{},
		RunRequest{Mode: "observe"}, func(StreamEvent) { delivered++ })
	if err != nil {
		t.Fatalf("getOrCreateSession: %v", err)
	}
	if sess != existing {
		t.Fatal("getOrCreateSession returned a different session than the one already in the map")
	}
	if onEvent := sess.getOnEvent(); onEvent == nil {
		t.Fatal("the live session came back with no event callback — a request that carries a mode " +
			"would produce a turn nobody can read")
	} else {
		onEvent(StreamEvent{})
	}
	if delivered != 1 {
		t.Errorf("the session's callback delivered %d events, want 1 — setOnEvent did not point at "+
			"this request's writer", delivered)
	}
}

// TestASessionWithNoGuardIsReportedRatherThanAssumed covers the branch that
// cannot answer the question. A session in the map with no guard is an internal
// inconsistency, and reading it as "nothing to report" would be the same
// silence this change exists to remove — but it is not the caller's fault, so
// it does not fail their turn.
func TestASessionWithNoGuardIsReportedRatherThanAssumed(t *testing.T) {
	const key = "agent:brigid:main"
	server := &Server{config: Config{DataDir: t.TempDir()}}
	server.sessions.Store(key, &Session{Key: key, Engine: &engine.Engine{}})

	output := captureLog(t, func() {
		if _, err := server.getOrCreateSession(context.Background(), key, "brigid", AgentConfig{},
			RunRequest{Mode: "observe"}, nil); err != nil {
			t.Fatalf("a guardless session failed the caller's turn: %v", err)
		}
	})

	if !strings.Contains(output, "observe") {
		t.Errorf("a mode asked of a session with no guard logged %q, and so went unrecorded", output)
	}
}
