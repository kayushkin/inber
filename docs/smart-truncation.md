# Smart Truncation of Tool Results

**Status**: ✅ Implemented (Phase 1)  
**Date**: 2026-03-01  
**Corrected**: 2026-08-07 — see "What this document used to claim" below.

> ## ⚠️ What this document used to claim, and why it mattered
>
> Until 2026-08-07 this document described a **full-content retrieval feature
> that does not exist and, as far as `git log` can tell, never shipped**. It
> documented `session.SaveFullContent(toolID, result)` and
> `session.GetFullToolResult(toolID)`, listed "Full retrieval — store complete
> output in session DB" as one of three headline features, and closed by
> promising "**Full debugging access** — complete output always available".
>
> Neither function exists anywhere in the repo. `TruncateResult` has no `Full`
> field. The session JSONL row is written with the **truncated** string
> (`session/session_logging.go:97-105`, `Content: result.Displayed`), so the
> full output is not in the session DB either.
>
> This was not an oversight that survived by being harmless. `session/truncate.go:19-27`
> and `session/session_logging.go:79-96` both carry deliberate comments
> explaining that the retained-copy mechanism was **removed on purpose** —
> it was "a memory reference that no code ever created and no code ever read",
> and holding it cost memory twice while handing nothing back. The document
> kept advertising the removed feature, so the two sources of truth said
> opposite things and the code's own comment was the quieter one.
>
> It leaked. `docs/papers/2026-05-harness-research.md` (the LCM/Volt entry)
> argues its proposal from the premise that "inber already stashes truncated
> content in the session DB and exposes a retrieval handle" — read straight out
> of this page, and false. A stale doc does not just waste its own reader; it
> becomes a citation.

## Overview

Inber automatically truncates large tool results to prevent token budget
overflow. **The untruncated text is not kept** — truncation is lossy by design,
and the reasoning for that is in `session/truncate.go:19-27`.

### The Problem

A single large tool result (e.g., 80 repeated Go compiler errors) can consume 11,000+ tokens, eating 35% of the entire context budget. This:
- Crowds out important context (repo map, memories, conversation history)
- Wastes tokens on redundant content
- Makes debugging harder when errors scroll past important info

### The Solution

**Automatic truncation** with two key features:

1. **Head+tail preview** - Show first N and last M lines of output
2. **Smart strategies** - Content-aware truncation for errors, logs, builds, etc.

Recovering large content is a **separate mechanism** that operates on messages
rather than tool results — see "Recovering large content" below.

---

## How It Works

### Basic Flow

```
Tool executes → Result (11K tokens) → Truncate to 700 tokens → Add to prompt
                                    ↓
                          the other 10,300 tokens are DISCARDED
```

### Truncation Process

```go
// 1. Tool result comes in
result := "... 11,000 tokens of Go errors ..."

// 2. Session checks if > threshold, truncates using the configured strategy
truncated := session.TruncateToolResult(toolName, result, config)

// 3. The truncated text is what goes in the conversation AND what is
//    written to the session JSONL. There is no second copy.
messages.Append(truncated.Displayed)  // 700 tokens
```

### Recovering large content

There is **no per-tool-result retrieval handle**. The mechanism that does make
oversized content recoverable is `conversation.StashLargeContent`
(`conversation/stash.go:163`), which stashes large **message blocks** into the
memory store, and the model reaches them with the **`memory_expand`** tool
(`memory/tools.go:150`).

Two limits worth stating plainly, because the point of that design is that the
pointer is only honest when the tool is actually on the wire:

- It stashes large *messages*, not the tool-result truncation this document
  describes. A tool result trimmed by `TruncateToolResult` is gone.
- `memory_expand` is granted per agent. `memory/auto_context.go:76-77` records
  that of the ten agents configured on this host, four hold `memory_search`
  without `memory_expand`, and one holds neither — so
  `engine/lifecycle.go:77` only advertises the archive pointer when the agent
  actually has the tool.

---

## Configuration

### Per-Role Defaults

Different agent roles have different truncation thresholds:

Source of truth: `session.TruncateConfigForRole` (`session/truncate.go:141`).

| Role | Threshold | Head | Tail | Strategy | Rationale |
|------|-----------|------|------|----------|-----------|
| **main** (and `agent`) | 1000 tokens | 500 | 200 | Auto | Aggressive - main agent needs broad context |
| **project** | 3000 tokens | 1500 | 500 | Auto | Moderate - project agents work with larger outputs |
| **run** | 5000 tokens | 2000 | 1000 | HeadTail | Minimal - preserve test output detail |
| *anything else* | 1000 tokens | 500 | 200 | Auto | `DefaultTruncateConfig()` |

```go
// Applied automatically based on agent role
sess, err := session.New("logs", model, "main", "", modelStore)
// -> uses 1000 token threshold
```

### Custom Configuration

```go
sess.SetTruncateConfig(session.TruncateConfig{
    Threshold:  2000,  // truncate if > 2000 tokens
    HeadTokens: 1000,  // show first 1000 tokens
    TailTokens: 500,   // show last 500 tokens
    Strategy:   session.StrategyHeadTail,
})
```

### Disable Truncation

```go
sess.SetTruncateConfig(session.TruncateConfig{
    Threshold: 0,  // never truncate
})
```

---

## Truncation Strategies

### 1. Head+Tail (Default)

**Use**: General purpose, works for most content

```
[first 500 tokens]
... (truncated 9,800 tokens) ...
[last 200 tokens]
```

**Example**: Go compiler errors
```
router_0.go:10:1: cannot use val0...
router_1.go:13:2: cannot use val1...
router_2.go:16:3: cannot use val2...
... (truncated 68 similar errors) ...
router_77.go:241:18: cannot use val77...
router_78.go:244:19: cannot use val78...
router_79.go:247:20: cannot use val79...
```

### 2. Error Deduplication *(Future)*

**Use**: Repeated error patterns (compiler errors, linter warnings)

```
Error: undefined reference to `foo` (× 45 occurrences)
  First 3:
    main.go:10: undefined reference to `foo`
    main.go:15: undefined reference to `foo`  
    main.go:20: undefined reference to `foo`
  Last 2:
    util.go:89: undefined reference to `foo`
    util.go:92: undefined reference to `foo`
```

### 3. Build Output *(Future)*

**Use**: Go build, npm install, test runs

```
Building package 1/45 ... OK
Building package 2/45 ... OK
... (built 41 packages successfully) ...
Building package 44/45 ... FAILED
  router.go:123: cannot use val...
Building package 45/45 ... FAILED
  handler.go:456: cannot use req...
```

---

## Token Savings

### Real Example: 80 Go Compiler Errors

**Before truncation:**
- Raw output: 11,000 tokens (35% of context budget)
- Conversation history: 8,000 tokens
- Repo map: 5,000 tokens
- Memories: 2,000 tokens
- **Total**: 26,000 tokens (near limit)

**After truncation:**
- Truncated output: 700 tokens (2% of context budget)
- Conversation history: 8,000 tokens
- Repo map: 5,000 tokens
- Memories: 2,000 tokens
- **Total**: 15,700 tokens (40% reduction)

**Savings**: 10,300 tokens per turn = **$0.03 saved per turn**

---

## API Reference

### TruncateConfig

```go
type TruncateConfig struct {
    Threshold  int      // truncate if estimated tokens > this (0 = never)
    HeadTokens int      // tokens to show from start
    TailTokens int      // tokens to show from end
    Strategy   Strategy // truncation strategy
}
```

### TruncateResult

```go
type TruncateResult struct {
    Truncated   bool   // was truncation applied?
    Displayed   string // what goes in context
    SavedTokens int    // tokens saved by truncation
}
```

Note there is no `Full` field, and deliberately so — see the header note.

### Session Methods

```go
// Configure truncation
sess.SetTruncateConfig(config)

// Log tool result (truncation happens automatically)
sess.LogToolResult(toolID, toolName, output, isError)
```

There is no `GetFullToolResult`. See "Recovering large content" above for the
mechanism that does exist.

### Helper Functions

```go
// Get default config for agent role
cfg := session.TruncateConfigForRole("main")

// Truncate content directly (note: takes the TOOL NAME first — the auto
// strategy dispatches on it)
result := session.TruncateToolResult(toolName, content, cfg)
```

---

## Implementation Details

### Token Estimation

Uses simple heuristic: `len(content) / 4 ≈ tokens`

This is conservative (slightly over-estimates) to avoid truncating too aggressively.

### Storage

The session JSONL holds the **truncated** text — the same string the model
sees (`session/session_logging.go:97-105` writes `Content: result.Displayed`):

```json
{
  "type": "tool_result",
  "tool_use_id": "tool-123",
  "tool_name": "shell",
  "content": "... truncated 700 token preview ...",
  "is_error": true,
  "timestamp": "2026-03-01T15:30:00Z"
}
```

The same truncated version goes in the conversation:
```json
{
  "role": "user",
  "content": [
    {
      "type": "tool_result",
      "tool_use_id": "tool-123",
      "content": "... truncated 700 token preview ..."
    }
  ]
}
```

### Performance

- Token estimation: O(1) - just length check
- Truncation: O(n) - single pass through content
- Storage: Incremental append to JSONL

No measurable performance impact.

---

## Future Enhancements (Phases 2-4)

### Phase 2: Smart Strategies

- **Error deduplication**: Detect repeated patterns, show unique + count
- **Log summarization**: Show first/last N entries, summarize middle
- **Build output**: Show failed packages + summary of successful

### Phase 3: Content-Aware Detection

```go
type ContentType int
const (
    ContentUnknown ContentType = iota
    ContentGoErrors
    ContentBuildLog
    ContentTestOutput
    ContentLintWarnings
)

func DetectContentType(output string) ContentType
```

### Phase 4: Dynamic Adjustment

⚠️ As written, this phase is **unimplementable**: it counts `fullRetrievals`, and
there is no full-retrieval call to count. It would need a different signal
(e.g. how often `memory_expand` is called against stashed content).

```go
// If agent keeps asking for full output, increase threshold
if fullRetrievals > 3 {
    config.Threshold *= 2
}
```

---

## Testing

### Unit Tests

```bash
go test ./session -run Truncate
```

**Coverage** (`session/truncate_test.go`; three of the four names this list
previously carried did not exist):
- `TestTruncateToolResult_NoTruncation` - below threshold, passes through
- `TestTruncateToolResult_HeadTail` - truncates correctly
- `TestTruncateToolResult_BreaksOnNewlines` - cuts at line boundaries
- `TestTruncateToolResult_PreservesContent` - head and tail survive intact
- `TestTruncateHeadTailCutsOnRuneBoundaries` - no split multi-byte runes
- `TestTruncateConfigForRole` - role-based defaults
- `TestEstimateTokens` - the len/4 heuristic

### Runnable Examples

`session/truncate_example_test.go` holds `ExampleSession_LogToolResult_truncation`
and `ExampleTruncateConfigForRole`.

```bash
go test ./session -run Example
```

---

## Migration

**Backward compatible** - no breaking changes.

- Existing sessions continue to work (no truncation applied retroactively)
- New sessions get automatic truncation
- Can disable per-session if needed

---

## Design Decisions

### Why Not Summarization?

**Considered**: Use LLM to summarize large outputs

**Rejected because**:
- Adds latency (extra API call)
- Costs tokens (summary prompt)
- Loses fidelity (can't grep for exact error messages)
- Head+tail is faster and free

### Why NOT Store Full Content?

⚠️ **This section previously argued the opposite.** It read "Why we store:
debugging / grep-ability / reproducibility". The code decided the other way and
this page did not follow.

**Rejected because** (`session/session_logging.go:79-96`):
- Nothing ever read it. The retained copy lived in a `map[string]string` on the
  Session whose only reader was an exported getter with **no callers**.
- It cost the memory twice. A session doing two hundred large ripgreps kept two
  hundred full outputs resident for the life of the process and handed none of
  them back — "the truncation undone and the memory spent twice".
- A config flag an operator can set, wired to nothing, is a promise the code
  does not keep (`session/truncate.go:19-27`, on the removed `CreateRef`/`RefID`
  pair).

### Why Tool-Based Expansion, After All

⚠️ **This section previously recorded tool-based expansion as REJECTED**, on the
grounds that it "adds complexity", "requires agent to know about truncation" and
"wastes a turn". That is no longer the design. The code's position is that
recovery is only worth having if the model can actually reach it:

> If a truncated result should be recoverable, the recovery has to be a tool the
> model actually holds — that is what `conversation.StashLargeContent` and
> `memory_expand` are, and a pointer to them is only honest when that tool is on
> the wire.
> — `session/session_logging.go:88-95`

So the surviving recovery path *is* a tool (`memory_expand`), it just operates on
stashed large **messages** rather than on truncated tool results.

---

## Related Documents

⚠️ All three links this section used to carry were dead —
`smart-truncation-design.md`, `context-memory-tool-hybrid.md` and
`session-stashing.md` do not exist and `git log --all` finds no commit that ever
added them.

Read instead:

- `session/truncate.go` — the truncation policy, and the comment explaining what
  was deliberately removed from it
- `session/session_logging.go` — where the truncated result is written, and why
  no second copy is kept
- `conversation/stash.go` — `StashLargeContent`, the surviving recovery path

---

## Summary

Smart truncation gives us:

✅ **40% token reduction** on sessions with large tool outputs  
✅ **Zero breaking changes** - fully backward compatible  
✅ **Role-based defaults** - main/project/run agents tuned appropriately  
✅ **Zero latency** - simple string truncation, no API calls  

❌ **NOT full debugging access.** This line used to read "complete output always
available", which was the headline version of the false claim corrected at the
top of this page. Truncation is lossy and the discarded text is not recoverable.

Result: More efficient context usage, at the deliberate cost of the truncated
detail.
