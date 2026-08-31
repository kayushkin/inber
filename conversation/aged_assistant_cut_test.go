package conversation

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/anthropics/anthropic-sdk-go"
)

// The 300-byte fallback cut in truncateToSummary is asked two separate
// questions here, in two tests with two fixtures, because the call site makes
// two separable claims and one test that reddens for either cannot say which
// one broke.
//
// Neither test names textutil, and neither calls truncateToSummary. Both drive
// ManageConversation and read the cut where it lands — in the assistant text
// block the managed conversation carries — so renaming or reimplementing the
// helper leaves them green, and reverting the CALL SITE does not.
//
// Why this site: commit 9ce1666 ranked its own repairs, and "the cuts that
// reach the model are the ones that matter". What this one returns replaces an
// aged assistant block in the managed conversation, so it is sent as prose in
// every later prompt of the session.
//
// Why the fixtures look the way they do: truncateToSummary reaches the 300-byte
// cut only as a FALLBACK, when it found no sentence worth keeping. It examines
// the first three sentences and skips any shorter than 10 bytes, so both
// fixtures are built from fragments under that length. A fixture of ordinary
// prose would take the sentence path and never reach the line under test.
//
// Why they exist: measured 2026-08-31 against branch
// test/the-truncatewith-call-sites-are-asked at 79078e0, reverting this call
// site to the pre-9ce1666 byte cut reddened nothing in the whole repository,
// and neither did removing the truncation altogether. Card 3388f950, residual
// 3bec041f.

// agedAssistantText runs the live management path over a conversation whose
// first assistant turn is old enough to be truncated, and hands back the text
// that turn carries afterwards.
func agedAssistantText(t *testing.T, text string) string {
	t.Helper()

	cfg := DefaultManagementConfig()
	cfg.KeepRecentTurns = 1
	cfg.ManageInterval = 0
	cfg.AssistantTruncateAfter = 0

	messages := []anthropic.MessageParam{
		{Role: anthropic.MessageParamRoleUser, Content: []anthropic.ContentBlockParamUnion{textBlock("what did you do?")}},
		{Role: anthropic.MessageParamRoleAssistant, Content: []anthropic.ContentBlockParamUnion{textBlock(text)}},
	}
	for i := 0; i < 3; i++ {
		messages = append(messages,
			anthropic.MessageParam{Role: anthropic.MessageParamRoleUser, Content: []anthropic.ContentBlockParamUnion{textBlock("carry on")}},
			anthropic.MessageParam{Role: anthropic.MessageParamRoleAssistant, Content: []anthropic.ContentBlockParamUnion{textBlock("carrying on")}},
		)
	}

	managed, _, err := ManageConversation(t.Context(), messages, nil, "sess-aged", cfg)
	if err != nil {
		t.Fatalf("ManageConversation: %v", err)
	}
	if len(managed) < 2 || managed[1].Role != anthropic.MessageParamRoleAssistant {
		t.Fatalf("the aged assistant turn is no longer the second message; this test is "+
			"reading the wrong block (managed has %d messages)", len(managed))
	}
	for _, block := range managed[1].Content {
		if block.OfText != nil {
			if block.OfText.Text == text {
				t.Fatal("the fixture never reached the truncation: the aged assistant " +
					"block came back unchanged, so this test observed nothing")
			}
			return block.OfText.Text
		}
	}
	t.Fatal("the aged assistant turn has no text block left")
	return ""
}

// TestAnAgedAssistantTurnIsCutOnARuneBoundary is the rune-boundary claim on its
// own. The fixture puts a two-byte rune across byte 300 exactly, so a plain
// s[:300] ends on a lone lead byte and the prose carried into every later
// prompt is not valid UTF-8. A cut that respects runes stops at 299 instead.
func TestAnAgedAssistantTurnIsCutOnARuneBoundary(t *testing.T) {
	text := "xx" + strings.Repeat("éé.", 80)
	if utf8.RuneStart(text[300]) {
		t.Fatalf("fixture does not straddle the cut: byte 300 begins a rune, so a byte "+
			"cut and a rune-safe cut would agree and this test could not tell them "+
			"apart (text is %d bytes)", len(text))
	}

	kept := agedAssistantText(t, text)

	if !utf8.ValidString(kept) {
		t.Errorf("the aged assistant turn carried into every later prompt is not valid "+
			"UTF-8: the 300-byte cut ended inside a rune.\nkept %q", kept)
	}
}

// TestAnAgedAssistantTurnIsCutToItsBudget is the does-it-cut-at-all claim on
// its own. The fixture is pure ASCII, so a byte cut and a rune-safe cut agree
// on it and this test is blind to the boundary question the one above asks.
func TestAnAgedAssistantTurnIsCutToItsBudget(t *testing.T) {
	const tail = "QTAI"
	text := strings.Repeat("aaaa.", 80) + tail

	kept := agedAssistantText(t, text)

	if strings.Contains(kept, tail) {
		t.Errorf("a %d-byte assistant turn was carried whole into every later prompt: "+
			"the 300-byte cut did not happen.\nkept %d bytes", len(text), len(kept))
	}
	if !strings.Contains(kept, "...") {
		t.Errorf("the assistant turn was shortened without saying it had been cut, so "+
			"the model reads a truncated turn as a complete one.\nkept %q", kept)
	}
}
