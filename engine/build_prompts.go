package engine

import (
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/memory"
	sessionMod "github.com/kayushkin/inber/session"
)

// contextBudget returns the token budget for memory context loading.
func (e *Engine) contextBudget(userMessage string) (minImportance float64, tokenBudget int) {
	msgTokens := memory.EstimateTokens(userMessage)
	
	// Use larger context budget for error recovery
	if e.consecutiveErrors >= 5 {
		return 0, 50000
	}
	if e.consecutiveErrors >= 3 {
		return 0, 35000
	}
	if e.consecutiveErrors >= 1 || e.lastTurnHadError {
		return 0, 20000
	}
	
	// First turn gets base budget
	if e.TurnCounter == 0 {
		return 0, 4000
	}
	
	// Scale budget based on message complexity
	if msgTokens > 1000 {
		return 0, 15000
	}
	if msgTokens > 300 {
		return 0, 10000
	}
	if e.TurnCounter > 15 {
		return 0, 8000
	}
	
	return 0, 6000
}

// BuildSystemPrompt builds a context-aware system prompt as individual named blocks.
func (e *Engine) BuildSystemPrompt(userMessage string) []sessionMod.NamedBlock {
	if e.MemStore != nil {
		messageTags := memory.AutoTag(userMessage, "user")
		minImportance, tokenBudget := e.contextBudget(userMessage)

		req := memory.BuildContextRequest{
			Tags:              messageTags,
			TokenBudget:       tokenBudget,
			MinImportance:     minImportance,
			IncludeAlwaysLoad: true,
			ExcludeTags:       []string{"session-summary", "repo-map", "code-introspection"},
			MaxChunkSize:      5000,
			TruncateThreshold: 500,
			TruncatePreview:   300,
		}

		memories, tokensUsed, err := e.MemStore.BuildContext(req)
		if err != nil {
			Log.Warn("failed to build context from memory: %v", err)
			return nil
		}

		Log.Info("context: %d memories, %d tokens (min_importance=%.1f, budget=%d)", len(memories), tokensUsed, minImportance, tokenBudget)

		var blocks []sessionMod.NamedBlock
		for _, m := range memories {
			text := m.Content
			if text == "" {
				text = m.Summary
			}
			if text == "" {
				continue
			}
			idPrefix := m.ID
			if len(idPrefix) > 8 {
				idPrefix = idPrefix[:8]
			}
			desc := fmt.Sprintf("%s (%.1f", idPrefix, m.Importance)
			if len(m.Tags) > 0 {
				desc += fmt.Sprintf(", tags: %s", strings.Join(m.Tags, ","))
			}
			desc += ")"
			blocks = append(blocks, sessionMod.NamedBlock{ID: desc, Text: text})
		}

		// Inject fleet status (active agents) for orchestrator awareness
		if status := e.buildFleetStatus(); status != "" {
			blocks = append(blocks, sessionMod.NamedBlock{ID: "fleet-status", Text: status})
		}

		// Inject server-provided context (live session status, workspace info, etc.)
		for _, injector := range e.contextInjectors {
			if extra := injector(); len(extra) > 0 {
				blocks = append(blocks, extra...)
			}
		}

		if e.workspace != nil {
			e.workspace.WriteSystem(blocks)
		}
		return blocks
	}
	
	if e.IdentityOverride == "" {
		return nil
	}

	blocks := []sessionMod.NamedBlock{
		{ID: "identity", Text: e.IdentityOverride},
	}

	if e.workspace != nil {
		e.workspace.WriteSystem(blocks)
	}
	return blocks
}

// buildSystemBlocks converts named blocks to anthropic system blocks with cache control.
func (e *Engine) buildSystemBlocks(blocks []sessionMod.NamedBlock) []anthropic.TextBlockParam {
	systemBlocks := make([]anthropic.TextBlockParam, 0, len(blocks)+1)

	// Claude Max OAuth tokens require Claude Code identity in the system prompt.
	// Without this, the API returns 400 "Error" for Claude 4 models.
	if e.modelClient != nil && e.modelClient.IsOAuth {
		systemBlocks = append(systemBlocks, anthropic.TextBlockParam{
			Text: "You are Claude Code, Anthropic's official CLI for Claude.",
		})
	}

	for _, b := range blocks {
		systemBlocks = append(systemBlocks, anthropic.TextBlockParam{Text: b.Text})
	}
	
	// Enable prompt caching: place cache_control at the stable/volatile boundary.
	// Stable memories (identity, decisions, prefs) come first and rarely change.
	// Volatile memories (file refs, recent files) come last and change every turn.
	// Caching the stable prefix avoids re-processing it when volatile content shifts.
	cacheIdx := len(systemBlocks) - 1 // default: last block
	for i, b := range blocks {
		if isVolatileBlock(b.ID) {
			if i > 0 {
				cacheIdx = i - 1 // last stable block
			}
			break
		}
	}
	if cacheIdx >= 0 && cacheIdx < len(systemBlocks) {
		systemBlocks[cacheIdx].CacheControl = anthropic.NewCacheControlEphemeralParam()
	}
	
	return systemBlocks
}

// isVolatileBlock checks if a named block ID indicates volatile content
// (file references, recent files) that changes between turns.
func isVolatileBlock(id string) bool {
	return strings.HasPrefix(id, "fileref:") ||
		strings.HasPrefix(id, "recent:") ||
		strings.HasPrefix(id, "file:") ||
		id == "fleet-status" ||
		id == "server-sessions"
}