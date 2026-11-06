package conversation

import (
	"context"
	"strings"
	"testing"
)

// A refusal that carries text used to pass as a finished summary, and the
// engine then wrote the refusal notice over the older turns. The text here is
// the shape the API returns: a notice, or the start of a reply the model was
// stopped partway through. The result has to be an error, so the caller keeps
// the transcript it handed in.
func TestARefusedSummaryWithTextIsAnErrorAndTheTranscriptIsKept(t *testing.T) {
	refusalText := "I can't help with reproducing or distilling these responses."
	server := stubSummaryServerWithStopReason(t, refusalText, "refusal")
	cfg := DefaultSummarizeConfig(RoleCoder)
	in := makeMessages(60)
	textBefore := make([]string, len(in))
	for i := range in {
		textBefore[i] = textOf(in[i])
	}

	out, result, err := SummarizeConversation(
		context.Background(), clientFor(server), in, nil, "sess-refused", cfg, "claude-sonnet-4-5-20250929")
	if err == nil {
		t.Fatal("a refusal was accepted as a summary")
	}
	if !strings.Contains(err.Error(), "refused") {
		t.Errorf("error does not say the model refused: %v", err)
	}
	if result.Summarized {
		t.Error("result says the conversation was summarized")
	}
	if len(out) != len(textBefore) {
		t.Fatalf("transcript length changed: %d -> %d", len(textBefore), len(out))
	}
	for i := range out {
		if textOf(out[i]) != textBefore[i] {
			t.Fatalf("message %d changed", i)
		}
		if strings.Contains(textOf(out[i]), refusalText) {
			t.Fatalf("message %d carries the refusal text", i)
		}
	}
}

// The control: the same text with stop_reason end_turn is a summary. Without
// it the test above passes just as well against a summarizer that fails on
// every reply.
func TestTheSameTextWithEndTurnIsAccepted(t *testing.T) {
	server := stubSummaryServerWithStopReason(t, "I can't help with reproducing or distilling these responses.", "end_turn")
	cfg := DefaultSummarizeConfig(RoleCoder)

	_, result, err := SummarizeConversation(
		context.Background(), clientFor(server), makeMessages(60), nil, "sess-end-turn", cfg, "claude-sonnet-4-5-20250929")
	if err != nil {
		t.Fatalf("summarization failed: %v", err)
	}
	if !result.Summarized {
		t.Error("an end_turn reply was not used as the summary")
	}
}
