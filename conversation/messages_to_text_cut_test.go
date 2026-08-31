package conversation

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/anthropics/anthropic-sdk-go"
)

// The 200-byte cut in extractToolResultText is asked two separate questions
// here, in two tests with two fixtures, because the call site makes two
// separable claims and one test that reddens for either cannot say which one
// broke.
//
// Neither test names textutil. Both read the cut where it lands — in the text
// messagesToText renders — so renaming or reimplementing the helper leaves them
// green, and reverting the CALL SITE does not.
//
// Why this site and not one of the other ten: commit 9ce1666 ranked its own
// repairs, and "the cuts that reach the model are the ones that matter".
// messagesToText's own doc comment says it is "the only thing the summarizing
// model ever sees of the turns being compacted, and the summary replaces them",
// so this cut reaches the model twice over — once in the summarizing prompt and
// again in every later prompt the summary is carried in.
//
// Why they exist: measured 2026-08-31 against branch
// test/the-truncatewith-call-sites-are-asked at 2c597ad, reverting this call
// site to the pre-9ce1666 byte cut reddened nothing in the whole repository,
// and neither did removing the truncation altogether. Card 3388f950, residual
// 3bec041f.

// renderedToolResult runs messagesToText over one assistant turn carrying a
// single tool result and hands back what the summarizing model would read.
func renderedToolResult(output string) string {
	return messagesToText([]anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewToolResultBlock("toolu_shell", output, false)),
	})
}

// TestARenderedToolResultIsCutOnARuneBoundary is the rune-boundary claim on its
// own. The fixture puts a two-byte rune across byte 200 exactly: one ASCII byte
// followed by 200 copies of "é" means the byte at index 200 is the SECOND byte
// of the rune that starts at 199, so a plain s[:200] ends on a lone lead byte
// and what reaches the summarizing model is not valid UTF-8.
//
// A cut that respects runes stops at 199 instead. The assertion is on the whole
// rendered text rather than on a remembered prefix, so it cannot go red for a
// reworded fixture and cannot go green for a cut that merely moved.
func TestARenderedToolResultIsCutOnARuneBoundary(t *testing.T) {
	output := "x" + strings.Repeat("é", 200)
	if utf8.RuneStart(output[200]) {
		t.Fatalf("fixture does not straddle the cut: byte 200 begins a rune, "+
			"so a byte cut and a rune-safe cut would agree and this test could "+
			"not tell them apart (output is %d bytes)", len(output))
	}

	rendered := renderedToolResult(output)

	if !utf8.ValidString(rendered) {
		t.Errorf("the text handed to the summarizing model is not valid UTF-8: "+
			"the tool-result cut ended inside a rune.\nrendered %q", rendered)
	}
}

// TestARenderedToolResultIsCutToItsBudget is the does-it-cut-at-all claim on
// its own. The fixture is pure ASCII, so a byte cut and a rune-safe cut agree
// on it and this test is blind to the boundary question the one above asks.
//
// It reads the tail rather than the length: a tool result that prints progress
// and then fails puts its error message last, so the tail is the part whose
// presence or absence changes what the summary says.
func TestARenderedToolResultIsCutToItsBudget(t *testing.T) {
	const tail = "LAST-LINE-OF-THE-TOOL-OUTPUT"
	output := strings.Repeat("a", 400) + tail

	rendered := renderedToolResult(output)

	if strings.Contains(rendered, tail) {
		t.Errorf("a %d-byte tool result reached the summarizing model whole: "+
			"the 200-byte cut did not happen.\nrendered %d bytes", len(output), len(rendered))
	}
	if !strings.Contains(rendered, "...") {
		t.Errorf("the tool result was shortened without saying it had been cut, "+
			"so the summarizing model reads a truncated command as a complete "+
			"one.\nrendered %q", rendered)
	}
}
