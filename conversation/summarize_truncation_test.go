package conversation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// stubSummaryServerWithStopReason answers the Messages API with one text block
// and the stop reason the caller names. stubSummaryServer hardcodes "end_turn",
// which is the one case that cannot exercise the truncation path.
func stubSummaryServerWithStopReason(t *testing.T, summary, stopReason string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":            "msg_stub",
			"type":          "message",
			"role":          "assistant",
			"model":         "claude-sonnet-4-5-20250929",
			"content":       []map[string]any{{"type": "text", "text": summary}},
			"stop_reason":   stopReason,
			"stop_sequence": nil,
			"usage":         map[string]any{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	t.Cleanup(server.Close)
	return server
}

// A summary the model was still writing when it ran out of output tokens is the
// expected case, not the edge: the coder role condenses 40 messages into 800
// tokens. Compaction is destructive, so the fragment becomes the transcript —
// the result has to say so or nothing downstream can.
func TestASummaryCutOffAtTheTokenLimitIsReportedAsCutOff(t *testing.T) {
	server := stubSummaryServerWithStopReason(t, "the model got this far and then ran ou", "max_tokens")
	cfg := DefaultSummarizeConfig(RoleCoder)

	_, result, err := SummarizeConversation(
		context.Background(), clientFor(server), makeMessages(60), nil, "sess-cut", cfg, "claude-sonnet-4-5-20250929")
	if err != nil {
		t.Fatalf("summarization failed: %v", err)
	}
	if !result.Summarized {
		t.Fatal("fixture did not summarize, so it cannot be exercising the truncation path")
	}
	if !result.SummaryWasCutOffAtTokenLimit {
		t.Error("a max_tokens stop was reported as a finished summary")
	}
}

// The control. Without it the assertion above passes just as well against a
// field that is hardcoded true, and the operator gets the warning on every
// compaction until the word stops meaning anything.
func TestAFinishedSummaryIsNotReportedAsCutOff(t *testing.T) {
	server := stubSummaryServerWithStopReason(t, "a summary the model finished writing", "end_turn")
	cfg := DefaultSummarizeConfig(RoleCoder)

	_, result, err := SummarizeConversation(
		context.Background(), clientFor(server), makeMessages(60), nil, "sess-whole", cfg, "claude-sonnet-4-5-20250929")
	if err != nil {
		t.Fatalf("summarization failed: %v", err)
	}
	if !result.Summarized {
		t.Fatal("fixture did not summarize")
	}
	if result.SummaryWasCutOffAtTokenLimit {
		t.Error("an end_turn stop was reported as cut off at the token limit")
	}
}
