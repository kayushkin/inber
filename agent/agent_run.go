package agent

import (
	"context"
	"fmt"

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
	
	// Build tool name enum for the "then" chain field.
	toolNames := make([]string, 0, len(a.tools))
	for _, t := range a.tools {
		toolNames = append(toolNames, t.Name)
	}
	thenSchema := buildThenSchema(toolNames)
	sbSchema := sidebandSchema()

	for i, t := range a.tools {
		// Inject sideband fields (done, note, split) and "then" chain into every tool's schema.
		schema := t.InputSchema
		schema = injectFields(schema, sbSchema)
		if t.Name != "end_turn" {
			schema = injectChainField(schema, thenSchema)
		}
		tool := &anthropic.ToolParam{
			Name:        t.Name,
			Description: anthropic.String(t.Description),
			InputSchema: schema,
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

	// When force-summarizing, omit tools so the model must produce text
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
			// Prepend volatile context to the last user message
			volatileBlock := anthropic.ContentBlockParamUnion{
				OfText: &anthropic.TextBlockParam{
					Text: a.VolatileContext,
				},
			}
			(*messages)[lastIdx].Content = append(
				[]anthropic.ContentBlockParamUnion{volatileBlock},
				(*messages)[lastIdx].Content...,
			)
			params.Messages = *messages
			// Clear so it's not re-injected on subsequent tool loop calls
			a.VolatileContext = ""
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

	// Add cache breakpoint at frozen/staging boundary
	addHistoryCacheBreakpoint(params.Messages, a.FrozenIdx)

	if a.hooks != nil && a.hooks.OnRequest != nil {
		a.hooks.OnRequest(&params)
	}

	return &params
}

// executeAPICall handles streaming vs non-streaming API calls with retry logic
func (a *Agent) executeAPICall(ctx context.Context, params *anthropic.MessageNewParams, messages *[]anthropic.MessageParam) (*anthropic.Message, error) {
	var resp *anthropic.Message
	var apiErr error

	if a.hooks != nil && a.hooks.OnTextDelta != nil {
		// Streaming API call
		streamResp, err := a.provider.CompleteStreaming(ctx, params)
		if err != nil {
			apiErr = err
		} else {
			var accumulated anthropic.Message
			for streamResp.Next() {
				event := streamResp.Current()
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
			if err := streamResp.Err(); err != nil {
				apiErr = err
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
			pruned := a.BeforeRequest(*messages, a.contextWindow/2)
			if len(pruned) < len(*messages) {
				*messages = pruned
				params.Messages = *messages
				resp, apiErr = a.provider.Complete(ctx, params)
			}
		}
		if apiErr != nil {
			return nil, fmt.Errorf("api call failed: %w", apiErr)
		}
	}

	return resp, nil
}

// processResponse handles thinking blocks and text extraction, updates result stats
func (a *Agent) processResponse(resp *anthropic.Message, result *TurnResult) {
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
		if a.readCache != nil {
			rawInput := string(block.Input)
			// Full re-read of a cached file? Return stub.
			if path, isFull := isFullRead(block.Name, rawInput); isFull {
				if stub, cached := a.readCache.Check(path); cached {
					if a.hooks != nil && a.hooks.OnToolResult != nil {
						a.hooks.OnToolResult(block.ID, block.Name, stub, false)
					}
					toolResults = append(toolResults, anthropic.NewToolResultBlock(block.ID, stub, false))
					continue
				}
			}
			// Partial re-read of a cached file? Also return stub.
			if path, partial := isPartialRead(block.Name, rawInput); partial {
				if stub, cached := a.readCache.Check(path); cached {
					if a.hooks != nil && a.hooks.OnToolResult != nil {
						a.hooks.OnToolResult(block.ID, block.Name, stub, false)
					}
					toolResults = append(toolResults, anthropic.NewToolResultBlock(block.ID, stub, false))
					continue
				}
			}
			// Write/edit invalidates the cache for that file.
			if paths := isFileWrite(block.Name, rawInput); len(paths) > 0 {
				for _, p := range paths {
					a.readCache.Invalidate(p)
				}
			}
		}

		// Execute tool with optional chain ("then" field).
		output, isError := executeWithChain(ctx, tools.toolMap, block.Name, string(block.Input), a.hooks, block.ID, a.sidebandCallbacks)
		if isError {
			finalOutput := output
			if a.hooks != nil && a.hooks.ModifyToolResult != nil {
				if modified := a.hooks.ModifyToolResult(block.ID, block.Name, output, true); modified != "" {
					finalOutput = modified
				}
			}
			toolResults = append(toolResults, anthropic.NewToolResultBlock(block.ID, finalOutput, true))
			continue
		}

		// Record full reads in the cache.
		if a.readCache != nil {
			if path, isFull := isFullRead(block.Name, string(block.Input)); isFull {
				if lines := extractCompleteFileLines(output); lines > 0 {
					a.readCache.RecordFullRead(path, result.ToolCalls, lines)
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