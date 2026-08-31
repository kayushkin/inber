package engine

import (
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/memory"

	_ "modernc.org/sqlite"
)

// SaveSessionSummary cuts TWICE, and only one of the two cuts was ever asked.
//
// engine/lifecycle.go:301 cuts each block to 200 bytes, and
// session_summary_cut_test.go pins it. Eleven lines further down, :312 cuts the
// whole joined summary to 2000 bytes before it becomes the memory row's
// Content. That second cut had no test of any kind: measured 2026-08-31 on
// branch test/the-truncatewith-call-sites-are-asked, reverting it to the
// pre-9ce1666 byte cut reddened nothing in the whole repository, and neither
// did deleting it.
//
// It was invisible for a reason worth writing down. The case list that judged
// these sites was enumerated from `grep -n TruncateWith`, and :312 calls
// textutil.Truncate — a different one of the package's three exported cuts. The
// helper is one package, the sweep was one spelling, and ten live cuts sat
// outside it. Card 3388f950, residual 3bec041f.
//
// Where this cut's output goes: memory.Memory.Content, tagged
// memory.TagSessionSummary, which later turns recall into the prompt. So a
// split rune here reaches the model, which is the class 9ce1666's own commit
// message calls "the cuts that matter".
//
// The sibling file's fixtures are built so this cut CANNOT fire — "one block,
// comfortably under 2000 bytes once cut". These are built so only this one can.

// savedSessionSummaryOf runs SaveSessionSummary over one user message per block
// and hands back the memory row it wrote. The multi-block sibling of
// savedSessionSummary in session_summary_cut_test.go, which takes exactly one.
func savedSessionSummaryOf(t *testing.T, blocks []string) string {
	t.Helper()

	store, err := memory.NewStore(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	messages := make([]anthropic.MessageParam, 0, len(blocks))
	for _, block := range blocks {
		messages = append(messages, anthropic.NewUserMessage(anthropic.NewTextBlock(block)))
	}
	SaveSessionSummary(store, messages, "claxon")

	saved, err := store.ListRecent(10, 0)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	for _, m := range saved {
		for _, tag := range m.Tags {
			if tag == memory.TagSessionSummary {
				return m.Content
			}
		}
	}
	t.Fatalf("SaveSessionSummary wrote no memory tagged %q; got %d row(s)",
		memory.TagSessionSummary, len(saved))
	return ""
}

// The fixture. Ten filler blocks of plain ASCII, each short enough that the
// 200-byte per-block cut leaves it alone, then one block of nothing but "é".
// The per-block cut takes that last block to exactly 200 bytes, and 200 is a
// rune boundary for a two-byte rune, so the per-block cut cannot be what these
// tests observe — whichever way it cuts, it agrees with itself. What is left in
// the joined summary is a 200-byte run of two-byte runes, and the 2000-byte cut
// lands inside it.
const (
	summaryFillerBlocks = 10
	summaryFillerBytes  = 180
	summaryTailRune     = "é" // two bytes
	summaryTailBlocks   = 150 // 300 bytes, cut to 200 by the per-block budget
)

func summaryFixture(pad int) []string {
	blocks := make([]string, 0, summaryFillerBlocks+1)
	for i := 0; i < summaryFillerBlocks; i++ {
		n := summaryFillerBytes
		if i == 0 {
			n += pad
		}
		blocks = append(blocks, strings.Repeat("a", n))
	}
	return append(blocks, strings.Repeat(summaryTailRune, summaryTailBlocks))
}

// TestTheJoinedSessionSummaryCutLandsOnARuneBoundary is the rune-boundary claim
// on the OUTER cut, on its own.
//
// It sweeps the fixture one byte at a time instead of building a single input
// engineered to straddle byte 2000, because the cut index is a constant in the
// code and a fixture pinned to today's value goes quiet the day it changes —
// quiet in the flattering direction. Shifting the prefix by one byte moves the
// cut's offset within the run of two-byte runes by one, so four consecutive
// pads cover both residues modulo the rune width. A byte cut is invalid UTF-8
// on the odd ones whatever the budget happens to be.
func TestTheJoinedSessionSummaryCutLandsOnARuneBoundary(t *testing.T) {
	for pad := 0; pad < 4; pad++ {
		content := savedSessionSummaryOf(t, summaryFixture(pad))

		// The fixture is only capable of failing while the outer cut lands
		// inside the run of two-byte runes. The per-block cut appends "..." to
		// that last block, so if the marker survived into the saved row the cut
		// happened after the run — or did not happen — and this case would pass
		// no matter how the code cut. Say so rather than report a green.
		if strings.Contains(content, "...") {
			t.Fatalf("pad=%d: the per-block marker survived into the saved summary, "+
				"so the 2000-byte cut did not land inside the multibyte run and this "+
				"case cannot tell a byte cut from a rune-safe one. The fixture needs "+
				"rebuilding against the current budget.\nsaved %d bytes", pad, len(content))
		}

		if !utf8.ValidString(content) {
			t.Errorf("pad=%d: the session summary saved to memory is not valid UTF-8: "+
				"the 2000-byte cut on the joined summary ended inside a rune.\n"+
				"saved %d bytes, tail %q", pad, len(content), content[len(content)-8:])
		}
	}
}

// TestTheJoinedSessionSummaryIsCutToItsBudget is the does-it-cut-at-all claim on
// its own, and its fixture is pure ASCII so it has no view of the rune boundary.
//
// Written as a comparison between two runs rather than a match against a
// remembered length: a summary that fits must survive whole, one that does not
// must come back shorter than it went in. A change to the budget moves one of
// them and not the other; a reworded fixture moves neither.
func TestTheJoinedSessionSummaryIsCutToItsBudget(t *testing.T) {
	short := []string{strings.Repeat("s", 100)}
	if got := savedSessionSummaryOf(t, short); !strings.Contains(got, short[0]) {
		t.Errorf("a one-block summary of %d bytes is inside the joined budget and "+
			"should reach memory whole; it did not.\nsaved %q", len(short[0]), got)
	}

	long := make([]string, 0, 40)
	for i := 0; i < 40; i++ {
		long = append(long, strings.Repeat("L", 150))
	}
	got := savedSessionSummaryOf(t, long)

	// Every block is inside the 200-byte per-block budget, so anything missing
	// from the saved row was removed by the joined cut and by nothing else.
	uncut := 0
	for _, block := range long {
		uncut += len("user: ") + len(block) + len("\n")
	}
	if len(got) >= uncut {
		t.Errorf("%d blocks totalling at least %d bytes reached memory uncut, so the "+
			"cut on the joined summary bounds nothing.\nsaved %d bytes",
			len(long), uncut, len(got))
	}
}
