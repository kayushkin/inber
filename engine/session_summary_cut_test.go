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

// The per-block cut in SaveSessionSummary is asked two separate questions here,
// in two tests with two fixtures, because the call site makes two separable
// claims and one test that reddens for either cannot say which one broke.
//
// Neither test names textutil. Both observe the cut where it lands — in the
// memory row that SaveSessionSummary saves — so renaming or reimplementing the
// helper leaves them green, and reverting the CALL SITE does not.
//
// Both fixtures are built so the outer 2000-byte cut on the joined summary
// cannot fire: one block, comfortably under 2000 bytes once cut. What these
// tests see is the 200-byte per-block cut and nothing else.
//
// Why they exist: measured 2026-08-31 against inber main d472cb2, reverting
// this call site to the pre-9ce1666 byte cut reddened nothing in the whole
// repository, and neither did deleting the truncation altogether. The one test
// that already executes SaveSessionSummary — TestSessionSummaryIsNotOffered-
// ForAutomaticInjection — passes two short ASCII messages and asks about tag
// exclusion, so it reaches the line and cannot observe it. Card 3388f950.

// savedSessionSummary runs SaveSessionSummary over a single user block and
// hands back the memory row it wrote.
func savedSessionSummary(t *testing.T, block string) string {
	t.Helper()

	store, err := memory.NewStore(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	SaveSessionSummary(store, []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock(block)),
	}, "claxon")

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

// TestTheSessionSummaryCutLandsOnARuneBoundary is the rune-boundary claim on its
// own. The fixture puts a two-byte rune across byte 200 exactly: one ASCII byte
// followed by 200 copies of "é" means byte 200 is the FIRST byte of the rune
// that starts at 199, so a plain s[:200] ends inside it and the summary that
// reaches the memory store is invalid UTF-8.
//
// A cut that respects runes stops at 199 instead. The assertion is on the whole
// saved row rather than on a remembered prefix, so it cannot go red for a
// reworded fixture and cannot go green for a cut that merely moved.
func TestTheSessionSummaryCutLandsOnARuneBoundary(t *testing.T) {
	block := "x" + strings.Repeat("é", 200)
	if utf8.RuneStart(block[200]) {
		t.Fatalf("fixture does not straddle the cut: byte 200 begins a rune, "+
			"so a byte cut and a rune-safe cut would agree and this test could "+
			"not tell them apart (block is %d bytes)", len(block))
	}

	content := savedSessionSummary(t, block)

	if !utf8.ValidString(content) {
		t.Errorf("the session summary saved to memory is not valid UTF-8: "+
			"the per-block cut ended inside a rune.\nsaved %q", content)
	}
}

// TestTheSessionSummaryCutsABlockToItsBudget is the does-it-cut-at-all claim on
// its own, and its fixture is pure ASCII so it has no view of the rune boundary.
//
// It is written as a comparison between two blocks rather than as a match
// against a remembered length: a short block must survive whole and a long one
// must not. A change to the budget moves one of them; a reworded fixture moves
// neither.
func TestTheSessionSummaryCutsABlockToItsBudget(t *testing.T) {
	const budget = 200

	short := strings.Repeat("s", budget/2)
	if got := savedSessionSummary(t, short); !strings.Contains(got, short) {
		t.Errorf("a block of %d bytes is inside the %d-byte budget and should "+
			"reach memory whole; it did not.\nsaved %q", len(short), budget, got)
	}

	long := strings.Repeat("L", budget*5)
	got := savedSessionSummary(t, long)
	if strings.Contains(got, long) {
		t.Errorf("a block of %d bytes reached memory uncut, so the %d-byte "+
			"per-block budget bounds nothing.\nsaved %d bytes",
			len(long), budget, len(got))
	}
}
