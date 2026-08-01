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

	// The same root setToolSet resolves the tools against. The read cache
	// identifies a file by the path the tool will open, so it has to be given
	// the root the tools were given or a relative read and an absolute write
	// land on two different entries.
	a.SetRepoRoot(e.repoRoot)

	// OAuth identity injection removed (2026-04-04): using API key auth now.
	
	// Hand over the volatile context for injection into the last user message.
	// Taking it leaves the engine with nothing to hand a rebuilt agent, which is
	// what stops the thinking-signature retry injecting a second copy.
	a.VolatileContext = e.takeVolatileContext()

	// Pass frozen/staging boundary for BP3 placement
	if e.staged != nil {
		a.FrozenIdx = e.staged.FrozenIdx
	}

	e.configureAgent(a)
	e.configureContextPruning(a)

	// Generate prompt blueprint for cache analysis
	if e.Cache.BlueprintEnabled {
		bp := BuildBlueprint(e.Turn.Counter, e.agentTools, systemBlocks, blocks, e.Messages, a.FrozenIdx)

		// Try loading previous blueprint from workspace for cross-invocation diffs
		if e.Cache.LastBlueprint == nil && e.workspace != nil {
			if prev, err := LoadBlueprintFromWorkspace(e.workspace); err == nil {
				e.Cache.LastBlueprint = prev
			}
		}

		if e.Cache.LastBlueprint != nil {
			diff := DiffBlueprints(e.Cache.LastBlueprint, bp)
			Log.Info("prompt blueprint:\n%s", FormatDiff(diff))
		} else {
			Log.Info("prompt blueprint:\n%s", FormatBlueprint(bp))
		}
		e.Cache.LastBlueprint = bp

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
	
	// Wire up sideband callbacks (task completion, notes, task splitting)
	if e.repoRoot != "" {
		a.SetSidebandCallbacks(e.buildSidebandCallbacks())
	}

	// Wire up mid-run message injection
	if e.injections != nil {
		a.InjectCheck = e.buildInjectCheck()
	}

	// Wire up the tool gate. Unlike the limit checks below it is wired
	// unconditionally: whether any tool is refused is the guard's answer to
	// give, and deciding here that a session needs no gate would put that
	// answer in two places.
	a.SetToolRefusal(e.buildToolRefusal())

	// Wire up turn/token/time limit checks
	if e.Limits.MaxTurns > 0 || e.Limits.MaxInputTokens > 0 || e.Limits.MaxResponseTime > 0 {
		a.SetLimitCheck(e.buildLimitCheck())
	}

	// Set context window based on model or default
	modelInfo := agent.GetModelInfo(e.Model, e.modelStore)
	a.SetContextWindow(modelInfo.ContextWindow)
}

// configureContextPruning sets up automatic context pruning when approaching token limits.
func (e *Engine) configureContextPruning(a *agent.Agent) {
	a.SetBeforeRequest(func(ctx context.Context, messages []anthropic.MessageParam, contextWindow int) []anthropic.MessageParam {
		cfg := e.pruneConfig()
		cfg.TokenBudget = contextWindow / 2

		if conversation.ShouldPrune(messages, cfg) {
			Log.Warn("context approaching limit (%d messages), pruning", len(messages))
			pruned, result, err := conversation.PruneConversation(ctx, messages, e.MemStore, "", cfg)
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
				if conversation.StartsUserTurn(msg) {
					break
				}
				dropTo++
			}
			if dropTo < len(messages) && dropTo > 0 {
				Log.Warn("hard-dropping %d old messages (%d → %d)", dropTo, len(messages), len(messages)-dropTo)
				messages = messages[dropTo:]
				// The agent shifts its own copy of the boundary (see
				// shiftBreakpointIndicesAfterHeadDrop), but that copy is rebuilt from
				// this one at the top of every turn, and pruneIfNeeded reads this one
				// to decide which messages are frozen. Both have to move with the slice
				// or the next turn stages the frozen zone.
				if e.staged != nil {
					e.staged.ShiftAfterHeadDrop(dropTo)
				}
			}
		}

		return messages
	})
}
