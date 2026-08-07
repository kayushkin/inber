package engine

import (
	"context"
	"testing"

	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/conversation"
)

// What these tests pin, and why they assert the WRONG answer on purpose.
//
// summarizeIfNeeded replaces the conversation wholesale — conversation/summarize.go
// swaps messages[:keepFrom] for one new summary message — and never touches
// e.staged.FrozenIdx. Both callers run pruneIfNeeded on the very next line
// (turn_prepare.go:66-69, engine.go:458-462) and that function reads the boundary
// three times: ManageStaging, the CrossZoneDedup split, and the copy into
// Agent.FrozenIdx at build.go:38, which is where BP3 is placed.
//
// A head drop was correctable by subtraction, and ShiftAfterHeadDrop does exactly
// that. Summarization is not a drop: there is no count that maps the old positions
// onto the new ones, because every message in front of the boundary was replaced by
// a single message that did not exist before. So the boundary cannot be repaired —
// it has to be re-decided, and that is the open question on todo
// `3553efe9-d90e-4350-a33a-023e73817e90`:
//
//	A — reset to 0. Nothing is frozen after a summarization; the next scheduled
//	    flush re-freezes, at the cost of one uncached turn.
//	B — freeze the summary immediately, Flush(FreezePoint(e.Messages)), which is
//	    what the flush branch of pruneIfNeeded already does with a pruned
//	    conversation.
//
// Both answers change what these tests assert, which is the point of writing them
// now: the choice becomes a visible test change instead of a silent one. Until then
// they say what the code does today, and the failure messages say why it is wrong.

// summarizingEngineWithFrozenBoundary drives the real summarizer against a stubbed
// API and hands back the engine with a frozen boundary already placed, plus the text
// of the message that boundary named before the summarization ran.
//
// The boundary is placed by hand rather than by Flush because the case under test is
// a boundary set on an earlier turn and carried into this one — which is every
// boundary, since Flush is the only thing that sets a non-zero one.
func summarizingEngineWithFrozenBoundary(t *testing.T, boundary int) (*Engine, string) {
	t.Helper()

	engine, _ := stubbedSummaryEngine(t, []agent.Tool{namedTool("read_files")})
	engine.staged = conversation.NewStagedConversation(conversation.DefaultPruneConfig().ManageInterval)
	engine.staged.FrozenIdx = boundary

	if boundary >= len(engine.Messages) {
		t.Fatalf("the fixture cannot place a boundary at %d in a %d-message transcript",
			boundary, len(engine.Messages))
	}
	named := firstTextOf(t, engine.Messages[boundary])

	before := len(engine.Messages)
	if err := engine.summarizeIfNeeded(context.Background()); err != nil {
		t.Fatalf("summarizeIfNeeded: %v", err)
	}
	// Without this the tests below would pass against a fixture that never reached
	// the summarizer at all — the boundary would be untouched because nothing
	// happened, not because nothing corrected it.
	if len(engine.Messages) >= before {
		t.Fatalf("fixture did not summarize: %d messages in, %d out", before, len(engine.Messages))
	}
	summaryBlockOf(t, engine)

	return engine, named
}

// The out-of-range case: the boundary sat past where the shortened conversation now
// ends, so the whole transcript — including the summary block this turn produced and
// has never sent — is claimed frozen, and the staging zone is empty.
func TestSummarizationLeavesTheFrozenBoundaryPastTheEndOfTheConversation(t *testing.T) {
	// 60 is inside the 80-message fixture and well past the 12 recent turns a
	// default-role summarization keeps.
	engine, named := summarizingEngineWithFrozenBoundary(t, 60)

	if engine.staged.FrozenIdx != 60 {
		t.Fatalf("frozen boundary = %d, want 60 — this test describes the boundary being left alone; if summarization now moves it, assert the new rule here",
			engine.staged.FrozenIdx)
	}
	if engine.staged.FrozenIdx < len(engine.Messages) {
		t.Fatalf("boundary %d is inside the %d-message conversation; this case needs it past the end",
			engine.staged.FrozenIdx, len(engine.Messages))
	}

	// The consequence, taken from the code that reads the boundary rather than from
	// the number itself. Every message is on the frozen side, so ManageStaging has
	// nothing to dedup or prune and CrossZoneDedup never runs.
	if staging := engine.staged.StagingSlice(engine.Messages); staging != nil {
		t.Errorf("staging zone = %d messages, want none — the boundary is past the end", len(staging))
	}
	deduped := conversation.ManageStaging(engine.Messages, engine.staged.FrozenIdx, engine.pruneConfig())
	if deduped != 0 {
		t.Errorf("ManageStaging deduped %d refs through an out-of-range boundary, want 0", deduped)
	}

	t.Logf("the boundary named %q before the summarization; that message no longer exists, and %d now points past the last of %d messages",
		named, engine.staged.FrozenIdx, len(engine.Messages))
}

// The in-range case, which is the worse of the two: nothing range-checks its way out
// of it, so the boundary silently names a different message and the zones are split
// in the wrong place.
func TestSummarizationLeavesAnInRangeFrozenBoundaryNamingADifferentMessage(t *testing.T) {
	// 10 survives the shortening — a default-role summarization keeps 12 turns plus
	// the two-message summary block, so the conversation stays longer than this.
	engine, named := summarizingEngineWithFrozenBoundary(t, 10)

	if engine.staged.FrozenIdx != 10 {
		t.Fatalf("frozen boundary = %d, want 10 — this test describes the boundary being left alone; if summarization now moves it, assert the new rule here",
			engine.staged.FrozenIdx)
	}
	if engine.staged.FrozenIdx >= len(engine.Messages) {
		t.Fatalf("boundary %d fell outside the %d-message conversation; that is the other test's case",
			engine.staged.FrozenIdx, len(engine.Messages))
	}

	// The assertion the head-drop tests make, inverted. There it is checked that the
	// boundary still names the same message; here it demonstrably does not, and
	// cannot, because the message it named was condensed away.
	now := firstTextOf(t, engine.Messages[engine.staged.FrozenIdx])
	if now == named {
		t.Fatalf("boundary still names %q after a summarization replaced everything in front of it — if that is now true by design, this whole file needs rewriting",
			named)
	}

	// So the frozen zone is a prefix of messages nobody froze, and the staging zone
	// ManageStaging is free to rewrite starts in the middle of the kept turns.
	staging := engine.staged.StagingSlice(engine.Messages)
	if len(staging) == 0 {
		t.Fatalf("expected a non-empty staging zone at boundary %d of %d messages",
			engine.staged.FrozenIdx, len(engine.Messages))
	}
	t.Logf("the boundary named %q before the summarization and names %q after it; %d of %d messages are now claimed frozen",
		named, now, engine.staged.FrozenIdx, len(engine.Messages))
}

// The control. Under the trigger there is no summarization, so the boundary is
// meaningful and stays put — which is what makes the two tests above evidence of the
// rewrite rather than of an engine that never moves the boundary at all.
func TestAConversationTooShortToSummarizeKeepsItsFrozenBoundary(t *testing.T) {
	engine, _ := stubbedSummaryEngine(t, []agent.Tool{namedTool("read_files")})
	// DefaultSummarizeConfig's default role triggers above 60 messages; 20 turns is
	// 40, and ShouldSummarize returns false without reaching the stubbed API.
	engine.Messages = summarizableTranscript(20)
	engine.staged = conversation.NewStagedConversation(conversation.DefaultPruneConfig().ManageInterval)
	engine.staged.FrozenIdx = 10
	named := firstTextOf(t, engine.Messages[10])

	before := len(engine.Messages)
	if err := engine.summarizeIfNeeded(context.Background()); err != nil {
		t.Fatalf("summarizeIfNeeded: %v", err)
	}
	if len(engine.Messages) != before {
		t.Fatalf("a transcript under the trigger was summarized anyway: %d → %d messages",
			before, len(engine.Messages))
	}
	if engine.staged.FrozenIdx != 10 {
		t.Errorf("frozen boundary = %d with nothing summarized, want 10", engine.staged.FrozenIdx)
	}
	if got := firstTextOf(t, engine.Messages[engine.staged.FrozenIdx]); got != named {
		t.Errorf("boundary names %q, it named %q and nothing changed the conversation", got, named)
	}
}

// pruneIfNeeded is the next line after summarizeIfNeeded in both callers, and it is
// the reader that turns a stale boundary into behaviour. This drives the real pair in
// the real order and pins that the second one does not repair the first — a flush
// would, by calling Flush(FreezePoint), but a flush is three turns away by default,
// so the wrong boundary governs the turns in between.
func TestPruneAfterSummarizationDoesNotRepairTheFrozenBoundary(t *testing.T) {
	engine, _ := summarizingEngineWithFrozenBoundary(t, 60)

	engine.pruneIfNeeded(context.Background())

	if engine.staged.ShouldFlush() {
		t.Fatal("this test needs the non-flush path: a flush re-freezes the conversation and would hide the stale boundary")
	}
	if engine.staged.FrozenIdx != 60 {
		t.Errorf("frozen boundary = %d after the prune that follows a summarization, want 60 — if prune now repairs it, this file needs rewriting",
			engine.staged.FrozenIdx)
	}
	if engine.staged.FrozenIdx < len(engine.Messages) {
		t.Errorf("boundary %d is back inside the %d-message conversation; something moved it",
			engine.staged.FrozenIdx, len(engine.Messages))
	}
}
