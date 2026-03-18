package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/google/uuid"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/conversation"
	"github.com/kayushkin/inber/memory"
	sessionMod "github.com/kayushkin/inber/session"
)

// RunTurn sends a user message, rebuilds the system prompt, runs the agent, and returns the result.
func (e *Engine) RunTurn(input string) (*agent.TurnResult, error) {
	// Increment and log turn number
	e.TurnCounter++
	e.turnStartTime = time.Now()
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
		tokens := memory.EstimateTokens(input)
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
		var stats agent.FilterStats
		e.Messages, stats = agent.FilterMessagesForAnthropic(e.Messages)
		if stats.ToolUseFiltered > 0 || stats.ToolResultFiltered > 0 {
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
	e.SessionCost += sessionMod.CalcCostWithCache(modelUsed, result.InputTokens, result.OutputTokens,
		result.CacheReadTokens, result.CacheCreationTokens)

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
	responseTokens := memory.EstimateTokens(result.Text)
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
			textTokens := memory.EstimateTokens(text)

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

// summarizeIfNeeded checks if the conversation is long enough to warrant summarization.
func (e *Engine) summarizeIfNeeded() {
	role := conversation.RoleDefault
	if e.AgentConfig != nil && e.AgentConfig.Role != "" {
		role = conversation.AgentRole(strings.ToLower(e.AgentConfig.Role))
	}
	cfg := conversation.DefaultSummarizeConfig(role)

	if !conversation.ShouldSummarize(e.Messages, cfg) {
		return
	}

	sessionID := ""
	if e.Session != nil {
		sessionID = e.Session.SessionID()
	}

	model := e.Model
	if model == "" {
		model = "claude-sonnet-4-5-20250929"
	}

	summarized, result, err := conversation.SummarizeConversation(
		context.Background(),
		e.Client,
		e.Messages,
		e.MemStore,
		sessionID,
		cfg,
		model,
	)

	if err != nil {
		Log.Warn("summarization failed: %v", err)
		return
	}

	if result.Summarized {
		e.Messages = summarized
		Log.Info("summarized %d turns → %d token summary (kept %d recent messages, memory: %s)",
			result.SummarizedTurns, result.SummaryTokens, result.KeptMessages, result.MemoryID)
		if e.Session != nil {
			e.Session.LogSummarize(result.SummarizedTurns, result.SummaryTokens, result.KeptMessages, result.MemoryID)
		}
	}
}

// pruneConfig returns the appropriate PruneConfig for this engine's agent role.
func (e *Engine) pruneConfig() conversation.PruneConfig {
	if e.AgentConfig != nil && e.AgentConfig.Role != "" {
		return conversation.PruneConfigForRole(e.AgentConfig.Role)
	}
	return conversation.DefaultPruneConfig()
}

// pruneIfNeeded checks if conversation should be pruned and does so if necessary.
func (e *Engine) pruneIfNeeded() {
	cfg := e.pruneConfig()

	if !conversation.ShouldPrune(e.Messages, cfg) {
		return
	}

	sessionID := ""
	if e.Session != nil {
		sessionID = e.Session.SessionID()
	}

	pruned, result, err := conversation.PruneConversation(
		context.Background(),
		e.Messages,
		e.MemStore,
		sessionID,
		cfg,
	)

	if err != nil {
		Log.Warn("pruning failed: %v", err)
		return
	}

	if result.PrunedMessages > 0 {
		e.Messages = pruned
		Log.Info("pruned %d messages (%d tokens freed, %d memories saved)",
			result.PrunedMessages, result.TokensFreed, result.MemoriesSaved)
		if e.Session != nil {
			e.Session.LogPrune(result.PrunedMessages, result.TokensFreed, result.Strategy)
		}
	}
}

// checkpointIfNeeded creates a checkpoint if we've reached the checkpoint interval.
func (e *Engine) checkpointIfNeeded() {
	if e.Session == nil {
		return
	}

	cfg := sessionMod.DefaultCheckpointConfig()
	if !sessionMod.ShouldCheckpoint(e.TurnCounter, cfg) {
		return
	}

	summary := sessionMod.GenerateConversationSummary(e.Messages)
	keyFacts := sessionMod.ExtractKeyFacts(e.Messages, 10)

	err := e.Session.SaveCheckpoint(e.Messages, summary, keyFacts)
	if err != nil {
		Log.Warn("checkpoint failed: %v", err)
	} else {
		Log.Info("checkpoint saved (turn %d)", e.TurnCounter)
	}
}

// saveMessages writes the current messages to the workspace and session log dir.
func (e *Engine) saveMessages() {
	data, err := json.Marshal(e.Messages)
	if err != nil {
		return
	}
	if e.workspace != nil {
		e.workspace.SaveMessages(data)
	}
	if e.Session != nil {
		sessDir := filepath.Dir(e.Session.FilePath())
		os.WriteFile(filepath.Join(sessDir, "messages.json"), data, 0644)
	}
}

// LogUser logs a user message to the session (for external callers that need pre-logging).
func (e *Engine) LogUser(input string) {
	if e.Session != nil {
		e.Session.LogUser(input)
	}
}

// LogAssistant logs an assistant response to the session.
func (e *Engine) LogAssistant(result *agent.TurnResult) {
	if e.Session != nil {
		e.Session.LogAssistant(result.Text, result.InputTokens, result.OutputTokens, result.ToolCalls)
	}
}

// Close saves session summary, closes memory store, and unregisters the active session.
func (e *Engine) Close() {
	if e.workflowHooks != nil {
		if summary := e.workflowHooks.FinishSession(); summary != "" {
			fmt.Fprintln(os.Stderr, "\n"+summary)
		}
	}

	if e.MemStore != nil && len(e.Messages) > 0 {
		SaveSessionSummary(e.MemStore, e.Messages, e.AgentName)
	}

	if e.MemStore != nil {
		e.MemStore.Close()
	}
	if e.Session != nil {
		e.Session.Close()
	}
	if e.SessionDB != nil {
		e.SessionDB.Close()
	}
	if e.modelStore != nil {
		e.modelStore.Close()
	}

	if e.forgeDB != nil {
		e.forgeDB.Close()
	}
}

// SaveSessionSummary generates a brief session summary and saves it to memory.
func SaveSessionSummary(store memory.MemoryStore, messages []anthropic.MessageParam, agentName string) {
	var parts []string
	for _, msg := range messages {
		role := string(msg.Role)
		for _, block := range msg.Content {
			if block.OfText != nil {
				text := block.OfText.Text
				if len(text) > 200 {
					text = text[:200] + "..."
				}
				parts = append(parts, fmt.Sprintf("%s: %s", role, text))
			}
		}
	}

	if len(parts) == 0 {
		return
	}

	summary := fmt.Sprintf("Session summary (%s):\n%s", agentName, strings.Join(parts, "\n"))
	if len(summary) > 2000 {
		summary = summary[:2000]
	}

	m := memory.Memory{
		ID:         uuid.New().String(),
		Content:    summary,
		Tags:       []string{"session-summary", agentName},
		Importance: 0.4,
		Source:     "system",
	}

	if err := store.Save(m); err != nil {
		Log.Warn("failed to save session summary: %v", err)
	}
}

// FindRepoRoot finds the repository root by looking for .git directory.
func FindRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		gitDir := filepath.Join(dir, ".git")
		if _, err := os.Stat(gitDir); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("not in a git repository")
		}
		dir = parent
	}
}
