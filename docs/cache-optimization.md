# Prompt Caching Optimization

**Status:** Design doc  
**Created:** 2026-04-04  
**Source commits:** inber@07e1244, agent-store@bcd6191  
**References:** Anthropic prompt caching docs, Claude Code source analysis, OpenClaw #49700

## Background

Anthropic's prompt caching uses **prefix matching** — the server compares the
byte-identical prefix of the current request against recently cached prefixes.
Cache hits cost 10x less than uncached input. Cache writes cost 1.25x more.

Key constraints:
- **Max 4 explicit breakpoints** per request
- **Prefix order:** tools → system → messages (server-side, not configurable)
- **5-minute TTL** (default), 1-hour available at higher cost
- Any byte change before a breakpoint invalidates it AND all downstream breakpoints
- First and last tokens in a block get disproportionate model attention (primacy/recency effect)

## Current State (measured)

3 breakpoints: system stable/volatile boundary, last tool, second-to-last message.

Measured on 2 consecutive turns (Sonnet):
```
Turn 1: cache: 0 read, 9211 created (cold — expected)
Turn 2: cache: 2594 read, 6669 created (partial — 72% miss!)
```

Root causes:
1. Fleet status (timestamps) changes every turn → busts system BP → cascades
2. Recent file IDs use random UUIDs → duplicate entries, never deduplicated
3. Volatile blocks sit between system BP and tools BP → tools cache also busts
4. No determinism guarantee on memory selection order

## Token Attention Model

First and last tokens in each section get disproportionate attention weight.
Identity/soul should be the first system block (model reads system before messages).
The user's latest message is naturally last in the request (highest recency attention).

## Optimal Request Layout

Prefix evaluation order is fixed: **tools → system → messages**.
Within each section, order by stability (most stable first).

```
TOOLS (~2200 tok) ── never changes within session
  [0-15] tool definitions
  [15] last tool ────────────────────── ●BP1

SYSTEM ── STATIC GROUP (~2700 tok) ── changes rarely or never
  [0] Identity/soul (1763 tok)          ← primacy position, always-load
  [1] Memory tool instructions (154 tok)  ← always-load
  [2] Tool description summary (761 tok)  ← always-load
  [3+] Persistent memories (~500 tok)     ← tag-matched, mostly stable
  [last stable] ─────────────────────── ●BP2

SYSTEM ── DYNAMIC GROUP (~100-300 tok) ── changes every turn
  [N] Fleet status                        no BP, billed at 1x input
  [N+1] Recent files (deduplicated)       no BP, billed at 1x input
  [N+2] Session context injectors         no BP, billed at 1x input

MESSAGES (~5000+ tok, growing) ── append-only
  [0-N-2] conversation history
  [N-1] second-to-last message ──────── ●BP3
  [N] latest user message                 UNCACHED

4th BP: reserved (future use, e.g., conversation history midpoint for long sessions)
```

Note: Conversation history and recent files are NOT grouped together.
Recent files change content every turn (age strings update, new files appear).
Conversation history is append-only — previous messages never mutate.
Grouping them would force the append-only history to bust cache whenever
a file ref changes. Keeping them separate means:
- History (after BP3) gets incremental cache hits on the growing prefix
- Recent files (after BP2, no BP) are cheap volatile input that doesn't bust anything

## Implementation Plan

### 1. Fix duplicate recent files (agent-store)

**File:** `agent-store/memory/prepare.go`  
**Bug:** `loadRecentFiles` uses `recent:` + random UUID as memory ID.
Same file gets multiple entries, never deduplicated.

**Fix:** Use deterministic ID: `recent:<relative-path>`  
```go
ID: "recent:" + f.RelativePath,  // was: "recent:" + uuid.NewString()
```
This makes Save() upsert on the same key.

### 2. Explicit boundary marker in BuildSystemPrompt (inber)

**File:** `engine/build_prompts.go`  
**Current:** Returns flat `[]NamedBlock`, volatile detection is heuristic.

**Change:** Insert an explicit sentinel block ID `__CACHE_BOUNDARY__`
after the last stable memory. buildSystemBlocks() splits on this marker
instead of scanning for volatile prefixes.

```go
// After memory loop, before fleet status:
blocks = append(blocks, sessionMod.NamedBlock{
    ID: "__CACHE_BOUNDARY__", Text: "",
})
// Then fleet status, recent files, context injectors
```

### 3. Reorder system blocks by stability (inber)

**File:** `engine/build_prompts.go`

Current order from BuildContext: tag-scored, partitioned (stable first, volatile last).
But fleet status and context injectors are appended AFTER the memory partition,
mixing volatile and "after-stable" content.

Change: Ensure order is:
1. Always-load memories (identity, instructions, tools) — from BuildContext
2. Tag-matched persistent memories — from BuildContext
3. `__CACHE_BOUNDARY__`
4. Fleet status
5. Recent files (from BuildContext volatile partition)
6. Context injectors

### 4. Update buildSystemBlocks to use boundary marker (inber)

**File:** `engine/build_prompts.go`

Replace heuristic `isVolatileBlock()` with boundary marker detection:
```go
func (e *Engine) buildSystemBlocks(blocks []sessionMod.NamedBlock) []anthropic.TextBlockParam {
    var systemBlocks []anthropic.TextBlockParam
    cacheIdx := -1
    for i, b := range blocks {
        if b.ID == "__CACHE_BOUNDARY__" {
            cacheIdx = len(systemBlocks) - 1  // last block before boundary
            continue  // don't emit boundary as a block
        }
        if b.Text == "" {
            continue
        }
        systemBlocks = append(systemBlocks, anthropic.TextBlockParam{Text: b.Text})
    }
    if cacheIdx < 0 {
        cacheIdx = len(systemBlocks) - 1  // fallback: cache everything
    }
    if cacheIdx >= 0 && cacheIdx < len(systemBlocks) {
        systemBlocks[cacheIdx].CacheControl = anthropic.NewCacheControlEphemeralParam()
    }
    return systemBlocks
}
```

### 5. Deterministic system block hashing (inber)

**File:** `engine/build_prompts.go` or `engine/build.go`

Hash the stable system blocks. If hash matches previous turn, reuse
the exact same byte sequence (don't rebuild). This prevents accidental
cache busting from floating-point precision, memory sort order jitter, etc.

```go
type cachedSystemPrefix struct {
    hash   [32]byte
    blocks []anthropic.TextBlockParam
}
```

### 6. Track source commit in system prompt (inber)

**File:** `engine/build_prompts.go`

When the system prompt references code or configuration, include a short
commit hash so we know when the prompt diverges from actual code:

```go
blocks = append(blocks, sessionMod.NamedBlock{
    ID:   "source-ref",
    Text: fmt.Sprintf("[source: inber@%s agent-store@%s]", inberCommit, agentStoreCommit),
})
```

This should go in the dynamic group (after boundary) since it changes on deploy.

## Measured Results (2026-04-04)

Before optimization (Sonnet, 2 turns):
```
Turn 1: 0 read, 9211 created (cold)
Turn 2: 2594 read, 6669 created (72% miss)
```

After optimization (Sonnet, 3 turns on new session):
```
Turn 1: 5390 read, 0 created (100% hit — reused from previous session!)
Turn 2: 2594 read, 3041 created (tools prefix cached, conversation growing)
Turn 3: 2594 read, 3060 created (stable pattern)
```

| Metric | Before | After | Change |
|---|---|---|---|
| Cache writes per turn | 6669 tok | ~3050 tok | **-54%** |
| Cross-session cache hit | 0% | 100% | **new capability** |
| Tools prefix hit rate | partial | 100% | stable |
| Fleet status NULL crash | yes | fixed | |

## Future Considerations

- **Global scope** (CC-style `scope: "global"`): share tool/identity cache across sessions
- **1-hour TTL**: for long-running sessions, pay more per write but fewer misses
- **Cache keepalive pings** (Aider-style): prevent 5-min expiration during idle
- **4th breakpoint**: could split conversation history at a midpoint for very long sessions
  (early history rarely changes, late history grows — two-prefix strategy)
