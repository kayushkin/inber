package conversation

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kayushkin/inber/memory"
)

// The stash takes a block out of a conversation and leaves a pointer to a memory.
// That trade is only worth making if the pointer names a tool the model holds:
// with no such tool the block is gone from the conversation and unreachable in
// the store, which is a deletion with a receipt.
//
// These tests fix the three answers — both tools, one tool, neither — against the
// text the model actually reads, because the defect they cover was invisible in
// every other signal. The stash returned a result, the memory row was written,
// the block came out of the message, and the pointer named memory_expand to an
// agent configured without it.

// stashConfigWithBothRecallTools is the config a session gets when it carries the
// whole memory tool set. Tests that are about stashing rather than about recall
// use it so they state their premise instead of inheriting it from a default.
func stashConfigWithBothRecallTools() StashConfig {
	cfg := DefaultStashConfig()
	cfg.RecallToolNames = []string{memory.ToolNameMemorySearch, memory.ToolNameMemoryExpand}
	return cfg
}

func newStashTestStore(t *testing.T) memory.MemoryStore {
	t.Helper()
	store, err := memory.NewSQLiteStore(filepath.Join(t.TempDir(), "stash.db"))
	if err != nil {
		t.Fatalf("create memory store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

const stashableBlock = "func handler() { /* a block well over the minimum */ }\n"

func TestStashPointerNamesOnlyTheRecallToolsOnTheWire(t *testing.T) {
	content := strings.Repeat(stashableBlock, 200)

	cases := []struct {
		name          string
		recallTools   []string
		wantInText    []string
		wantNotInText []string
	}{
		{
			name:        "both tools",
			recallTools: []string{memory.ToolNameMemorySearch, memory.ToolNameMemoryExpand},
			wantInText:  []string{"memory_search", "memory_expand(id="},
		},
		{
			name:          "expand only",
			recallTools:   []string{memory.ToolNameMemoryExpand},
			wantInText:    []string{"memory_expand(id="},
			wantNotInText: []string{"memory_search"},
		},
		{
			// No agent on this host is this case as of 2026-08-17: all ten
			// carrying memory tools have memory_expand as well as
			// memory_search. It read four of ten when this was written. The
			// case stays because the recall text is built from whatever tool
			// list it is handed, and that list is configuration — what no
			// agent happens to be configured as today is not a reason to stop
			// pinning it.
			name:          "search only",
			recallTools:   []string{memory.ToolNameMemorySearch},
			wantInText:    []string{"memory_search"},
			wantNotInText: []string{"memory_expand"},
		},
		{
			// A tool list holding neither, and a tool list holding unrelated
			// tools, are the same answer: no way back.
			name:        "no memory tools",
			recallTools: []string{"read_files", "list_files"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newStashTestStore(t)
			cfg := DefaultStashConfig()
			cfg.RecallToolNames = tc.recallTools

			result, err := StashLargeContent(content, "sess", store, cfg)
			if err != nil {
				t.Fatalf("stash: %v", err)
			}

			if len(tc.wantInText) == 0 {
				if result != nil {
					t.Fatalf("stashed with no recall tool on the wire: %q", result.Summary)
				}
				return
			}
			if result == nil {
				t.Fatal("did not stash a block that is over the minimum and recallable")
			}
			for _, want := range tc.wantInText {
				if !strings.Contains(result.Summary, want) {
					t.Errorf("pointer %q does not name %q", result.Summary, want)
				}
			}
			for _, unwanted := range tc.wantNotInText {
				if strings.Contains(result.Summary, unwanted) {
					t.Errorf("pointer %q names %q, which is not on the wire", result.Summary, unwanted)
				}
			}
		})
	}
}

// A pointer that names memory_expand has to carry an id memory_expand resolves,
// and it has to be the id of the row that was written.
func TestStashPointerIdResolvesToTheRowItNames(t *testing.T) {
	store := newStashTestStore(t)

	result, err := StashLargeContent(strings.Repeat(stashableBlock, 200), "sess", store, stashConfigWithBothRecallTools())
	if err != nil {
		t.Fatalf("stash: %v", err)
	}
	if result == nil {
		t.Fatal("did not stash a recallable block")
	}

	wantPointer := fmt.Sprintf("memory_expand(id=%q)", result.MemoryID)
	if !strings.Contains(result.Summary, wantPointer) {
		t.Fatalf("pointer %q does not name the row it wrote (%s)", result.Summary, result.MemoryID)
	}

	// The id in the pointer, read back the way the model would read it.
	recalled, err := store.Get(result.MemoryID)
	if err != nil {
		t.Fatalf("memory_expand on the id the pointer names: %v", err)
	}
	if !strings.Contains(recalled.Content, "a block well over the minimum") {
		t.Error("the recalled memory is not the stashed content")
	}
}

// The block-scanning entry point is the one the engine calls, and it is a
// separate gate from StashLargeContent's own. A sabotage of either has to be
// visible here: with no recall tool the text comes back untouched, which is the
// whole point — the content stays in the conversation instead of vanishing.
func TestDetectAndStashLeavesTextWholeWhenNothingCanRecallIt(t *testing.T) {
	store := newStashTestStore(t)
	input := "Here it is:\n```go\n" + strings.Repeat(stashableBlock, 200) + "```\nWhat do you think?"

	cfg := DefaultStashConfig()
	cfg.MinBlockSize = 100
	cfg.RecallToolNames = nil

	modified, stashed, err := DetectAndStashLargeBlocks(input, "sess", store, cfg)
	if err != nil {
		t.Fatalf("detect and stash: %v", err)
	}
	if len(stashed) != 0 {
		t.Fatalf("stashed %d blocks with no recall tool on the wire", len(stashed))
	}
	if modified != input {
		t.Error("text was modified even though nothing was stashed")
	}

	// And the mirror: the same input with a recall tool does stash, so the test
	// above is not passing because the fixture is unstashable.
	cfg.RecallToolNames = []string{memory.ToolNameMemoryExpand}
	modified, stashed, err = DetectAndStashLargeBlocks(input, "sess", store, cfg)
	if err != nil {
		t.Fatalf("detect and stash with recall: %v", err)
	}
	if len(stashed) == 0 {
		t.Fatal("did not stash a block that is recallable — the fixture is wrong")
	}
	if !strings.Contains(modified, "Large content stashed") {
		t.Error("stashed text does not carry the pointer")
	}
}
