package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/anthropics/anthropic-sdk-go"
)

// writeAllMessages cuts each message to an 80-byte preview for the table it
// writes into prompts/turn-N.md, and that cut did not hold the rune boundary.
// TestWritePromptBreakdown already reaches the line, but its fixture is
// "hello" — five ASCII bytes that never reach the budget, so the case cannot
// tell a byte cut from a rune-safe one. Measured 2026-09-07 on branch
// test/the-spawn-tool-preview-is-asked: reverting session/prompts_write.go:202
// to the pre-9ce1666 byte cut reddened nothing in the whole repository.
//
// The file is the destination, not an intermediate: a turn breakdown is read
// by a person and by dash, and a partial rune renders there as a replacement
// character in the middle of the user's own words.
//
// The fixture is driven through WritePromptBreakdown rather than through
// writeAllMessages, so the 80 stays where the production code puts it. A test
// that restates the budget goes quiet the day the budget moves, and it goes
// quiet in the flattering direction.
func TestThePromptTablePreviewIsCutOnARuneBoundary(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir, "claude-sonnet-4-20250514", "test", "", nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	// One ASCII byte in front of a run of two-byte runes, so every rune in the
	// run starts at an odd offset and any even budget necessarily cuts inside
	// one. The straddle guard below fails loudly rather than passing if a
	// changed budget ever moves the cut out of that run.
	long := "x" + strings.Repeat("é", 120)

	params := anthropic.MessageNewParams{
		Model:    "claude-sonnet-4-20250514",
		Messages: []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(long))},
	}
	if err := s.WritePromptBreakdown(1, &params, nil); err != nil {
		t.Fatalf("WritePromptBreakdown: %v", err)
	}

	path := filepath.Join(filepath.Dir(s.FilePath()), "prompts", "turn-1.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read turn-1.md: %v", err)
	}

	preview := previewCell(t, string(data))
	cut := strings.TrimSuffix(preview, "...")
	if cut == preview {
		t.Fatalf("the preview of a %d-byte message was not cut at all, so this case "+
			"cannot see the cut.\ngot %q", len(long), preview)
	}
	if !straddledCut(cut) {
		t.Fatalf("the cut landed in the fixture's ASCII prefix, so a byte cut and a "+
			"rune-safe cut would agree and this case could not tell them apart. The "+
			"fixture needs rebuilding against the current budget.\ngot %q", preview)
	}
	if !utf8.ValidString(preview) {
		t.Errorf("the message preview written into prompts/turn-1.md is not valid "+
			"UTF-8: the cut ended inside a rune.\ngot %q", preview)
	}
}

// previewCell pulls the Content column out of the single message row in a turn
// breakdown. Asserting on the whole file would be answered by the ASCII around
// it — the claim is about one cell, so the assertion reads one cell.
func previewCell(t *testing.T, doc string) string {
	t.Helper()
	for _, line := range strings.Split(doc, "\n") {
		if !strings.HasPrefix(line, "| 1 | user | ") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), " | ")
		if len(cells) != 4 {
			t.Fatalf("the message row does not have four cells, so the Content column "+
				"cannot be located.\ngot %q", line)
		}
		return strings.TrimSpace(cells[3])
	}
	t.Fatalf("no message row in the turn breakdown, so the preview cut was never "+
		"reached.\ngot %q", doc)
	return ""
}
