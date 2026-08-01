package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/kayushkin/inber/agent"
	memorystore "github.com/kayushkin/memory-store"
	_ "modernc.org/sqlite"
)

// conversation.SummarizeConversation decides whether the summary block may name
// the archived transcript, but it cannot know whether the model holds
// memory_expand — only the engine knows what it put on the wire. These tests
// drive summarizeIfNeeded rather than the flag, because a test that sets
// ArchiveIsRecallable by hand passes just as well when nothing ever sets it.

func stubbedSummaryEngine(t *testing.T, tools []agent.Tool) (*Engine, memorystore.MemoryStore) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":            "msg_stub",
			"type":          "message",
			"role":          "assistant",
			"model":         "claude-sonnet-4-5-20250929",
			"content":       []map[string]any{{"type": "text", "text": "the summary"}},
			"stop_reason":   "end_turn",
			"stop_sequence": nil,
			"usage":         map[string]any{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	t.Cleanup(server.Close)

	client := anthropic.NewClient(
		option.WithBaseURL(server.URL),
		option.WithAPIKey("unused"),
		option.WithMaxRetries(0),
	)

	store, err := memorystore.NewStore(t.TempDir() + "/memory.db")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	e := &Engine{
		Messages:   summarizableTranscript(40),
		Client:     &client,
		MemStore:   store,
		Model:      "claude-sonnet-4-5-20250929",
		agentTools: tools,
	}
	return e, store
}

func namedTool(name string) agent.Tool { return agent.Tool{Name: name} }

// summaryBlockOf returns the injected summary message a compaction left at the
// head of the transcript.
func summaryBlockOf(t *testing.T, e *Engine) string {
	t.Helper()
	if len(e.Messages) == 0 {
		t.Fatal("compaction left no messages at all")
	}
	var b strings.Builder
	for _, block := range e.Messages[0].Content {
		if block.OfText != nil {
			b.WriteString(block.OfText.Text)
		}
	}
	block := b.String()
	if !strings.Contains(block, "Conversation Summary") {
		t.Fatalf("the first message is not the summary block: %q", block)
	}
	return block
}

func TestCompactionPointsAtTheArchiveWhenMemoryExpandIsOnTheWire(t *testing.T) {
	e, store := stubbedSummaryEngine(t, []agent.Tool{namedTool("read_files"), namedTool(memoryExpandToolName)})

	if err := e.summarizeIfNeeded(context.Background()); err != nil {
		t.Fatalf("summarizeIfNeeded: %v", err)
	}

	block := summaryBlockOf(t, e)
	id, present := archivePointerIn(block)
	if !present {
		t.Fatalf("an agent holding memory_expand was given no way back to its archived turns:\n%s", block)
	}
	if id == "" {
		t.Fatalf("the block calls memory_expand with no id:\n%s", block)
	}
	if _, err := store.Get(id); err != nil {
		t.Errorf("the id the block names does not resolve in the store the archive was written to: %v", err)
	}
}

func TestCompactionPointsAtNothingWhenMemoryExpandIsNotOnTheWire(t *testing.T) {
	e, _ := stubbedSummaryEngine(t, []agent.Tool{namedTool("read_files")})

	if err := e.summarizeIfNeeded(context.Background()); err != nil {
		t.Fatalf("summarizeIfNeeded: %v", err)
	}

	if id, present := archivePointerIn(summaryBlockOf(t, e)); present {
		t.Errorf("block tells an agent without memory_expand to call memory_expand(id=%q)", id)
	}
}

// SetDisabledTools is the second way memory_expand leaves the wire, and it takes
// effect after the tool set was installed. A check that only reads the agent
// config would not see it.
func TestCompactionPointsAtNothingAfterMemoryExpandIsDisabled(t *testing.T) {
	e, _ := stubbedSummaryEngine(t, []agent.Tool{namedTool("read_files"), namedTool(memoryExpandToolName)})
	e.SetDisabledTools([]string{memoryExpandToolName})

	if err := e.summarizeIfNeeded(context.Background()); err != nil {
		t.Fatalf("summarizeIfNeeded: %v", err)
	}

	if id, present := archivePointerIn(summaryBlockOf(t, e)); present {
		t.Errorf("block names memory_expand(id=%q) after the tool was disabled", id)
	}
}

// archivePointerIn reports whether the block tells the model to call
// memory_expand, and with which id. Presence is answered separately from the id
// so a pointer carrying an empty id — what a half-reverted gate emits — does not
// read as no pointer at all.
func archivePointerIn(block string) (string, bool) {
	const open = `memory_expand(id="`
	i := strings.Index(block, open)
	if i < 0 {
		return "", strings.Contains(block, "memory_expand")
	}
	rest := block[i+len(open):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		return "", true
	}
	return rest[:j], true
}
