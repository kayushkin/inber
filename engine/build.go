package engine

import (
	"context"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/conversation"
	sessionMod "github.com/kayushkin/inber/session"
)

// buildAgent creates a fresh Agent with current system prompt, tools, and hooks.
// Automatically adds cache_control to system blocks for prompt caching.
func (e *Engine) buildAgent(blocks []sessionMod.NamedBlock) *agent.Agent {
	systemBlocks := e.buildSystemBlocks(blocks)
	
	// Use the model client to get the appropriate provider
	// Currently only Anthropic is supported via the Provider interface
	var provider agent.Provider
	if e.modelClient != nil && e.modelClient.AnthropicClient != nil {
		provider = agent.NewAnthropicProvider(e.modelClient.AnthropicClient)
	} else {
		// Fallback for backward compatibility
		provider = agent.NewAnthropicProvider(e.Client)
	}
	
	a := agent.NewWithSystemBlocks(provider, systemBlocks)

	// OAuth identity injection removed (2026-04-04): using API key auth now.
	
	// Pass volatile context for injection into last user message
	a.VolatileContext = e.volatileContext

	e.configureAgent(a)
	e.configureContextPruning(a)

	// Generate prompt blueprint for cache analysis
	if e.blueprintEnabled {
		bp := BuildBlueprint(e.TurnCounter, e.agentTools, systemBlocks, blocks, e.Messages)

		// Try loading previous blueprint from workspace for cross-invocation diffs
		if e.lastBlueprint == nil && e.workspace != nil {
			if prev, err := LoadBlueprintFromWorkspace(e.workspace); err == nil {
				e.lastBlueprint = prev
			}
		}

		if e.lastBlueprint != nil {
			diff := DiffBlueprints(e.lastBlueprint, bp)
			Log.Info("prompt blueprint:\n%s", FormatDiff(diff))
		} else {
			Log.Info("prompt blueprint:\n%s", FormatBlueprint(bp))
		}
		e.lastBlueprint = bp

		// Persist to workspace for next invocation
		if e.workspace != nil {
			SaveBlueprintToWorkspace(e.workspace, bp)
		}
	}
	
	e.Agent = a
	return a
}

// configureAgent sets up tools, hooks, limits, and other agent settings.
func (e *Engine) configureAgent(a *agent.Agent) {
	for _, t := range e.agentTools {
		a.AddTool(t)
	}
	
	if e.thinkingBud > 0 {
		a.SetThinking(e.thinkingBud)
	}
	
	a.SetHooks(e.buildHooks())

	// Wire up mid-run message injection
	if e.injections != nil {
		a.InjectCheck = e.buildInjectCheck()
	}

	// Wire up turn/token/time limit checks
	if e.maxTurns > 0 || e.maxInputTokens > 0 || e.maxResponseTime > 0 {
		a.SetLimitCheck(e.buildLimitCheck())
	}

	// Set context window based on model or default
	modelInfo := agent.GetModelInfo(e.Model, e.modelStore)
	a.SetContextWindow(modelInfo.ContextWindow)
}

// configureContextPruning sets up automatic context pruning when approaching token limits.
func (e *Engine) configureContextPruning(a *agent.Agent) {
	a.SetBeforeRequest(func(messages []anthropic.MessageParam, contextWindow int) []anthropic.MessageParam {
		cfg := e.pruneConfig()
		cfg.TokenBudget = contextWindow / 2

		if conversation.ShouldPrune(messages, cfg) {
			Log.Warn("context approaching limit (%d messages), pruning", len(messages))
			pruned, result, err := conversation.PruneConversation(context.Background(), messages, e.MemStore, "", cfg)
			if err == nil {
				Log.Info("pruned: %d tokens freed", result.TokensFreed)
				messages = pruned
			}
		}

		maxMessages := cfg.KeepRecentTurns * 2
		if len(messages) > maxMessages {
			dropTo := len(messages) - maxMessages
			for dropTo < len(messages) {
				msg := messages[dropTo]
				if msg.Role == anthropic.MessageParamRoleUser && !hasToolResult(msg) {
					break
				}
				dropTo++
			}
			if dropTo < len(messages) && dropTo > 0 {
				Log.Warn("hard-dropping %d old messages (%d → %d)", dropTo, len(messages), len(messages)-dropTo)
				messages = messages[dropTo:]
			}
		}

		return messages
	})
}

// hasToolResult checks if a message contains any tool_result content blocks.
func hasToolResult(msg anthropic.MessageParam) bool {
	for _, block := range msg.Content {
		if block.OfToolResult != nil {
			return true
		}
	}
	return false
}