package server

import (
	"testing"
	"time"
)

// TestReleaseSessionClosesWhatItRemoves is the pin on the defect this file was
// written for: `new_session` removed the live session with a bare Delete and
// closed nothing, so the engine's stores stayed open and the conversation it
// replaced was never summarized into memory.
//
// Session.close is what makes the difference observable without an engine:
// close calls stop, and stop marks the session Completed. A bare Delete leaves
// it Idle.
func TestReleaseSessionClosesWhatItRemoves(t *testing.T) {
	g := &Server{}
	key := "agent:test:main"
	sess := &Session{Key: key, Status: Idle}
	g.sessions.Store(key, sess)

	if closed := g.releaseSession(key); !closed {
		t.Fatal("releaseSession reported it did not close an idle session")
	}

	if _, still := g.sessions.Load(key); still {
		t.Error("the released session is still in the live map")
	}

	sess.mu.Lock()
	status := sess.Status
	sess.mu.Unlock()
	if status != Completed {
		t.Errorf("status after release = %v, want Completed — the session was dropped, not closed", status)
	}
}

// TestReleaseSessionCancelsTheTurnContext checks that the close reaches the
// context the session's work runs under, so nothing the old session started
// keeps running against stores that are being closed.
func TestReleaseSessionCancelsTheTurnContext(t *testing.T) {
	g := &Server{}
	key := "agent:test:main"
	turnCtx, cancel := withoutCallerCancellation(t.Context())
	defer cancel()

	sess := &Session{Key: key, Status: Idle}
	sess.mu.Lock()
	sess.cancel = cancel
	sess.mu.Unlock()
	g.sessions.Store(key, sess)

	g.releaseSession(key)

	select {
	case <-turnCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("releasing the session did not cancel its context")
	}
}

// TestReleaseSessionDoesNotCloseARunningSession pins the half that must NOT
// happen. Session.close cancels the turn and then closes the engine's stores
// without waiting for the turn goroutine to notice, so closing under a live
// turn is a use-after-close. run keeps this unreachable by releasing inside the
// queue; if it ever happens the session is removed and left open, loudly, and
// not closed out from under the turn.
func TestReleaseSessionDoesNotCloseARunningSession(t *testing.T) {
	g := &Server{}
	key := "agent:test:main"
	sess := &Session{Key: key, Status: Running}
	g.sessions.Store(key, sess)

	if closed := g.releaseSession(key); closed {
		t.Fatal("releaseSession closed a session whose turn was still running")
	}

	if _, still := g.sessions.Load(key); still {
		t.Error("the running session was left in the live map, so the fresh session would reuse it")
	}

	sess.mu.Lock()
	status := sess.Status
	sess.mu.Unlock()
	if status != Running {
		t.Errorf("status = %v, want Running — the running session was stopped or closed", status)
	}
}

// TestReleaseSessionOnAnEmptyKeyIsQuiet: `new_session` is the ordinary way to
// start the first conversation of the day, when there is nothing under the key
// at all.
func TestReleaseSessionOnAnEmptyKeyIsQuiet(t *testing.T) {
	g := &Server{}
	if closed := g.releaseSession("agent:test:main"); closed {
		t.Fatal("releaseSession reported closing a session that was never there")
	}
}

// TestFreshSessionRequestIsNotInjectedIntoTheRunningOne pins the ordering the
// fix depends on. Releasing inside the queue means the old session is still in
// the map when run checks whether to inject — and injecting a fresh-session
// request into the running conversation would deliver the message to the exact
// session the caller asked to replace.
func TestFreshSessionRequestIsNotInjectedIntoTheRunningOne(t *testing.T) {
	g := &Server{}
	key := "agent:test:main"
	injections := make(chan string, 4)
	g.sessions.Store(key, &Session{Key: key, Status: Running, injections: injections})

	resp, injected := g.injectIfBusy(key, "start over", RunRequest{NewSession: true})
	if injected {
		t.Fatalf("a new_session request was injected into the running session: %+v", resp)
	}
	if len(injections) != 0 {
		t.Errorf("%d message(s) reached the running session's injection channel", len(injections))
	}
}

// TestOrdinaryRequestStillInjectsIntoTheRunningOne is the other side of that
// guard: refusing the shortcut for every request would queue messages that are
// meant to reach the agent mid-work.
func TestOrdinaryRequestStillInjectsIntoTheRunningOne(t *testing.T) {
	g := &Server{}
	key := "agent:test:main"
	injections := make(chan string, 4)
	g.sessions.Store(key, &Session{Key: key, Status: Running, injections: injections})

	resp, injected := g.injectIfBusy(key, "one more thing", RunRequest{})
	if !injected {
		t.Fatal("an ordinary request to a busy session was not injected")
	}
	if resp == nil || resp.SessionKey != key {
		t.Errorf("response = %+v, want one naming session %s", resp, key)
	}
	select {
	case got := <-injections:
		if got != "one more thing" {
			t.Errorf("injected %q, want %q", got, "one more thing")
		}
	default:
		t.Error("nothing reached the injection channel")
	}
}

// TestIdleSessionIsNotInjectedInto: an idle session takes its message as a
// turn, through the queue. Injecting into it would drop the message into a
// channel nobody is reading.
func TestIdleSessionIsNotInjectedInto(t *testing.T) {
	g := &Server{}
	key := "agent:test:main"
	injections := make(chan string, 4)
	g.sessions.Store(key, &Session{Key: key, Status: Idle, injections: injections})

	if _, injected := g.injectIfBusy(key, "hello", RunRequest{}); injected {
		t.Fatal("a message was injected into an idle session instead of run as a turn")
	}
}
