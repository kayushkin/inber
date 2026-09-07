package conversation

import (
	"bytes"
	"context"
	"log"
	"strings"
	"testing"
)

// autoSaveToMemory only reaches assistant messages that are ABOUT to be
// truncated. So when substantialLeadingFragments returns nothing, the caller's
// `continue` drops the fact and the truncation in manage.go shortens the same
// message anyway: the detail leaves the live conversation and no memory row
// replaces it, in one step, saying nothing.
//
// The file already states this rule for the other failure on the same path —
// "a save that failed is content leaving the process for good. Say so rather
// than counting it and moving on" — and enforced it only for a Save error.
// These tests hold the empty-extraction half to the same rule.
//
// This does NOT pin what the right answer is. Whether an empty extraction
// should also block the truncation, and whether the 20-byte floor here should
// agree with the 10 in manage_text_utils.go, are open questions on todo
// 86347015-9924-40f4-99db-1d79c1e767ca. Reporting the loss is the floor under
// every one of those answers.
func TestAnAutoSaveThatExtractedNothingSaysSo(t *testing.T) {
	store := openTestMemoryStore(t)

	// A numbered list is the most common way agent prose opens, and it is the
	// shape that defeats the extractor: FieldsFunc splits on every '.', so the
	// first three pieces are "1", "Fixed it" and "2" — all at or under the
	// 20-byte floor. The budget is gone before the substantive third item is
	// ever looked at.
	messages := assistantFacts(
		"1. Fixed it. 2. Created a test. 3. Implemented the retry loop across the delivery path and verified every row landed.",
	)

	logged := captureConversationLog(t)

	saved, err := autoSaveToMemory(context.Background(), messages, store, "session-empty",
		autoSaveTestConfig(), agesOlderThanTruncation(len(messages)))
	if err != nil {
		t.Fatalf("autoSaveToMemory: %v", err)
	}

	// Establish the premise rather than assuming it: if this message ever
	// starts saving, the test below is asserting about a case that no longer
	// happens and must be rewritten, not deleted.
	if saved != 0 {
		t.Fatalf("premise gone: this message now saves %d fact(s); the empty-extraction path is no longer exercised", saved)
	}
	rows, err := store.ListRecent(50, 0)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("premise gone: the store holds %d row(s) for a message that extracted nothing", len(rows))
	}

	if got := logged.String(); !strings.Contains(got, "auto-save extracted nothing") {
		t.Fatalf("a message was truncated with nothing saved and nothing was reported.\nlog was: %q", got)
	}
}

// The negative control. It is not supposed to move when the report above is
// added or removed, and it reads the row back so an arm that never reached the
// save cannot satisfy the assertion by doing nothing.
func TestAnAutoSaveThatSavedSomethingReportsNoLoss(t *testing.T) {
	store := openTestMemoryStore(t)

	messages := assistantFacts(
		"I implemented the retry loop in the delivery path and verified every row landed.",
	)

	logged := captureConversationLog(t)

	saved, err := autoSaveToMemory(context.Background(), messages, store, "session-saved",
		autoSaveTestConfig(), agesOlderThanTruncation(len(messages)))
	if err != nil {
		t.Fatalf("autoSaveToMemory: %v", err)
	}
	if saved != 1 {
		t.Fatalf("autoSaveToMemory reported %d facts saved, want 1", saved)
	}
	rows, err := store.ListRecent(50, 0)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("reported 1 fact saved and the store holds %d rows", len(rows))
	}

	if got := logged.String(); strings.Contains(got, "auto-save extracted nothing") {
		t.Fatalf("a fact that WAS saved was reported as lost.\nlog was: %q", got)
	}
}

// The name substantialLeadingFragments has to keep earning itself. Each row is
// one word of it: the pieces are fragments and not sentences, only the leading
// maxFragmentsScanned of them are looked at, and the budget is spent on pieces
// that are then rejected.
func TestSubstantialLeadingFragmentsSpendsItsBudgetOnRejectedPieces(t *testing.T) {
	cases := []struct {
		name string
		text string
		want int
	}{
		{"a numbered list burns the budget on its digits", "1. First item. 2. Second item. 3. Third item.", 0},
		{"a version number splits into four pieces", "Deployed v2.1.4 to production. The rollout completed without incident across all regions.", 0},
		{"prose reaches the substantive fragment", "The migration finished and every row was verified against the source table.", 1},
		{"one digit is enough to cost a slot", "I made three changes. 1. Rewrote the parser to handle nesting properly. 2. Added tests.", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := len(substantialLeadingFragments(tc.text, 3)); got != tc.want {
				t.Fatalf("substantialLeadingFragments(%q, 3) returned %d fragment(s), want %d", tc.text, got, tc.want)
			}
		})
	}
}

// captureConversationLog redirects the standard logger for one test. No test in
// this package calls t.Parallel(), so the global logger belongs to one test at
// a time.
func captureConversationLog(t *testing.T) *bytes.Buffer {
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
	return &buf
}
