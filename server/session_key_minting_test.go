package server

import (
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/kayushkin/inber/guard"
	sessionMod "github.com/kayushkin/inber/session"
)

// A child's session key used to be a clock remainder over a space of 100,000,
// handed out with no check. The key is this server's identity for a session —
// the directory its transcript and its guard sidecar live in, the row its agent
// and lineage are recorded against, and its entry in the live session map — so a
// second child handed a first child's key inherited all three silently.
//
// These tests pin that a key already in use anywhere is never handed out again,
// that two concurrent spawns cannot be told the same key is free, and that a
// check which cannot be completed refuses rather than guesses.

const mintParentKey = "agent:claxon:main"

// proposing scripts the proposal half of the mint. The clock produces a
// collision roughly once in tens of thousands of spawns, which is not something
// a test can wait for.
func proposing(suffixes ...string) func(string) string {
	var next atomic.Int64
	return func(parentKey string) string {
		i := int(next.Add(1)) - 1
		if i >= len(suffixes) {
			i = len(suffixes) - 1
		}
		return parentKey + childKeySeparator + suffixes[i]
	}
}

// serverWithDataDir builds the least server the mint needs: somewhere to look
// for persisted sessions, and a store to ask about recorded ones.
func serverWithDataDir(t *testing.T) *Server {
	t.Helper()
	dir := t.TempDir()
	return &Server{config: Config{DataDir: dir}, store: tempStore(t)}
}

func TestAKeyWhoseTranscriptIsOnDiskIsNotHandedToANewChild(t *testing.T) {
	server := serverWithDataDir(t)

	// A retired sibling: its directory holds the caps it ran under and the
	// turns, tokens and dollars it spent against them.
	taken := mintParentKey + childKeySeparator + "41287"
	retiredDir := filepath.Join(server.config.DataDir, "sessions", taken)
	if err := sessionMod.SaveGuardState(retiredDir, guard.State{MaxCost: 5, Cost: 4.80, Turns: 12}); err != nil {
		t.Fatalf("persist the retired sibling: %v", err)
	}

	// The premise, asserted rather than assumed: this is a directory a rebuild
	// really does read a budget out of. If the sidecar ever moves, this fails
	// here and says so, instead of leaving the mint checking an empty path.
	restored := guard.New(guard.Config{MaxCost: 5})
	server.restoreGuardState(taken, restored)
	if restored.State().Cost != 4.80 {
		t.Fatalf("a session rebuilt on %s inherited $%.2f of spend; the whole point of not reusing the key is that it inherits $4.80",
			taken, restored.State().Cost)
	}

	minted, err := server.mintChildSessionKeyFrom(mintParentKey, proposing("41287", "41288"))
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if minted == taken {
		t.Fatalf("minted %s, which already holds a retired sibling's guard state ($4.80 of $5 spent, 12 turns)", minted)
	}
	if want := mintParentKey + childKeySeparator + "41288"; minted != want {
		t.Fatalf("minted %s, want the next proposal %s", minted, want)
	}
}

func TestAKeyThatIsAlreadyRecordedIsNotHandedToANewChild(t *testing.T) {
	server := serverWithDataDir(t)

	// brigid was spawned under this key. UpsertSession leaves agent alone on
	// conflict, so a second child handed the same key keeps brigid's name and
	// agentForSession rebuilds it as brigid.
	taken := mintParentKey + childKeySeparator + "22927"
	if err := server.store.UpsertSession(taken, "brigid", "spawn",
		SessionLineage{ParentKey: mintParentKey, SpawnDepth: 1}, nil); err != nil {
		t.Fatalf("record the sibling: %v", err)
	}

	minted, err := server.mintChildSessionKeyFrom(mintParentKey, proposing("22927", "22928"))
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if minted == taken {
		t.Fatalf("minted %s, which is already recorded as brigid's; a fionn child on that key would be rebuilt as brigid", minted)
	}
}

func TestAKeyHeldByALiveSessionIsNotHandedToANewChild(t *testing.T) {
	server := serverWithDataDir(t)

	taken := mintParentKey + childKeySeparator + "60077"
	server.sessions.Store(taken, &Session{Key: taken, AgentName: "fionn"})

	minted, err := server.mintChildSessionKeyFrom(mintParentKey, proposing("60077", "60078"))
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if minted == taken {
		t.Fatalf("minted %s while a session is running under it; storing the new child would drop the running one out of the map", minted)
	}
}

func TestTwoConcurrentSpawnsAreNotBothToldTheSameKeyIsFree(t *testing.T) {
	server := serverWithDataDir(t)

	// Both callers are offered the SAME first key — proposing() hands out its
	// suffixes in turn, which would give the two goroutines different keys
	// however the mint behaved and prove nothing. Nothing has been stored yet,
	// so both find that key free on disk, in the store and in the session map:
	// the reservation is the only thing that can tell them apart.
	var proposals atomic.Int64
	propose := func(parentKey string) string {
		if proposals.Add(1) <= 2 {
			return parentKey + childKeySeparator + "77001"
		}
		return parentKey + childKeySeparator + "77002"
	}

	var wait sync.WaitGroup
	minted := make([]string, 2)
	errs := make([]error, 2)
	start := make(chan struct{})
	for i := range minted {
		wait.Add(1)
		go func(i int) {
			defer wait.Done()
			<-start
			minted[i], errs[i] = server.mintChildSessionKeyFrom(mintParentKey, propose)
		}(i)
	}
	close(start)
	wait.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("mint %d: %v", i, err)
		}
	}
	if minted[0] == minted[1] {
		t.Fatalf("both spawns were handed %s; the second child would inherit the first's budget and agent", minted[0])
	}
}

func TestAMintedKeyIsFreeAgainOnceItIsReleased(t *testing.T) {
	server := serverWithDataDir(t)

	minted, err := server.mintChildSessionKeyFrom(mintParentKey, proposing("30001"))
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	// The session could not be built, so nothing was ever stored under the key.
	server.releaseSessionKeyReservation(minted)

	again, err := server.mintChildSessionKeyFrom(mintParentKey, proposing("30001"))
	if err != nil {
		t.Fatalf("mint after release: %v", err)
	}
	if again != minted {
		t.Fatalf("a released key was still held: minted %s, then %s", minted, again)
	}
}

func TestAMintThatCannotCheckTheStoreRefusesRatherThanGuesses(t *testing.T) {
	server := serverWithDataDir(t)
	if err := server.store.Close(); err != nil {
		t.Fatalf("close the store: %v", err)
	}

	minted, err := server.mintChildSessionKeyFrom(mintParentKey, proposing("50001"))
	if err == nil {
		t.Fatalf("an unreadable store produced key %s; a key that cannot be checked has to be refused, not assumed free", minted)
	}
}

func TestAMintThatKeepsLandingOnTakenKeysGivesUpWithAnError(t *testing.T) {
	server := serverWithDataDir(t)

	// One proposal, forever taken: the shape of a suffix space so full that
	// drawing from it cannot succeed.
	taken := mintParentKey + childKeySeparator + "99999"
	server.sessions.Store(taken, &Session{Key: taken})

	minted, err := server.mintChildSessionKeyFrom(mintParentKey, proposing("99999"))
	if err == nil {
		t.Fatalf("minted %s, which is taken; the loop has to end in an error rather than a key or a hang", minted)
	}
	if !strings.Contains(err.Error(), "taken") {
		t.Fatalf("the refusal does not say why: %v", err)
	}
}
