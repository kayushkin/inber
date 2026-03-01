# Reference-Based Prompt Architecture

## Vision

Instead of passing full content in prompts, use **references** that Sonnet can selectively expand via tool calls based on what's actually needed for the task.

## Current vs. New Architecture

### Current (Direct Content)
```
System Prompt:
┌─────────────────────────────────────┐
│ Identity (3000 tokens)              │
│ Soul (1500 tokens)                  │
│ User info (800 tokens)              │
│ Tool registry (2000 tokens)         │
│ Recent file: agent.go (5000 tokens) │
│ Memory: "Decided to use..." (400)   │
│ Repo map (8000 tokens)              │
└─────────────────────────────────────┘
Total: 20,700 tokens → Claude Sonnet
Cost: $0.062/turn
```

### New (Reference-Based)
```
System Prompt (References):
┌─────────────────────────────────────┐
│ Available References:               │
│ - @ref:identity (3K tokens)         │
│ - @ref:soul (1.5K tokens)           │
│ - @ref:user (800 tokens)            │
│ - @ref:tools (2K tokens)            │
│ - @ref:file:agent.go (5K tokens)    │
│ - @ref:memory:abc123 (400 tokens)   │
│ - @ref:repo-map (8K tokens)         │
│                                     │
│ Use expand_reference tool to load   │
│ content as needed for your task.    │
└─────────────────────────────────────┘
Total: ~2,000 tokens → Claude Sonnet

Sonnet thinks: "I need agent.go for this bug fix"
Makes tool call: expand_reference("file:agent.go", "EXPAND_FULL")
Gets content, completes task

Final tokens: ~6,000 (vs 20,700)
Cost: $0.021/turn (66% savings)
```

## 🎯 The Architecture

### Single-Pass: Sonnet with Tool-Based Expansion

**Cost**: ~$0.021 per turn (66% cheaper than sending everything)  
**Tokens**: 2000-6000 input (varies by what's expanded), ~1000 output

#### Step 1: Lightweight Prompt with References

Sonnet receives:
```
User Query: "Fix the bug in agent.go where tool calls hang"

Available References (use expand_reference tool to load content):
- @ref:identity (3000 tokens) - Your identity and personality
- @ref:file:agent.go (4200 tokens) - Agent orchestration
- @ref:file:tools.go (2800 tokens) - Tool execution
- @ref:repo-map:agent (8000 tokens) - Agent package structure
- @ref:memory:abc123 (1200 tokens) - "Fixed tool timeout issue..."
- @ref:recent-files (600 tokens) - Last 10 modified files
```

#### Step 2: Sonnet Decides What to Expand

```
<thinking>
The bug is in agent.go. I need to see the full file.
The memory about a previous timeout fix is likely relevant.
I'll keep the rest as references for now.
</thinking>
```

#### Step 3: Makes Tool Calls

```xml
<expand_reference>
  <id>file:agent.go</id>
  <mode>EXPAND_FULL</mode>
</expand_reference>

<expand_reference>
  <id>memory:abc123</id>
  <mode>EXPAND_FULL</mode>
</expand_reference>
```

#### Step 4: Receives Content and Completes Task

```
[Expanded: file:agent.go]
<full file content - 4200 tokens>

[Expanded: memory:abc123]  
<full memory content - 1200 tokens>

[Available as references if needed]:
@ref:identity
@ref:file:tools.go
@ref:repo-map:agent
@ref:recent-files
```

## 📊 Reference Types

### Memory References (Pure Thoughts)
```go
memory.NewMemoryReference(
    content: "Decided to use SQLite for persistence because...",
    tags: ["decision", "architecture"],
    importance: 0.85,
)
```
- **Stored in DB**: ✅ Full content
- **IsLazy**: ❌ False (content already in DB)
- **Expanded**: Returns content from DB

### File References (Code Files)
```go
memory.NewFileReference(
    path: "cmd/inber/engine.go",
    summary: "Main orchestration logic",
)
```
- **Stored in DB**: ❌ Only summary
- **IsLazy**: ✅ True (reads from disk)
- **Expanded**: `os.ReadFile()` - always fresh

### Identity References (Config Files)
```go
memory.NewIdentityReference(
    path: ".inber/identity.md",
)
```
- **Stored in DB**: ❌ Only summary
- **IsLazy**: ✅ True (reads from disk)
- **Expanded**: `os.ReadFile()` - always fresh
- **AlwaysLoad**: ✅ True (included in every prompt as reference)

### Repo Map References (Structure)
```go
memory.NewRepoMapReference(
    scope: "agent/",
)
```
- **Stored in DB**: ❌ Only summary
- **IsLazy**: ✅ True (generates on-demand)
- **Expanded**: `BuildRepoMap()` - always fresh
- **ExpiresAt**: 1 hour (stale if code changes)

### Tool Registry References
```go
memory.NewToolRegistryReference(
    tools: allTools,
)
```
- **Stored in DB**: ❌ Only summary
- **IsLazy**: ✅ True (builds from current tools)
- **Expanded**: `BuildToolSchemas()` - always fresh
- **AlwaysLoad**: ✅ True (agent needs to know available tools)

## 🔧 Expansion Modes

### KEEP_REF (10 tokens)
```
@ref:file:agent.go [Main orchestration logic - 4200 tokens available]
```
Just a pointer, no content loaded.

### EXPAND_SUMMARY (50 tokens)
```
@ref:file:agent.go
Summary: Main agent execution loop. Handles tool calls, conversation
management, and session lifecycle. Key functions: Run(), handleTool().
```

### EXPAND_PARTIAL (varies)
```
@ref:file:agent.go [lines 45-120]
<specific section of file>
```

Supports:
- **Lines**: `lines:45-120`
- **Functions**: `function:Run`
- **Package**: `package:agent`

### EXPAND_FULL (full content)
```
@ref:file:agent.go [EXPANDED]
<entire file content>
```

## 💰 Cost Savings

### Traditional Approach (send everything)
- Turn 1: 8K tokens × $3/MTok (Sonnet) = **$0.024**
- Turn 50: 25K tokens × $3/MTok (Sonnet) = **$0.075**
- **50-turn session: ~$2.90**

### With Reference Optimization (Sonnet + tool expansion)
- Lightweight prompt: 2K tokens × $3/MTok = **$0.006**
- Typical expansion: 4K tokens × $3/MTok = **$0.012**
- Response: 1K tokens × $15/MTok = **$0.015**
- **Per turn: ~$0.021** (70% cheaper for complex turns)
- **50-turn session: ~$1.52**

**Savings**: $1.38 per session, 48% cost reduction

### Why Not Two-Pass with Haiku?

Initial design used Haiku as optimizer, but math showed:
- **Haiku two-pass**: $0.035/turn (pass 1: Haiku decision, pass 2: Sonnet execution)
- **Sonnet single-pass**: $0.021/turn (Sonnet decides + executes)
- **Result**: Two-pass is 68% MORE expensive

Most session costs come from simple turns with full context anyway (for personality/awareness), not optimization decisions. Better to have the smartest model (Sonnet) decide what context it needs.

## 🎁 Benefits

### 1. Token Efficiency
- **Simple turns**: Still get full context for personality
- **Complex turns**: Only load what's needed (70% savings)
- **Overall**: 48% cost reduction per session

### 2. Always Fresh
- Files read from disk on each expansion
- Repo maps generated on-demand
- No stale data issues

### 3. Better Context Decisions
- Smartest model (Sonnet) decides what it needs
- Can expand more references mid-task if needed
- Adaptive to task complexity

### 4. Simpler Architecture
- One model, not coordination between two
- Tool-based expansion (existing pattern)
- No separate optimizer service

## 🏗️ Implementation Phases

### Phase 1: Memory-Reference Integration (Week 1)
- [ ] Add reference fields to `Memory` struct (`ref_type`, `ref_target`, `is_lazy`)
- [ ] Update schema with new columns
- [ ] Create reference helpers (`NewFileRef`, `NewMemoryRef`, etc.)
- [ ] Implement `loadContent()` dispatcher for lazy loading

### Phase 2: Auto-Reference Creation (Week 1-2)
- [ ] Hook into `read_file` to create file references
- [ ] Hook into `repo_map` to create repo-map references
- [ ] Create identity/soul/user references at session start
- [ ] Add tool registry reference with AlwaysLoad

### Phase 3: Reference-Based Prompt Builder (Week 2-3)
- [ ] Build `PromptBuilder` that creates reference-heavy prompts
- [ ] Implement reference list formatting with summaries
- [ ] Add token counting for references
- [ ] Track reference usage statistics

### Phase 4: Expansion Tool (Week 3)
- [ ] Create `expand_reference` tool for Sonnet
- [ ] Implement expansion modes (KEEP_REF, EXPAND_SUMMARY, EXPAND_PARTIAL, EXPAND_FULL)
- [ ] Add partial expansion (lines, sections)
- [ ] Add caching for recently expanded content

### Phase 5: Integration & Testing (Week 4)
- [ ] Update engine to use reference-based prompts
- [ ] Modify context builder to use unified memory store
- [ ] Performance testing and token usage analysis
- [ ] Documentation and examples

## 📝 Example Session

```
Turn 1: "Hi!"
├─ Prompt: Full context (identity, repo, recent work) - 8K tokens
├─ Response: Greeting with awareness - 100 tokens
└─ Cost: $0.025 (simple turn, full context for personality)

Turn 15: "Fix bug in agent.go where tools hang"
├─ Prompt: Reference list - 2K tokens
├─ Tool call: expand_reference("file:agent.go") - 4K tokens
├─ Tool call: expand_reference("memory:timeout-fix") - 1K tokens
├─ Response: Bug fix with explanation - 800 tokens
└─ Cost: $0.021 (complex turn, selective expansion)

Turn 16: "Looks good"
├─ Prompt: Full context - 8K tokens  
├─ Response: Confirmation with awareness - 50 tokens
└─ Cost: $0.025 (simple turn, full context maintained)

Session total: ~$1.52 (vs $2.90 without optimization)
```

## 🎯 Key Design Decisions

1. **No separate optimizer model**: Sonnet decides what it needs
2. **Tool-based expansion**: Uses existing tool call pattern
3. **Lazy loading**: Content loaded on-demand, never stale
4. **Simple turns unchanged**: Full context for personality/awareness
5. **Complex turns optimized**: 70% token reduction where it matters

## 🚀 Next Steps

Ready to start implementation with Phase 1: Memory-Reference Integration.
