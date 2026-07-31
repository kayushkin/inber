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
(All four are in use as of 2026-07-31 — the history section takes two. See the
07-31 entry at the end.)

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
  [0-F-1] frozen zone ── never mutated again
  [F-1] frozen boundary ─────────────── ●BP3
  [F-N] staging zone ── this and the last few turns
  [N] this turn's user message ──────── ●BP4   ← every tool call of the turn reads up to here
  [N+1...] tool_use / tool_result appended by the loop   UNCACHED

Both history breakpoints are live (2026-07-31). The frozen one is the long-lived
entry; the turn anchor is what makes a multi-tool-call turn cost one write instead
of one per round trip.
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

## 2026-05-11 — retarget BP3 to the latest user message

opencode's [PR 26786](https://github.com/sst/opencode/pull/26786) (auto-placement) anchors its message-side breakpoint on the **latest user message**, not the second-to-last message. The named insight: a single user turn expands into multiple assistant↔tool API calls that all share the prefix up to that user message, so a user-anchored BP gets hit by every intra-turn round-trip. Inber's current BP3 ("second-to-last message" — see `engine/build_prompts.go`) misses that win for tool-loop turns.

~~**Action:** retarget BP3 to the latest user message; measure intra-turn cache hit rate on a multi-tool-call turn before/after.~~ **Done — but not as a retarget. See "2026-07-31" below: the turn anchor is a *second* breakpoint, because retargeting the only one would have thrown the frozen boundary away.** Still open from this entry: the `auto`-style "always cache" default (Anthropic's 5m cache write is 1.25×, read is 0.1× — single reuse beats no-cache).

Companion empirical result from [arXiv:2601.06007](https://arxiv.org/abs/2601.06007) ("Don't Break the Cache"): 41–80% API cost reduction and 13–31% TTFT improvement across providers when dynamic content is kept *out* of the cached prefix — already this doc's thesis, but now backed by 500+ session measurements. See `docs/papers/2026-05-harness-research.md` for the writeup.

## 2026-07-01 — volatile *system* content silently busts the *message* cache

goose [PR #10030](https://github.com/block/goose/pull/10030) proves (with an Anthropic isolation
test) that keeping volatile bytes out of the *system* prefix is not sufficient: because Anthropic
hashes `tools → system → messages`, a per-turn block placed even at the **tail of `system`** still
precedes the message breakpoints and re-creates the *message* cache every call. inber has this exact
shape — `engine/turn_prompt.go` keeps volatile blocks (fleet status, recent files, injectors) at the
tail of the `system` array while BP3 caches the latest user message downstream, so the conversation
prefix is being re-created every turn. ~~**Action:** move per-turn volatile blocks out of `system` and
append them as the **last message** (after the final `cache_control`).~~ **Already done when this was
written, and the entry did not check: `engine/turn_prompt.go:131-160` puts every volatile block —
fleet status, volatile memories, injector output, the source ref — into `e.Turn.VolatileContext`
instead of a system block, and `agent/agent_run.go:90-121` splices that into the last user message on
the first call of the turn. The system array carries stable blocks only.** What the 07-30 entry below
adds is that the placement is right and the *authority* is wrong: those bytes arrive labelled as if
the user typed them. Full analysis + measurements: `comparisons/goose.md`
(07-01 entry). Companion paper: TokenPilot ([arXiv:2606.17016](https://arxiv.org/abs/2606.17016),
`papers/2026-06-harness-research.md`) — batch-turn eviction so shrinking context doesn't churn the prefix.

## 2026-07-30 — the API grew a primitive for the trick inber hand-rolled, and it also frees the `tools` array

Anthropic shipped [mid-conversation system messages](https://platform.claude.com/docs/en/build-with-claude/mid-conversation-system-messages)
(generally available) and **mid-conversation tool changes** (beta, header
`mid-conversation-tool-changes-2026-07-01`, [released 2026-07-24](https://platform.claude.com/docs/en/release-notes/api)
with Opus 5). Both exist for exactly the reason the 07-01 entry above describes, and the docs
restate the hashing order verbatim — "`tools`, then `system`, then `messages`". Two distinct wins.
First, a `{"role": "system"}` message appended in `messages` carries **operator-level priority**
while sitting after the cached prefix; the docs are explicit that a `user` message "is treated as
coming from the end user" and that system instructions win when the two conflict. inber already
does the cache half of this and pays the priority half: `engine/turn_prompt.go:130-155` packs fleet
status, volatile memories and injector output into `e.Turn.VolatileContext`, and
`agent/agent_run.go:92-119` splices it into the **last user message** — so inber's own operator
facts arrive labelled as if the user typed them. Second, the beta decouples the `tools` array from
the prefix: declare the full tool set once, never edit it, and use `tool_addition` / `tool_removal`
blocks (which *reference* tools — `tool_reference`, `mcp_tool_reference`, `mcp_toolset_reference`)
to change what is offered from a point in the conversation onward. `defer_loading: true` withholds
a declared tool until an addition surfaces it.

**What inber should consider:**
- Move `VolatileContext` from the last *user* message to a mid-conversation *system* message. Same
  cache behaviour, correct authority — and it removes the current situation where "[Context]" text
  competes with the user's own words. Placement rules are strict and inber's loop already satisfies
  them: the message must immediately follow a `user` turn (a turn carrying `tool_result` blocks
  counts), must not be first, and must not sit between a `tool_use` and its `tool_result`.
- **Do not put tool output, retrieved documents or web content in a system message.** The docs call
  this out as a limitation, and it directly constrains inber: `contextInjectors`
  (`engine/turn_prompt.go:141-147`) is an open extension point, and anything it injects would
  inherit operator authority. Keep third-party bytes in `tool_result`.
- The beta is the real answer to the phase-scoped-toolset question: inber could narrow the offered
  tool surface per phase, or surface an MCP toolset only when a task needs it, without moving the
  prefix. Two caveats worth writing down before anyone scopes work. `defer_loading` is **not** a
  token saving — the full `tools` array is still declared and still hashed, so deferral buys model
  attention and cache stability, not context-window budget; a catalog-budget argument cannot lean
  on it. And it is unavailable on Sonnet 5 (Fable 5, Mythos 5, Opus 4.8, Opus 5 only), so the
  routing layer has to know which models can take the header. inber *can* set it: it talks to the
  Messages API directly through `anthropic-sdk-go` (`go.mod:6`, `agent/agent_run.go:149`), with no
  CLI subprocess in the path — the `claude-cli/2.1.44` string at `agent/clients.go:104` is a
  spoofed user-agent, not a wrapper.
- Mid-conversation system messages are also the clean mechanism for relaying input that arrived
  while a turn was in flight, which inber needs: the autoworker hold gate and the reaper both
  interrupt live sessions. The docs' phrasing guidance is load-bearing — state the fact ("new input
  arrived: X"), do not phrase it as an override of the user.

## 2026-07-30 — the cache has a *floor*, and a short prefix falls through it

[CAPC (arXiv:2607.15516](https://arxiv.org/abs/2607.15516), 07-17) measures Anthropic's cache
directly instead of assuming it and reports that the hit rate is not the ρ=1.0 the compression
literature assumes: on Sonnet 4.6 the cache behaves as **two tiers with a sharp threshold near
3,500 tokens**, below which the hit rate plateaus at **ρ≈0.83** over 30-call sessions. The cost
model built on that predicts — and τ-bench confirms — that per-query compression is *negative ROI*
below a compression ratio of ~6, because a prefix that differs per call cannot be reused at all;
CAPC instead pairs query-agnostic compression with explicit `cache_control` and a **tier-preserving
ratio bound** that forbids compressing the cached prefix down across the threshold. Cheapest of
four strategies in 16/16 LongBench-v2 configs, and the validation set includes a tool-using
assistant with a 94k-token schema prefix (51.7% cheaper at r=3).

**What inber should consider:** this doc optimizes prefix *stability* and has no notion of a
minimum prefix *size*, which is a live gap because inber's prefix is dynamically sized —
`contextBudget()` feeds `BuildContext`, so a small repo, a low-importance turn or a tightened
budget can shrink the stable block set. Add a floor: if the assembled stable prefix lands under
~3,500 tokens, there is little point paying the 1.25× write multiplier, and trimming it further is
actively wrong. Treat the threshold as a number to **re-measure against inber's own model mix**
rather than a constant to hardcode — the paper's figure is Sonnet 4.6 and inber routes across
several models. Corollary for any future compression of tool schemas or recent-files blocks: it
must be query-agnostic (byte-identical across calls in a session), or it converts a cache hit into
a full-price call.

## 2026-07-31 — the turn anchor, and why it is a second breakpoint rather than a retarget

The 05-11 entry above prescribed retargeting BP3 to the latest user message. Doing exactly that
would have been a regression, because by the time anyone acted on it BP3 no longer sat on the
second-to-last message: the frozen/staging design had moved it to the frozen boundary
(`agent/agent.go`, `frozenIdx-1`), which is a *better* place to keep one long-lived entry and a
*worse* place to start a tool loop from. Retargeting would have traded one for the other. Inber
uses three of the four `cache_control` blocks a request may carry — tools, system, history — so the
answer was to spend the fourth: **place two history breakpoints, the frozen boundary and this
turn's opening user message.**

**Why the anchor is where the money is.** A turn is not one API call. The model asks for a tool,
inber runs it, appends the result and calls again, up to fifty times. Every one of those calls
shares the prefix up to the user message that started the turn, because the loop only ever appends
after it. With the breakpoint parked behind the staging zone, each call re-sends the whole staging
zone *and* everything the earlier calls of the same turn appended, at full price. With an anchor on
the turn's own input, the first call writes that prefix and every later call reads it.

**Measured**, `agent/cache_prefix_measure_test.go`, an A/B over one transcript (10 frozen + 8
staging messages, a 6-round-trip turn, ~1k-token tool results):

| | full-price message tokens over the turn |
|---|---|
| frozen boundary only | 49,231 |
| frozen boundary + turn anchor | 21,168 |

and the first call of the turn drops to **zero** tokens outside the cached prefix. That is the
count of tokens sent uncached; the saved ones also move from 1.0× to 0.1×, so the billed
difference is larger than the ratio above.

**Verified on the wire**, not just in tests: `inber-server` built from this tree, `ANTHROPIC_BASE_URL`
pointed at a recording HTTP server that answers three tool calls per turn, seven turns through
`POST /api/run`. Every request carried exactly two history breakpoints and **four** `cache_control`
blocks in total, never five. The frozen boundary sat at message 13 for four turns and moved to 45
when the staging zone flushed; the anchor advanced 14 → 22 → 30 → 38 → 46 → 54 → 62, once per turn,
and did not move during a turn's four calls. That recipe — a fake provider on `ANTHROPIC_BASE_URL`,
no API spend, the live server on :8200 untouched — is the cheapest end-to-end check for anything on
the request path.

**Two things worth not re-deriving.**

- **Pruning is the only thing that shortens the conversation, and it drops from the head**
  (`engine/build.go`), so both indices shift back by exactly the number dropped. Anything that
  falls off the front stops being placed rather than being placed on whatever message inherited the
  index. The frozen index was *not* shifted before this change, so after a hard drop the one
  breakpoint inber had landed on an arbitrary message.
- **The blueprint was reporting a position the request did not use.** `engine/prompt_blueprint.go`
  decided `BP3` by re-stating the rule as `i == len(messages)-2`, which stopped being true the day
  the frozen zone could move it — so the instrument this whole document reasons with had been
  describing a prompt inber was not sending. It now asks
  `agent.HistoryCacheBreakpointIndices`, the single place the rule lives.

Still open from the 05-11 entry: flipping the `auto`-style "always cache" default. Still open from
07-30: moving `VolatileContext` from the last *user* message to a mid-conversation *system* message,
which is an authority fix rather than a cache one.
