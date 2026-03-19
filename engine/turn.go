package engine

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
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
	
	// Take memory snapshot at start of turn
	if e.memoryProfiler != nil {
		e.memoryProfiler.TakeSnapshot(e.TurnCounter)
	}
	
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

	// Repair alternation before appending: if last message is also user role
	// (e.g. previous turn errored after appending user but before assistant response),
	// merge into the existing user message to maintain alternation.
	if len(e.Messages) > 0 && e.Messages[len(e.Messages)-1].Role == anthropic.MessageParamRoleUser {
		e.Messages[len(e.Messages)-1].Content = append(
			e.Messages[len(e.Messages)-1].Content,
			anthropic.NewTextBlock(processedInput),
		)
	} else {
		e.Messages = append(e.Messages, anthropic.NewUserMessage(anthropic.NewTextBlock(processedInput)))
	}

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

	// Take memory snapshot at end of turn
	if e.memoryProfiler != nil {
		e.memoryProfiler.TakeSnapshot(e.TurnCounter)
	}

	return result, nil
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

	// Close memory profiler
	if e.memoryProfiler != nil {
		e.memoryProfiler.Close()
	}
}