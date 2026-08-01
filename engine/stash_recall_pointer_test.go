package engine

import (
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/conversation"
	"github.com/kayushkin/inber/memory"
	memorystore "github.com/kayushkin/memory-store"
	_ "modernc.org/sqlite"
)

// The stash is the other writer that takes content out of a conversation, and it
// leaves a pointer naming the tool that brings it back. Only the engine knows
// which tools it put on the wire, so these tests drive stashAssistantResponse
// rather than the config field: a test that fills RecallToolNames by hand passes
// just as well when nothing ever fills it.

func stashingEngine(t *testing.T, tools []agent.Tool) *Engine {
	t.Helper()

	store, err := memorystore.NewStore(t.TempDir() + "/memory.db")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	stashCfg := conversation.DefaultStashConfig()
	stashCfg.MinBlockSize = 50
	stashCfg.AssistantThreshold = 50

	e := &Engine{
		MemStore:   store,
		agentTools: tools,
		stashCfg:   stashCfg,
	}
	e.Messages = []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("go on then")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock(stashableAssistantText())),
	}
	return e
}

// stashableAssistantText is a fenced code block big enough to clear both the
// assistant threshold and the minimum block size.
func stashableAssistantText() string {
	return "Here is the file:\n```go\n" +
		strings.Repeat("func handler() { /* a block well over the minimum */ }\n", 200) +
		"```\nThat is all of it.\n"
}

// assistantTextAfterStashing returns the text the model will see next turn.
func assistantTextAfterStashing(t *testing.T, e *Engine) string {
	t.Helper()
	last := e.Messages[len(e.Messages)-1]
	var b strings.Builder
	for _, block := range last.Content {
		if block.OfText != nil {
			b.WriteString(block.OfText.Text)
		}
	}
	return b.String()
}

func TestStashPointerNamesMemoryExpandWhenItIsOnTheWire(t *testing.T) {
	e := stashingEngine(t, []agent.Tool{namedTool("read_files"), namedTool(memory.ToolNameMemoryExpand)})

	e.stashAssistantResponse("sess", &agent.TurnResult{Text: stashableAssistantText()})

	text := assistantTextAfterStashing(t, e)
	if !strings.Contains(text, "Large content stashed") {
		t.Fatalf("nothing was stashed out of a message well over the threshold:\n%s", text)
	}
	id, present := archivePointerIn(text)
	if !present {
		t.Fatalf("an agent holding memory_expand was given no way back to its stashed block:\n%s", text)
	}
	if id == "" {
		t.Fatalf("the pointer calls memory_expand with no id:\n%s", text)
	}
	if _, err := e.MemStore.Get(id); err != nil {
		t.Errorf("the id the pointer names does not resolve in the store the block was written to: %v", err)
	}
}

// An agent with memory_search and no memory_expand is the commonest shape on
// this host. Its block may still be stashed — search reaches it — but the
// pointer must not name a tool the agent cannot call.
func TestStashPointerNamesOnlySearchWhenExpandIsNotOnTheWire(t *testing.T) {
	e := stashingEngine(t, []agent.Tool{namedTool("read_files"), namedTool(memory.ToolNameMemorySearch)})

	e.stashAssistantResponse("sess", &agent.TurnResult{Text: stashableAssistantText()})

	text := assistantTextAfterStashing(t, e)
	if !strings.Contains(text, "Large content stashed") {
		t.Fatalf("nothing was stashed for an agent that can still search for it:\n%s", text)
	}
	if !strings.Contains(text, memory.ToolNameMemorySearch) {
		t.Errorf("the pointer does not name the one recall tool the agent holds:\n%s", text)
	}
	if _, present := archivePointerIn(text); present {
		t.Errorf("the pointer names memory_expand to an agent that does not hold it:\n%s", text)
	}
}

// With no recall tool at all, stashing is a deletion: the block leaves the
// message and nothing can ask for it back. The engine must leave the message
// alone instead.
func TestNothingIsStashedWhenNoRecallToolIsOnTheWire(t *testing.T) {
	e := stashingEngine(t, []agent.Tool{namedTool("read_files"), namedTool("list_files")})
	before := assistantTextAfterStashing(t, e)

	e.stashAssistantResponse("sess", &agent.TurnResult{Text: stashableAssistantText()})

	after := assistantTextAfterStashing(t, e)
	if strings.Contains(after, "Large content stashed") {
		t.Errorf("stashed a block an agent with no memory tools can never recall:\n%s", after)
	}
	if after != before {
		t.Error("the assistant message was rewritten even though nothing was stashed")
	}
}

// SetDisabledTools is the second way a recall tool leaves the wire, and it takes
// effect after the tool set was installed. A stash config captured when the
// engine was built would not see it.
func TestNothingIsStashedAfterEveryRecallToolIsDisabled(t *testing.T) {
	e := stashingEngine(t, []agent.Tool{
		namedTool("read_files"),
		namedTool(memory.ToolNameMemorySearch),
		namedTool(memory.ToolNameMemoryExpand),
	})
	e.SetDisabledTools([]string{memory.ToolNameMemorySearch, memory.ToolNameMemoryExpand})

	e.stashAssistantResponse("sess", &agent.TurnResult{Text: stashableAssistantText()})

	if text := assistantTextAfterStashing(t, e); strings.Contains(text, "Large content stashed") {
		t.Errorf("stashed a block after both recall tools were disabled:\n%s", text)
	}
}

// The user-message path is a separate call site, and it has to read the wire set
// the same way. Asserted in the positive: a negative-only test passes whenever
// the config carries no recall tools, which is exactly what a call site that
// forgot to fill them looks like.
func TestUserMessageIsStashedWithAPointerToTheToolOnTheWire(t *testing.T) {
	e := stashingEngine(t, []agent.Tool{namedTool("read_files"), namedTool(memory.ToolNameMemoryExpand)})
	e.Messages = nil
	e.stashCfg.UserMessageThreshold = 50

	processed := e.prepareInput(t.Context(), stashableAssistantText(), "sess")

	if !strings.Contains(processed, "Large content stashed") {
		t.Fatalf("nothing was stashed out of a user message well over the threshold:\n%s", processed)
	}
	id, present := archivePointerIn(processed)
	if !present {
		t.Fatalf("the user-message stash left no way back for an agent holding memory_expand:\n%s", processed)
	}
	if _, err := e.MemStore.Get(id); err != nil {
		t.Errorf("the id the pointer names does not resolve in the store the block was written to: %v", err)
	}
}

func TestUserMessageIsNotStashedWhenNoRecallToolIsOnTheWire(t *testing.T) {
	e := stashingEngine(t, []agent.Tool{namedTool("read_files")})
	e.Messages = nil
	e.stashCfg.UserMessageThreshold = 50

	input := stashableAssistantText()
	processed := e.prepareInput(t.Context(), input, "sess")

	if strings.Contains(processed, "Large content stashed") {
		t.Errorf("stashed a user block an agent with no memory tools can never recall:\n%s", processed)
	}
	if processed != input {
		t.Error("the user message was rewritten even though nothing was stashed")
	}
}
