package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// toolInfo holds tool configuration for the agent run
type toolInfo struct {
	params  []anthropic.ToolUnionParam
	toolMap map[string]Tool
}

// prepareTools builds tool params with cache control and creates tool mapping
func (a *Agent) prepareTools() *toolInfo {
	var toolParams []anthropic.ToolUnionParam
	toolMap := make(map[string]Tool)

	// Sideband fields (done, note, split) and the "then" chain go into every
	// tool's schema. Both are added by AddChainAndSidebandFields, which every
	// path that builds a request has to use, or one provider advertises the
	// fields and another does not.
	prepared := AddChainAndSidebandFields(a.tools)

	for i, t := range prepared {
		tool := &anthropic.ToolParam{
			Name:        t.Name,
			Description: anthropic.String(t.Description),
			InputSchema: t.InputSchema,
		}
		// Add cache control to the last tool definition
		if i == len(prepared)-1 {
			tool.CacheControl = anthropic.NewCacheControlEphemeralParam()
		}
		toolParams = append(toolParams, anthropic.ToolUnionParam{
			OfTool: tool,
		})
		// The map holds the tool as registered, not the copy carrying the
		// injected schema: what runs is the caller's tool, and the injected
		// fields are stripped from the input before it ever sees them.
		toolMap[t.Name] = a.tools[i]
	}

	return &toolInfo{
		params:  toolParams,
		toolMap: toolMap,
	}
}

// buildRequest creates the API request parameters
func (a *Agent) buildRequest(ctx context.Context, model string, messages *[]anthropic.MessageParam, tools *toolInfo, forceSummary bool) *anthropic.MessageNewParams {
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		Messages:  *messages,
		MaxTokens: 16384,
	}

	if len(a.systemBlocks) > 0 {
		params.System = a.systemBlocks
	} else if a.system != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: a.system},
		}
	}

	// OAuth identity injection removed (2026-04-04): Anthropic policy change
	// blocks third-party OAuth usage. Using API key auth now.

	// When force-summarizing, omit tools so the model must produce text.
	//
	// This is the only place inber withholds tools from a request, and it is
	// not free: see toolsWereWithheld below and APICallUsage.ToolsWithheld for
	// what it costs. Whether to keep buying prose this way is an open question
	// (todo 8754300f); the counters exist so it can be answered with a number.
	if !forceSummary && len(tools.params) > 0 {
		params.Tools = tools.params
	}

	if a.thinkingBudget > 0 {
		params.Thinking = anthropic.ThinkingConfigParamUnion{
			OfEnabled: &anthropic.ThinkingConfigEnabledParam{
				BudgetTokens: a.thinkingBudget,
			},
		}
	}

	// Inject volatile context (fleet status, recent files) into the last user message.
	// This keeps it AFTER BP3, preventing cache busting on the conversation prefix.
	// Only injected on the first API call of each turn (when VolatileContext is set).
	if a.VolatileContext != "" && len(*messages) > 0 {
		lastIdx := len(*messages) - 1
		if (*messages)[lastIdx].Role == anthropic.MessageParamRoleUser {
			// Insert volatile context into the last user message, AFTER any tool_result blocks.
			// Anthropic requires tool_result blocks to appear before text in a user message
			// that follows an assistant message with tool_use.
			volatileBlock := anthropic.ContentBlockParamUnion{
				OfText: &anthropic.TextBlockParam{
					Text: a.VolatileContext,
				},
			}
			content := (*messages)[lastIdx].Content
			// Find the insertion point: after the last tool_result block
			insertAt := 0
			for i, block := range content {
				if block.OfToolResult != nil {
					insertAt = i + 1
				}
			}
			// Insert at the right position
			newContent := make([]anthropic.ContentBlockParamUnion, 0, len(content)+1)
			newContent = append(newContent, content[:insertAt]...)
			newContent = append(newContent, volatileBlock)
			newContent = append(newContent, content[insertAt:]...)
			(*messages)[lastIdx].Content = newContent
			params.Messages = *messages
			// Clear so it's not re-injected on subsequent tool loop calls
			a.VolatileContext = ""
		}
	}

	// Guard against context overflow: let caller prune if needed
	if a.BeforeRequest != nil && a.contextWindow > 0 {
		pruned := a.BeforeRequest(ctx, *messages, a.contextWindow)
		if len(pruned) < len(*messages) {
			a.shiftBreakpointIndicesAfterHeadDrop(len(*messages) - len(pruned))
			*messages = pruned
			params.Messages = *messages
		}
	}

	placeHistoryCacheBreakpoints(params.Messages, a.FrozenIdx, a.turnAnchorIdx)

	if a.hooks != nil && a.hooks.OnRequest != nil {
		a.hooks.OnRequest(&params)
	}

	return &params
}

// toolsWereWithheld reports that a built request carries no tools block while
// the agent had tools to send.
//
// It reads the request that is about to go out rather than re-deriving the
// condition from forceSummary. Those are two ways of asking the same question,
// and the second would be a copy of buildRequest's `if` that nothing keeps in
// step with it: change how tools are withheld and this would keep answering
// about the old rule. Observing the outcome cannot drift from the decision.
func toolsWereWithheld(params *anthropic.MessageNewParams, tools *toolInfo) bool {
	return len(params.Tools) == 0 && len(tools.params) > 0
}

// executeAPICall handles streaming vs non-streaming API calls with retry logic.
//
// A streaming call can fail after the model has already written text, and the
// hooks have already shown that text to the user. When that happens the
// partially accumulated message comes back alongside the error, so the caller
// can keep what the user saw instead of discarding it.
func (a *Agent) executeAPICall(ctx context.Context, params *anthropic.MessageNewParams, messages *[]anthropic.MessageParam) (*anthropic.Message, error) {
	var resp *anthropic.Message
	var partial *anthropic.Message
	var apiErr error

	if a.hooks != nil && a.hooks.OnTextDelta != nil {
		// Streaming API call
		streamResp, err := a.provider.CompleteStreaming(ctx, params)
		if err != nil {
			apiErr = err
		} else {
			var accumulated anthropic.Message
			var announcedID string
			for streamResp.Next() {
				event := streamResp.Current()
				if err := accumulated.Accumulate(event); err != nil {
					continue
				}
				// message_start names the message before any of its content
				// arrives. Report it here, ahead of the first delta below, so
				// everything downstream can key this message's deltas on it.
				if accumulated.ID != "" && accumulated.ID != announcedID {
					announcedID = accumulated.ID
					a.announceMessageID(accumulated.ID)
				}
				// Emit text deltas
				if delta, ok := event.AsAny().(anthropic.ContentBlockDeltaEvent); ok {
					if textDelta, ok := delta.Delta.AsAny().(anthropic.TextDelta); ok && textDelta.Text != "" {
						a.hooks.OnTextDelta(textDelta.Text)
					}
				}
			}
			if err := streamResp.Err(); err != nil {
				apiErr = err
				partial = &accumulated
			} else {
				resp = &accumulated
			}
		}
	} else {
		// Non-streaming API call
		resp, apiErr = a.provider.Complete(ctx, params)
	}

	if apiErr != nil {
		// If we hit a context length error, try pruning and retry once
		if a.BeforeRequest != nil && a.contextWindow > 0 && isContextLengthError(apiErr) {
			pruned := a.BeforeRequest(ctx, *messages, a.contextWindow/2)
			if len(pruned) < len(*messages) {
				a.shiftBreakpointIndicesAfterHeadDrop(len(*messages) - len(pruned))
				*messages = pruned
				params.Messages = *messages
				// The breakpoints were placed against the longer slice, and the
				// messages carrying them may be the ones that just went away.
				placeHistoryCacheBreakpoints(params.Messages, a.FrozenIdx, a.turnAnchorIdx)
				resp, apiErr = a.provider.Complete(ctx, params)
			}
		}
		if apiErr != nil {
			return partial, fmt.Errorf("api call failed: %w", apiErr)
		}
	}

	return resp, nil
}

// deliveredText returns the text a response managed to deliver before it
// failed. Only text blocks are read: a tool_use block cut off mid-arguments was
// never dispatched, and carrying one into the conversation would leave a
// tool_use with no matching tool_result, which makes the next request invalid.
func deliveredText(resp *anthropic.Message) string {
	if resp == nil {
		return ""
	}
	var text strings.Builder
	for _, block := range resp.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}
	return text.String()
}

// announceMessageID reports the provider's identifier for the assistant
// message now being produced. Streaming calls it at message_start;
// processResponse calls it for responses that were not streamed. Both may
// report the same id for one message, which is why the hook is documented as
// repeatable.
func (a *Agent) announceMessageID(messageID string) {
	if messageID == "" {
		return
	}
	if a.hooks != nil && a.hooks.OnMessageID != nil {
		a.hooks.OnMessageID(messageID)
	}
}

// processResponse handles thinking blocks and text extraction, updates result stats.
//
// toolsWithheld describes the request this response answered, not the response,
// and it is carried here because this is where the call's usage is read. It is
// what tells a full-price call apart from the cheap ones it is averaged in with.
func (a *Agent) processResponse(resp *anthropic.Message, result *TurnResult, toolsWithheld bool) {
	a.announceMessageID(resp.ID)

	call := APICallUsage{
		InputTokens:         int(resp.Usage.InputTokens),
		OutputTokens:        int(resp.Usage.OutputTokens),
		CacheCreationTokens: int(resp.Usage.CacheCreationInputTokens),
		CacheReadTokens:     int(resp.Usage.CacheReadInputTokens),
		ToolsWithheld:       toolsWithheld,
	}
	result.APICalls = append(result.APICalls, call)

	result.InputTokens += call.InputTokens
	result.OutputTokens += call.OutputTokens

	// Cache tokens (prompt caching)
	if call.CacheCreationTokens > 0 {
		result.CacheCreationTokens += call.CacheCreationTokens
	}
	if call.CacheReadTokens > 0 {
		result.CacheReadTokens += call.CacheReadTokens
	}

	if a.hooks != nil && a.hooks.OnResponse != nil {
		a.hooks.OnResponse(resp)
	}

	// Extract thinking blocks
	for _, block := range resp.Content {
		if block.Type == "thinking" {
			result.Thinking += block.Thinking
			if a.hooks != nil && a.hooks.OnThinking != nil {
				a.hooks.OnThinking(block.Thinking)
			}
		}
	}
}

// executeTools processes tool calls and returns tool results and post-injections
func (a *Agent) executeTools(ctx context.Context, resp *anthropic.Message, tools *toolInfo, result *TurnResult) ([]anthropic.ContentBlockParamUnion, []string, error) {
	var toolResults []anthropic.ContentBlockParamUnion
	var postInjections []string

	for _, block := range resp.Content {
		if block.Type != "tool_use" {
			continue
		}

		result.ToolCalls++

		if a.hooks != nil && a.hooks.OnToolCall != nil {
			a.hooks.OnToolCall(block.ID, block.Name, []byte(block.Input))
		}

		// Check read cache — block redundant reads of files already in context.
		// A hit answers for the read itself and for nothing else: the same
		// tool_use block may carry sideband fields and a "then" chain, which are
		// separate instructions and still have to run, so the stub is handed to
		// the executor rather than short-circuiting the whole block.
		var cachedPrimaryOutput *string
		if a.readCache != nil {
			rawInput := string(block.Input)
			// Full re-read of a cached file? Return stub.
			if path, isFull := isFullRead(block.Name, rawInput); isFull {
				if stub, cached := a.readCache.Check(path); cached {
					cachedPrimaryOutput = &stub
				}
			}
			// Partial re-read of a cached file? Also return stub.
			if cachedPrimaryOutput == nil {
				if path, partial := isPartialRead(block.Name, rawInput); partial {
					if stub, cached := a.readCache.Check(path); cached {
						cachedPrimaryOutput = &stub
					}
				}
			}
		}

		// Execute tool with optional chain ("then" field).
		outcome, isError := executeWithChain(ctx, tools.toolMap, block.Name, string(block.Input), a.hooks, block.ID, a.sidebandCallbacks, cachedPrimaryOutput, a.ToolRefusal)

		// The primary call's cache evictions, now that the gate has answered.
		//
		// This asks outcome.primaryRan rather than isError, because "refused"
		// and "failed" are different answers and only one of them means the
		// files went untouched. A tool that ran and returned an error may have
		// written part of what it was asked to, so it still evicts — that is
		// the conservative reading this block has always taken, and the two
		// controls in refused_call_read_cache_test.go hold it in place.
		//
		// It used to run BEFORE the call, where the gate's verdict was not yet
		// available. Observe mode refuses every write, so an Observe session
		// paid to re-read every file it ever attempted to write. Asking the
		// gate a second time here is not the alternative: in Assist,
		// guard.CheckTool invokes the approval callback, so a second ask is a
		// second prompt to a human.
		//
		// isFileWrite reads the primary call's own arguments, the same ones the
		// tool was handed. It is the chained call's rule (outcome.chainInput,
		// below) applied to the primary, and the chained half has read that way
		// since it was written.
		if a.readCache != nil && outcome.primaryRan {
			for _, p := range isFileWrite(block.Name, outcome.primaryInput) {
				a.readCache.Invalidate(p)
			}
			// A call that can write files its input does not name takes the
			// whole cache with it.
			if invalidatesEverything(block.Name) {
				a.readCache.InvalidateAll()
			}
		}

		output := outcome.combined
		if isError {
			// The dispatcher reports a result it produced itself; the ones it
			// returns as errors — refused, unknown tool, the tool failed — it
			// hands back for the caller to report, so say so here or they reach
			// neither the display nor the session log. Without this the
			// transcript carries a tool_use with no tool_result, and
			// Turn.ConsecutiveErrors — incremented only in this hook — never
			// moves, which leaves the whole error-recovery context ladder
			// unreachable. Same line, same reason, as the OpenAI-served loop in
			// engine/turn_openai.go; the two paths have to agree.
			if a.hooks != nil && a.hooks.OnToolResult != nil {
				a.hooks.OnToolResult(block.ID, block.Name, output, true)
			}
			finalOutput := output
			if a.hooks != nil && a.hooks.ModifyToolResult != nil {
				if modified := a.hooks.ModifyToolResult(block.ID, block.Name, output, true); modified != "" {
					finalOutput = modified
				}
			}
			toolResults = append(toolResults, anthropic.NewToolResultBlock(block.ID, finalOutput, true))
			continue
		}

		// Record full reads in the cache. Each call is recorded from its own
		// output: reading the combined text would credit the primary read with
		// the line count printed by whatever the chain read afterwards.
		if a.readCache != nil {
			if path, isFull := isFullRead(block.Name, outcome.primaryInput); isFull {
				if lines := extractCompleteFileLines(outcome.primaryOutput); lines > 0 {
					a.readCache.RecordFullRead(path, lines)
				}
			}
			// The chained call is a real tool call and needs the same
			// bookkeeping. Without it a chained write leaves a stale entry
			// behind and a later read is answered from it.
			if outcome.chainTool != "" {
				for _, p := range isFileWrite(outcome.chainTool, outcome.chainInput) {
					a.readCache.Invalidate(p)
				}
				// Same rule for the chained call, and it has to run after the
				// primary's RecordFullRead above: `read_files(x) then
				// shell_commands(...)` records x and then the shell rewrites
				// it, so the flush must be the last word on this block.
				if invalidatesEverything(outcome.chainTool) {
					a.readCache.InvalidateAll()
				}
				if !outcome.chainFailed {
					if path, isFull := isFullRead(outcome.chainTool, outcome.chainInput); isFull {
						if lines := extractCompleteFileLines(outcome.chainOutput); lines > 0 {
							a.readCache.RecordFullRead(path, lines)
						}
					}
				}
			}
		}

		// Apply truncation/modification before adding to conversation
		finalOutput := output
		if a.hooks != nil && a.hooks.ModifyToolResult != nil {
			if modified := a.hooks.ModifyToolResult(block.ID, block.Name, output, false); modified != "" {
				finalOutput = modified
			}
		}

		toolResults = append(toolResults, anthropic.NewToolResultBlock(
			block.ID, finalOutput, false,
		))

		// Post-tool-result hook: inject build/test feedback
		if a.hooks != nil && a.hooks.PostToolResult != nil {
			if injection := a.hooks.PostToolResult(block.ID, block.Name, output, false); injection != "" {
				postInjections = append(postInjections, injection)
			}
		}
	}

	return toolResults, postInjections, nil
}