# Smart Truncation for Large Tool Results

## Problem

Large tool results (especially errors) eat context unnecessarily:
- 11K token Go compiler error → only need first 20 lines
- Long file reads → often only need specific sections
- Large shell output → usually first/last N lines are sufficient

Current: Model sees full 11K tokens, wastes budget on repeated error messages.

Better: Auto-truncate large results, create reference, allow targeted expansion.

---

## Design: Intelligent Result Truncation

### 1. Auto-Truncate Large Results

When tool result exceeds threshold, automatically:
1. **Truncate** to first N + last M lines (or chars)
2. **Create reference** in memory for full content
3. **Show summary** in context with expansion instructions

```
┌─────────────────────────────────────────────────┐
│ Tool: shell (go build)                          │
│ ❌ Error (11,247 tokens)                        │
│                                                 │
│ [First 500 tokens shown]                        │
│ pkg/handler/router_0.go:10:1: cannot use...    │
│ pkg/handler/router_1.go:13:2: cannot use...    │
│ pkg/handler/router_2.go:16:3: cannot use...    │
│ ...                                             │
│                                                 │
│ [Truncated 10,247 tokens]                       │
│                                                 │
│ 📎 Full output saved as ref:tool-result:abc123  │
│ Use expand_reference(id, mode) to see:          │
│   • lines:N-M    - specific line range          │
│   • search:TEXT  - grep for pattern             │
│   • tail:N       - last N lines                 │
│   • full         - entire output                │
└─────────────────────────────────────────────────┘
```

---

## 2. Content-Aware Truncation Strategies

### Strategy: Error Logs (default for shell errors)

**Pattern:** Repeated similar errors across many files

**Strategy:**
- Show first 10 errors (demonstrates pattern)
- Show last 5 errors (context of failure point)
- Show summary: "... 65 similar errors omitted ..."

**Example:**
```
router_0.go:10: cannot use *Response0 (want Response0)
router_1.go:13: cannot use *Response1 (want Response1)
router_2.go:16: cannot use *Response2 (want Response2)
... (8 more similar errors)

[65 similar errors omitted - all pointer/value type mismatches]

router_77.go:241: cannot use *Response77 (want Response77)
router_78.go:244: cannot use *Response78 (want Response78)
router_79.go:247: cannot use *Response79 (want Response79)

📎 ref:tool-result:abc123 (11,247 tokens)
```

### Strategy: File Contents

**Pattern:** Large file read

**Strategy:**
- Show first 100 lines
- Show structure outline (function/type names)
- Offer targeted expansion

**Example:**
```
[First 100 lines of large_file.go]

... [1,234 more lines]

Structure:
  • func ProcessData() (line 150)
  • type DataProcessor struct (line 200)
  • func (d *DataProcessor) Handle() (line 300)
  ... 15 more definitions

📎 ref:file:large_file.go (3,456 tokens)
Use expand_reference(id, "lines:150-200") for specific section
```

### Strategy: Build Output

**Pattern:** Successful build with lots of package names

**Strategy:**
- First 5 packages
- Last 5 packages
- Summary count

**Example:**
```
Building packages:
  ✓ github.com/kayushkin/inber/agent
  ✓ github.com/kayushkin/inber/cmd/inber
  ✓ github.com/kayushkin/inber/context
  ... (42 packages)
  ✓ github.com/kayushkin/inber/tools
  
Build successful (47 packages, 2.3s)
```

---

## 3. Implementation

### Phase 1: Basic Truncation (1-2 days)

**File:** `session/truncate.go`

```go
type TruncateStrategy string

const (
	StrategyNone       TruncateStrategy = "none"        // no truncation
	StrategyHeadTail   TruncateStrategy = "head-tail"   // first N + last M
	StrategyErrorLog   TruncateStrategy = "error-log"   // smart error deduplication
	StrategyStructure  TruncateStrategy = "structure"   // outline + samples
)

type TruncateConfig struct {
	Threshold     int              // truncate if > N tokens
	HeadTokens    int              // show first N tokens
	TailTokens    int              // show last N tokens
	Strategy      TruncateStrategy // which strategy to use
	CreateRef     bool             // create memory reference
}

type TruncateResult struct {
	Truncated   bool   // was truncation applied?
	Original    string // original content
	Displayed   string // what goes in context
	RefID       string // memory reference ID (if created)
	SavedTokens int    // tokens saved by truncation
}

func TruncateToolResult(toolName, output string, cfg TruncateConfig) TruncateResult
```

### Phase 2: Content-Aware Strategies (2-3 days)

**File:** `session/truncate_strategies.go`

```go
// DetectStrategy analyzes content to pick best truncation strategy
func DetectStrategy(toolName, output string) TruncateStrategy {
	// Shell errors with repeated patterns
	if strings.Contains(output, "cannot use") && 
	   strings.Count(output, "\n") > 100 {
		return StrategyErrorLog
	}
	
	// File reads (based on tool name)
	if toolName == "read_file" {
		return StrategyStructure
	}
	
	// Default: simple head/tail
	return StrategyHeadTail
}

// TruncateErrorLog deduplicates repeated error patterns
func TruncateErrorLog(output string, cfg TruncateConfig) TruncateResult

// TruncateStructure shows outline + key sections
func TruncateStructure(output string, cfg TruncateConfig) TruncateResult
```

### Phase 3: Reference Integration (1 day)

**File:** `memory/tool_result_refs.go`

```go
// CreateToolResultRef stores full output as lazy reference
func (m *Store) CreateToolResultRef(toolID, toolName, output string) (string, error) {
	refID := "tool-result:" + toolID
	
	return m.Save(Memory{
		ID:         refID,
		Content:    fmt.Sprintf("Full %s output (%d tokens)", toolName, EstimateTokens(output)),
		RefType:    "tool-result",
		RefTarget:  output, // stored inline (not on disk)
		IsLazy:     true,
		Importance: 0.3,    // low importance, ephemeral
		ExpiresAt:  time.Now().Add(1 * time.Hour),
		Tags:       []string{"tool-result", toolName},
	})
}
```

### Phase 4: Expansion Tool (1 day)

**Update:** `tools/memory/expand_reference.go`

Add expansion modes:
```go
type ExpandMode struct {
	Mode  string // "full", "lines", "search", "tail"
	Query string // "150-200", "error pattern", "100"
}

// expand_reference(id: "tool-result:abc", mode: "lines:10-30")
// expand_reference(id: "tool-result:abc", mode: "search:router_50")
// expand_reference(id: "tool-result:abc", mode: "tail:20")
```

---

## 4. Configuration

### Default Configs by Agent Role

```go
// Main agent: aggressive truncation (low token budget)
TruncateConfig{
	Threshold:  1000,  // truncate if > 1K tokens
	HeadTokens: 500,   // first 500 tokens
	TailTokens: 200,   // last 200 tokens
	Strategy:   StrategyAuto,
	CreateRef:  true,
}

// Project agent: moderate (needs more context)
TruncateConfig{
	Threshold:  3000,
	HeadTokens: 1500,
	TailTokens: 500,
	Strategy:   StrategyAuto,
	CreateRef:  true,
}

// Run agent: minimal truncation (expects large output)
TruncateConfig{
	Threshold:  5000,
	HeadTokens: 2000,
	TailTokens: 1000,
	Strategy:   StrategyHeadTail,
	CreateRef:  false,
}
```

---

## 5. Benefits

### Token Savings

**Before:**
```
Turn 1: 11,247 token error → full context
Turn 2: User: "fix it"
        Context: 16K system + 11K error + 2K conversation = 29K tokens
```

**After:**
```
Turn 1: 11,247 token error → truncated to 700 tokens, ref created
Turn 2: User: "fix it"
        Context: 16K system + 700 error stub + 2K conversation = 18.7K tokens
        Saved: 10,547 tokens (36% reduction)
```

### Better UX

- **Faster responses** (less input tokens = faster API)
- **Clearer errors** (pattern visible in first 10 lines)
- **Targeted expansion** (only load what's needed)
- **Automatic** (no manual truncation needed)

---

## 6. Open Questions

1. **Where to truncate?**
   - Option A: In `session.LogToolResult()` (before writing to JSONL)
   - Option B: In `Engine.Run()` after tool execution
   - Option C: In tool result hook (flexible, configurable)
   
   **Recommendation:** Option C (hook) - clean separation, configurable per agent

2. **How to detect "search in middle" patterns?**
   - Stack traces: show error location (middle), not entire trace
   - JSON output: show structure, not full data
   - Logs: show error lines, not entire log
   
   **Solution:** Add `StrategyStackTrace`, `StrategyJSON`, etc. in Phase 2

3. **Should we show truncation in prompt?**
   ```
   Note: Large tool results are auto-truncated. Use expand_reference(id, mode)
   to see more. Common modes: lines:N-M, search:TEXT, tail:N, full.
   ```
   
   **Recommendation:** Yes - add to system prompt when truncation is active

---

## 7. Timeline

- **Phase 1 (Basic):** 1-2 days → immediate token savings
- **Phase 2 (Smart):** 2-3 days → better UX for errors/files
- **Phase 3 (Refs):** 1 day → seamless integration with memory system
- **Phase 4 (Expansion):** 1 day → targeted retrieval

**Total:** ~1 week for full implementation

**Quick win:** Implement Phase 1 only (head/tail truncation) → 80% of benefit in 20% of time
