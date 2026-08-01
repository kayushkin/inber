package conversation

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kayushkin/inber/memory"

	_ "modernc.org/sqlite"
)

// The reads below go through memory.AutomaticContextRequest — the same function
// Engine.BuildSystemPrompt calls — rather than a literal repeating its fields,
// so this test cannot pass against a request the engine no longer makes.
// minImportance is 0 because Engine.contextBudget returns 0 on every branch.
func automaticContextRequestForTest(tags []string) memory.BuildContextRequest {
	return memory.AutomaticContextRequest(tags, 0, 6000)
}

func containsTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}

// TestArchiveWritersTagThemselvesExcluded is the half of the contract that lives
// at the writers: whatever a writer attaches, at least one of those tags must be
// one the automatic-context reader excludes.
//
// This is the assertion that was missing when the compaction archive tagged
// itself `conversation`/`history` and the reader excluded `session-summary`.
func TestArchiveWritersTagThemselvesExcluded(t *testing.T) {
	writers := map[string][]string{
		"conversation archive": ConversationArchiveTags("sess-1"),
		"stashed content":      StashedContentTags("sess-1", ContentTypeCodeBlock),
	}

	for name, tags := range writers {
		t.Run(name, func(t *testing.T) {
			for _, excluded := range memory.TagsExcludedFromAutomaticContext {
				if containsTag(tags, excluded) {
					return
				}
			}
			t.Fatalf("%s writes tags %v, none of which is in memory.TagsExcludedFromAutomaticContext (%v) — "+
				"its content will be injected back into the prompt it was written to remove",
				name, tags, memory.TagsExcludedFromAutomaticContext)
		})
	}
}

// TestArchivesAreNotOfferedForAutomaticInjection is the other half, measured
// against a real store: a memory written by either archive writer must not come
// back from the read that assembles the system prompt.
func TestArchivesAreNotOfferedForAutomaticInjection(t *testing.T) {
	store, err := memory.NewStore(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	transcript := strings.Repeat("[User]\nrefactor the auth handler\n\n[Assistant]\nDone.\n\n", 60)

	// Ten compactions, from ten different sessions. BuildContext has no session
	// scope, so every one of them is a candidate for any session's prompt.
	for i := 0; i < 10; i++ {
		sessionID := fmt.Sprintf("sess-%d", i)
		if err := store.Save(memory.Memory{
			ID:         "conversation-summary:" + sessionID + ":abcd",
			Content:    transcript,
			Summary:    "Full conversation history from session " + sessionID,
			Tags:       ConversationArchiveTags(sessionID),
			Importance: 0.4,
			Source:     "summarization",
		}); err != nil {
			t.Fatalf("save archive: %v", err)
		}
	}

	// Two stashed blocks, written by the real stasher rather than by hand, so
	// this also fails if StashLargeContent stops using StashedContentTags.
	stashedIDs := map[string]bool{}
	for i := 0; i < 2; i++ {
		stashed, err := StashLargeContent(
			strings.Repeat("func handler() { /* stashed body */ }\n", 120),
			fmt.Sprintf("sess-stash-%d", i),
			store,
			stashConfigWithBothRecallTools(),
		)
		if err != nil {
			t.Fatalf("stash: %v", err)
		}
		if stashed == nil {
			t.Fatal("stash returned nil — the fixture block is below MinBlockSize")
		}
		stashedIDs[stashed.MemoryID] = true
	}

	// One memory that genuinely belongs in a prompt, so a test that excluded
	// everything would not pass by accident.
	if err := store.Save(memory.Memory{
		ID:         "decision-auth",
		Content:    "Decision: auth tokens are resolved from auth-store, never from env.",
		Tags:       []string{"decision", "code"},
		Importance: 0.9,
		Source:     "user",
	}); err != nil {
		t.Fatalf("save decision: %v", err)
	}

	memories, tokens, err := store.BuildContext(automaticContextRequestForTest([]string{"code"}))
	if err != nil {
		t.Fatalf("BuildContext: %v", err)
	}

	for _, m := range memories {
		if strings.HasPrefix(m.ID, "conversation-summary:") || stashedIDs[m.ID] {
			t.Errorf("archive %s was offered for automatic injection (%d tokens, %d chars of content)",
				m.ID, m.Tokens, len(m.Content))
		}
	}

	if len(memories) != 1 || memories[0].ID != "decision-auth" {
		ids := make([]string, len(memories))
		for i, m := range memories {
			ids[i] = m.ID
		}
		t.Fatalf("expected only the real memory to be assembled, got %v (%d tokens)", ids, tokens)
	}
}

// TestArchivesStayReachableByID pins the other side of the exclusion: keeping an
// archive out of the automatic context must not make it unrecoverable. The whole
// point of writing it is that memory_expand can still fetch it by id.
func TestArchivesStayReachableByID(t *testing.T) {
	store, err := memory.NewStore(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	transcript := "[User]\nrefactor the auth handler\n\n[Assistant]\nDone.\n"
	const id = "conversation-summary:sess-1:abcd"

	if err := store.Save(memory.Memory{
		ID:         id,
		Content:    transcript,
		Summary:    "Full conversation history from session sess-1",
		Tags:       ConversationArchiveTags("sess-1"),
		Importance: 0.4,
		Source:     "summarization",
	}); err != nil {
		t.Fatalf("save archive: %v", err)
	}

	got, err := store.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Content != transcript {
		t.Errorf("archive content not recoverable by id: got %q, want %q", got.Content, transcript)
	}
}
