package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
	sessionMod "github.com/kayushkin/inber/session"
)

// runOpenAITurn executes a turn using an OpenAI-compatible API.
func (e *Engine) runOpenAITurn(ctx context.Context, systemBlocks []sessionMod.NamedBlock) (*agent.TurnResult, error) {
	result := &agent.TurnResult{}
	
	client, err := e.modelClient.GetOpenAIClient()
	if err != nil {
		return nil, err
	}
	
	// Build tool map for execution
	toolMap := make(map[string]agent.Tool)
	for _, t := range e.agentTools {
		toolMap[t.Name] = t
	}
	
	// Convert tools to OpenAI format, carrying the same "then" chain and
	// done/note/split sideband fields the Anthropic path advertises. Without
	// them a session served by this provider silently has fewer things a model
	// can write into a call than the same session served by the other one.
	openAITools := agent.ConvertAnthropicToolsToOpenAI(agent.AddChainAndSidebandFields(e.agentTools))

	// The same hooks the Anthropic path reports through: session logging, the
	// display (resolved per call so a gateway can swap it), tool-input caching,
	// truncation, and the workflow/forge post-hooks. This loop used to reach
	// for e.display and e.Session itself and reproduced only some of that.
	hooks := e.buildHooks()

	// Sideband callbacks are the agent's task list and scratchpad; without a
	// repo there is nothing to apply them to, and processSideband says so to
	// the model rather than dropping the field. Same condition as configureAgent.
	var sidebandCallbacks *agent.SidebandCallbacks
	if e.repoRoot != "" {
		sidebandCallbacks = e.buildSidebandCallbacks()
	}

	// Build system message from blocks
	var systemParts []string
	for _, block := range systemBlocks {
		systemParts = append(systemParts, block.Text)
	}
	systemMessage := strings.Join(systemParts, "\n\n")
	
	// Tool call loop
	for {
		oaiMessages := agent.ConvertAnthropicMessagesToOpenAI(e.Messages)
		
		if systemMessage != "" {
			oaiMessages = append([]agent.OpenAIMessage{
				{Role: "system", Content: systemMessage},
			}, oaiMessages...)
		}
		
		req := agent.OpenAIRequest{
			Model:    client.Model,
			Messages: oaiMessages,
		}
		// o-series models (o1, o3, etc.) require max_completion_tokens instead of max_tokens.
		if strings.HasPrefix(client.Model, "o1") || strings.HasPrefix(client.Model, "o3") {
			req.MaxCompletionTokens = 16384
		} else {
			req.MaxTokens = 16384
		}
		if len(openAITools) > 0 {
			req.Tools = openAITools
		}
		
		resp, err := client.ChatCompletion(ctx, req)
		if err != nil {
			return result, fmt.Errorf("OpenAI API call failed: %w", err)
		}
		
		anthropicResp := agent.ConvertOpenAIResponseToAnthropic(resp)
		
		result.InputTokens += int(anthropicResp.Usage.InputTokens)
		result.OutputTokens += int(anthropicResp.Usage.OutputTokens)
		
		// Ends the turn in the session, logs the response, and clears the
		// consecutive-error count when a response arrives clean. This loop used
		// to call EndTurn by hand and do neither of the other two, so an
		// OpenAI-served session logged no responses and its error count only
		// ever went up.
		if hooks.OnResponse != nil {
			hooks.OnResponse(anthropicResp)
		}

		e.Messages = append(e.Messages, anthropicResp.ToParam())
		
		if anthropicResp.StopReason == anthropic.StopReasonEndTurn || 
		   anthropicResp.StopReason == anthropic.StopReasonMaxTokens {
			for _, block := range anthropicResp.Content {
				if block.Type == "text" {
					result.Text += block.Text
				}
			}
			return result, nil
		}
		
		if anthropicResp.StopReason == anthropic.StopReasonToolUse {
			var toolResults []anthropic.ContentBlockParamUnion
			var postInjections []string
			refusal := e.buildToolRefusal()

			for _, block := range anthropicResp.Content {
				if block.Type != "tool_use" {
					continue
				}
				
				result.ToolCalls++

				if hooks.OnToolCall != nil {
					hooks.OnToolCall(block.ID, block.Name, block.Input)
				}

				// One call to the same dispatcher the Anthropic path uses. It
				// runs the primary call, the "then" chain the block may carry,
				// and the done/note/split sideband fields, and it asks the gate
				// about all three — none of which this loop did while it
				// dispatched tools itself.
				output, isError := agent.ExecuteToolCallWithChainAndSideband(
					ctx, toolMap, block.Name, string(block.Input),
					hooks, block.ID, sidebandCallbacks, refusal,
				)

				// The dispatcher reports a result it produced itself; the ones
				// it returns as errors — refused, unknown tool, the tool failed
				// — it hands back for the caller to report, so say so here or
				// they reach neither the display nor the session log.
				if isError && hooks.OnToolResult != nil {
					hooks.OnToolResult(block.ID, block.Name, output, true)
				}

				finalOutput := output
				if hooks.ModifyToolResult != nil {
					if modified := hooks.ModifyToolResult(block.ID, block.Name, output, isError); modified != "" {
						finalOutput = modified
					}
				}

				toolResults = append(toolResults, anthropic.NewToolResultBlock(block.ID, finalOutput, isError))

				if hooks.PostToolResult != nil {
					if injection := hooks.PostToolResult(block.ID, block.Name, output, isError); injection != "" {
						postInjections = append(postInjections, injection)
					}
				}
			}
			
			if len(postInjections) > 0 {
				toolResults = append(toolResults, anthropic.NewTextBlock(
					"[system: post-write hook]\n"+strings.Join(postInjections, "\n"),
				))
			}
			
			e.Messages = append(e.Messages, anthropic.NewUserMessage(toolResults...))
			continue
		}
		
		return result, fmt.Errorf("unexpected stop reason: %s", anthropicResp.StopReason)
	}
}