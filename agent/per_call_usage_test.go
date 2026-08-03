package agent

import (
	"context"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// scriptedProvider answers each call from a list, so a test can give the calls
// of one turn different usage numbers and different stop reasons.
type scriptedProvider struct {
	responses []*anthropic.Message
	// sawTools records, per call, whether the request carried a tools block.
	// The point of the feature under test is to report this, so the test reads
	// it from the wire rather than believing the agent's own answer.
	sawTools []bool
}

func (p *scriptedProvider) Complete(ctx context.Context, params *anthropic.MessageNewParams) (*anthropic.Message, error) {
	p.sawTools = append(p.sawTools, len(params.Tools) > 0)
	resp := p.responses[len(p.sawTools)-1]
	return resp, nil
}

func (p *scriptedProvider) CompleteStreaming(ctx context.Context, params *anthropic.MessageNewParams) (StreamingResponse, error) {
	return nil, context.Canceled
}

func toolCallResponse(usage anthropic.Usage) *anthropic.Message {
	return &anthropic.Message{
		StopReason: anthropic.StopReasonToolUse,
		Usage:      usage,
		Content: []anthropic.ContentBlockUnion{{
			Type: "tool_use", ID: "call", Name: "spin", Input: []byte(`{}`),
		}},
	}
}

func finalTextResponse(usage anthropic.Usage) *anthropic.Message {
	return &anthropic.Message{
		StopReason: anthropic.StopReasonEndTurn,
		Usage:      usage,
		Content:    []anthropic.ContentBlockUnion{{Type: "text", Text: "done"}},
	}
}

func agentWithOneTool(provider Provider) *Agent {
	a := New(provider, "system")
	a.tools = []Tool{{
		Name:        "spin",
		Description: "spins",
		Run:         func(ctx context.Context, input string) (string, error) { return "ok", nil },
	}}
	return a
}

func runOneTurn(t *testing.T, a *Agent) *TurnResult {
	t.Helper()
	messages := []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("go"))}
	result, err := a.Run(context.Background(), "claude-sonnet-4-5-20250929", &messages)
	if err != nil {
		t.Fatalf("turn failed: %v", err)
	}
	return result
}

// TestEveryAPICallOfATurnIsRecordedSeparately is the whole point of the change:
// a turn's four counters are a sum, and a sum cannot say which call spent what.
func TestEveryAPICallOfATurnIsRecordedSeparately(t *testing.T) {
	provider := &scriptedProvider{responses: []*anthropic.Message{
		toolCallResponse(anthropic.Usage{InputTokens: 10, OutputTokens: 1, CacheReadInputTokens: 100}),
		toolCallResponse(anthropic.Usage{InputTokens: 20, OutputTokens: 2, CacheReadInputTokens: 200}),
		finalTextResponse(anthropic.Usage{InputTokens: 30, OutputTokens: 3, CacheCreationInputTokens: 300}),
	}}
	result := runOneTurn(t, agentWithOneTool(provider))

	if got, want := len(result.APICalls), 3; got != want {
		t.Fatalf("turn made 3 API calls but recorded %d of them; per-call usage is %v", got, result.APICalls)
	}
	for i, want := range []APICallUsage{
		{InputTokens: 10, OutputTokens: 1, CacheReadTokens: 100},
		{InputTokens: 20, OutputTokens: 2, CacheReadTokens: 200},
		{InputTokens: 30, OutputTokens: 3, CacheCreationTokens: 300},
	} {
		if result.APICalls[i] != want {
			t.Errorf("call %d recorded %+v, want %+v", i+1, result.APICalls[i], want)
		}
	}
}

// TestTheTurnTotalsAreStillTheSumOfTheCalls pins that adding the per-call
// record changed nothing about the numbers every cost consumer already reads.
// The session's cost line, the guard's MaxCost comparison and model-store's
// usage rows all read the totals, so a per-call record that shifted them would
// be a billing change wearing an observability change's clothes.
func TestTheTurnTotalsAreStillTheSumOfTheCalls(t *testing.T) {
	provider := &scriptedProvider{responses: []*anthropic.Message{
		toolCallResponse(anthropic.Usage{InputTokens: 10, OutputTokens: 1, CacheReadInputTokens: 100, CacheCreationInputTokens: 7}),
		finalTextResponse(anthropic.Usage{InputTokens: 30, OutputTokens: 3, CacheCreationInputTokens: 300}),
	}}
	result := runOneTurn(t, agentWithOneTool(provider))

	var input, output, read, write int
	for _, call := range result.APICalls {
		input += call.InputTokens
		output += call.OutputTokens
		read += call.CacheReadTokens
		write += call.CacheCreationTokens
	}
	if input != result.InputTokens || output != result.OutputTokens ||
		read != result.CacheReadTokens || write != result.CacheCreationTokens {
		t.Fatalf("the per-call records do not add up to the turn totals: calls sum to "+
			"%d/%d/%d/%d, turn reports %d/%d/%d/%d (in/out/read/write)",
			input, output, read, write,
			result.InputTokens, result.OutputTokens, result.CacheReadTokens, result.CacheCreationTokens)
	}
	if result.InputTokens != 40 || result.CacheCreationTokens != 307 {
		t.Fatalf("turn totals changed: got %d input and %d cache write, want 40 and 307",
			result.InputTokens, result.CacheCreationTokens)
	}
}

// TestTheForceSummaryCallIsMarkedAsHavingWithheldItsTools is the finding this
// change exists to size. LimitCheck trips, buildRequest drops the tools block,
// and that request diverges from every cached prefix at offset 0 — so it is
// billed as a full cache write on the longest prompt of the turn. Nothing said
// which call that was.
func TestTheForceSummaryCallIsMarkedAsHavingWithheldItsTools(t *testing.T) {
	provider := &scriptedProvider{responses: []*anthropic.Message{
		toolCallResponse(anthropic.Usage{InputTokens: 10, CacheReadInputTokens: 100}),
		finalTextResponse(anthropic.Usage{InputTokens: 200000, CacheCreationInputTokens: 200000}),
	}}
	a := agentWithOneTool(provider)
	// Trip the limit on the second call, which is what forceSummary keys off.
	a.LimitCheck = func(result *TurnResult) (bool, string) { return true, "budget" }

	result := runOneTurn(t, a)

	if len(provider.sawTools) != 2 {
		t.Fatalf("expected 2 API calls, got %d", len(provider.sawTools))
	}
	if !provider.sawTools[0] {
		t.Fatalf("the first call went out with no tools block; only the force-summary call should")
	}
	if provider.sawTools[1] {
		t.Fatalf("the force-summary call still carried its tools; the fixture no longer reproduces the case")
	}

	// Length first, and fatally: indexing a short slice panics, and a panic
	// takes the whole test binary down with it, so every other test in the
	// package reports nothing and a sabotage looks like it reddened one case
	// when it reddened five.
	if len(result.APICalls) != 2 {
		t.Fatalf("turn made 2 API calls but recorded %d of them", len(result.APICalls))
	}
	if result.APICalls[0].ToolsWithheld {
		t.Errorf("the ordinary tool call was marked as having withheld its tools")
	}
	if !result.APICalls[1].ToolsWithheld {
		t.Fatalf("the force-summary call sent no tools block and was not recorded as such — " +
			"its 200k-token cache write is once again indistinguishable from the cheap call beside it")
	}
	if got := result.APICalls[1].CacheCreationTokens; got != 200000 {
		t.Errorf("force-summary call recorded %d cache-write tokens, want 200000", got)
	}
	if got := result.APICalls[1].CacheReadTokens; got != 0 {
		t.Errorf("force-summary call recorded %d cache-read tokens; a request that matches no "+
			"prefix reads nothing, so a non-zero here means the fixture is wrong", got)
	}
}

// TestAnAgentWithNoToolsNeverWithholdsThem is the mislabel this flag is easy to
// get wrong. "Sent no tools" and "had tools and did not send them" look like the
// same condition and are not: an agent that never sends tools has the same
// (empty) prefix on every call, so nothing diverges and nothing is re-bought.
// Reading the flag off an empty tools array would mark every one of its calls
// as the expensive kind and put a cost in the log that nobody is paying.
func TestAnAgentWithNoToolsNeverWithholdsThem(t *testing.T) {
	provider := &scriptedProvider{responses: []*anthropic.Message{
		finalTextResponse(anthropic.Usage{InputTokens: 10, CacheCreationInputTokens: 10}),
	}}
	a := New(provider, "system") // no tools at all

	result := runOneTurn(t, a)

	if len(result.APICalls) != 1 {
		t.Fatalf("expected 1 API call, got %d", len(result.APICalls))
	}
	if result.APICalls[0].ToolsWithheld {
		t.Fatalf("a tool-less agent's call was recorded as having withheld its tools; " +
			"the flag is reading 'no tools on the wire' instead of 'tools existed and were dropped'")
	}
}

// TestToolsWereWithheldReadsTheRequestNotTheReason guards the coupling directly.
// The condition could equally have been re-derived from forceSummary at the
// record site, which would be a second copy of buildRequest's `if` that nothing
// keeps in step with it. This asserts the answer comes from the built request,
// so a change to how tools are withheld cannot leave the reporting behind.
func TestToolsWereWithheldReadsTheRequestNotTheReason(t *testing.T) {
	withTools := &anthropic.MessageNewParams{Tools: []anthropic.ToolUnionParam{{}}}
	withoutTools := &anthropic.MessageNewParams{}
	agentHasTools := &toolInfo{params: []anthropic.ToolUnionParam{{}}}
	agentHasNone := &toolInfo{}

	if toolsWereWithheld(withTools, agentHasTools) {
		t.Errorf("a request carrying its tools was reported as withholding them")
	}
	if !toolsWereWithheld(withoutTools, agentHasTools) {
		t.Errorf("a request with no tools, from an agent that has them, was not reported as withholding them")
	}
	if toolsWereWithheld(withoutTools, agentHasNone) {
		t.Errorf("an agent with no tools was reported as withholding them")
	}
	if toolsWereWithheld(withTools, agentHasNone) {
		t.Errorf("impossible shape reported as withholding; the check should read both sides")
	}
}
