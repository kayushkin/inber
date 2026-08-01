package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	sessionMod "github.com/kayushkin/inber/session"
)

// persistedWorkspace writes a transcript and its turn count where a resuming
// session looks for them.
func persistedWorkspace(t *testing.T, repoRoot, agentName string, turns, counter int) {
	t.Helper()
	ws := sessionMod.NewWorkspace(repoRoot, agentName)
	data, err := json.Marshal(restoredTranscript(turns))
	if err != nil {
		t.Fatalf("marshal transcript: %v", err)
	}
	if err := ws.SaveMessages(data); err != nil {
		t.Fatalf("SaveMessages: %v", err)
	}
	if err := sessionMod.SaveTurnCounter(ws.Dir, counter); err != nil {
		t.Fatalf("SaveTurnCounter: %v", err)
	}
}

func TestSetupSession_ResumeLoadsTheTurnCountWithTheTranscript(t *testing.T) {
	repoRoot := t.TempDir()
	persistedWorkspace(t, repoRoot, "tester", 9, 9)

	_, _, _, messages, turnCounter, err := setupSession(repoRoot, "tester", "chat", false, false)
	if err != nil {
		t.Fatalf("setupSession: %v", err)
	}
	if len(messages) == 0 {
		t.Fatal("setupSession loaded no messages; the fixture is not being read at all")
	}
	if turnCounter != 9 {
		t.Fatalf("turnCounter = %d, want 9", turnCounter)
	}
}

// --new discards the transcript, so it has to discard the count too: a count
// that survived would describe messages this session no longer has.
func TestSetupSession_NewSessionDiscardsTheTurnCount(t *testing.T) {
	repoRoot := t.TempDir()
	persistedWorkspace(t, repoRoot, "tester", 9, 9)

	_, _, _, messages, turnCounter, err := setupSession(repoRoot, "tester", "chat", true, false)
	if err != nil {
		t.Fatalf("setupSession: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("--new kept %d messages", len(messages))
	}
	if turnCounter != 0 {
		t.Fatalf("turnCounter = %d after --new, want 0", turnCounter)
	}

	// And it is gone from disk, not merely unread: the next resume must not
	// find it either.
	onDisk, err := sessionMod.LoadTurnCounter(sessionMod.NewWorkspace(repoRoot, "tester").Dir)
	if err != nil {
		t.Fatalf("LoadTurnCounter: %v", err)
	}
	if onDisk != 0 {
		t.Fatalf("turn count %d still on disk after --new", onDisk)
	}
}

// The other half of the loop. Every test above reads a fixture someone else
// wrote; this one checks that a running turn writes the count where the next
// invocation looks for it.
func TestSaveResumableState_WritesTheTurnCountBesideTheTranscript(t *testing.T) {
	repoRoot := t.TempDir()
	e := &Engine{workspace: sessionMod.NewWorkspace(repoRoot, "tester")}
	e.RestoreSession(restoredTranscript(4), 4)
	e.Turn.Counter++ // the turn that just ran

	e.saveResumableState()

	_, _, _, messages, turnCounter, err := setupSession(repoRoot, "tester", "chat", false, false)
	if err != nil {
		t.Fatalf("setupSession: %v", err)
	}
	if len(messages) == 0 {
		t.Fatal("the saved transcript did not come back")
	}
	if turnCounter != 5 {
		t.Fatalf("resumed at turn %d, want 5 — the count this session reached", turnCounter)
	}
}

// The count only means anything as a description of the transcript it was
// written beside. saveResumableState used to throw away SaveMessages' error and
// write the count regardless, so a failed transcript write left the previous
// transcript on disk under a count that belonged to a longer conversation — and
// nothing said so. Now the count follows the transcript or is not written.
func TestSaveResumableState_SkipsTheTurnCountWhenTheTranscriptCannotBeWritten(t *testing.T) {
	repoRoot := t.TempDir()
	workspace := sessionMod.NewWorkspace(repoRoot, "tester")

	// A directory where messages.json goes: the write fails, the directory it
	// lives in stays perfectly writable, so only the transcript is blocked.
	if err := os.MkdirAll(filepath.Join(workspace.Dir, "messages.json"), 0o755); err != nil {
		t.Fatalf("stage the unwritable transcript: %v", err)
	}

	e := &Engine{workspace: workspace}
	e.RestoreSession(restoredTranscript(4), 4)
	e.Turn.Counter++

	e.saveResumableState()

	counter, err := sessionMod.LoadTurnCounter(workspace.Dir)
	if err != nil {
		t.Fatalf("LoadTurnCounter: %v", err)
	}
	if counter != 0 {
		t.Fatalf("turn count %d was written beside a transcript that was not, so the next resume would replay an older conversation as if it had reached turn %d", counter, counter)
	}
}

func TestRestoreSession_RestoresTheTurnCount(t *testing.T) {
	e := &Engine{}
	e.RestoreSession(restoredTranscript(12), 12)

	if e.Turn.Counter != 12 {
		t.Fatalf("Turn.Counter = %d after restoring a 12-turn session, want 12", e.Turn.Counter)
	}
}

// The first reader of the count, and the reason the doc calls this a defect
// rather than a cosmetic one: a resumed conversation asks the memory store for
// the first-turn budget.
func TestRestoreSession_ResumedSessionGetsAnAgedMemoryBudget(t *testing.T) {
	e := &Engine{}
	e.RestoreSession(restoredTranscript(20), 20)

	_, budget := e.contextBudget("what were we doing?")
	if budget != 8000 {
		t.Fatalf("memory budget = %d on a 20-turn resumed session, want 8000", budget)
	}
}

// The complement. Without it the assertion above would also pass if
// contextBudget started answering 8000 for everything, including a session
// that really is on its first turn.
func TestRestoreSession_GenuineFirstTurnStillGetsTheFirstTurnBudget(t *testing.T) {
	e := &Engine{}
	e.RestoreSession(nil, 0)

	_, budget := e.contextBudget("hello")
	if budget != 4000 {
		t.Fatalf("memory budget = %d on a genuinely new session, want 4000", budget)
	}
}

// The second reader, and the one that loses data rather than tokens: the
// checkpoint interval is counted in turns, so a counter that restarts every
// invocation is a counter that never reaches it.
//
// This is a measurement, not an assertion about one number. It runs the real
// default interval over a plausible usage pattern — a short session, resumed
// again and again — and counts the checkpoints each way.
func TestRestoreSession_WithoutTheRestoredCountCheckpointsNeverFire(t *testing.T) {
	const (
		invocations = 30
		turnsPerRun = 6
		totalTurns  = invocations * turnsPerRun
	)
	cfg := sessionMod.DefaultCheckpointConfig()

	restarting, carried := 0, 0
	carriedCounter := 0
	for run := 0; run < invocations; run++ {
		restartingCounter := 0
		for turn := 0; turn < turnsPerRun; turn++ {
			restartingCounter++
			carriedCounter++
			if sessionMod.ShouldCheckpoint(restartingCounter, cfg) {
				restarting++
			}
			if sessionMod.ShouldCheckpoint(carriedCounter, cfg) {
				carried++
			}
		}
	}

	wantCarried := totalTurns / cfg.Interval
	if carried != wantCarried {
		t.Fatalf("carrying the count checkpointed %d times over %d turns, want %d",
			carried, totalTurns, wantCarried)
	}
	if restarting != 0 {
		t.Fatalf("a count that restarts every %d turns checkpointed %d times at interval %d; "+
			"the fixture no longer demonstrates the defect",
			turnsPerRun, restarting, cfg.Interval)
	}
}
