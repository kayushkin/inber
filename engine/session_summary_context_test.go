package engine

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/memory"

	_ "modernc.org/sqlite"
)

// TestSessionSummaryIsNotOfferedForAutomaticInjection covers the third writer of
// a shrink-the-conversation memory, through the real SaveSessionSummary.
//
// This writer was already correct before the archive tags were collected in one
// place — it is here as a complement: if a future edit renames the constant or
// drops it from TagsExcludedFromAutomaticContext, the one writer that always
// worked has to say so too, rather than the exclusion quietly covering nothing.
func TestSessionSummaryIsNotOfferedForAutomaticInjection(t *testing.T) {
	store, err := memory.NewStore(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	SaveSessionSummary(store, []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("please refactor the auth handler")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("Done — the handler now resolves tokens from auth-store.")),
	}, "claxon")

	// The summary has to be in the store before the exclusion below means
	// anything. SaveSessionSummary reports a failed write with a log line and
	// returns, and it also gives up silently when the messages carry no text, so
	// "no summary was written" is a state this test reaches without failing —
	// and in that state "the assembled context holds only decision-auth" is true
	// for the wrong reason. Measured 2026-08-14: stubbing the Save out left this
	// test green, and every other suite in the repository green with it.
	//
	// ListRecent is the read to check it with because it applies no tag
	// exclusion, so it can see a memory that the read under test must not offer.
	// Asking BuildContext instead would be asking the code under test whether it
	// had anything to hide.
	stored, err := store.ListRecent(50, 0)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	var summaryIDs []string
	for _, m := range stored {
		if slices.Contains(m.Tags, memory.TagSessionSummary) {
			summaryIDs = append(summaryIDs, m.ID)
		}
	}
	if len(summaryIDs) != 1 {
		t.Fatalf("SaveSessionSummary left %d memories tagged %q in the store, want exactly 1 — "+
			"with none, everything below passes without testing the exclusion at all",
			len(summaryIDs), memory.TagSessionSummary)
	}
	summaryID := summaryIDs[0]

	// A memory that does belong in a prompt, so an assembly that returned nothing
	// at all would not be mistaken for the exclusion working.
	if err := store.Save(memory.Memory{
		ID:         "decision-auth",
		Content:    "Decision: auth tokens are resolved from auth-store, never from env.",
		Tags:       []string{"decision", "code"},
		Importance: 0.9,
		Source:     "user",
	}); err != nil {
		t.Fatalf("save decision: %v", err)
	}

	memories, _, err := store.BuildContext(memory.AutomaticContextRequest([]string{"code"}, 0, 6000))
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}

	for _, m := range memories {
		if m.ID == summaryID {
			t.Errorf("the session summary %s was offered for automatic injection (%d tokens) — "+
				"the digest of a finished session is being put back into the prompts it was written to compact",
				m.ID, m.Tokens)
		}
	}

	if len(memories) != 1 || memories[0].ID != "decision-auth" {
		ids := make([]string, len(memories))
		for i, m := range memories {
			ids[i] = m.ID
		}
		t.Fatalf("expected only the real memory to be assembled, got %v", ids)
	}
}
