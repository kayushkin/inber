package engine

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
	inbercontext "github.com/kayushkin/inber/context"
	"github.com/kayushkin/inber/conversation"
	sessionMod "github.com/kayushkin/inber/session"
)

// RunTurn sends a user message, rebuilds the system prompt, runs the agent, and returns the result.
func (e *Engine) RunTurn(input string) (*agent.TurnResult, error) {
	// Increment and log turn number
	e.TurnCounter++
	fmt.Fprintf(os.Stderr, "\n%s━━━ Turn %d ━━━%s\n", cyan+bold, e.TurnCounter, reset)
	
	// Get session ID for tagging
	sessionID := ""
	if e.Session != nil {
		sessionID = e.Session.SessionID()
		e.Session.LogUser(input)
	}

	// 1. STASH LARGE USER MESSAGES (before sending to LLM)
	processedInput := input
	if e.stashCfg.Enabled && e.MemStore != nil {
		tokens := inbercontext.EstimateTokens(input)
		if tokens > e.stashCfg.UserMessageThreshold {
			modifiedInput, stashed, err := conversation.DetectAndStashLargeBlocks(input, sessionID, e.MemStore, e.stashCfg)
			if err != nil {
				Log.Warn("failed to stash large user message: %v", err)
			} else if len(stashed) > 0 {
				processedInput = modifiedInput
				totalStashed := 0
				for _, s := range stashed {
					totalStashed += s.Tokens
				}
				Log.Info("stashed %d large blocks from user message (%d tokens)", len(stashed), totalStashed)
				
				if e.Session != nil {
					e.Session.LogStash("user", len(stashed), totalStashed)
				}
			}
		}
	}

	e.Messages = append(e.Messages, anthropic.NewUserMessage(anthropic.NewTextBlock(processedInput)))

	// 1a. Summarize if conversation is very long (compress old turns into summary)
	e.summarizeIfNeeded()
	// 1b. Prune remaining conversation (truncate tool results, old messages)
	e.pruneIfNeeded()

	systemBlocks := e.BuildSystemPrompt(processedInput)
	e.lastNamedBlocks = systemBlocks
	
	// Select model based on health data (failover if primary is down)
	modelUsed, _ := e.selectModel()

	// Ensure we have the right client for the selected model
	if e.modelClient == nil || (e.modelClient.Model != nil && e.modelClient.Model.ID != modelUsed) {
		mc, mcErr := agent.NewModelClient(modelUsed, e.modelStore)
		if mcErr == nil {
			e.modelClient = mc
		}
	}
	e.Model = modelUsed

	var result *agent.TurnResult
	var err error
	apiStart := time.Now()

	if e.modelClient != nil && e.modelClient.IsOpenAI() {
		result, err = e.runOpenAITurn(context.Background(), systemBlocks)
	} else {
		// Filter out OpenAI-sourced tool_use/tool_result pairs for Anthropic
		originalLen := len(e.Messages)
		e.Messages = agent.FilterMessagesForAnthropic(e.Messages)
		if stats := agent.LastFilterStats(); stats.ToolUseFiltered > 0 || stats.ToolResultFiltered > 0 {
			Log.Info("filtered %d tool_use, %d tool_result blocks from OpenAI provider (%d→%d messages)",
				stats.ToolUseFiltered, stats.ToolResultFiltered, originalLen, len(e.Messages))
		}
		e.buildAgent(systemBlocks)
		result, err = e.Agent.Run(context.Background(), e.Model, &e.Messages)
	}

	// Record health regardless of success/failure
	e.recordModelHealth(modelUsed, time.Since(apiStart).Milliseconds(), err)

	if err != nil {
		return nil, err
	}

	if e.Session != nil {
		e.Session.LogAssistant(result.Text, result.InputTokens, result.OutputTokens, result.ToolCalls)
	}

	// 2. BACKGROUND MEMORY EXTRACTION (after turn completes, async)
	if e.extractCfg.Enabled && e.MemStore != nil {
		var toolCalls []conversation.ToolCallSummary
		go conversation.BackgroundExtractMemories(
			context.Background(),
			e.Client,
			input,
			result.Text,
			toolCalls,
			sessionID,
			e.MemStore,
			e.extractCfg,
		)
	}

	// 3. STASH LARGE ASSISTANT RESPONSES (for next turn)
	e.stashAssistantResponse(sessionID, result)

	// Save messages snapshot for session resume
	e.saveMessages()
	
	// Checkpoint if needed (every 20 turns)
	e.checkpointIfNeeded()
	
	// Track cumulative session tokens
	e.SessionInputTokens += result.InputTokens
	e.SessionOutputTokens += result.OutputTokens
	e.SessionCost += sessionMod.CalcCost(modelUsed, result.InputTokens, result.OutputTokens)

	// Track usage in model-store
	if e.modelStore != nil {
		agentName := e.AgentName
		if agentName == "" {
			agentName = "inber"
		}
		if err := e.modelStore.TrackUsage(agentName, e.Model, int64(result.InputTokens), int64(result.OutputTokens)); err != nil {
			Log.Warn("failed to track usage in model-store: %v", err)
		}
	}

	return result, nil
}

// stashAssistantResponse stashes large blocks from the assistant's last response in conversation history.
func (e *Engine) stashAssistantResponse(sessionID string, result *agent.TurnResult) {
	if !e.stashCfg.Enabled || e.MemStore == nil {
		return
	}
	responseTokens := inbercontext.EstimateTokens(result.Text)
	if responseTokens <= e.stashCfg.AssistantThreshold {
		return
	}
	if len(e.Messages) == 0 || e.Messages[len(e.Messages)-1].Role != anthropic.MessageParamRoleAssistant {
		return
	}

	lastMsg := &e.Messages[len(e.Messages)-1]
	var modifiedContent []anthropic.ContentBlockParamUnion
	stashedAny := false

	for _, block := range lastMsg.Content {
		if block.OfText != nil {
			text := block.OfText.Text
			textTokens := inbercontext.EstimateTokens(text)

			if textTokens > e.stashCfg.MinBlockSize {
				modifiedText, stashed, err := conversation.DetectAndStashLargeBlocks(text, sessionID, e.MemStore, e.stashCfg)
				if err != nil {
					Log.Warn("failed to stash assistant response: %v", err)
					modifiedContent = append(modifiedContent, block)
				} else if len(stashed) > 0 {
					stashedAny = true
					modifiedContent = append(modifiedContent, anthropic.ContentBlockParamUnion{
						OfText: &anthropic.TextBlockParam{Text: modifiedText},
					})
					totalStashed := 0
					for _, s := range stashed {
						totalStashed += s.Tokens
					}
					Log.Info("stashed %d large blocks from assistant response (%d tokens)", len(stashed), totalStashed)
					if e.Session != nil {
						e.Session.LogStash("assistant", len(stashed), totalStashed)
					}
				} else {
					modifiedContent = append(modifiedContent, block)
				}
			} else {
				modifiedContent = append(modifiedContent, block)
			}
		} else {
			modifiedContent = append(modifiedContent, block)
		}
	}

	if stashedAny {
		lastMsg.Content = modifiedContent
	}
}

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
	oaiAPIcalls := 0
	for {
		oaiAPIcalls++

		// Check for mid-run injected messages
		if oaiAPIcalls > 1 && e.injections != nil {
			injectCheck := e.buildInjectCheck()
			if injected := injectCheck(); len(injected) > 0 {
				if len(e.Messages) > 0 {
					lastIdx := len(e.Messages) - 1
					for _, text := range injected {
						e.Messages[lastIdx].Content = append(e.Messages[lastIdx].Content,
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
		if oaiAPIcalls > 1 && (e.maxTurns > 0 || e.maxInputTokens > 0) {
			limitCheck := e.buildLimitCheck()
			if exceeded, reason := limitCheck(result); exceeded {
				forceSummary = true
				if len(e.Messages) > 0 {
					lastIdx := len(e.Messages) - 1
					e.Messages[lastIdx].Content = append(e.Messages[lastIdx].Content,
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

		oaiMessages := agent.ConvertAnthropicMessagesToOpenAI(e.Messages)
		
		if systemMessage != "" {
			oaiMessages = append([]agent.OpenAIMessage{
				{Role: "system", Content: systemMessage},
			}, oaiMessages...)
		}
		
		req := agent.OpenAIRequest{
			Model:     client.Model,
			Messages:  oaiMessages,
			MaxTokens: 16384,
		}
		if !forceSummary && len(openAITools) > 0 {
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
				
				output, err := tool.Run(ctx, string(block.Input))
				if err != nil {
					errMsg := fmt.Sprintf("error: %s", err)
					if e.display != nil && e.display.OnToolResult != nil {
						e.display.OnToolResult(block.Name, errMsg, true)
					}
					if e.Session != nil {
						e.Session.LogToolResult(block.ID, block.Name, errMsg, true)
					}
					e.consecutiveErrors++
					e.lastTurnHadError = true
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
				
				if e.autoRefMgr != nil && e.toolInputsCache != nil {
					inputJSON := e.toolInputsCache[block.ID]
					if err := e.autoRefMgr.OnToolResult(block.ID, block.Name, inputJSON, output); err != nil {
						fmt.Fprintf(os.Stderr, "warning: failed to auto-create reference for %s: %v\n", block.Name, err)
					}
				}
				
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
