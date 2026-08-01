package conversation

import (
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// The token estimate is the only thing standing between a growing conversation
// and a request the model refuses for length. Two readers compare it against a
// budget derived from the model's context window — ShouldPrune's token branch,
// and the emergency flush in engine.pruneIfNeeded — and a third prunes with it
// after the API has already rejected a turn (agent.executeAPICall).
//
// It used to measure text blocks only, so the two largest things in an agentic
// conversation were both worth nothing: a tool result, and a tool call's input.
// These tests pin that what goes on the wire is what gets counted.

// toolOutput is the shape that actually fills a context window: a read_files or
// shell_commands result carrying tens of kilobytes of file content.
func toolOutput(bytes int) string {
	return strings.Repeat("a line of file content\n", bytes/23)
}

func TestToolResultCountsTowardTheEstimate(t *testing.T) {
	output := toolOutput(40000)
	msg := anthropic.NewUserMessage(anthropic.NewToolResultBlock("toolu_read", output, false))

	got := EstimateTokens([]anthropic.MessageParam{msg})

	// 40KB of output is on the order of ten thousand tokens. Anything near
	// zero means the walk is not looking inside the result.
	if got < 5000 {
		t.Fatalf("a %d-byte tool result estimated at %d tokens; the estimate is not counting tool output", len(output), got)
	}
}

func TestToolCallInputCountsTowardTheEstimate(t *testing.T) {
	// A write_files call carries the whole file body as its INPUT. That costs
	// the request exactly as much as the result of reading it back does.
	body := toolOutput(40000)
	msg := anthropic.NewAssistantMessage(
		anthropic.NewToolUseBlock("toolu_write", map[string]any{"path": "big.go", "content": body}, "write_files"))

	got := EstimateTokens([]anthropic.MessageParam{msg})

	if got < 5000 {
		t.Fatalf("a tool call carrying a %d-byte body estimated at %d tokens; the estimate is not counting tool inputs", len(body), got)
	}
}

func TestThinkingCountsTowardTheEstimate(t *testing.T) {
	thinking := toolOutput(20000)
	msg := anthropic.MessageParam{
		Role: anthropic.MessageParamRoleAssistant,
		Content: []anthropic.ContentBlockParamUnion{
			{OfThinking: &anthropic.ThinkingBlockParam{Thinking: thinking, Signature: "sig"}},
		},
	}

	got := EstimateTokens([]anthropic.MessageParam{msg})

	if got < 2500 {
		t.Fatalf("a %d-byte thinking block estimated at %d tokens; the estimate is not counting thinking", len(thinking), got)
	}
}

// The end of the chain, and the reason the miscount mattered: ShouldPrune's
// token branch has to be able to fire. It is only reachable when the message
// count sits between KeepRecentTurns and twice that, so this conversation is
// built to land in that band and to be heavy rather than long.
func TestShouldPruneFiresOnWeightNotJustCount(t *testing.T) {
	cfg := DefaultManagementConfig()

	var msgs []anthropic.MessageParam
	for i := 0; i < 20; i++ {
		id := "toolu_" + strings.Repeat("x", i+1)
		msgs = append(msgs, anthropic.NewAssistantMessage(
			anthropic.NewToolUseBlock(id, map[string]any{"path": "big.txt"}, "read_files")))
		msgs = append(msgs, anthropic.NewUserMessage(
			anthropic.NewToolResultBlock(id, toolOutput(20000), false)))
	}

	if len(msgs) <= cfg.KeepRecentTurns || len(msgs) > cfg.KeepRecentTurns*2 {
		t.Fatalf("test setup: %d messages is outside the band where the token branch is reachable (%d..%d)",
			len(msgs), cfg.KeepRecentTurns+1, cfg.KeepRecentTurns*2)
	}
	if got := EstimateTokens(msgs); got <= cfg.TokenBudget {
		t.Fatalf("%d messages holding 20KB of tool output each estimated at %d tokens, under the %d budget",
			len(msgs), got, cfg.TokenBudget)
	}
	if !ShouldPrune(msgs, cfg) {
		t.Fatal("a conversation over its token budget was not selected for pruning")
	}
}

// The complement, and the reason this is not simply "count more things": the
// prose walk that auto-save uses must keep ignoring tool traffic, or every
// large tool result becomes a candidate memory.
func TestProseWalkStillIgnoresToolTraffic(t *testing.T) {
	content := []anthropic.ContentBlockParamUnion{
		anthropic.NewTextBlock("I decided to use the queue's mutex as the join."),
		anthropic.NewToolResultBlock("toolu_read", toolOutput(40000), false),
		anthropic.NewToolUseBlock("toolu_write", map[string]any{"content": toolOutput(40000)}, "write_files"),
	}

	got := extractTextBlockContent(content)

	if got != "I decided to use the queue's mutex as the join." {
		t.Fatalf("the prose walk returned %d bytes; it must see text blocks only", len(got))
	}
}
