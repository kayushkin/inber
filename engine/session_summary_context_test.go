package engine

import (
	"path/filepath"
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

	if len(memories) != 1 || memories[0].ID != "decision-auth" {
		ids := make([]string, len(memories))
		for i, m := range memories {
			ids[i] = m.ID
		}
		t.Fatalf("expected only the real memory to be assembled, got %v", ids)
	}
}
