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
	
	// Convert tools to OpenAI format
	openAITools := agent.ConvertAnthropicToolsToOpenAI(e.agentTools)
	
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
		
		if e.Session != nil {
			stopReason := string(anthropicResp.StopReason)
			toolCalls := 0
			for _, block := range anthropicResp.Content {
				if block.Type == "tool_use" {
					toolCalls++
				}
			}
			e.Session.EndTurn(
				int(anthropicResp.Usage.InputTokens),
				int(anthropicResp.Usage.OutputTokens),
				toolCalls,
				stopReason,
				"",
			)
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
				
				if e.display != nil && e.display.OnToolCall != nil {
					e.display.OnToolCall(block.Name, string(block.Input))
				}
				if e.Session != nil {
					e.Session.LogToolCall(block.ID, block.Name, block.Input)
				}
				if e.toolInputsCache != nil {
					e.toolInputsCache[block.ID] = string(block.Input)
				}
				
				tool, ok := toolMap[block.Name]
				if !ok {
					errMsg := fmt.Sprintf("error: unknown tool %q", block.Name)
					if e.display != nil && e.display.OnToolResult != nil {
						e.display.OnToolResult(block.Name, errMsg, true)
					}
					if e.Session != nil {
						e.Session.LogToolResult(block.ID, block.Name, errMsg, true)
					}
					toolResults = append(toolResults, anthropic.NewToolResultBlock(block.ID, errMsg, true))
					continue
				}
				
				// The same gate as the Anthropic path. This loop dispatches
				// tools itself instead of going through agent.executeTools, so
				// a gate wired only there would leave every OpenAI-served
				// session ungated.
				if refusal != nil {
					if reason := refusal(block.Name, string(block.Input)); reason != "" {
						refused := agent.RefuseToolCall(block.Name, reason)
						if e.display != nil && e.display.OnToolResult != nil {
							e.display.OnToolResult(block.Name, refused, true)
						}
						if e.Session != nil {
							e.Session.LogToolResult(block.ID, block.Name, refused, true)
						}
						toolResults = append(toolResults, anthropic.NewToolResultBlock(block.ID, refused, true))
						continue
					}
				}

				output, err := tool.Run(ctx, string(block.Input))
				if err != nil {
					errMsg := fmt.Sprintf("error: %s", err)
					if e.display != nil && e.display.OnToolResult != nil {
						e.display.OnToolResult(block.Name, errMsg, true)
					}
					if e.Session != nil {
						e.Session.LogToolResult(block.ID, block.Name, errMsg, true)
					}
					e.Turn.ConsecutiveErrors++
					e.Turn.LastHadError = true
					toolResults = append(toolResults, anthropic.NewToolResultBlock(block.ID, errMsg, true))
					continue
				}
				
				if e.display != nil && e.display.OnToolResult != nil {
					e.display.OnToolResult(block.Name, output, false)
				}
				if e.Session != nil {
					e.Session.LogToolResult(block.ID, block.Name, output, false)
				}
				
				finalOutput := output
				if e.Session != nil {
					truncated := e.Session.TruncateToolResult(block.Name, output, false)
					if truncated != "" {
						finalOutput = truncated
					}
				}
				
				toolResults = append(toolResults, anthropic.NewToolResultBlock(block.ID, finalOutput, false))
				
				if e.workflowHooks != nil {
					toolInput := e.toolInputsCache[block.ID]
					if injection := e.workflowHooks.OnToolResult(block.Name, toolInput, output, false); injection != "" {
						postInjections = append(postInjections, injection)
					}
				}
				
				if e.toolInputsCache != nil {
					delete(e.toolInputsCache, block.ID)
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