package engine

import (
	"strings"
	"testing"

	memorystore "github.com/kayushkin/memory-store"
)

// fixedMemoryStore answers every BuildContext with the same memories.
type fixedMemoryStore struct {
	emptyMemoryStore
	memories []memorystore.Memory
}

func (s *fixedMemoryStore) BuildContext(memorystore.BuildContextRequest) ([]memorystore.Memory, int, error) {
	return s.memories, 100, nil
}

// The stable/volatile split is the whole reason the system prefix can be cached,
// and until this test it was pinned by nothing — which is how the code came to
// carry a "__CACHE_BOUNDARY__" sentinel constant, a zero-caller
// buildDynamicBlocks and a doc comment listing fleet status and recent files as
// system blocks 4-6, none of which had been true since volatile content moved
// into the user message.
//
// The rule the sentinel used to express: anything that changes between turns
// must not reach a system block, because a system block sits in front of BP2 and
// rewriting one invalidates the cached prefix for the whole session.
func TestVolatileMemoriesStayOutOfTheCachedSystemPrefix(t *testing.T) {
	store := &fixedMemoryStore{memories: []memorystore.Memory{
		{ID: "identity-1", Content: "You are Claxon.", Importance: 1.0},
		{ID: "recent:engine/turn_prompt.go", Content: "Recently modified: engine/turn_prompt.go", Importance: 0.7},
		{ID: "fileref:agent/agent.go", Content: "File ref: agent/agent.go", Importance: 0.6},
	}}
	e := &Engine{MemStore: store}

	blocks, err := e.BuildSystemPrompt("what changed?")
	if err != nil {
		t.Fatalf("BuildSystemPrompt returned an error: %v", err)
	}

	// The stable memory is the only thing that may become a system block.
	if !blocksContain(blocks, "You are Claxon.") {
		t.Fatalf("stable identity memory did not reach the system blocks: %v", blocks)
	}
	for _, volatile := range []string{"Recently modified", "File ref"} {
		if blocksContain(blocks, volatile) {
			t.Fatalf("volatile memory %q reached a system block, which invalidates the BP2 prefix every turn: %v", volatile, blocks)
		}
	}

	// ...and it has to actually arrive on the other side, or the split is just
	// dropping context. This is the half a "no volatile in system" assertion
	// alone would let a regression delete.
	for _, volatile := range []string{"Recently modified", "File ref"} {
		if !strings.Contains(e.Turn.VolatileContext, volatile) {
			t.Fatalf("volatile memory %q reached neither the system blocks nor VolatileContext — it was dropped: %q", volatile, e.Turn.VolatileContext)
		}
	}
	if strings.Contains(e.Turn.VolatileContext, "You are Claxon.") {
		t.Fatalf("stable identity memory was duplicated into VolatileContext: %q", e.Turn.VolatileContext)
	}
}

// The prefix is only reusable if it is byte-identical, so a turn whose volatile
// memories changed and whose stable ones did not must produce the same system
// blocks. This is what the cache actually rides on.
func TestChangingVolatileMemoriesLeavesTheSystemBlocksIdentical(t *testing.T) {
	stable := memorystore.Memory{ID: "identity-1", Content: "You are Claxon.", Importance: 1.0}

	first := &fixedMemoryStore{memories: []memorystore.Memory{
		stable,
		{ID: "recent:a.go", Content: "Recently modified (1 minute ago): a.go", Importance: 0.7},
	}}
	second := &fixedMemoryStore{memories: []memorystore.Memory{
		stable,
		{ID: "recent:b.go", Content: "Recently modified (9 hours ago): b.go", Importance: 0.5},
	}}

	blocksOf := func(store *fixedMemoryStore) string {
		e := &Engine{MemStore: store}
		blocks, err := e.BuildSystemPrompt("question")
		if err != nil {
			t.Fatalf("BuildSystemPrompt returned an error: %v", err)
		}
		var texts []string
		for _, b := range blocks {
			texts = append(texts, b.Text)
		}
		return strings.Join(texts, "\x00")
	}

	if a, b := blocksOf(first), blocksOf(second); a != b {
		t.Fatalf("volatile churn changed the cacheable system prefix:\n first: %q\nsecond: %q", a, b)
	}
}
