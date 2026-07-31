package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/conversation"
	memorystore "github.com/kayushkin/memory-store"
)

// emptyMemoryStore is a memory.MemoryStore that holds nothing. It exists so a
// test can put the engine on the production path (e.MemStore != nil) without a
// database: that path is the one where BuildSystemPrompt assigns
// e.Turn.VolatileContext, which is what used to overwrite the cross-zone note.
type emptyMemoryStore struct{}

func (emptyMemoryStore) Save(memorystore.Memory) error { return nil }
func (emptyMemoryStore) Get(string) (*memorystore.Memory, error) {
	return nil, nil
}
func (emptyMemoryStore) Search(string, int) ([]memorystore.Memory, error) { return nil, nil }
func (emptyMemoryStore) SearchFiltered(string, int, string) ([]memorystore.Memory, error) {
	return nil, nil
}
func (emptyMemoryStore) Forget(string) error    { return nil }
func (emptyMemoryStore) DecayImportance() error { return nil }
func (emptyMemoryStore) ListRecent(int, float64) ([]memorystore.Memory, error) {
	return nil, nil
}
func (emptyMemoryStore) Compact(time.Duration, int) ([]memorystore.CompactionResult, error) {
	return nil, nil
}
func (emptyMemoryStore) BuildContext(memorystore.BuildContextRequest) ([]memorystore.Memory, int, error) {
	return nil, 0, nil
}
func (emptyMemoryStore) PrepareSession(memorystore.PrepareSessionConfig) error { return nil }
func (emptyMemoryStore) LoadToolRegistry([]memorystore.ToolMetadata) error     { return nil }
func (emptyMemoryStore) UpdateToolUsageSummary(string, string, int64) error    { return nil }
func (emptyMemoryStore) TrackMemoryUsage(string, string, int, string) error    { return nil }
func (emptyMemoryStore) Close() error                                          { return nil }

// fileReadMessages builds the smallest conversation CrossZoneDedup reports on:
// an assistant message in the frozen zone that read a file, and an assistant
// message in the staging zone that read the same file again.
func fileReadMessages(path string) []anthropic.MessageParam {
	read := func() anthropic.MessageParam {
		return anthropic.MessageParam{
			Role: anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{
				anthropic.NewToolUseBlock("tool_1", map[string]any{"path": path}, "read_files"),
			},
		}
	}
	return []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("read it")),
		read(),
		anthropic.NewUserMessage(anthropic.NewTextBlock("read it again")),
		read(),
	}
}

// engineWithSupersededRead returns an engine whose staging zone re-reads a file
// the frozen zone already read, so pruneIfNeeded produces the cross-zone note.
func engineWithSupersededRead(path string) *Engine {
	e := &Engine{Messages: fileReadMessages(path)}
	e.staged = conversation.NewStagedConversation(conversation.DefaultPruneConfig().ManageInterval)
	e.staged.FrozenIdx = 2
	return e
}

// The note exists to tell the model that its frozen-zone reads are stale. On the
// production path — a memory store present, so BuildSystemPrompt assigns the
// volatile context — it has to survive prompt building to say anything at all.
func TestCrossZoneNoteSurvivesSystemPromptBuild(t *testing.T) {
	e := engineWithSupersededRead("/a.go")
	e.MemStore = emptyMemoryStore{}

	e.pruneIfNeeded()
	e.buildTurnContext("read it again")

	if !strings.Contains(e.Turn.VolatileContext, "/a.go") {
		t.Fatalf("cross-zone note did not reach the volatile context: %q", e.Turn.VolatileContext)
	}
	if !strings.Contains(e.Turn.VolatileContext, "re-read since last context snapshot") {
		t.Fatalf("volatile context lost the note's wording: %q", e.Turn.VolatileContext)
	}
}

// Complement: prove the fixture, not just the fix. With nothing superseded there
// is no note, so a test that passes only because every build emits the string
// would be caught here.
func TestNoCrossZoneNoteWhenNothingIsSuperseded(t *testing.T) {
	e := &Engine{Messages: fileReadMessages("/a.go")}
	e.staged = conversation.NewStagedConversation(conversation.DefaultPruneConfig().ManageInterval)
	e.staged.FrozenIdx = 0 // everything is staging; there is no frozen read to supersede
	e.MemStore = emptyMemoryStore{}

	e.pruneIfNeeded()
	e.buildTurnContext("read it again")

	if strings.Contains(e.Turn.VolatileContext, "re-read since last context snapshot") {
		t.Fatalf("note appeared with no superseded file: %q", e.Turn.VolatileContext)
	}
}

// Without a memory store BuildSystemPrompt returns early and never touches the
// volatile context, so a note appended straight onto that field is never
// cleared: turn 2 re-appends it beneath turn 1's copy.
func TestCrossZoneNoteIsNotRepeatedOnALaterTurn(t *testing.T) {
	e := engineWithSupersededRead("/a.go")

	for turn := 1; turn <= 2; turn++ {
		e.Turn.Counter = turn
		e.pruneIfNeeded()
		e.buildTurnContext("read it again")
	}

	if got := strings.Count(e.Turn.VolatileContext, "re-read since last context snapshot"); got != 1 {
		t.Fatalf("want the note once on turn 2, got %d copies: %q", got, e.Turn.VolatileContext)
	}
}

// Agent.Run injects the volatile context into the last user message of the
// engine's own message slice, so the injection is already persisted when the
// thinking-signature repair path rebuilds the agent and runs again. A rebuilt
// agent that still carried the context would inject a second copy into the same
// message.
func TestVolatileContextIsHandedToTheAgentOnlyOnce(t *testing.T) {
	e := &Engine{Messages: fileReadMessages("/a.go")}
	e.Turn.VolatileContext = "[Context]\nfleet status"

	first := e.buildAgent(nil)
	if first.VolatileContext != "[Context]\nfleet status" {
		t.Fatalf("first agent did not get the volatile context: %q", first.VolatileContext)
	}

	retry := e.buildAgent(nil)
	if retry.VolatileContext != "" {
		t.Fatalf("rebuilt agent would inject the context a second time: %q", retry.VolatileContext)
	}
}
