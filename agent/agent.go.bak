// Package agent implements the core agent loop: send messages to Claude,
// handle tool calls, collect results, repeat until a final text response.
package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// isContextLengthError checks if an API error is due to exceeding the model's context window.
func isContextLengthError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "prompt is too long") ||
		strings.Contains(msg, "context_length_exceeded") ||
		strings.Contains(msg, "maximum context length") ||
		strings.Contains(msg, "too many tokens")
}

// Tool defines a tool the agent can use.
type Tool struct {
	Name        string
	Description string
	InputSchema anthropic.ToolInputSchemaParam
	Run         func(ctx context.Context, input string) (string, error)
}

// Hooks allows callers to observe tool calls and results (e.g., for logging).
type Hooks struct {
	OnRequest    func(params *anthropic.MessageNewParams)                    // called before each API request
	OnResponse   func(resp *anthropic.Message)                               // called after each API response
	OnThinking   func(text string)                                           // called when thinking blocks are received
	OnTextDelta  func(text string)                                           // called for each text chunk during streaming
	OnToolCall   func(toolID, name string, input []byte)
	OnToolResult func(toolID, name, output string, isError bool)
	// ModifyToolResult is called before a tool result is added to the conversation.
	// If it returns a non-empty string, that string replaces the original output.
	// Used for truncation, filtering, or transformation of large results.
	ModifyToolResult func(toolID, name, output string, isError bool) string
	// PostToolResult is called after a tool result is collected. If it returns
	// a non-empty string, that string is appended as an extra user text block
	// in the same turn (useful for build/test feedback injection).
	PostToolResult func(toolID, name, output string, isError bool) string
}

// Agent runs the conversation loop with Claude.
type Agent struct {
	client         *anthropic.Client
	system         string
	systemBlocks   []anthropic.TextBlockParam
	tools          []Tool
	hooks          *Hooks
	agentName      string
	sessionID      string
	thinkingBudget int64 // 0 = disabled, >0 = budget tokens for extended thinking
	contextWindow  int   // max context tokens for the model (0 = no guard)

	// BeforeRequest is called before each API call with a mutable reference to
	// the messages slice. Use it to prune/compact if the conversation is too large.
	// Return the (possibly pruned) messages. Called after OnRequest hook.
	BeforeRequest func(messages []anthropic.MessageParam, contextWindow int) []anthropic.MessageParam

	// LimitCheck is called before each API call (after the first) to check
	// whether turn/token limits have been exceeded. If it returns (true, reason),
	// the agent will make one final tool-less API call asking the model to
	// summarize its progress, then return.
	LimitCheck func(result *TurnResult) (exceeded bool, reason string)

	// InjectCheck is called before each API call (after the first) to check
	// for mid-run messages from the user. Returns any pending messages to inject
	// into the conversation before the next API call.
	InjectCheck func() []string
}

// New creates an agent with the given system prompt.
func New(client *anthropic.Client, system string) *Agent {
	return &Agent{
		client: client,
		system: system,
	}
}

// NewWithSystemBlocks creates an agent with pre-built system blocks.
func NewWithSystemBlocks(client *anthropic.Client, blocks []anthropic.TextBlockParam) *Agent {
	return &Agent{
		client:       client,
		systemBlocks: blocks,
	}
}

// SetHooks attaches observation hooks for tool calls/results.
func (a *Agent) SetHooks(h *Hooks) {
	a.hooks = h
}

// SetThinking enables extended thinking with the given token budget.
// Budget must be >= 1024. Set to 0 to disable.
func (a *Agent) SetThinking(budgetTokens int64) {
	a.thinkingBudget = budgetTokens
}

// SetContextWindow sets the model's context window size for overflow protection.
func (a *Agent) SetContextWindow(tokens int) {
	a.contextWindow = tokens
}

// SetBeforeRequest sets a callback invoked before each API call to allow
// pruning messages if they're approaching the context window limit.
func (a *Agent) SetBeforeRequest(fn func(messages []anthropic.MessageParam, contextWindow int) []anthropic.MessageParam) {
	a.BeforeRequest = fn
}

// SetLimitCheck sets a callback that checks turn/token limits before each API call.
// When the callback returns (true, reason), the agent makes one final tool-less call
// asking the model to summarize progress, then returns.
func (a *Agent) SetLimitCheck(fn func(result *TurnResult) (bool, string)) {
	a.LimitCheck = fn
}

// AddTool registers a tool the agent can call.
func (a *Agent) AddTool(t Tool) {
	a.tools = append(a.tools, t)
}

// TurnResult is what comes back from a single Run call.
type TurnResult struct {
	Text         string // Final text response
	Thinking     string // Thinking/reasoning text (if extended thinking enabled)
	ToolCalls    int    // Total tool calls made during this turn
	InputTokens  int
	OutputTokens int
	// Cache tokens (Anthropic prompt caching)
	CacheCreationTokens int // tokens written to cache (first request)
	CacheReadTokens     int // tokens read from cache (subsequent requests)
}

// Run sends a conversation to Claude and loops until it gets a final text
// response (no more tool calls). It mutates the messages slice in place,
// appending assistant and tool result messages.
// The model parameter specifies which Claude model to use for this run.
func (a *Agent) Run(ctx context.Context, model string, messages *[]anthropic.MessageParam) (*TurnResult, error) {
	result := &TurnResult{}

	// Build tool params with cache control on last tool for prompt caching
	var toolParams []anthropic.ToolUnionParam
	toolMap := make(map[string]Tool)
	for i, t := range a.tools {
		tool := &anthropic.ToolParam{
			Name:        t.Name,
			Description: anthropic.String(t.Description),
			InputSchema: t.InputSchema,
		}
		// Add cache control to the last tool definition
		if i == len(a.tools)-1 {
			tool.CacheControl = anthropic.NewCacheControlEphemeralParam()
		}
		toolParams = append(toolParams, anthropic.ToolUnionParam{
			OfTool: tool,
		})
		toolMap[t.Name] = t
	}

	apiCalls := 0
	for {
		apiCalls++

		// Check context cancellation (e.g. spawn timeout).
		if ctx.Err() != nil {
			if result.Text == "" {
				result.Text = fmt.Sprintf("[Agent stopped: %s after %d API calls]", ctx.Err(), apiCalls-1)
			}
			return result, ctx.Err()
		}

		// Hard cap on API round-trips per turn to prevent runaway agents.
		const maxAPICalls = 50
		if apiCalls > maxAPICalls {
			if result.Text == "" {
				result.Text = fmt.Sprintf("[Agent stopped: exceeded %d API calls in one turn]", maxAPICalls)
			}
			return result, fmt.Errorf("exceeded max API calls (%d)", maxAPICalls)
		}

		// Check for mid-run injected messages from the user
		if apiCalls > 1 && a.InjectCheck != nil {
			if injected := a.InjectCheck(); len(injected) > 0 {
				// Append injected messages to the last user message (tool results)
				if len(*messages) > 0 {
					lastIdx := len(*messages) - 1
					for _, text := range injected {
						(*messages)[lastIdx].Content = append((*messages)[lastIdx].Content,
							anthropic.ContentBlockParamUnion{
								OfText: &anthropic.TextBlockParam{
									Text: "\n\n[New message from user while you were working]\n" + text,
								},
							},
						)
					}
				}
			}
		}

		// Check limits before each API call (after the first)
		forceSummary := false
		if apiCalls > 1 && a.LimitCheck != nil {
			if exceeded, reason := a.LimitCheck(result); exceeded {
				forceSummary = true
				// Inject limit notice into the last user message
				if len(*messages) > 0 {
					lastIdx := len(*messages) - 1
					(*messages)[lastIdx].Content = append((*messages)[lastIdx].Content,
						anthropic.ContentBlockParamUnion{
							OfText: &anthropic.TextBlockParam{
								Text: fmt.Sprintf("\n\n[BUDGET LIMIT REACHED] %s\n\n"+
									"You must stop making tool calls. Summarize your progress:\n"+
									"1. What you've accomplished so far\n"+
									"2. What remains to be done\n"+
									"3. Any issues or blockers encountered\n"+
									"Keep it concise.", reason),
							},
						},
					)
				}
			}
		}

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

		// When force-summarizing, omit tools so the model must produce text
		if !forceSummary && len(toolParams) > 0 {
			params.Tools = toolParams
		}

		if a.thinkingBudget > 0 {
			params.Thinking = anthropic.ThinkingConfigParamUnion{
				OfEnabled: &anthropic.ThinkingConfigEnabledParam{
					BudgetTokens: a.thinkingBudget,
				},
			}
		}

		// Guard against context overflow: let caller prune if needed
		if a.BeforeRequest != nil && a.contextWindow > 0 {
			pruned := a.BeforeRequest(*messages, a.contextWindow)
			if len(pruned) < len(*messages) {
				*messages = pruned
				params.Messages = *messages
			}
		}

		// Add cache breakpoint on conversation history.
		// Place it on the last content block of the second-to-last message
		// so all prior conversation is cached, and only the new user message is fresh.
		// Uses breakpoint 3 of 4 (system + tools already use 2).
		addHistoryCacheBreakpoint(params.Messages)

		if a.hooks != nil && a.hooks.OnRequest != nil {
			a.hooks.OnRequest(&params)
		}

		// Use streaming if OnTextDelta hook is set, otherwise use non-streaming.
		var resp *anthropic.Message
		var apiErr error

		if a.hooks != nil && a.hooks.OnTextDelta != nil {
			// Streaming API call
			stream := a.client.Messages.NewStreaming(ctx, params)
			var accumulated anthropic.Message
			for stream.Next() {
				event := stream.Current()
				if err := accumulated.Accumulate(event); err != nil {
					continue
				}
				// Emit text deltas
				if delta, ok := event.AsAny().(anthropic.ContentBlockDeltaEvent); ok {
					if textDelta, ok := delta.Delta.AsAny().(anthropic.TextDelta); ok && textDelta.Text != "" {
						a.hooks.OnTextDelta(textDelta.Text)
					}
				}
			}
			if err := stream.Err(); err != nil {
				apiErr = err
			} else {
				resp = &accumulated
			}
			stream.Close()
		} else {
			// Non-streaming API call
			resp, apiErr = a.client.Messages.New(ctx, params)
		}

		if apiErr != nil {
			// If we hit a context length error, try pruning and retry once
			if a.BeforeRequest != nil && a.contextWindow > 0 && isContextLengthError(apiErr) {
				pruned := a.BeforeRequest(*messages, a.contextWindow/2)
				if len(pruned) < len(*messages) {
					*messages = pruned
					params.Messages = *messages
					resp, apiErr = a.client.Messages.New(ctx, params)
				}
			}
			if apiErr != nil {
				return result, fmt.Errorf("api call failed: %w", apiErr)
			}
		}

		result.InputTokens += int(resp.Usage.InputTokens)
		result.OutputTokens += int(resp.Usage.OutputTokens)
		// Cache tokens (prompt caching)
		if resp.Usage.CacheCreationInputTokens > 0 {
			result.CacheCreationTokens += int(resp.Usage.CacheCreationInputTokens)
		}
		if resp.Usage.CacheReadInputTokens > 0 {
			result.CacheReadTokens += int(resp.Usage.CacheReadInputTokens)
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

		// Append assistant message
		*messages = append(*messages, resp.ToParam())

		// If stop reason is "end" or "max_tokens", extract text and return
		if resp.StopReason == anthropic.StopReasonEndTurn || resp.StopReason == anthropic.StopReasonMaxTokens {
			for _, block := range resp.Content {
				if block.Type == "text" {
					result.Text += block.Text
				}
			}
			return result, nil
		}

		// If stop reason is "tool_use", execute tools and continue
		if resp.StopReason == anthropic.StopReasonToolUse {
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

				tool, ok := toolMap[block.Name]
				if !ok {
					errMsg := fmt.Sprintf("error: unknown tool %q", block.Name)
					if a.hooks != nil && a.hooks.OnToolResult != nil {
						a.hooks.OnToolResult(block.ID, block.Name, errMsg, true)
					}
					finalErrMsg := errMsg
					if a.hooks != nil && a.hooks.ModifyToolResult != nil {
						if modified := a.hooks.ModifyToolResult(block.ID, block.Name, errMsg, true); modified != "" {
							finalErrMsg = modified
						}
					}
					toolResults = append(toolResults, anthropic.NewToolResultBlock(
						block.ID, finalErrMsg, true,
					))
					continue
				}

				output, err := tool.Run(ctx, string(block.Input))
				if err != nil {
					errMsg := fmt.Sprintf("error: %s", err)
					if a.hooks != nil && a.hooks.OnToolResult != nil {
						a.hooks.OnToolResult(block.ID, block.Name, errMsg, true)
					}
					finalErrMsg := errMsg
					if a.hooks != nil && a.hooks.ModifyToolResult != nil {
						if modified := a.hooks.ModifyToolResult(block.ID, block.Name, errMsg, true); modified != "" {
							finalErrMsg = modified
						}
					}
					toolResults = append(toolResults, anthropic.NewToolResultBlock(
						block.ID, finalErrMsg, true,
					))
					continue
				}

				if a.hooks != nil && a.hooks.OnToolResult != nil {
					a.hooks.OnToolResult(block.ID, block.Name, output, false)
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

			if len(postInjections) > 0 {
				toolResults = append(toolResults, anthropic.NewTextBlock(
					"[system: post-write hook]\n"+strings.Join(postInjections, "\n"),
				))
			}
			*messages = append(*messages, anthropic.NewUserMessage(toolResults...))
			continue
		}

		// Unexpected stop reason
		return result, fmt.Errorf("unexpected stop reason: %s", resp.StopReason)
	}
}

// addHistoryCacheBreakpoint places a cache_control breakpoint on the last content
// block of the second-to-last message. This caches the entire conversation history
// prefix so only the new user message is uncached input.
// First clears any existing history breakpoints to avoid exceeding the 4-block limit.
func addHistoryCacheBreakpoint(messages []anthropic.MessageParam) {
	if len(messages) < 2 {
		return
	}
	// Clear existing cache_control from all message content blocks.
	// System blocks and tools manage their own breakpoints separately.
	var zero anthropic.CacheControlEphemeralParam
	for i := range messages {
		for j := range messages[i].Content {
			b := &messages[i].Content[j]
			if b.OfText != nil {
				b.OfText.CacheControl = zero
			} else if b.OfToolUse != nil {
				b.OfToolUse.CacheControl = zero
			} else if b.OfToolResult != nil {
				b.OfToolResult.CacheControl = zero
			}
		}
	}
	// Target the second-to-last message's last content block.
	msg := &messages[len(messages)-2]
	if len(msg.Content) == 0 {
		return
	}
	last := &msg.Content[len(msg.Content)-1]
	cc := anthropic.NewCacheControlEphemeralParam()
	if last.OfText != nil {
		last.OfText.CacheControl = cc
	} else if last.OfToolUse != nil {
		last.OfToolUse.CacheControl = cc
	} else if last.OfToolResult != nil {
		last.OfToolResult.CacheControl = cc
	}
}
