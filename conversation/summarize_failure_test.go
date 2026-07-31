package conversation

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	memorystore "github.com/kayushkin/memory-store"
)

// unreachableClient talks to a closed port, so every summarization API call fails
// on connect. No network, no fixtures, deterministic.
func unreachableClient() *anthropic.Client {
	c := anthropic.NewClient(
		option.WithBaseURL("http://127.0.0.1:1/"),
		option.WithAPIKey("unused"),
		option.WithMaxRetries(0),
	)
	return &c
}

// recordingStore records every Save so a test can assert what a failed
// summarization filed.
type recordingStore struct{ saved []memorystore.Memory }

func (r *recordingStore) Save(m memorystore.Memory) error {
	r.saved = append(r.saved, m)
	return nil
}
func (r *recordingStore) Get(string) (*memorystore.Memory, error)          { return nil, nil }
func (r *recordingStore) Search(string, int) ([]memorystore.Memory, error) { return nil, nil }
func (r *recordingStore) SearchFiltered(string, int, string) ([]memorystore.Memory, error) {
	return nil, nil
}
func (r *recordingStore) Forget(string) error                                   { return nil }
func (r *recordingStore) DecayImportance() error                                { return nil }
func (r *recordingStore) ListRecent(int, float64) ([]memorystore.Memory, error) { return nil, nil }
func (r *recordingStore) Compact(time.Duration, int) ([]memorystore.CompactionResult, error) {
	return nil, nil
}
func (r *recordingStore) BuildContext(memorystore.BuildContextRequest) ([]memorystore.Memory, int, error) {
	return nil, 0, nil
}
func (r *recordingStore) PrepareSession(context.Context, memorystore.PrepareSessionConfig) error {
	return nil
}
func (r *recordingStore) LoadToolRegistry([]memorystore.ToolMetadata) error  { return nil }
func (r *recordingStore) UpdateToolUsageSummary(string, string, int64) error { return nil }
func (r *recordingStore) TrackMemoryUsage(string, string, int, string) error { return nil }
func (r *recordingStore) Close() error                                       { return nil }

// A summary the model never produced must not be substituted for the turns it was
// supposed to replace. This is the whole defect: before the fix the call returned
// err == nil and a word-frequency list in place of 42 messages, and the engine
// wrote that over messages.json.
func TestFailedSummaryLeavesTheTranscriptWhole(t *testing.T) {
	cfg := DefaultSummarizeConfig(RoleCoder)
	in := makeMessages(60)

	out, result, err := SummarizeConversation(
		context.Background(), unreachableClient(), in, nil, "sess-fail", cfg, "claude-sonnet-4-5-20250929")

	if err == nil {
		t.Fatal("a summarization whose API call failed reported success")
	}
	if result.Summarized {
		t.Error("result claims the conversation was summarized when no summary exists")
	}
	if len(out) != len(in) {
		t.Fatalf("failed summarization changed the transcript: %d messages in, %d out", len(in), len(out))
	}
	for i := range in {
		if textOf(out[i]) != textOf(in[i]) {
			t.Fatalf("message %d was rewritten by a failed summarization: %q → %q",
				i, textOf(in[i]), textOf(out[i]))
		}
	}
	if !strings.Contains(err.Error(), "summarization API call failed") {
		t.Errorf("error does not name the cause: %v", err)
	}
}

// The memory row says a compaction happened. Writing it before the summary exists
// files a copy of the entire transcript on every retry, for compactions that never
// occurred — the leak that moving the save after the summary prevents.
func TestFailedSummaryFilesNoMemoryRow(t *testing.T) {
	cfg := DefaultSummarizeConfig(RoleCoder)
	store := &recordingStore{}
	msgs := makeMessages(60)

	for turn := 1; turn <= 3; turn++ {
		out, _, err := SummarizeConversation(
			context.Background(), unreachableClient(), msgs, store, "sess-retry", cfg, "claude-sonnet-4-5-20250929")
		if err == nil {
			t.Fatalf("turn %d: expected the unreachable summarizer to fail", turn)
		}
		msgs = out
	}

	if len(store.saved) != 0 {
		t.Errorf("three failed summarizations filed %d conversation-history memories; want 0", len(store.saved))
	}
}

// Complement: the success path must still save, or the test above could pass by
// the save being broken outright rather than merely reordered.
func TestSuccessfulSummaryStillFilesItsMemoryRow(t *testing.T) {
	server := stubSummaryServer(t, "the model's actual summary")
	cfg := DefaultSummarizeConfig(RoleCoder)
	store := &recordingStore{}

	out, result, err := SummarizeConversation(
		context.Background(), clientFor(server), makeMessages(60), store, "sess-ok", cfg, "claude-sonnet-4-5-20250929")
	if err != nil {
		t.Fatalf("summarization against a working endpoint failed: %v", err)
	}
	if !result.Summarized {
		t.Fatal("summarization against a working endpoint did not summarize")
	}
	if len(out) >= 60 {
		t.Errorf("nothing was compacted: %d messages out of 60", len(out))
	}
	if len(store.saved) != 1 {
		t.Fatalf("successful summarization filed %d memory rows; want 1", len(store.saved))
	}
	if !result.MemorySaved || result.MemoryID == "" {
		t.Error("result does not report the memory row it wrote")
	}
	if !strings.Contains(textOf(out[0]), "the model's actual summary") {
		t.Errorf("summary message does not carry the model's summary: %q", textOf(out[0]))
	}
}

// KeptMessages is what the session log and the operator see. Counting before the
// strip/alternation passes reports messages that are not in the returned list.
func TestKeptMessagesCountsWhatIsReturned(t *testing.T) {
	server := stubSummaryServer(t, "summary")
	cfg := DefaultSummarizeConfig(RoleCoder)

	out, result, err := SummarizeConversation(
		context.Background(), clientFor(server), messagesWithOrphanedToolResults(60), nil, "sess-count", cfg, "claude-sonnet-4-5-20250929")
	if err != nil {
		t.Fatalf("summarization failed: %v", err)
	}
	if result.KeptMessages != len(out) {
		t.Errorf("result reports %d kept messages, returned %d", result.KeptMessages, len(out))
	}
	// Guard the fixture: if nothing is ever dropped the assertion above holds for
	// the wrong reason and the old count would pass too.
	if len(out) >= 18 {
		t.Fatalf("fixture stopped exercising the strip: %d messages survived", len(out))
	}
}

func textOf(m anthropic.MessageParam) string {
	var parts []string
	for _, b := range m.Content {
		if b.OfText != nil {
			parts = append(parts, b.OfText.Text)
		}
	}
	return strings.Join(parts, "")
}

// messagesWithOrphanedToolResults builds a transcript whose kept tail carries
// tool_results whose tool_use lands in the summarized half, so stripOrphanedToolResults
// drops them. They replace assistant messages, so each drop leaves two user messages
// adjacent and fixAlternation merges rather than pads — the returned list really is
// shorter than the tail it was built from.
func messagesWithOrphanedToolResults(n int) []anthropic.MessageParam {
	msgs := makeMessages(n)
	for i := 41; i < n; i += 4 {
		msgs[i] = anthropic.NewUserMessage(anthropic.NewToolResultBlock("orphan-tool-id", "result", false))
	}
	return msgs
}

// stubSummaryServer answers the Messages API with one text block, so the success
// path can be exercised without a network or an API key.
func stubSummaryServer(t *testing.T, summary string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":            "msg_stub",
			"type":          "message",
			"role":          "assistant",
			"model":         "claude-sonnet-4-5-20250929",
			"content":       []map[string]any{{"type": "text", "text": summary}},
			"stop_reason":   "end_turn",
			"stop_sequence": nil,
			"usage":         map[string]any{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	t.Cleanup(server.Close)
	return server
}

func clientFor(server *httptest.Server) *anthropic.Client {
	c := anthropic.NewClient(
		option.WithBaseURL(server.URL),
		option.WithAPIKey("unused"),
		option.WithMaxRetries(0),
	)
	return &c
}
