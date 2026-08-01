package conversation

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/memory"
	_ "modernc.org/sqlite"
)

// autoSaveToMemory used to save every fact with no ID at all. memory-store
// defaults five fields of a row and never that one, and it upserts on it, so a
// pruning pass that found three facts wrote three times onto the key "" and
// left one row — while telling its caller three had been saved.
func TestEveryAutoSavedFactGetsItsOwnRow(t *testing.T) {
	store := openTestMemoryStore(t)

	messages := assistantFacts(
		"I implemented the retry loop in the delivery path.",
		"I fixed the session key collision by checking the store as well.",
		"I created the workspace reaper and wired it to the scheduler.",
	)

	saved, err := autoSaveToMemory(context.Background(), messages, store, "session-a",
		autoSaveTestConfig(), agesOlderThanTruncation(len(messages)))
	if err != nil {
		t.Fatalf("autoSaveToMemory: %v", err)
	}
	if saved != 3 {
		t.Fatalf("autoSaveToMemory reported %d facts saved, want 3", saved)
	}

	rows, err := store.ListRecent(50, 0)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(rows) != saved {
		t.Fatalf("reported %d facts saved and the store holds %d rows", saved, len(rows))
	}
	for _, m := range rows {
		if m.ID == "" {
			t.Fatalf("a fact was saved under the empty id: %q", m.Content)
		}
	}
}

// The id is derived from the fact, so pruning a conversation it has already
// pruned updates the rows it wrote rather than accumulating a copy per pass.
func TestAutoSavingTheSameFactTwiceLeavesOneRow(t *testing.T) {
	store := openTestMemoryStore(t)

	messages := assistantFacts(
		"I implemented the retry loop in the delivery path.",
		"I fixed the session key collision by checking the store as well.",
	)
	cfg := autoSaveTestConfig()
	ages := agesOlderThanTruncation(len(messages))

	for pass := 0; pass < 2; pass++ {
		if _, err := autoSaveToMemory(context.Background(), messages, store, "session-a", cfg, ages); err != nil {
			t.Fatalf("pass %d: %v", pass, err)
		}
	}

	rows, err := store.ListRecent(50, 0)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("two passes over the same two facts left %d rows, want 2", len(rows))
	}
}

// The same sentence said in two different sessions is two memories, because the
// session tag is what a later recall filters on.
func TestTheSameFactInTwoSessionsIsTwoMemories(t *testing.T) {
	fact := "I implemented the retry loop in the delivery path."
	if autoSaveMemoryID("session-a", fact) == autoSaveMemoryID("session-b", fact) {
		t.Fatal("two sessions collapsed onto one memory id")
	}
	if autoSaveMemoryID("session-a", fact) != autoSaveMemoryID("session-a", fact) {
		t.Fatal("the same fact in the same session did not resolve to one id")
	}
}

// The fact is on its way out of the conversation, so a save that failed is
// content the process is about to lose. Counting it reports a memory that is
// not there, and the count is what the pruning result hands back to its caller.
func TestAFactThatFailedToSaveIsNotCounted(t *testing.T) {
	store := &refusingMemoryStore{}

	saved, err := autoSaveToMemory(context.Background(), assistantFacts(
		"I implemented the retry loop in the delivery path.",
		"I fixed the session key collision by checking the store as well.",
	), store, "session-a", autoSaveTestConfig(), agesOlderThanTruncation(2))
	if err != nil {
		t.Fatalf("autoSaveToMemory: %v", err)
	}
	if store.attempts != 2 {
		t.Fatalf("the store saw %d saves, want 2", store.attempts)
	}
	if saved != 0 {
		t.Fatalf("%d facts counted as saved by a store that saved none", saved)
	}
}

// refusingMemoryStore fails every Save. The embedded interface is nil on
// purpose: any method this test does not expect to be called panics rather than
// quietly answering.
type refusingMemoryStore struct {
	memory.MemoryStore
	attempts int
}

func (s *refusingMemoryStore) Save(memory.Memory) error {
	s.attempts++
	return errRefused
}

var errRefused = errors.New("this store saves nothing")

func openTestMemoryStore(t *testing.T) memory.MemoryStore {
	t.Helper()
	store, err := memory.NewSQLiteStore(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatalf("open memory store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// assistantFacts builds assistant messages that clear every gate in
// autoSaveToMemory: a decision word, and enough text to pass the token floor.
func assistantFacts(facts ...string) []anthropic.MessageParam {
	messages := make([]anthropic.MessageParam, 0, len(facts))
	for _, fact := range facts {
		messages = append(messages, anthropic.MessageParam{
			Role:    anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock(fact)},
		})
	}
	return messages
}

func agesOlderThanTruncation(n int) []int {
	ages := make([]int, n)
	for i := range ages {
		ages[i] = 99
	}
	return ages
}

func autoSaveTestConfig() ManagementConfig {
	return ManagementConfig{
		AssistantTruncateAfter: 1,
		AutoSaveThreshold:      1,
		MinimumImportance:      0.5,
	}
}
