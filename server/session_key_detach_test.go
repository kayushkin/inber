package server

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kayushkin/inber/guard"
	sessionMod "github.com/kayushkin/inber/session"
)

// `detach` mints a key of its own precisely so a one-off run cannot touch the
// agent's main session. It used to stamp that key from a bare
// time.Now().UnixMilli() with no check, which is the one way that promise
// breaks: two detached runs of one agent inside a millisecond, or a detached
// run landing on a key an earlier one left on disk, inherit each other exactly
// as two children did before mintChildSessionKey existed.
//
// These tests hold the millisecond still, which is the only way to reach the
// window — a real clock produces the collision far too rarely to wait for.

const detachAgent = "claxon"

// frozenMillisecond scripts the proposal half of the detached mint as if every
// attempt read the same clock. attempt 0 returns the bare key; later attempts
// disambiguate, exactly as detachedSessionKey does.
func frozenMillisecond(millis int64) func(agentName string, attempt int) string {
	return func(agentName string, attempt int) string {
		key := fmt.Sprintf("agent:%s:detach-%d", agentName, millis)
		if attempt > 0 {
			key = fmt.Sprintf("%s-%d", key, attempt)
		}
		return key
	}
}

func TestTwoDetachedRunsInOneMillisecondGetDifferentKeys(t *testing.T) {
	server := serverWithDataDir(t)
	clock := frozenMillisecond(1754570000000)

	first, err := server.mintDetachedSessionKeyFrom(detachAgent, clock)
	if err != nil {
		t.Fatalf("mint the first detached key: %v", err)
	}
	// The first run's session is not in g.sessions yet — the reservation is
	// what has to hold the key in the meantime.
	second, err := server.mintDetachedSessionKeyFrom(detachAgent, clock)
	if err != nil {
		t.Fatalf("mint the second detached key: %v", err)
	}

	if first == second {
		t.Fatalf("two detached runs in one millisecond were both handed %s; the second inherits the first's guard state, agent and place in the session map", first)
	}
}

func TestADetachedKeyLeftOnDiskIsNotHandedOutAgain(t *testing.T) {
	server := serverWithDataDir(t)
	clock := frozenMillisecond(1754570001000)

	// A detached run that has already retired, with the caps it ran under and
	// the money it spent against them still on disk.
	taken := clock(detachAgent, 0)
	retiredDir := filepath.Join(server.config.DataDir, "sessions", taken)
	if err := sessionMod.SaveGuardState(retiredDir, guard.State{MaxCost: 5, Cost: 4.80, Turns: 12}); err != nil {
		t.Fatalf("persist the retired run: %v", err)
	}

	// The premise, asserted rather than assumed: this really is a directory a
	// rebuild reads a budget out of. If the sidecar ever moves, this fails here
	// and says so, instead of leaving the mint checking an empty path.
	restored := guard.New(guard.Config{MaxCost: 5})
	server.restoreGuardState(taken, restored)
	if restored.State().Cost != 4.80 {
		t.Fatalf("a run rebuilt on %s inherited $%.2f of spend; the whole point of not reusing the key is that it inherits $4.80",
			taken, restored.State().Cost)
	}

	minted, err := server.mintDetachedSessionKeyFrom(detachAgent, clock)
	if err != nil {
		t.Fatalf("mint a detached key: %v", err)
	}
	if minted == taken {
		t.Fatalf("a new detached run was handed %s, the retired run's key, and starts having already spent $4.80 of a $5 cap", taken)
	}
}

// A detached run is not a child of anything. childKeySeparator is what
// backfillSessionLineageFromChildKeys counts to derive spawn depth, so a key
// wearing one would be repaired into a child of an agent it has no parent in.
func TestADetachedKeyIsNotShapedLikeAChildsKey(t *testing.T) {
	for attempt := 0; attempt < 4; attempt++ {
		key := detachedSessionKey(detachAgent, attempt)
		if strings.Contains(key, childKeySeparator) {
			t.Errorf("detached key %q (attempt %d) wears %q, so lineage backfill reads it as a child",
				key, attempt, childKeySeparator)
		}
	}
}

// The disambiguator appears only on a retry, so the key shape a session list
// has always shown is unchanged in the case that is not a collision.
func TestTheFirstDetachedKeyProposedIsTheBareMillisecond(t *testing.T) {
	first := detachedSessionKey(detachAgent, 0)
	retry := detachedSessionKey(detachAgent, 1)

	prefix := "agent:" + detachAgent + ":detach-"
	if !strings.HasPrefix(first, prefix) {
		t.Fatalf("detached key %q does not start with %q", first, prefix)
	}
	if strings.Contains(strings.TrimPrefix(first, prefix), "-") {
		t.Errorf("the first proposal %q carries a disambiguator; it should be the bare millisecond", first)
	}
	if !strings.HasSuffix(retry, "-1") {
		t.Errorf("the second proposal %q does not disambiguate itself, so a frozen millisecond proposes the same taken key %d times",
			retry, sessionKeyMintAttempts)
	}
}

// Every test above proves the mint works. This one proves a detached run
// reaches it: put the bare fmt.Sprintf back into the key resolution and the
// mint keeps passing its own tests while nothing calls it.
func TestADetachedRunTakesAReservationOnItsKey(t *testing.T) {
	server := serverWithDataDir(t)

	key, release, err := server.resolveSessionKey(detachAgent, RunRequest{Detach: true})
	if err != nil {
		t.Fatalf("resolve a detached run's key: %v", err)
	}
	if _, held := server.pendingSessionKeys.Load(key); !held {
		t.Fatalf("detached key %s was handed out without a reservation, so a concurrent detached run can be told it is free", key)
	}

	release()
	if _, held := server.pendingSessionKeys.Load(key); held {
		t.Errorf("detached key %s is still reserved after release, so it is out of circulation until the process exits", key)
	}
}

// The other three arms mint nothing, so they must reserve nothing: a
// reservation nobody releases is a key leaked for the process's lifetime, and
// the main key is one every later run needs.
func TestOnlyADetachedRunReservesAKey(t *testing.T) {
	cases := []struct {
		name string
		req  RunRequest
		want string
	}{
		{"a fresh session takes the agent's main key", RunRequest{NewSession: true}, mainSessionKey(detachAgent)},
		{"an unnamed session takes the agent's main key", RunRequest{}, mainSessionKey(detachAgent)},
		{"a named session keeps its name", RunRequest{SessionKey: "agent:claxon:bridge-7"}, "agent:claxon:bridge-7"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			server := serverWithDataDir(t)

			key, release, err := server.resolveSessionKey(detachAgent, c.req)
			if err != nil {
				t.Fatalf("resolve: %v", err)
			}
			if key != c.want {
				t.Errorf("resolved %s, want %s", key, c.want)
			}
			if _, held := server.pendingSessionKeys.Load(key); held {
				t.Errorf("%s was reserved; only the detach arm mints, and only a mint needs releasing", key)
			}
			release()
		})
	}
}

// A mint that cannot check refuses, and the refusal names what it could not
// give a key to. The shared checking half is exercised through the child mint
// too; this pins that the detached caller gets the same treatment and that its
// error says which run failed.
func TestADetachedMintThatKeepsLandingOnTakenKeysGivesUpWithAnError(t *testing.T) {
	server := serverWithDataDir(t)

	// One proposal, forever taken, whatever the attempt: the shape of a clock
	// that has stopped.
	stuck := func(agentName string, _ int) string { return "agent:" + agentName + ":detach-stuck" }
	server.sessions.Store(stuck(detachAgent, 0), &Session{Key: stuck(detachAgent, 0)})

	minted, err := server.mintDetachedSessionKeyFrom(detachAgent, stuck)
	if err == nil {
		t.Fatalf("minted %s, which is taken; the loop has to end in an error rather than a key or a hang", minted)
	}
	if !strings.Contains(err.Error(), "taken") {
		t.Fatalf("the refusal does not say why: %v", err)
	}
	if !strings.Contains(err.Error(), detachAgent) {
		t.Fatalf("the refusal does not say what it could not give a key to: %v", err)
	}
}
