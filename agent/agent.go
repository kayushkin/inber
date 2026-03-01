// Package agent implements the core agent loop: send messages to Claude,
// handle tool calls, collect results, repeat until a final text response.
package agent

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
)

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
	OnToolCall   func(toolID, name string, input []byte)
	OnToolResult func(toolID, name, output string, isError bool)
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
}

// Run sends a conversation to Claude and loops until it gets a final text
// response (no more tool calls). It mutates the messages slice in place,
// appending assistant and tool result messages.
// The model parameter specifies which Claude model to use for this run.
func (a *Agent) Run(ctx context.Context, model string, messages *[]anthropic.MessageParam) (*TurnResult, error) {
	result := &TurnResult{}

	// Build tool params
	var toolParams []anthropic.ToolUnionParam
	toolMap := make(map[string]Tool)
	for _, t := range a.tools {
		toolParams = append(toolParams, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        t.Name,
				Description: anthropic.String(t.Description),
				InputSchema: t.InputSchema,
			},
		})
		toolMap[t.Name] = t
	}

	for {
		params := anthropic.MessageNewParams{
			Model:     anthropic.Model(model),
			Messages:  *messages,
			MaxTokens: 8192,
		}

		if len(a.systemBlocks) > 0 {
			params.System = a.systemBlocks
		} else if a.system != "" {
			params.System = []anthropic.TextBlockParam{
				{Text: a.system},
			}
		}

		if len(toolParams) > 0 {
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

		if a.hooks != nil && a.hooks.OnRequest != nil {
			a.hooks.OnRequest(&params)
		}

		resp, err := a.client.Messages.New(ctx, params)
		if err != nil {
			return result, fmt.Errorf("api call failed: %w", err)
		}

		result.InputTokens += int(resp.Usage.InputTokens)
		result.OutputTokens += int(resp.Usage.OutputTokens)

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

		// If stop reason is "end", extract text and return
		if resp.StopReason == anthropic.StopReasonEndTurn {
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
					toolResults = append(toolResults, anthropic.NewToolResultBlock(
						block.ID, errMsg, true,
					))
					continue
				}

				output, err := tool.Run(ctx, string(block.Input))
				if err != nil {
					errMsg := fmt.Sprintf("error: %s", err)
					if a.hooks != nil && a.hooks.OnToolResult != nil {
						a.hooks.OnToolResult(block.ID, block.Name, errMsg, true)
					}
					toolResults = append(toolResults, anthropic.NewToolResultBlock(
						block.ID, errMsg, true,
					))
					continue
				}

				if a.hooks != nil && a.hooks.OnToolResult != nil {
					a.hooks.OnToolResult(block.ID, block.Name, output, false)
				}
				toolResults = append(toolResults, anthropic.NewToolResultBlock(
					block.ID, output, false,
				))
			}

			*messages = append(*messages, anthropic.NewUserMessage(toolResults...))
			continue
		}

		// Unexpected stop reason
		return result, fmt.Errorf("unexpected stop reason: %s", resp.StopReason)
	}
}
