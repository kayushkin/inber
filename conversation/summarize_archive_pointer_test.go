package conversation

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	memorystore "github.com/kayushkin/memory-store"
	_ "modernc.org/sqlite"
)

// A compaction takes turns out of the conversation and files them verbatim in
// memory. The archive is tagged out of the automatic context on purpose, so the
// only way back to it is memory_expand by id — and the id was returned to the
// caller and logged, never put where the model could read it. The write had no
// reachable read.
//
// These tests pin the pointer to the two facts that make it true: the archive
// was actually saved, and memory_expand is on the wire.

// archivePointer reports whether the injected summary block tells the model to
// call memory_expand, and with which id.
//
// The two answers are separate on purpose. A gate that has lost its
// "was anything archived" half emits the call with an empty id, and a helper
// that returned only the id would read that as "no pointer" and pass — the
// sabotage round found exactly that hole.
var archivePointer = func() func(string) (string, bool) {
	pattern := regexp.MustCompile(`memory_expand\(id="([^"]*)"\)`)
	return func(block string) (string, bool) {
		m := pattern.FindStringSubmatch(block)
		if m == nil {
			return "", strings.Contains(block, "memory_expand")
		}
		return m[1], true
	}
}()

func recallableConfig() SummarizeConfig {
	cfg := DefaultSummarizeConfig(RoleCoder)
	cfg.ArchiveIsRecallable = true
	return cfg
}

// The whole defect, end to end: the id in the block has to resolve in the store
// the archive was written to. Asserting only that *some* id is printed would
// pass on a truncated or stale one.
func TestArchivePointerResolvesInTheStore(t *testing.T) {
	store, err := memorystore.NewStore(t.TempDir() + "/memory.db")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	server := stubSummaryServer(t, "the model's actual summary")
	in := uniquelyMarkedMessages(60)
	out, result, err := SummarizeConversation(
		context.Background(), clientFor(server), in, store, "sess-archive",
		recallableConfig(), "claude-sonnet-4-5-20250929")
	if err != nil {
		t.Fatalf("summarization failed: %v", err)
	}
	if !result.MemorySaved {
		t.Fatal("nothing was archived, so there is no pointer to test")
	}

	block := textOf(out[0])
	id, present := archivePointer(block)
	if !present {
		t.Fatalf("the summary block names no way back to the archived turns:\n%s", block)
	}
	if id == "" {
		t.Fatalf("the summary block calls memory_expand with no id:\n%s", block)
	}
	if id != result.MemoryID {
		t.Errorf("block names memory %q, the archive was written as %q", id, result.MemoryID)
	}

	archived, err := store.Get(id)
	if err != nil {
		t.Fatalf("memory_expand(id=%q) — the read the block tells the model to make — cannot resolve it: %v", id, err)
	}
	// The archive is the turns the summary replaced, so it must carry the earliest
	// one — and the compacted conversation must not, or nothing was compacted and
	// the recall would be pointless.
	const earliest = "turn-0:"
	if !strings.Contains(archived.Content, earliest) {
		t.Errorf("the archived memory does not hold the condensed turns; content begins %q", truncateForMessage(archived.Content))
	}
	for i, msg := range out {
		if strings.Contains(textOf(msg), earliest) {
			t.Fatalf("message %d still carries the earliest turn, so nothing was taken out to archive", i)
		}
	}
	if strings.Contains(block, archived.Content) {
		t.Error("the block inlines the archive it points at, which un-does the compaction")
	}
}

// uniquelyMarkedMessages is makeMessages with a marker per message. makeMessages
// cycles a-z, so a test cannot tell the first turn from the twenty-seventh and
// "the archive holds what was dropped" cannot be asserted on it.
func uniquelyMarkedMessages(n int) []anthropic.MessageParam {
	msgs := make([]anthropic.MessageParam, n)
	for i := range msgs {
		text := fmt.Sprintf("turn-%d: user says something", i)
		if i%2 == 0 {
			msgs[i] = anthropic.NewUserMessage(anthropic.NewTextBlock(text))
		} else {
			msgs[i] = anthropic.NewAssistantMessage(anthropic.NewTextBlock(
				fmt.Sprintf("turn-%d: assistant answers", i)))
		}
	}
	return msgs
}

// A pointer at a tool the agent does not hold is the same lie inverted: the
// model is told to make a call it cannot make.
func TestNoArchivePointerWhenMemoryExpandIsNotOnTheWire(t *testing.T) {
	server := stubSummaryServer(t, "a summary")
	cfg := DefaultSummarizeConfig(RoleCoder) // ArchiveIsRecallable stays false
	store := &recordingStore{}

	out, result, err := SummarizeConversation(
		context.Background(), clientFor(server), makeMessages(60), store, "sess-no-tool",
		cfg, "claude-sonnet-4-5-20250929")
	if err != nil {
		t.Fatalf("summarization failed: %v", err)
	}
	if !result.MemorySaved {
		t.Fatal("fixture stopped archiving, so the assertion below holds for the wrong reason")
	}
	if id, present := archivePointer(textOf(out[0])); present {
		t.Errorf("block names memory_expand(id=%q) to an agent that has no memory_expand", id)
	}
}

// Nothing was written, so there is nothing to promise. This is the gate the
// original bug was the mirror of.
func TestNoArchivePointerWhenNothingWasArchived(t *testing.T) {
	server := stubSummaryServer(t, "a summary")

	for _, c := range []struct {
		name  string
		store memorystore.MemoryStore
		cfg   func() SummarizeConfig
	}{
		{"no store", nil, recallableConfig},
		{"saving disabled", &recordingStore{}, func() SummarizeConfig {
			cfg := recallableConfig()
			cfg.SaveToMemory = false
			return cfg
		}},
		{"save failed", &failingSaveStore{}, recallableConfig},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, result, err := SummarizeConversation(
				context.Background(), clientFor(server), makeMessages(60), c.store, "sess-none",
				c.cfg(), "claude-sonnet-4-5-20250929")
			if err != nil {
				t.Fatalf("summarization failed: %v", err)
			}
			if result.MemorySaved {
				t.Fatal("fixture archived after all; this case is meant to have no archive")
			}
			block := textOf(out[0])
			if id, present := archivePointer(block); present {
				t.Errorf("block promises memory_expand(id=%q) for an archive that was never written", id)
			}
			if !strings.Contains(block, "End of summary") {
				t.Errorf("block lost its closing marker:\n%s", block)
			}
		})
	}
}

// failingSaveStore archives nothing and says so. SummarizeConversation logs the
// failure and carries on, which is the branch that leaves MemorySaved false with
// a store present.
type failingSaveStore struct{ recordingStore }

func (f *failingSaveStore) Save(memorystore.Memory) error {
	return errSaveRefused
}

var errSaveRefused = &saveRefusedError{}

type saveRefusedError struct{}

func (*saveRefusedError) Error() string { return "store refused the write" }

func truncateForMessage(s string) string {
	if len(s) <= 80 {
		return s
	}
	return s[:80] + "…"
}
