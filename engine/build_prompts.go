package engine

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/memory"
	sessionMod "github.com/kayushkin/inber/session"
)

// cacheBoundaryID is the sentinel block ID that separates stable (cacheable)
// system blocks from volatile (per-turn) blocks. buildSystemBlocks places
// cache_control on the last block before this marker.
const cacheBoundaryID = "__CACHE_BOUNDARY__"

// cachedPrefix stores the hash and blocks of the last stable system prefix,
// allowing us to reuse the exact byte sequence when content hasn't changed.
type cachedPrefix struct {
	hash   [32]byte
	blocks []anthropic.TextBlockParam
}

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
//
// Block ordering is optimized for prompt caching (most stable first):
//  1. Always-load memories (identity, instructions, tools) — never change
//  2. Tag-matched persistent memories — rarely change
//  3. __CACHE_BOUNDARY__ sentinel (not emitted as a block)
//  4. Fleet status — changes every turn
//  5. Recent files — changes every turn
//  6. Context injectors — changes every turn
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

		// BuildContext returns memories partitioned: stable first, volatile last.
		// We split them into two groups separated by a cache boundary marker.
		var stableBlocks, volatileBlocks []sessionMod.NamedBlock
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
			block := sessionMod.NamedBlock{ID: desc, Text: text}

			if isVolatileMemoryID(m.ID) {
				volatileBlocks = append(volatileBlocks, block)
			} else {
				stableBlocks = append(stableBlocks, block)
			}
		}

		// Assemble: stable → boundary → volatile + injectors
		var blocks []sessionMod.NamedBlock
		blocks = append(blocks, stableBlocks...)

		// Explicit cache boundary — buildSystemBlocks places BP2 before this
		blocks = append(blocks, sessionMod.NamedBlock{ID: cacheBoundaryID})

		// Dynamic/volatile content (after boundary, never cached)
		if status := e.buildFleetStatus(); status != "" {
			blocks = append(blocks, sessionMod.NamedBlock{ID: "fleet-status", Text: status})
		}
		blocks = append(blocks, volatileBlocks...)

		// Server-provided context (live session status, workspace info, etc.)
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
//
// Cache breakpoint placement (BP2) uses the explicit __CACHE_BOUNDARY__ marker:
// everything before it is stable and cached; everything after is volatile and uncached.
// If no boundary marker is found, falls back to caching the last block.
func (e *Engine) buildSystemBlocks(blocks []sessionMod.NamedBlock) []anthropic.TextBlockParam {
	var stableTexts []string
	var systemBlocks []anthropic.TextBlockParam
	cacheIdx := -1

	for _, b := range blocks {
		if b.ID == cacheBoundaryID {
			// Mark the last emitted block as the cache boundary
			cacheIdx = len(systemBlocks) - 1
			continue
		}
		if b.Text == "" {
			continue
		}
		systemBlocks = append(systemBlocks, anthropic.TextBlockParam{Text: b.Text})

		// Track stable block texts (before boundary) for hash comparison
		if cacheIdx < 0 {
			stableTexts = append(stableTexts, b.Text)
		}
	}

	// Fallback: if no boundary marker, cache last block
	if cacheIdx < 0 {
		cacheIdx = len(systemBlocks) - 1
	}

	// Deterministic prefix check: if stable blocks haven't changed since last turn,
	// reuse the exact same TextBlockParam slice to guarantee byte-identical prefix.
	if cacheIdx >= 0 && len(stableTexts) > 0 {
		hash := hashStrings(stableTexts)
		if e.lastStablePrefix != nil && e.lastStablePrefix.hash == hash {
			// Stable prefix unchanged — reuse cached blocks for byte-identical prefix
			reused := make([]anthropic.TextBlockParam, 0, len(systemBlocks))
			reused = append(reused, e.lastStablePrefix.blocks...)
			if cacheIdx+1 < len(systemBlocks) {
				reused = append(reused, systemBlocks[cacheIdx+1:]...)
			}
			return reused
		}
		// Store new stable prefix for next comparison
		stableBlocks := make([]anthropic.TextBlockParam, cacheIdx+1)
		copy(stableBlocks, systemBlocks[:cacheIdx+1])
		if cacheIdx < len(stableBlocks) {
			stableBlocks[cacheIdx].CacheControl = anthropic.NewCacheControlEphemeralParam()
		}
		e.lastStablePrefix = &cachedPrefix{hash: hash, blocks: stableBlocks}
	}

	if cacheIdx >= 0 && cacheIdx < len(systemBlocks) {
		systemBlocks[cacheIdx].CacheControl = anthropic.NewCacheControlEphemeralParam()
	}

	return systemBlocks
}

// isVolatileMemoryID checks if a memory ID indicates volatile content
// (file references, recent files) that changes between turns.
func isVolatileMemoryID(id string) bool {
	return strings.HasPrefix(id, "fileref:") ||
		strings.HasPrefix(id, "recent:") ||
		strings.HasPrefix(id, "file:")
}

// isVolatileBlock checks if a named block ID indicates volatile content.
// Used by agent-store's partitionStableFirst for memory ordering.
func isVolatileBlock(id string) bool {
	return isVolatileMemoryID(id) ||
		id == "fleet-status" ||
		id == "server-sessions"
}

// hashStrings produces a SHA-256 hash of concatenated strings,
// used to detect when stable system blocks have changed between turns.
func hashStrings(ss []string) [32]byte {
	h := sha256.New()
	for _, s := range ss {
		h.Write([]byte(s))
		h.Write([]byte{0}) // separator to prevent "ab"+"c" == "a"+"bc"
	}
	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result
}
