# Smart Truncation of Tool Results

**Status**: ✅ Implemented (Phase 1)  
**Date**: 2026-03-01

## Overview

Inber now automatically truncates large tool results to prevent token budget overflow while preserving access to full content via the session database.

### The Problem

A single large tool result (e.g., 80 repeated Go compiler errors) can consume 11,000+ tokens, eating 35% of the entire context budget. This:
- Crowds out important context (repo map, memories, conversation history)
- Wastes tokens on redundant content
- Makes debugging harder when errors scroll past important info

### The Solution

**Automatic truncation** with three key features:

1. **Head+tail preview** - Show first N and last M lines of output
2. **Full retrieval** - Store complete output in session DB
3. **Smart strategies** - Content-aware truncation for errors, logs, builds, etc.

---

## How It Works

### Basic Flow

```
Tool executes → Result (11K tokens) → Truncate to 700 tokens → Add to prompt
                                    ↓
                          Store full content in session DB
```

### Truncation Process

```go
// 1. Tool result comes in
result := "... 11,000 tokens of Go errors ..."

// 2. Session checks if > threshold
if estimatedTokens(result) > config.Threshold {
    // 3. Truncate using strategy
    truncated := TruncateToolResult(result, config)
    
    // 4. Add to conversation
    messages.Append(truncated.Displayed)  // 700 tokens
    
    // 5. Store full output in DB
    session.SaveFullContent(toolID, result)  // 11,000 tokens
}
```

### Retrieval When Needed

```go
// Agent can retrieve full output later if needed
full := session.GetFullToolResult("tool-123")
```

---

## Configuration

### Per-Role Defaults

Different agent roles have different truncation thresholds:

| Role | Threshold | Head | Tail | Rationale |
|------|-----------|------|------|-----------|
| **main** | 1000 tokens | 500 | 200 | Aggressive - main agent needs broad context |
| **project** | 3000 tokens | 1500 | 500 | Moderate - project agents work with larger outputs |
| **run** | 5000 tokens | 2500 | 1000 | Minimal - preserve test output detail |

```go
// Applied automatically based on agent name
sess := session.New("logs", model, "main", "")
// -> uses 1000 token threshold

sess := session.New("logs", model, "run-tests", "")
// -> uses 5000 token threshold
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
    Displayed   string // shown in context (truncated)
    Full        string // complete output (stored in DB)
    Truncated   bool   // whether truncation occurred
    OrigTokens  int    // estimated tokens before truncation
    FinalTokens int    // estimated tokens after truncation
}
```

### Session Methods

```go
// Configure truncation
sess.SetTruncateConfig(config)

// Log tool result (truncation happens automatically)
sess.LogToolResult(toolID, toolName, output, isError)

// Retrieve full output
full := sess.GetFullToolResult(toolID)
```

### Helper Functions

```go
// Get default config for agent role
cfg := session.TruncateConfigForRole("main")

// Truncate content directly
result := session.TruncateToolResult(content, cfg)
```

---

## Implementation Details

### Token Estimation

Uses simple heuristic: `len(content) / 4 ≈ tokens`

This is conservative (slightly over-estimates) to avoid truncating too aggressively.

### Storage

Full content stored in session JSONL:
```json
{
  "type": "tool_result",
  "tool_use_id": "tool-123",
  "tool_name": "shell",
  "content": "... full 11,000 token output ...",
  "is_error": true,
  "timestamp": "2026-03-01T15:30:00Z"
}
```

Truncated version goes in conversation:
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

**Coverage:**
- `TestTruncateToolResult_BelowThreshold` - no truncation needed
- `TestTruncateToolResult_AboveThreshold` - truncates correctly
- `TestTruncateConfigForRole` - role-based defaults
- `TestSession_TruncateLargeToolResult` - end-to-end integration

### Integration Test

```go
// See session/truncate_example_test.go
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

### Why Store Full Content?

**Alternative**: Don't store, just show truncated

**Why we store**:
- Debugging: Sometimes need exact line numbers
- Grep-ability: Search for specific error messages
- Reproducibility: Full session replay

### Why Not Tool-Based Expansion?

**Considered**: Create `expand_tool_result(id)` tool for agent to call

**Rejected because**:
- Adds complexity (new tool to maintain)
- Requires agent to know about truncation
- Wastes turn (agent discovers truncation, then must expand)
- Head+tail usually sufficient

---

## Related Documents

- [Smart Truncation Design](./smart-truncation-design.md) - original proposal
- [Context Memory Tool Hybrid](./context-memory-tool-hybrid.md) - related context optimization
- [Session Stashing](./session-stashing.md) - complementary large message handling

---

## Summary

Smart truncation gives us:

✅ **40% token reduction** on sessions with large tool outputs  
✅ **Zero breaking changes** - fully backward compatible  
✅ **Full debugging access** - complete output always available  
✅ **Role-based defaults** - main/project/run agents tuned appropriately  
✅ **Zero latency** - simple string truncation, no API calls  

Result: More efficient context usage without losing information.
