// Package agent implements the core agent loop: send messages to Claude,
// handle tool calls, collect results, repeat until a final text response.
package agent

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
)

// ModelInfo describes a Claude model with metadata for tracking.
type ModelInfo struct {
	ID              string  // e.g., "claude-sonnet-4-5-20250929"
	ContextWindow   int     // max tokens
	InputCostPer1M  float64 // cost per 1M input tokens
	OutputCostPer1M float64 // cost per 1M output tokens
}

// Models is a registry of known Claude models.
var Models = map[string]ModelInfo{
	"claude-sonnet-4-5-20250929": {
		ID:              "claude-sonnet-4-5-20250929",
		ContextWindow:   200000,
		InputCostPer1M:  3.00,
		OutputCostPer1M: 15.00,
	},
	"claude-sonnet-4-6": {
		ID:              "claude-sonnet-4-6",
		ContextWindow:   200000,
		InputCostPer1M:  3.00,
		OutputCostPer1M: 15.00,
	},
	"claude-opus-4-6": {
		ID:              "claude-opus-4-6",
		ContextWindow:   200000,
		InputCostPer1M:  15.00,
		OutputCostPer1M: 75.00,
	},
}

// Tool defines a tool the agent can use.
type Tool struct {
	Name        string
	Description string
	InputSchema anthropic.ToolInputSchemaParam
	Run         func(ctx context.Context, input string) (string, error)
}

// Agent runs the conversation loop with Claude.
type Agent struct {
	client *anthropic.Client
	system string
	tools  []Tool
}

// New creates an agent with the given system prompt.
func New(client *anthropic.Client, system string) *Agent {
	return &Agent{
		client: client,
		system: system,
	}
}

// AddTool registers a tool the agent can call.
func (a *Agent) AddTool(t Tool) {
	a.tools = append(a.tools, t)
}

// TurnResult is what comes back from a single Run call.
type TurnResult struct {
	Text       string // Final text response
	ToolCalls  int    // Total tool calls made during this turn
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

		if a.system != "" {
			params.System = []anthropic.TextBlockParam{
				{Text: a.system},
			}
		}

		if len(toolParams) > 0 {
			params.Tools = toolParams
		}

		resp, err := a.client.Messages.New(ctx, params)
		if err != nil {
			return result, fmt.Errorf("api call failed: %w", err)
		}

		result.InputTokens += int(resp.Usage.InputTokens)
		result.OutputTokens += int(resp.Usage.OutputTokens)

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

				tool, ok := toolMap[block.Name]
				if !ok {
					toolResults = append(toolResults, anthropic.NewToolResultBlock(
						block.ID, fmt.Sprintf("error: unknown tool %q", block.Name), true,
					))
					continue
				}

				output, err := tool.Run(ctx, string(block.Input))
				if err != nil {
					toolResults = append(toolResults, anthropic.NewToolResultBlock(
						block.ID, fmt.Sprintf("error: %s", err), true,
					))
					continue
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
