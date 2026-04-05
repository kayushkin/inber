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
	provider       Provider
	system         string
	systemBlocks   []anthropic.TextBlockParam
	tools          []Tool
	hooks          *Hooks
	agentName      string
	sessionID      string
	thinkingBudget int64 // 0 = disabled, >0 = budget tokens for extended thinking
	contextWindow  int   // max context tokens for the model (0 = no guard)
	isOAuth        bool  // true = Claude Max OAuth token; requires Claude Code identity in system prompt

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

	// VolatileContext is per-turn context (fleet status, recent files) that
	// gets prepended to the last user message instead of being in system blocks.
	// This prevents cache busting: system blocks stay stable → BP2 always hits.
	VolatileContext string

	// FrozenIdx is the message index where the frozen/staging boundary is.
	// BP3 is placed here instead of second-to-last message.
	// Messages before FrozenIdx are frozen (cached), after are staging (uncached).
	// 0 means no frozen zone → fall back to default BP3 placement.
	FrozenIdx int
}

// New creates an agent with the given system prompt.
func New(provider Provider, system string) *Agent {
	return &Agent{
		provider: provider,
		system:   system,
	}
}

// NewWithSystemBlocks creates an agent with pre-built system blocks.
func NewWithSystemBlocks(provider Provider, blocks []anthropic.TextBlockParam) *Agent {
	return &Agent{
		provider:     provider,
		systemBlocks: blocks,
	}
}

// NewWithClient creates an agent with an Anthropic client (backward compatibility).
func NewWithClient(client *anthropic.Client, system string) *Agent {
	return New(NewAnthropicProvider(client), system)
}

// NewWithClientAndSystemBlocks creates an agent with client and system blocks (backward compatibility).
func NewWithClientAndSystemBlocks(client *anthropic.Client, blocks []anthropic.TextBlockParam) *Agent {
	return NewWithSystemBlocks(NewAnthropicProvider(client), blocks)
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

// SetOAuth marks this agent as using a Claude Max OAuth token.
// When set, the system prompt will be prefixed with Claude Code identity
// (required for Claude 4 model access via Max subscription).
func (a *Agent) SetOAuth(isOAuth bool) {
	a.isOAuth = isOAuth
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

// GetTools returns a copy of the agent's tools slice.
func (a *Agent) GetTools() []Tool {
	tools := make([]Tool, len(a.tools))
	copy(tools, a.tools)
	return tools
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
	
	// Prepare tools and mapping
	tools := a.prepareTools()

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

		// Build request parameters
		params := a.buildRequest(ctx, model, messages, tools, forceSummary)

		// Execute API call with retry logic
		resp, err := a.executeAPICall(ctx, params, messages)
		if err != nil {
			return result, err
		}

		// Process response and update result stats
		a.processResponse(resp, result)

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
			toolResults, postInjections, err := a.executeTools(ctx, resp, tools, result)
			if err != nil {
				return result, err
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

// addHistoryCacheBreakpoint places a cache_control breakpoint at the frozen/staging
// boundary. Messages before the boundary are frozen (cached), after are staging (uncached).
//
// If frozenIdx > 0: BP3 goes on the last content block of message[frozenIdx-1].
// If frozenIdx == 0: falls back to second-to-last message (legacy behavior).
//
// First clears any existing history breakpoints to avoid exceeding the 4-block limit.
func addHistoryCacheBreakpoint(messages []anthropic.MessageParam, frozenIdx int) {
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

	// Determine BP3 target index
	targetIdx := len(messages) - 2 // default: second-to-last
	if frozenIdx > 0 && frozenIdx <= len(messages) {
		targetIdx = frozenIdx - 1 // last frozen message
	}
	if targetIdx < 0 || targetIdx >= len(messages) {
		return
	}

	msg := &messages[targetIdx]
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
