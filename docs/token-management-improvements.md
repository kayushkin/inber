# Token Management Analysis & Improvement Opportunities

## Design Philosophy

### ⚠️ **Important: Don't Optimize Away Personality**

**Simple turns should NOT get reduced context budgets.**

Even simple responses like "yes", "ok", or greetings need full context to:
1. Understand implications and subtext
2. Maintain personality and contextual awareness
3. Build relationship over time

**Priority:** Save 10K+ tokens per turn on LARGE sessions (turn 10+), not 3K on occasional simple greetings.

Focus on:
- ✅ Conversation summarization for long sessions
- ✅ Sliding windows for old conversation history
- ✅ Efficient memory retrieval
- ❌ NOT reducing budget for simple turns

---

## Current System

### Token Budget (Dynamic)
- **Turn 1**: 4K tokens (base - just identity)
- **Normal turns**: 6K tokens
- **Moderate complexity (300+ token message)**: 10K tokens  
- **Complex message (1000+ tokens)**: 15K tokens
- **Long session (15+ turns)**: 8K tokens
- **Errors**: 20K → 35K → 50K (escalating)

### What's Good ✅
1. **Adaptive budgets** - scales with need
2. **Error escalation** - more context when stuck
3. **AlwaysLoad** - identity is always included
4. **Tool-based on-demand loading** - repo_map(), recent_files()

### Current Optimizations Already Implemented
- Compact repo map (49% reduction)
- File stubs instead of full content
- Conversation summarization (NOT ACTIVE YET)
- Smart filtering (large chunks need multiple matching tags)
- Deduplication

---

## Problems & Opportunities

### 1. **No Conversation Pruning After Turn 1** 🔥

**Problem:** Every turn includes ALL previous conversation history.
- Turn 1: User(100) + Assistant(200) = 300 tokens
- Turn 2: 300 + User(100) + Assistant(200) = 600 tokens
- Turn 10: Thousands of tokens from old turns

**Impact:**
- Simple "yes/no" answers still load 10+ previous turns
- Short turns waste budget on irrelevant history

**Solution:** Implement **Sliding Window + Summarization**
```go
type ConversationWindow struct {
    KeepLastNTurns int // Keep full content for last 5 turns
    SummarizeOlderAs string // "Turn 1-5: Built context system..."
}
```

**Savings:** 40-60% on turns 10+

---

### 2. **Conversation Summarizer Not Active** 🔥

We built `context/summarize.go` but it's **never called**.

**Solution:** Wire it into the engine:
```go
// In engine after each turn:
if e.TurnCounter > 0 && e.TurnCounter % 10 == 0 {
    summarizer := context.NewConversationSummarizer(store, 10)
    summarizer.CompactConversationHistory()
}
```

**Savings:** 30-50% on long sessions

---

### 3. **No Turn-Level Budget Adjustment** ⚠️

Simple turns get the same budget as complex ones:
- "yes" → 6000 token budget (waste)
- "Implement X with Y and Z" → 6000 tokens (might need more)

**Solution:** Detect simple responses
```go
func isSimpleTurn(userMessage string) bool {
    msg := strings.ToLower(strings.TrimSpace(userMessage))
    
    // One word responses
    if len(strings.Fields(msg)) <= 3 {
        return true
    }
    
    // Common simple patterns
    simple := []string{"yes", "no", "ok", "thanks", "done", "continue", "go ahead"}
    for _, s := range simple {
        if msg == s {
            return true
        }
    }
    
    return false
}

// In contextBudget():
case isSimpleTurn(userMessage):
    return 0, 3000 // Minimal - just recent context
```

**Savings:** 50% on simple turns (common in long sessions)

---

### 4. **Recent File Stubs Expire Too Quickly** ⚠️

Recent files have 10min TTL - but they expire even if still relevant.

**Solution:** Refresh-on-access pattern
```go
// When loading a recent file stub, refresh its ExpiresAt
if memory.Tags.Contains("recent") && time.Until(memory.ExpiresAt) < 5*time.Minute {
    memory.ExpiresAt = time.Now().Add(10 * time.Minute)
    store.Save(memory)
}
```

**Benefit:** Keep relevant context without re-loading

---

### 5. **No "Focus Mode" for File-Specific Work** 💡

When working on a single file across many turns:
- Turn 1: "Fix bug in agent.go"
- Turns 2-10: All about agent.go

**Problem:** We keep loading broad context when we only need agent.go

**Solution:** Detect file-focused sessions
```go
func detectFocusedFile(recentMessages []string) (string, bool) {
    filePattern := regexp.MustCompile(`\b(\w+/)*\w+\.\w+\b`)
    fileCounts := make(map[string]int)
    
    for _, msg := range recentMessages {
        for _, file := range filePattern.FindAllString(msg, -1) {
            fileCounts[file]++
        }
    }
    
    // If one file dominates (50%+ mentions)
    for file, count := range fileCounts {
        if count >= len(recentMessages)/2 {
            return file, true
        }
    }
    
    return "", false
}

// In BuildSystemPrompt():
if focusFile, ok := detectFocusedFile(last5Messages); ok {
    // Override tags to focus on this file
    tags = []string{"file:" + focusFile, "pkg:" + filepath.Dir(focusFile)}
    tokenBudget = 8000 // smaller, focused budget
}
```

**Savings:** 30-40% when working on single file

---

### 6. **Tool Results in Conversation History** 💡

Tool results (repo maps, file reads) get stored in conversation:
- Turn 1: repo_map() → 5000 tokens
- Turn 5: Still in history (but likely not needed)

**Solution:** Mark tool results as ephemeral
```go
// In HandleToolCall:
if toolName == "repo_map" || toolName == "recent_files" {
    // Don't include in conversation history after 2 turns
    markAsEphemeral(toolResult, expiresAfterTurns: 2)
}
```

**Savings:** 20-30% on tool-heavy sessions

---

### 7. **No Prompt Caching Strategy** 💡

Anthropic supports prompt caching - identical prefixes across turns get cached.

**Current:** Every turn rebuilds system prompt → no caching

**Solution:** Stable prefix pattern
```go
systemBlocks := []Block{
    {ID: "identity", Text: identity},        // STABLE
    {ID: "tools", Text: toolRegistry},       // STABLE
    {ID: "memories", Text: topMemories},     // CHANGES
    {ID: "recent-context", Text: recentWork}, // CHANGES
}

// Mark stable blocks with cache_control
params := anthropic.MessageNewParams{
    System: []anthropic.SystemBlock{
        anthropic.NewTextBlock("identity", CacheControl: "ephemeral"),
        anthropic.NewTextBlock("tools", CacheControl: "ephemeral"),
        anthropic.NewTextBlock("memories", nil), // not cached
    },
}
```

**Savings:** 50-90% on repeated turns (billed at 10% cost for cached content)

---

## Recommended Implementation Order

### **Phase 1: High-Impact Wins (1-2 hours)** 🎯
1. ✅ **Activate conversation summarizer** - it's already built!
   - Wire into engine after turn 10, 20, 30...
   - Save full context to memory for retrieval
   - Saves 30-50% immediately

2. ✅ **Sliding conversation window** - keep last 5 turns, summarize older
   - Modify message building to only include recent turns
   - Save summaries to memory with proper tags
   - Saves 40-60% on turn 10+

### **Phase 2: Medium Impact (3-4 hours)**
4. **Tool result expiration** - mark repo_map/recent_files as ephemeral
   - Add ExpiresAfterTurns to tool results
   - Remove from conversation after N turns
   - Saves 20-30% on tool-heavy sessions

5. **File focus detection** - detect when working on single file
   - Build file mention tracker
   - Override tags when focused
   - Saves 30-40% on focused work

### **Phase 3: Advanced (4-6 hours)**
6. **Prompt caching** - stable prefix pattern
   - Restructure system blocks
   - Add cache_control markers
   - Saves 50-90% on cache hits

7. **Refresh-on-access** - extend TTL for actively used memories
   - Update ExpiresAt when memory accessed
   - Keeps relevant context fresh

---

## Expected Impact

**Current:** 
- Turn 1: 6K tokens (context) + conversation
- Turn 10: 6K tokens + ~8K conversation = 14K
- Turn 20: 8K tokens + ~15K conversation = 23K

**After Phase 1:**
- Turn 1: 6K + conversation = 6K
- Turn 10: 6K + 2K (last 5 turns) = 8K (-43%)
- Turn 20: 8K + 2K (window) = 10K (-57%)
- Greetings: FULL CONTEXT (personality > tokens)

**After Phase 2:**
- Focused sessions: 5K + 2K = 7K (-70% vs current turn 20)
- Tool-heavy: 6K + 2K = 8K (-65%)

**After Phase 3 (with caching):**
- Cached turns: ~1.5K billable (90% cache hit) (-93%)

---

## Want me to implement Phase 1 now?

The quick wins would give you immediate 30-50% savings on long sessions.
