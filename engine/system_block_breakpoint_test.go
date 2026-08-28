package engine

import (
	"fmt"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	sessionMod "github.com/kayushkin/inber/session"
)

// buildSystemBlocks used to carry four conditions guarding for a system block
// sitting after the BP2 breakpoint. None of them could fire: the breakpoint goes
// on the last block, so there is never anything after it. Driven exhaustively
// over 0-4 blocks in every empty/non-empty combination, across the cold, reuse
// and miss paths, the tail-append condition was evaluated 26 times and was false
// 26 times; the two length conditions were true every time they were evaluated,
// including when the slice was empty.
//
// Removing dead guards is only safe if something else holds the invariant they
// were pretending to hold. These tests are that something: they pin the shape of
// what buildSystemBlocks returns, so a later change that appends a block after
// the breakpoint reddens here instead of being dropped in silence.
//
// They pin placement only. How many cache_control markers the whole REQUEST may
// carry across tools, system and history is a separate open question and is not
// asked here.

// breakpointIndices returns the positions carrying a cache_control marker.
func breakpointIndices(blocks []anthropic.TextBlockParam) []int {
	var out []int
	for i, b := range blocks {
		if b.CacheControl.Type != "" {
			out = append(out, i)
		}
	}
	return out
}

func namedBlocks(texts ...string) []sessionMod.NamedBlock {
	out := make([]sessionMod.NamedBlock, 0, len(texts))
	for i, t := range texts {
		out = append(out, sessionMod.NamedBlock{ID: fmt.Sprintf("id%d", i), Text: t})
	}
	return out
}

// TestBuildSystemBlocksPutsOneBreakpointOnTheLastBlock is the guard that replaces
// the deleted tail-append. The expected index is COMPUTED from the returned
// slice, not written down, so it keeps holding as the block count changes.
func TestBuildSystemBlocksPutsOneBreakpointOnTheLastBlock(t *testing.T) {
	for n := 1; n <= 5; n++ {
		texts := make([]string, 0, n)
		for i := 0; i < n; i++ {
			texts = append(texts, fmt.Sprintf("block-%d", i))
		}
		e := &Engine{}
		got := e.buildSystemBlocks(namedBlocks(texts...))

		if len(got) != n {
			t.Fatalf("n=%d: got %d system blocks, want %d", n, len(got), n)
		}
		marked := breakpointIndices(got)
		if len(marked) != 1 {
			t.Fatalf("n=%d: %d blocks carry cache_control, want exactly 1 (at %v)", n, len(marked), marked)
		}
		if marked[0] != len(got)-1 {
			t.Fatalf("n=%d: breakpoint on block %d, want the last block %d — a block after the "+
				"breakpoint is outside the cached prefix and pays full price every turn",
				n, marked[0], len(got)-1)
		}
	}
}

// TestBuildSystemBlocksSkipsEmptyBlocksAndStillMarksTheLast pins that the
// empty-text filter cannot move the breakpoint off the end. This is the shape
// that would have made the deleted tail-append fire if anything could.
func TestBuildSystemBlocksSkipsEmptyBlocksAndStillMarksTheLast(t *testing.T) {
	cases := [][]string{
		{"", "a", "b"},
		{"a", "", "b"},
		{"a", "b", ""},
		{"", "a", "", "b", ""},
	}
	for _, in := range cases {
		e := &Engine{}
		got := e.buildSystemBlocks(namedBlocks(in...))
		if len(got) != 2 {
			t.Fatalf("%q: got %d blocks, want the 2 non-empty ones", in, len(got))
		}
		marked := breakpointIndices(got)
		if len(marked) != 1 || marked[0] != len(got)-1 {
			t.Fatalf("%q: breakpoints at %v, want exactly one on the last block %d",
				in, marked, len(got)-1)
		}
	}
}

// TestBuildSystemBlocksOnNoUsableBlocksReturnsNothing pins the empty case, which
// is the only case the deleted `cacheIdx >= 0` half genuinely distinguished.
func TestBuildSystemBlocksOnNoUsableBlocksReturnsNothing(t *testing.T) {
	for _, in := range [][]string{{}, {""}, {"", "", ""}} {
		e := &Engine{}
		if got := e.buildSystemBlocks(namedBlocks(in...)); len(got) != 0 {
			t.Fatalf("%q: got %d blocks, want none", in, len(got))
		}
		if e.Cache.LastStablePrefix != nil {
			t.Fatalf("%q: cached a stable prefix for a request with no system blocks", in)
		}
	}
}

// TestBuildSystemBlocksKeepsTheBreakpointOnAPrefixCacheHit pins the reuse path.
// A hit that dropped the marker would cost real money and break nothing visible.
func TestBuildSystemBlocksKeepsTheBreakpointOnAPrefixCacheHit(t *testing.T) {
	e := &Engine{}
	in := namedBlocks("alpha", "beta", "gamma")

	cold := e.buildSystemBlocks(in)
	warm := e.buildSystemBlocks(in)

	if len(warm) != len(cold) {
		t.Fatalf("reuse path returned %d blocks, cold path returned %d", len(warm), len(cold))
	}
	for i := range cold {
		if warm[i].Text != cold[i].Text {
			t.Fatalf("block %d: reuse path sent %q, cold path sent %q — the prefix is not "+
				"byte-identical, so the cache cannot hit", i, warm[i].Text, cold[i].Text)
		}
		if (warm[i].CacheControl.Type != "") != (cold[i].CacheControl.Type != "") {
			t.Fatalf("block %d: cache_control present=%v on the reuse path, %v on the cold path",
				i, warm[i].CacheControl.Type != "", cold[i].CacheControl.Type != "")
		}
	}
	if marked := breakpointIndices(warm); len(marked) != 1 || marked[0] != len(warm)-1 {
		t.Fatalf("reuse path: breakpoints at %v, want exactly one on the last block %d",
			marked, len(warm)-1)
	}
}

// TestBuildSystemBlocksDoesNotHandTheCallerItsOwnCache pins the copy on the reuse
// path. Returning the stored slice directly is the obvious simplification and it
// lets any caller that edits its system blocks silently reshape the next turn's
// cached prefix — a cache miss with no failure anywhere.
func TestBuildSystemBlocksDoesNotHandTheCallerItsOwnCache(t *testing.T) {
	e := &Engine{}
	in := namedBlocks("alpha", "beta")

	e.buildSystemBlocks(in)
	warm := e.buildSystemBlocks(in)
	warm[0].Text = "MUTATED BY THE CALLER"

	again := e.buildSystemBlocks(in)
	if again[0].Text != "alpha" {
		t.Fatalf("a caller's edit reached the cached prefix: next turn sends %q, want %q",
			again[0].Text, "alpha")
	}
	if e.Cache.LastStablePrefix.blocks[0].Text != "alpha" {
		t.Fatalf("a caller's edit reached the stored prefix: cache holds %q, want %q",
			e.Cache.LastStablePrefix.blocks[0].Text, "alpha")
	}
}
