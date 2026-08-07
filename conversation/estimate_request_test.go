package conversation

import (
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// bigToolResult builds a user message carrying one tool_result of roughly
// size bytes — the shape of every read_files or shell_commands answer in an
// agentic conversation, and the shape a text-block walk cannot see.
func bigToolResult(size int) anthropic.MessageParam {
	body := strings.Repeat("x", size)
	return anthropic.MessageParam{
		Role: anthropic.MessageParamRoleUser,
		Content: []anthropic.ContentBlockParamUnion{
			{OfToolResult: &anthropic.ToolResultBlockParam{
				ToolUseID: "call_1",
				Content: []anthropic.ToolResultBlockParamContentUnion{
					{OfText: &anthropic.TextBlockParam{Text: body}},
				},
			}},
		},
	}
}

func systemBlocks(sizes ...int) []anthropic.TextBlockParam {
	blocks := make([]anthropic.TextBlockParam, 0, len(sizes))
	for _, n := range sizes {
		blocks = append(blocks, anthropic.TextBlockParam{Text: strings.Repeat("s", n)})
	}
	return blocks
}

func toolsBlock(schemaSize int) []anthropic.ToolUnionParam {
	return []anthropic.ToolUnionParam{
		{OfTool: &anthropic.ToolParam{
			Name:        "read_files",
			Description: anthropic.String("Read files from disk"),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: map[string]any{
					"paths": map[string]any{
						"type":        "string",
						"description": strings.Repeat("d", schemaSize),
					},
				},
			},
		}},
	}
}

// A system prompt is several thousand tokens that no message-list estimator
// can see. Pin that EstimateSystemTokens scales with the text rather than
// answering a constant.
func TestSystemTokensScaleWithTheText(t *testing.T) {
	small := EstimateSystemTokens(systemBlocks(300))
	large := EstimateSystemTokens(systemBlocks(30000))

	if small == 0 {
		t.Fatal("a 300-byte system prompt priced at 0 tokens")
	}
	if large < small*50 {
		t.Errorf("a 100x larger system prompt priced at %d vs %d — not scaling with its text", large, small)
	}
}

// The blocks of a system prompt must sum to the whole, or the per-block table
// in prompts/system.md and its own Total row disagree.
func TestSystemBlocksSumToTheWholeSystemPrompt(t *testing.T) {
	blocks := systemBlocks(500, 1500, 40)

	sum := 0
	for _, b := range blocks {
		sum += EstimateSystemBlockTokens(b)
	}
	if whole := EstimateSystemTokens(blocks); whole != sum {
		t.Errorf("blocks sum to %d, whole prices at %d", sum, whole)
	}
}

// A tool is priced by marshalling it, so its schema counts. A flat
// per-parameter constant would answer the same for both of these.
func TestToolTokensReadTheWholeSchema(t *testing.T) {
	small := EstimateToolsTokens(toolsBlock(20))
	large := EstimateToolsTokens(toolsBlock(20000))

	if small == 0 {
		t.Fatal("a tool definition priced at 0 tokens")
	}
	if large <= small*10 {
		t.Errorf("a tool with a 1000x larger schema priced at %d vs %d — the schema is not being read", large, small)
	}
}

// The defect that made this whole estimator necessary: the largest thing in
// an agentic conversation is a tool result, and any walk that charges it a
// flat constant is wrong by orders of magnitude.
func TestARequestIsPricedByWhatItCarries(t *testing.T) {
	params := &anthropic.MessageNewParams{
		Messages: []anthropic.MessageParam{bigToolResult(20000)},
	}

	got := EstimateRequestTokens(params)
	// 20,000 bytes at memory-store's ~3 chars/token is ~6,600 tokens. Anything
	// in the low hundreds means the tool result was charged a constant.
	if got < 5000 {
		t.Errorf("a request carrying a 20KB tool result priced at %d tokens — the result is not being counted", got)
	}
}

// EstimateRequestTokens' whole reason to exist: it sees the three sections a
// message-list estimator cannot add up. Each section is verified to move the
// total on its own, so a total that silently dropped one would redden here.
func TestEveryRequestSectionMovesTheTotal(t *testing.T) {
	messages := []anthropic.MessageParam{bigToolResult(4000)}

	base := &anthropic.MessageNewParams{Messages: messages}
	baseTokens := EstimateRequestTokens(base)

	withSystem := &anthropic.MessageNewParams{Messages: messages, System: systemBlocks(9000)}
	withTools := &anthropic.MessageNewParams{Messages: messages, Tools: toolsBlock(9000)}

	if got := EstimateRequestTokens(withSystem); got <= baseTokens {
		t.Errorf("adding a 9KB system prompt did not raise the total: %d vs %d", got, baseTokens)
	}
	if got := EstimateRequestTokens(withTools); got <= baseTokens {
		t.Errorf("adding a 9KB tools block did not raise the total: %d vs %d", got, baseTokens)
	}

	// And the whole is exactly its parts, so a caller can attribute the total.
	full := &anthropic.MessageNewParams{Messages: messages, System: systemBlocks(9000), Tools: toolsBlock(9000)}
	parts := EstimateSystemTokens(full.System) + EstimateToolsTokens(full.Tools) + EstimateTokens(full.Messages)
	if whole := EstimateRequestTokens(full); whole != parts {
		t.Errorf("request prices at %d, its three sections sum to %d", whole, parts)
	}
}

// MaxTokens and the thinking budget bound the model's OUTPUT. Counting them
// here would price a request for tokens that are not in it, and would make
// every gate that ever compares this to a context window fire early for a
// reason nobody wrote down.
func TestOutputBudgetsAreNotPartOfARequestsSize(t *testing.T) {
	messages := []anthropic.MessageParam{bigToolResult(2000)}

	plain := &anthropic.MessageNewParams{Messages: messages}
	budgeted := &anthropic.MessageNewParams{
		Messages:  messages,
		MaxTokens: 64000,
		Thinking: anthropic.ThinkingConfigParamUnion{
			OfEnabled: &anthropic.ThinkingConfigEnabledParam{BudgetTokens: 32000},
		},
	}

	if EstimateRequestTokens(plain) != EstimateRequestTokens(budgeted) {
		t.Errorf("output budgets changed the priced size of a request: %d vs %d",
			EstimateRequestTokens(plain), EstimateRequestTokens(budgeted))
	}
}

// A nil request is priced at nothing rather than panicking. The gates that
// will eventually call this run on paths where a request may not have been
// built yet.
func TestNilRequestPricesAtZero(t *testing.T) {
	if got := EstimateRequestTokens(nil); got != 0 {
		t.Errorf("nil request priced at %d, want 0", got)
	}
}

// The per-message estimator exported for other packages must be the same one
// the prune gates weigh. If these ever diverge, a breakdown a human reads
// stops matching the number that decides whether their conversation is pruned.
func TestExportedPerMessageEstimatorMatchesTheGates(t *testing.T) {
	messages := []anthropic.MessageParam{
		bigToolResult(3000),
		{Role: anthropic.MessageParamRoleAssistant, Content: []anthropic.ContentBlockParamUnion{
			{OfText: &anthropic.TextBlockParam{Text: strings.Repeat("t", 900)}},
		}},
	}

	sum := 0
	for _, m := range messages {
		sum += EstimateMessageTokens(m)
	}
	if whole := EstimateTokens(messages); whole != sum {
		t.Errorf("EstimateMessageTokens sums to %d, EstimateTokens answers %d", sum, whole)
	}
}
