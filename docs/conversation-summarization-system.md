# Conversation Summarization & Pruning System

## Current Implementation Status: ✅ ACTIVE

The conversation summarization system is **already implemented and working** in inber. Here's exactly how it works:

---

## How It Works

### 1. **Trigger Mechanism**

Pruning triggers in **two places**:

#### A. **Before Each API Request** (cmd/inber/engine.go:474)
```go
// In BeforeRequest hook:
if ShouldPrune(messages, cfg) {
    pruned, result, err := PruneConversation(context.Background(), messages, e.MemStore, "", cfg)
    if err == nil {
        messages = pruned
        Log.Info("pruned %d messages (%d tokens freed, %d memories saved)",
            result.PrunedMessages, result.TokensFreed, result.MemoriesSaved)
    }
}
```

#### B. **During Checkpoints** (cmd/inber/engine.go:754)
```go
// When saving checkpoint:
if !ShouldPrune(e.Messages, cfg) {
    return nil // no pruning needed
}

pruned, result, err := PruneConversation(
    context.Background(),
    e.Messages,
    e.MemStore,
    e.Session.SessionID(),
    cfg,
)
```

### 2. **When Pruning Triggers**

`ShouldPrune()` returns true when:
- **Message count** > 70 (2x KeepRecentTurns of 35)
- OR **Token budget** > 50,000 tokens

```go
func ShouldPrune(messages []anthropic.MessageParam, cfg PruneConfig) bool {
    if len(messages) > cfg.KeepRecentTurns*2 {  // > 70 messages
        return true
    }
    
    if len(messages) <= cfg.KeepRecentTurns {  // <= 35 messages
        return false
    }
    
    totalTokens := estimateMessageTokens(messages)
    return totalTokens > cfg.TokenBudget  // > 50K tokens
}
```

---

## Pruning Strategy (Role-Based)

### Default Configuration (DefaultPruneConfig)
```go
KeepRecentTurns:        35   // Keep last 35 turns in full
AssistantTruncateAfter: 10   // Truncate assistant messages older than 10 turns
ToolResultKeepFull:     3    // Keep tool results in full for last 3 turns
ToolResultSummary:      10   // Summarize tool results 3-10 turns ago
ToolResultDrop:         10   // Drop tool results older than 10 turns
ToolCallKeepFull:       5    // Keep tool call inputs in full for last 5 turns
AutoSaveThreshold:      500  // Auto-save assistant messages with 500+ tokens
MemorySaveThreshold:    10   // Save to memory if pruning removes 10+ turns
TokenBudget:            50000
MinimumImportance:      0.3
```

### What Gets Pruned

#### **User Messages**: NEVER truncated
- User input is always kept in full
- Tool results within user messages ARE pruned (see below)

#### **Assistant Messages** (older than 10 turns):
- **Truncated to first 2-3 sentences**
- Example:
  ```
  Original (500 tokens):
  "I've implemented the authentication system using JWT tokens. 
   The login flow works like this: [... detailed explanation ...]"
  
  Truncated (50 tokens):
  "I've implemented the authentication system using JWT tokens. 
   The login flow works like this."
  ```

#### **Tool Results**:
- **Last 3 turns**: Full content kept
- **3-10 turns ago**: Summarized
  - File reads: `[shell: 150 lines] package main...`
  - Shell output: `[shell: 23 lines] go build successful`
  - Repo map: `[repo: 15 packages, 142 files]`
- **10+ turns ago**: Dropped
  - Replaced with: `[result dropped - too old]`

#### **Tool Calls**:
- **Last 5 turns**: Full input JSON kept
- **5+ turns ago**: Summarized
  - `read_file: {"path": "/very/long/path/file.go", "offset": 100, "limit": 50}`
  - Becomes: `read_file: {"path": "/very/long/path/file...`

---

## Auto-Save to Memory (Before Pruning)

**Critical feature**: Before pruning, important content is automatically saved to memory for later retrieval.

### What Gets Auto-Saved

1. **Assistant messages** older than 10 turns with 500+ tokens
2. **Must contain decision indicators**:
   - "decided to"
   - "choosing"
   - "will use"
   - "plan is to"
   - "implemented"
   - "created"
   - "built"
   - "fixed"
   - "important:"
   - "note:"
   - "remember:"

3. **Importance scoring**:
   - Base: 0.5
   - Contains "important": 0.7
   - Only saved if >= MinimumImportance (0.3)

### Example

**Original assistant message** (turn 25, will be truncated):
```
"I've decided to use SQLite for the memory store instead of PostgreSQL 
because it's simpler to deploy and we don't need concurrent writes. 

Important: The schema uses WITHOUT ROWID optimization for better 
performance on tag queries. Remember to add indexes on (tag, importance)."
```

**Before pruning**:
1. Extract key sentences: "I've decided to use SQLite... Remember to add indexes..."
2. Save to memory:
   ```go
   memory.Memory{
       Content: "I've decided to use SQLite for the memory store... indexes on (tag, importance).",
       Tags: ["auto-saved", "decision", "session-2024-12-15_143022"],
       Importance: 0.7,  // Contains "important"
       Source: "pruning",
   }
   ```
3. Truncate message in conversation to summary

**Result**: Full context is preserved in memory, conversation stays lean.

---

## Memory Retrieval During Context Building

When building context for the next turn, memories are retrieved:

```go
// In BuildSystemPrompt():
req := memory.BuildContextRequest{
    Tags:              messageTags,       // Auto-tagged from user message
    TokenBudget:       6000,              // Default budget
    MinImportance:     0.0,               // Load all matching memories
    IncludeAlwaysLoad: true,              // Identity, tools, etc.
    ExcludeTags:       []string{"session-summary"},
}

memories, tokensUsed, err := e.MemStore.BuildContext(req)
```

**If the user asks about a pruned decision**:
1. User: "Why did we choose SQLite?"
2. Tags extracted: ["sqlite", "database", "choice"]
3. Memory search finds auto-saved decision (tag: "decision")
4. Full context included in prompt
5. Agent can answer accurately

---

## Token Savings

### Example Session (50 turns)

**Without Pruning:**
- Turn 1: 6K conversation
- Turn 10: 15K conversation
- Turn 25: 40K conversation
- Turn 50: 80K+ conversation
- **Total**: ~80,000+ tokens per turn at turn 50

**With Pruning (Current System):**
- Turn 1-35: Full conversation (no pruning)
- Turn 36-70: Pruning not triggered yet (< 70 messages)
- Turn 71+: Pruning kicks in
  - Keep last 35 turns in full: ~20K tokens
  - Older turns truncated: ~5K tokens
  - Tool results summarized/dropped: ~3K tokens
  - **Total**: ~28,000 tokens per turn

**Savings at turn 71+**: ~65% reduction (80K → 28K)

### Why Pruning Triggers Late (Turn 70+)

**By design**: You want full context for most sessions.

- Most sessions are < 35 turns → no pruning ever needed
- Medium sessions (35-70 turns) → conversation growing but manageable
- Long sessions (70+ turns) → pruning kicks in to prevent bloat

**Philosophy**: Don't optimize away context prematurely. Let short/medium sessions have full history for personality and coherence.

---

## Role-Specific Configs

Different agent roles have different needs:

### **Orchestrator** (delegates work to sub-agents)
```go
KeepRecentTurns:        40   // More history (coordinating multiple agents)
ToolResultDrop:         8    // Drop tool results faster (lots of delegation)
AssistantTruncateAfter: 8    // Truncate faster (focus on delegation, not details)
```

### **Coder** (implements features)
```go
KeepRecentTurns:        20   // Less history needed
ToolResultKeepFull:     10   // Keep file reads longer (referencing code)
ToolResultSummary:      20   // Keep tool results longer
```

### **Tester** (validates code)
```go
ToolResultKeepFull:     15   // Keep test output in full longer
ToolResultSummary:      25   // Summarize test results slowly
```

**Config is auto-detected** from agent name/role in agents.json.

---

## Current Limitations & Future Improvements

### ✅ **What Works Well**
1. Auto-saves important decisions before pruning
2. Role-based strategies for different agent types
3. Preserves user messages (never truncated)
4. Smart tool result handling (keep recent, summarize old, drop ancient)
5. Memory retrieval brings back pruned context when needed

### ⚠️ **Known Issues**
1. **Pruning triggers late** (turn 70+)
   - By design, but could be more aggressive for very long sessions
   - **Fix**: Add token-based early trigger (turn 40+ AND 30K+ tokens)

2. **No LLM-based summarization**
   - Current: Simple truncation (first 2-3 sentences)
   - **Future**: Use Claude to generate summaries
   ```go
   // Instead of truncateToSummary():
   summary := claude.Summarize(oldMessages, "Extract key decisions and actions")
   ```

3. **Memory retrieval relies on good tags**
   - If auto-tagging misses a relevant tag, pruned context won't be retrieved
   - **Fix**: Improve AutoTag() to extract more semantic tags

4. **No session-level summaries**
   - **Future**: After checkpoint, generate session summary
   ```go
   sessionSummary := claude.Summarize(allMessages, "Summarize this work session")
   memory.Save(Memory{
       Content: sessionSummary,
       Tags: ["session-summary", sessionID],
       Importance: 0.8,
   })
   ```

---

## How to Monitor Pruning

### 1. **Log Output**
```bash
inber chat

# When pruning triggers:
[INFO] pruned 35 messages (42,314 tokens freed, 3 memories saved)
```

### 2. **Session Files**
Check `logs/<agent>/<session>/messages.json`:
```json
[
  {"role": "user", "content": "..."},
  {"role": "assistant", "content": "[TRUNCATED]..."},  // Pruned message
  {"role": "assistant", "content": "..."}  // Recent, still full
]
```

### 3. **Memory Store**
Check `.inber/memory.db`:
```sql
SELECT * FROM memories WHERE tags LIKE '%auto-saved%';
```

---

## Recommended Next Steps

### **Phase 1: Lower Pruning Threshold** (Quick Win)
Make pruning trigger earlier for very long sessions:

```go
// In ShouldPrune():
if len(messages) > 80 || (len(messages) > 40 && totalTokens > 30000) {
    return true
}
```

**Impact**: Pruning kicks in at turn 40 instead of turn 70 for token-heavy sessions.

### **Phase 2: LLM-Based Summarization** (Higher Quality)
Use Claude to generate summaries instead of simple truncation:

```go
func summarizeWithLLM(messages []anthropic.MessageParam) string {
    // Call Claude with minimal context
    return claude.Messages([]Message{
        {Role: "user", Content: "Summarize these messages in 2-3 sentences: " + format(messages)},
    })
}
```

**Impact**: Better summaries, more semantic compression.

### **Phase 3: Session Summaries** (Long-Term Memory)
After each checkpoint, generate session-level summary:

```go
func saveSessionSummary(messages []anthropic.MessageParam, memStore *memory.Store) {
    summary := claude.Summarize(messages, "What was accomplished in this session?")
    memStore.Save(Memory{
        Content: summary,
        Tags: ["session-summary", sessionID],
        Importance: 0.8,
    })
}
```

**Impact**: Can reference entire past sessions in 200-300 tokens instead of 20K.

---

## Summary

**Your conversation summarization system is already working!** 

It just doesn't kick in until turn 70+ because:
1. You want full context for short/medium sessions (personality, coherence)
2. Most sessions don't hit 70 turns
3. When they do, pruning saves ~65% tokens (80K → 28K per turn)

**Important decisions are auto-saved to memory before pruning**, so nothing is lost—just moved from conversation history to the memory store where it can be retrieved on-demand.

**If you want more aggressive pruning**: Lower the threshold to trigger at turn 40 for token-heavy sessions. But the current design is intentional and aligns with your philosophy: *Don't optimize away personality for minor savings.*
