package engine

import (
	"crypto/sha256"
	"fmt"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/memory"
	sessionMod "github.com/kayushkin/inber/session"
)

// sourceRef returns a short string identifying the build (git commit + dirty flag).
// Cached after first call. Goes in the dynamic group so it doesn't bust cache on redeploy.
var (
	sourceRefOnce  sync.Once
	sourceRefValue string
)

func sourceRef() string {
	sourceRefOnce.Do(func() {
		info, ok := debug.ReadBuildInfo()
		if !ok {
			sourceRefValue = "unknown"
			return
		}
		var rev, modified string
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				rev = s.Value
			case "vcs.modified":
				modified = s.Value
			}
		}
		if rev == "" {
			sourceRefValue = "dev"
			return
		}
		if len(rev) > 8 {
			rev = rev[:8]
		}
		sourceRefValue = "inber@" + rev
		if modified == "true" {
			sourceRefValue += "+dirty"
		}
	})
	return sourceRefValue
}

// cachedPrefix stores the hash and blocks of the last stable system prefix,
// allowing us to reuse the exact byte sequence when content hasn't changed.
type cachedPrefix struct {
	hash   [32]byte
	blocks []anthropic.TextBlockParam
}

// BuildSystemPrompt builds a context-aware system prompt as individual named blocks.
//
// Only stable content becomes a system block. The returned blocks are, in order:
//  1. Always-load memories (identity, instructions, tools) — never change
//  2. Tag-matched persistent memories — rarely change
//
// Everything that changes between turns — volatile memories (file refs, recent
// files), fleet status, context injectors and the source ref — is assembled into
// e.Turn.VolatileContext instead and injected into this turn's user message by
// agent.Run. That keeps it after the turn-anchor breakpoint, so writing it can
// never invalidate the cached system prefix.
//
// This replaced an earlier design that kept volatile content in the system
// section and separated the two halves with a "__CACHE_BOUNDARY__" sentinel
// block. There is no sentinel any more: the split is the two destinations above,
// decided by isVolatileMemoryID. Design notes written against the sentinel
// (docs/cache-optimization.md, docs/papers/*) describe the superseded layout —
// "place it before __CACHE_BOUNDARY__" now means "put it in VolatileContext".
//
// A memory lookup that fails is returned as an error rather than as an empty
// prompt. The agent's identity, its instructions and its tool memories all live
// in that lookup, so a turn built without them is not a degraded turn — it is a
// different agent answering. buildTurnContext decides what to do about it.
func (e *Engine) BuildSystemPrompt(userMessage string) ([]sessionMod.NamedBlock, error) {
	if e.MemStore != nil {
		messageTags := memory.AutoTag(userMessage, "user")
		minImportance, tokenBudget := e.contextBudget(userMessage)

		req := memory.AutomaticContextRequest(messageTags, minImportance, tokenBudget)

		memories, tokensUsed, err := e.MemStore.BuildContext(req)
		if err != nil {
			return nil, fmt.Errorf("build context from memory: %w", err)
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

		// Assemble system blocks: ONLY stable content (cached via BP2).
		// Volatile content goes into e.Turn.VolatileContext for injection into the
		// user's message (after BP3), preventing cache busting.
		var blocks []sessionMod.NamedBlock
		blocks = append(blocks, stableBlocks...)

		// Build volatile context string for user message injection
		var volParts []string
		if status := e.buildFleetStatus(); status != "" {
			volParts = append(volParts, status)
		}
		for _, vb := range volatileBlocks {
			volParts = append(volParts, vb.Text)
		}
		for _, injector := range e.contextInjectors {
			if extra := injector(); len(extra) > 0 {
				for _, eb := range extra {
					if eb.Text != "" {
						volParts = append(volParts, eb.Text)
					}
				}
			}
		}
		if ref := sourceRef(); ref != "" {
			volParts = append(volParts, "[source: "+ref+"]")
		}
		if len(volParts) > 0 {
			e.Turn.VolatileContext = "[Context]\n" + strings.Join(volParts, "\n")
		} else {
			e.Turn.VolatileContext = ""
		}

		if e.workspace != nil {
			if err := e.workspace.WriteSystem(blocks); err != nil {
				Log.Warn("failed to write workspace system blocks: %v", err)
			}
		}
		return blocks, nil
	}

	if e.IdentityOverride == "" {
		return nil, nil
	}

	blocks := []sessionMod.NamedBlock{
		{ID: "identity", Text: e.IdentityOverride},
	}

	if e.workspace != nil {
		if err := e.workspace.WriteSystem(blocks); err != nil {
			Log.Warn("failed to write workspace system blocks: %v", err)
		}
	}
	return blocks, nil
}

// buildSystemBlocks converts named blocks to anthropic system blocks with cache control.
//
// All system blocks are now stable (volatile content moved to user message injection).
// BP2 is placed on the last system block. The entire system section is cached.
func (e *Engine) buildSystemBlocks(blocks []sessionMod.NamedBlock) []anthropic.TextBlockParam {
	var stableTexts []string
	var systemBlocks []anthropic.TextBlockParam

	for _, b := range blocks {
		if b.Text == "" {
			continue
		}
		systemBlocks = append(systemBlocks, anthropic.TextBlockParam{Text: b.Text})
		stableTexts = append(stableTexts, b.Text)
	}

	// BP2 on last system block (all are stable now)
	cacheIdx := len(systemBlocks) - 1

	// Deterministic prefix check: if stable blocks haven't changed since last turn,
	// reuse the exact same TextBlockParam slice to guarantee byte-identical prefix.
	if cacheIdx >= 0 && len(stableTexts) > 0 {
		hash := hashStrings(stableTexts)
		if e.Cache.LastStablePrefix != nil && e.Cache.LastStablePrefix.hash == hash {
			// Stable prefix unchanged — reuse cached blocks for byte-identical prefix
			reused := make([]anthropic.TextBlockParam, 0, len(systemBlocks))
			reused = append(reused, e.Cache.LastStablePrefix.blocks...)
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
		e.Cache.LastStablePrefix = &cachedPrefix{hash: hash, blocks: stableBlocks}
	}

	if cacheIdx >= 0 && cacheIdx < len(systemBlocks) {
		systemBlocks[cacheIdx].CacheControl = anthropic.NewCacheControlEphemeralParam()
	}

	return systemBlocks
}

// isVolatileMemoryID checks if a memory ID indicates volatile file content
// that should always be fetched fresh (not cached across turns).
func isVolatileMemoryID(id string) bool {
	return strings.HasPrefix(id, "fileref:") ||
		strings.HasPrefix(id, "recent:") ||
		strings.HasPrefix(id, "file:")
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
