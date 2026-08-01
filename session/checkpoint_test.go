package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// A checkpoint is stamped with the turn the gate that fired it counted, and two
// counters that both call themselves "turn" run side by side: the engine counts
// user messages, this package counts API round-trips. One user message can cost
// many round-trips, so the two diverge immediately and never converge again.
//
// The record used to carry the round-trip number while the gate read the
// user-turn number, so a checkpoint taken at user turn 3 filed itself under
// whatever round-trip happened to be in flight. The only reader of the file is
// a human with jq, and the number they would reach for was the wrong one.

// sessionAfterRoundTrips returns a session that has completed roundTrips API
// round-trips, so its own counter is deliberately not the user-turn number the
// checkpoint gate would pass in.
func sessionAfterRoundTrips(t *testing.T, roundTrips int) *Session {
	t.Helper()

	session, err := New(t.TempDir(), "claude-sonnet-4-20250514", "test-agent", "",
		registryWithTwoDifferentlyPricedModels(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(session.Close)

	for range roundTrips {
		session.LogRequest(json.RawMessage(`{"messages":[]}`))
		session.LogAssistant("reply", TurnTokens{Input: 100, Output: 50}, 0)
	}

	if got := session.CurrentTurn(); got != roundTrips {
		t.Fatalf("CurrentTurn() = %d, want %d — the fixture is not set up", got, roundTrips)
	}
	return session
}

// readCheckpoint reads back the checkpoint file written beside a session log.
func readCheckpoint(t *testing.T, session *Session) Checkpoint {
	t.Helper()

	path := filepath.Join(filepath.Dir(session.FilePath()), "checkpoint.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read checkpoint: %v", err)
	}

	var checkpoint Checkpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		t.Fatalf("unmarshal checkpoint: %v", err)
	}
	return checkpoint
}

func TestSaveCheckpointRecordsTheUserTurnTheGateFiredOn(t *testing.T) {
	const roundTrips = 7
	const userTurn = 3

	session := sessionAfterRoundTrips(t, roundTrips)

	if err := session.SaveCheckpoint(userTurn, nil, "a summary", []string{"a fact"}); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	checkpoint := readCheckpoint(t, session)
	if checkpoint.UserTurn != userTurn {
		t.Errorf("UserTurn = %d, want %d", checkpoint.UserTurn, userTurn)
	}
	if checkpoint.UserTurn == roundTrips {
		t.Errorf("UserTurn = %d, which is the API round-trip count — the two counters are crossed again", checkpoint.UserTurn)
	}
}

// The on-disk field is what a human with jq actually types, so pin the JSON key
// as well as the Go field. A rename that reached the struct and not the tag
// would leave the reader looking up a name the file does not use.
func TestCheckpointFileNamesTheCounterItHolds(t *testing.T) {
	session := sessionAfterRoundTrips(t, 7)

	if err := session.SaveCheckpoint(3, nil, "a summary", nil); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	path := filepath.Join(filepath.Dir(session.FilePath()), "checkpoint.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read checkpoint: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal checkpoint: %v", err)
	}

	value, ok := raw["user_turn"]
	if !ok {
		t.Fatalf("checkpoint has no user_turn field; keys are %v", raw)
	}
	if value != float64(3) {
		t.Errorf("user_turn = %v, want 3", value)
	}
	if _, ok := raw["turn"]; ok {
		t.Errorf("checkpoint still carries a bare %q field, which is the ambiguity this record was fixed to remove", "turn")
	}
}

// Checkpointing is a single overwritten file, which is why the config carries
// no retention setting. It used to carry MaxCheckpoints: 5, enforced by a
// pruneOldCheckpoints that returned nil without looking at the directory — so
// the knob read as "four older checkpoints are kept" and no second file was
// ever created. This pins the scheme the config now describes: anyone adding
// rotation has to answer this test and the doc comment together.
func TestCheckpointingKeepsExactlyOneFile(t *testing.T) {
	session := sessionAfterRoundTrips(t, 7)

	for _, userTurn := range []int{20, 40, 60} {
		if err := session.SaveCheckpoint(userTurn, nil, "a summary", nil); err != nil {
			t.Fatalf("SaveCheckpoint at user turn %d: %v", userTurn, err)
		}
	}

	sessionDir := filepath.Dir(session.FilePath())
	entries, err := os.ReadDir(sessionDir)
	if err != nil {
		t.Fatalf("read session dir: %v", err)
	}

	var checkpoints []string
	for _, entry := range entries {
		if name := entry.Name(); filepath.Ext(name) == ".json" && name != "messages.json" {
			checkpoints = append(checkpoints, name)
		}
	}

	if len(checkpoints) != 1 || checkpoints[0] != "checkpoint.json" {
		t.Fatalf("checkpoint files = %v, want exactly [checkpoint.json]", checkpoints)
	}

	// The surviving file is the last checkpoint, not the first: the write
	// overwrites, and a reader that found a stale turn 20 here would be reading
	// a snapshot two checkpoints out of date.
	if got := readCheckpoint(t, session); got.UserTurn != 60 {
		t.Errorf("surviving checkpoint UserTurn = %d, want 60 (the most recent)", got.UserTurn)
	}
}

// The token totals a checkpoint reports are written by LogAssistant under the
// session mutex, and a checkpoint is taken while the turn that produced them is
// still logging. Reading them unlocked was a data race that only the race
// detector would ever report, because the torn read produces a plausible
// number. Run with -race; without it this test only proves the write survives
// concurrent logging.
func TestSaveCheckpointReadsTokenTotalsUnderTheMutex(t *testing.T) {
	session := sessionAfterRoundTrips(t, 1)

	var waitGroup sync.WaitGroup
	waitGroup.Add(2)

	go func() {
		defer waitGroup.Done()
		for range 200 {
			session.LogAssistant("reply", TurnTokens{Input: 100, Output: 50}, 0)
		}
	}()

	go func() {
		defer waitGroup.Done()
		for turn := range 200 {
			if err := session.SaveCheckpoint(turn, []anthropic.MessageParam{}, "s", nil); err != nil {
				t.Errorf("SaveCheckpoint: %v", err)
				return
			}
		}
	}()

	waitGroup.Wait()
}
