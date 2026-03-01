# Reference-Based Prompt Architecture

## Vision

Instead of passing full content in prompts, use **references** that can be selectively expanded by a lightweight "prompt optimizer" model before sending to the main model.

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
Total: 20,700 tokens → Claude Opus
```

### New (Reference-Based)
```
System Prompt (References):
┌─────────────────────────────────────┐
│ @ref:identity.md                    │
│ @ref:soul.md                        │
│ @ref:user.md                        │
│ @ref:tools/registry                 │
│ @ref:file:agent.go                  │
│ @ref:memory:187ce8b (sqlite design) │
│ @ref:repo-map:current               │
└─────────────────────────────────────┘
Total: ~500 tokens → Haiku (optimizer)
                   ↓
Haiku analyzes user query + references
                   ↓
Expanded Prompt:
┌─────────────────────────────────────┐
│ @ref:identity.md [KEEP_REF]         │
│ @ref:soul.md [KEEP_REF]             │
│ @ref:user.md [EXPAND]               │
│   Content: "User is Slava..."       │
│ @ref:tools/registry [KEEP_REF]      │
│ @ref:file:agent.go [EXPAND_LINES]   │
│   Lines 45-120: "func Run()..."     │
│ @ref:memory:187ce8b [EXPAND]        │
│   Content: "Decided to use SQLite..." │
│ @ref:repo-map:current [KEEP_REF]    │
└─────────────────────────────────────┘
Total: ~6,000 tokens → Claude Opus
```

**Savings**: 20.7K → 6K = **71% reduction**

---

## Reference Types

### 1. **File References** (`@ref:file:<path>`)
```json
{
  "ref_type": "file",
  "ref_id": "file:cmd/inber/engine.go",
  "metadata": {
    "path": "cmd/inber/engine.go",
    "lines": 523,
    "modified": "2h ago",
    "size_tokens": 4200
  },
  "summary": "Engine orchestration: BuildSystemPrompt, Run loop, checkpoint logic",
  "expand_modes": ["full", "lines:N-M", "summary"]
}
```

**Expand strategies**:
- `KEEP_REF`: "File cmd/inber/engine.go (523 lines, modified 2h ago)"
- `EXPAND_SUMMARY`: Include the summary (50 tokens)
- `EXPAND_LINES:45-120`: Expand specific line range (500 tokens)
- `EXPAND_FULL`: Full file content (4200 tokens)

### 2. **Memory References** (`@ref:memory:<id>`)
```json
{
  "ref_type": "memory",
  "ref_id": "memory:187ce8b",
  "metadata": {
    "tags": ["decision", "architecture", "sqlite"],
    "importance": 0.85,
    "created": "2024-01-15T14:30:22Z",
    "size_tokens": 340
  },
  "summary": "Decision to use SQLite for memory store",
  "expand_modes": ["full", "summary"]
}
```

**Expand strategies**:
- `KEEP_REF`: "Memory: Decision to use SQLite for memory store (0.85 importance)"
- `EXPAND_FULL`: Full memory content (340 tokens)

### 3. **Identity/Config References** (`@ref:identity`, `@ref:soul`, `@ref:user`)
```json
{
  "ref_type": "identity",
  "ref_id": "identity.md",
  "metadata": {
    "size_tokens": 3000,
    "always_load": true
  },
  "summary": "Claxon identity: helpful coding assistant, direct communication style",
  "expand_modes": ["full", "summary"]
}
```

**Expand strategies**:
- `KEEP_REF`: "Identity: Claxon (helpful coding assistant)" (10 tokens)
- `EXPAND_FULL`: Full identity.md content (3000 tokens)

### 4. **Repo Map References** (`@ref:repo-map:<scope>`)
```json
{
  "ref_type": "repo-map",
  "ref_id": "repo-map:current",
  "metadata": {
    "packages": 15,
    "files": 142,
    "size_tokens": 8000,
    "scope": "."
  },
  "summary": "Repository: 15 packages, 142 Go files (agent, context, memory, tools, cmd)",
  "expand_modes": ["full", "package:<name>", "summary"]
}
```

**Expand strategies**:
- `KEEP_REF`: "Repo: 15 packages, 142 files" (10 tokens)
- `EXPAND_SUMMARY`: Package list only (200 tokens)
- `EXPAND_PACKAGE:agent`: Just the agent package structure (800 tokens)
- `EXPAND_FULL`: Full repo map (8000 tokens)

### 5. **Tool Registry References** (`@ref:tools/registry`)
```json
{
  "ref_type": "tools",
  "ref_id": "tools/registry",
  "metadata": {
    "tool_count": 9,
    "size_tokens": 2000
  },
  "summary": "9 tools: read_file, write_file, edit_file, shell, memory_search, etc.",
  "expand_modes": ["full", "list", "tool:<name>"]
}
```

**Expand strategies**:
- `KEEP_REF`: "9 tools available" (5 tokens)
- `EXPAND_LIST`: Tool names + 1-line descriptions (300 tokens)
- `EXPAND_TOOL:read_file`: Full schema for one tool (200 tokens)
- `EXPAND_FULL`: All tool schemas (2000 tokens)

### 6. **Web References** (`@ref:web:<url>`) [Future]
```json
{
  "ref_type": "web",
  "ref_id": "web:https://docs.anthropic.com/...",
  "metadata": {
    "url": "https://docs.anthropic.com/claude/reference",
    "title": "Claude API Reference",
    "size_tokens": 15000,
    "cached_at": "2024-01-15T10:00:00Z"
  },
  "summary": "Anthropic Claude API documentation",
  "expand_modes": ["full", "summary", "section:<name>"]
}
```

---

## Workflow: Reference Creation

### When to Create References

#### 1. **Session Start** (Automatic)
```go
// On engine.New():
refs := []Reference{
    NewFileRef(".inber/identity.md", AlwaysLoad: true),
    NewFileRef(".inber/soul.md", AlwaysLoad: true),
    NewFileRef(".inber/user.md", AlwaysLoad: true),
    NewToolRegistryRef(tools),
    NewRepoMapRef(repoRoot),
}
store.SaveReferences(refs)
```

#### 2. **After Memory Save** (Automatic)
```go
// When memory is saved with importance >= 0.5:
mem := memory.Save(Memory{
    Content: "Decided to use SQLite...",
    Tags: ["decision", "architecture"],
    Importance: 0.85,
})

// Automatically create reference:
ref := NewMemoryRef(mem.ID, mem.Tags, mem.Importance)
store.SaveReference(ref)
```

#### 3. **After File Read** (Automatic)
```go
// When read_file is called:
result := tools.ReadFile(path)

// Create reference for future use:
ref := NewFileRef(path, 
    ModifiedAt: fileInfo.ModTime(),
    SizeTokens: EstimateTokens(result),
    Summary: extractFileSummary(result), // First comment or package doc
)
store.SaveReference(ref)
```

#### 4. **After Repo Map Generation** (Automatic)
```go
// When repo_map() is called:
repoMap := tools.RepoMap(path)

// Create/update reference:
ref := NewRepoMapRef(path,
    Packages: countPackages(repoMap),
    Files: countFiles(repoMap),
    SizeTokens: EstimateTokens(repoMap),
    Summary: buildRepoSummary(repoMap),
)
store.SaveReference(ref)
```

#### 5. **Manual Reference** (User/Agent Decision)
```go
// New tool: reference_create
{
  "name": "reference_create",
  "description": "Create a reference for future use (file, url, etc.)",
  "input_schema": {
    "type": "object",
    "properties": {
      "ref_type": {"type": "string", "enum": ["file", "web", "memory"]},
      "path_or_url": {"type": "string"},
      "summary": {"type": "string"}
    }
  }
}
```

---

## Workflow: Prompt Optimizer (Two-Pass System)

### Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                    USER MESSAGE                               │
│              "Fix the bug in agent.go"                        │
└──────────────────────────────────────────────────────────────┘
                            ↓
┌──────────────────────────────────────────────────────────────┐
│               STEP 1: BUILD REFERENCE PROMPT                  │
│                                                               │
│  1. Load available references from store                      │
│  2. Rank by relevance (semantic match to user message)        │
│  3. Create minimal prompt with references                     │
│                                                               │
│  Output:                                                      │
│  ┌────────────────────────────────────────────┐              │
│  │ User: "Fix the bug in agent.go"            │              │
│  │                                            │              │
│  │ Available context (select what to expand): │              │
│  │ - @ref:identity.md (3000 tokens)           │              │
│  │ - @ref:file:agent.go (4200 tokens)         │              │
│  │ - @ref:file:agent_test.go (2100 tokens)    │              │
│  │ - @ref:memory:abc123 (bug fix pattern)     │              │
│  │ - @ref:repo-map:current (8000 tokens)      │              │
│  │ - @ref:tools/registry (2000 tokens)        │              │
│  └────────────────────────────────────────────┘              │
└──────────────────────────────────────────────────────────────┘
                            ↓
┌──────────────────────────────────────────────────────────────┐
│        STEP 2: PROMPT OPTIMIZER (Claude Haiku 3.5)            │
│                                                               │
│  System Prompt:                                               │
│  "You are a prompt optimizer. Given a user request and        │
│   available references, decide which references to expand.    │
│                                                               │
│   For each reference, output ONE of:                          │
│   - KEEP_REF (just mention it exists)                         │
│   - EXPAND_SUMMARY (show summary, ~50 tokens)                 │
│   - EXPAND_PARTIAL (specific section/lines)                   │
│   - EXPAND_FULL (full content)                                │
│                                                               │
│   Optimize for relevance and token budget (target: 10K)."     │
│                                                               │
│  Output (JSON):                                               │
│  {                                                            │
│    "expansions": [                                            │
│      {"ref": "identity.md", "mode": "KEEP_REF"},              │
│      {"ref": "file:agent.go", "mode": "EXPAND_FULL"},         │
│      {"ref": "file:agent_test.go", "mode": "KEEP_REF"},       │
│      {"ref": "memory:abc123", "mode": "EXPAND_FULL"},         │
│      {"ref": "repo-map:current", "mode": "KEEP_REF"},         │
│      {"ref": "tools/registry", "mode": "EXPAND_LIST"}         │
│    ],                                                         │
│    "reasoning": "Need full agent.go to fix bug. Memory has    │
│                  relevant pattern. Tools list for reference." │
│  }                                                            │
└──────────────────────────────────────────────────────────────┘
                            ↓
┌──────────────────────────────────────────────────────────────┐
│           STEP 3: EXPAND REFERENCES & BUILD PROMPT            │
│                                                               │
│  For each expansion decision:                                 │
│  1. Load reference from store                                 │
│  2. Apply expansion mode                                      │
│  3. Build final prompt blocks                                 │
│                                                               │
│  Final Prompt:                                                │
│  ┌────────────────────────────────────────────┐              │
│  │ @ref:identity.md [context available]       │              │
│  │                                            │              │
│  │ @ref:file:agent.go [EXPANDED]              │              │
│  │ ```go                                      │              │
│  │ package main                               │              │
│  │ // ... full file content ...               │              │
│  │ ```                                        │              │
│  │                                            │              │
│  │ @ref:memory:abc123 [EXPANDED]              │              │
│  │ "When fixing similar bugs, check for       │              │
│  │  nil pointer dereferences in..."           │              │
│  │                                            │              │
│  │ @ref:tools/registry [LIST]                 │              │
│  │ - read_file: Read file contents            │              │
│  │ - write_file: Write to file                │              │
│  │ - edit_file: Edit file with replacement    │              │
│  │ ...                                        │              │
│  └────────────────────────────────────────────┘              │
│                                                               │
│  Tokens: 6,200 (vs 20,700 if all expanded)                   │
└──────────────────────────────────────────────────────────────┘
                            ↓
┌──────────────────────────────────────────────────────────────┐
│            STEP 4: SEND TO MAIN MODEL (Opus/Sonnet)           │
│                                                               │
│  Main model sees:                                             │
│  - Optimized prompt (6K tokens)                               │
│  - User message                                               │
│  - Conversation history                                       │
│                                                               │
│  Can request expansion:                                       │
│  "I need to see the full repo map to understand structure."   │
│  → triggers re-expansion with EXPAND_FULL for repo-map        │
└──────────────────────────────────────────────────────────────┘
```

---

## Implementation Steps

### Phase 1: Reference Store (Foundation)

**Goal**: Create reference storage and management system

#### 1.1 Create Reference Types
```go
// reference/reference.go
package reference

type Reference struct {
    ID          string            // "file:agent.go", "memory:abc123"
    Type        RefType           // file, memory, identity, repo-map, tools, web
    Metadata    map[string]any    // type-specific metadata
    Summary     string            // Brief description
    SizeTokens  int               // Size if fully expanded
    ExpandModes []string          // ["full", "summary", "lines:N-M"]
    CreatedAt   time.Time
    UpdatedAt   time.Time
    ExpiresAt   *time.Time        // For ephemeral refs (repo maps)
}

type RefType string

const (
    RefTypeFile     RefType = "file"
    RefTypeMemory   RefType = "memory"
    RefTypeIdentity RefType = "identity"
    RefTypeRepoMap  RefType = "repo-map"
    RefTypeTools    RefType = "tools"
    RefTypeWeb      RefType = "web"
)
```

#### 1.2 Create Reference Store
```go
// reference/store.go
type Store struct {
    db *sql.DB
}

func (s *Store) Save(ref Reference) error
func (s *Store) Get(id string) (Reference, error)
func (s *Store) Search(query string, types []RefType, limit int) ([]Reference, error)
func (s *Store) List() ([]Reference, error)
func (s *Store) Delete(id string) error

// Schema:
CREATE TABLE references (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    metadata TEXT, -- JSON
    summary TEXT,
    size_tokens INTEGER,
    expand_modes TEXT, -- JSON array
    created_at INTEGER,
    updated_at INTEGER,
    expires_at INTEGER
);

CREATE INDEX idx_ref_type ON references(type);
CREATE INDEX idx_ref_expires ON references(expires_at);
```

#### 1.3 Auto-Create References
```go
// Hook into existing systems:

// In memory/store.go Save():
func (s *Store) Save(mem Memory) error {
    // ... existing save logic ...
    
    if mem.Importance >= 0.5 {
        refStore.Save(reference.NewMemoryRef(mem))
    }
}

// In tools/read_file.go:
func (t *ReadFile) Run(input map[string]any) (string, error) {
    result, err := os.ReadFile(path)
    
    // Create reference
    refStore.Save(reference.NewFileRef(path, result))
    
    return result, err
}

// In tools/repo_map.go:
func (t *RepoMap) Run(input map[string]any) (string, error) {
    repoMap := buildRepoMap(path)
    
    // Create/update reference
    refStore.Save(reference.NewRepoMapRef(path, repoMap))
    
    return repoMap, err
}
```

**Deliverable**: Reference store with auto-population

---

### Phase 2: Prompt Builder with References

**Goal**: Build prompts using references instead of direct content

#### 2.1 Create Reference Prompt Builder
```go
// reference/builder.go
type PromptBuilder struct {
    refStore *Store
    memStore *memory.Store
}

// BuildReferencePrompt creates a minimal prompt with references
func (pb *PromptBuilder) BuildReferencePrompt(
    userMessage string,
    tokenBudget int,
) ([]ReferenceBlock, error) {
    
    // 1. Get relevant references
    refs := pb.findRelevantReferences(userMessage)
    
    // 2. Rank by relevance
    ranked := pb.rankReferences(refs, userMessage)
    
    // 3. Build reference blocks
    blocks := []ReferenceBlock{}
    for _, ref := range ranked {
        blocks = append(blocks, ReferenceBlock{
            RefID:      ref.ID,
            Type:       ref.Type,
            Summary:    ref.Summary,
            SizeTokens: ref.SizeTokens,
            Modes:      ref.ExpandModes,
        })
    }
    
    return blocks, nil
}

type ReferenceBlock struct {
    RefID      string   // "file:agent.go"
    Type       RefType  // file
    Summary    string   // "Engine orchestration logic"
    SizeTokens int      // 4200
    Modes      []string // ["full", "lines:N-M", "summary"]
}
```

#### 2.2 Update Engine to Use References
```go
// cmd/inber/engine.go
func (e *Engine) BuildSystemPrompt(userMessage string) []ReferenceBlock {
    if e.RefStore == nil {
        // Fallback to old system
        return e.buildLegacyPrompt(userMessage)
    }
    
    builder := reference.NewPromptBuilder(e.RefStore, e.MemStore)
    return builder.BuildReferencePrompt(userMessage, 10000)
}
```

**Deliverable**: Engine can build reference-based prompts

---

### Phase 3: Prompt Optimizer (Haiku)

**Goal**: Use Haiku to decide which references to expand

#### 3.1 Create Optimizer
```go
// reference/optimizer.go
type Optimizer struct {
    client *anthropic.Client
    model  string // "claude-3-5-haiku-20241022"
}

type ExpansionDecision struct {
    RefID    string // "file:agent.go"
    Mode     string // "EXPAND_FULL", "KEEP_REF", etc.
    Reason   string // Why this decision was made
}

func (o *Optimizer) OptimizePrompt(
    userMessage string,
    references []ReferenceBlock,
    tokenBudget int,
) ([]ExpansionDecision, error) {
    
    // Build optimizer prompt
    prompt := o.buildOptimizerPrompt(userMessage, references, tokenBudget)
    
    // Call Haiku
    resp, err := o.client.Messages.New(ctx, anthropic.MessageNewParams{
        Model: o.model,
        Messages: []anthropic.MessageParam{
            {Role: "user", Content: prompt},
        },
        MaxTokens: 2000,
    })
    
    // Parse JSON response
    var decisions []ExpansionDecision
    json.Unmarshal(resp.Content[0].Text, &decisions)
    
    return decisions, nil
}

func (o *Optimizer) buildOptimizerPrompt(
    userMessage string,
    references []ReferenceBlock,
    tokenBudget int,
) string {
    return fmt.Sprintf(`You are a prompt optimizer. The user asked: "%s"

Available references:
%s

Token budget: %d

For each reference, decide how to include it. Output JSON:
{
  "expansions": [
    {"ref": "identity.md", "mode": "KEEP_REF", "reason": "..."},
    {"ref": "file:agent.go", "mode": "EXPAND_FULL", "reason": "..."},
    ...
  ]
}

Modes:
- KEEP_REF: Just mention it exists (~10 tokens)
- EXPAND_SUMMARY: Include summary (~50 tokens)
- EXPAND_PARTIAL: Specific section (varies)
- EXPAND_FULL: Full content (see size_tokens)

Prioritize relevance. Stay within budget.`,
        userMessage,
        formatReferences(references),
        tokenBudget,
    )
}
```

#### 3.2 Integrate Optimizer into Engine
```go
// cmd/inber/engine.go
func (e *Engine) BuildOptimizedPrompt(userMessage string) ([]Block, error) {
    // 1. Build reference prompt
    refBlocks := e.BuildSystemPrompt(userMessage)
    
    // 2. Optimize with Haiku
    optimizer := reference.NewOptimizer(e.Client)
    decisions, err := optimizer.OptimizePrompt(userMessage, refBlocks, 10000)
    if err != nil {
        return nil, err
    }
    
    // 3. Expand according to decisions
    expander := reference.NewExpander(e.RefStore, e.MemStore)
    finalBlocks := expander.ExpandReferences(decisions)
    
    return finalBlocks, nil
}
```

**Deliverable**: Two-pass system (Haiku optimizes, then expand)

---

### Phase 4: Reference Expander

**Goal**: Expand references according to optimizer decisions

#### 4.1 Create Expander
```go
// reference/expander.go
type Expander struct {
    refStore *Store
    memStore *memory.Store
}

func (ex *Expander) ExpandReferences(
    decisions []ExpansionDecision,
) ([]Block, error) {
    
    var blocks []Block
    
    for _, decision := range decisions {
        ref, err := ex.refStore.Get(decision.RefID)
        if err != nil {
            continue
        }
        
        var content string
        switch decision.Mode {
        case "KEEP_REF":
            content = fmt.Sprintf("@ref:%s [%s]", ref.ID, ref.Summary)
            
        case "EXPAND_SUMMARY":
            content = fmt.Sprintf("@ref:%s\n%s", ref.ID, ref.Summary)
            
        case "EXPAND_FULL":
            content = ex.expandFull(ref)
            
        case "EXPAND_PARTIAL":
            content = ex.expandPartial(ref, decision.Params)
        }
        
        blocks = append(blocks, Block{
            ID:      ref.ID,
            Content: content,
        })
    }
    
    return blocks, nil
}

func (ex *Expander) expandFull(ref Reference) string {
    switch ref.Type {
    case RefTypeFile:
        path := ref.Metadata["path"].(string)
        content, _ := os.ReadFile(path)
        return fmt.Sprintf("@ref:%s [EXPANDED]\n```\n%s\n```", ref.ID, content)
        
    case RefTypeMemory:
        memID := strings.TrimPrefix(ref.ID, "memory:")
        mem, _ := ex.memStore.Get(memID)
        return fmt.Sprintf("@ref:%s [EXPANDED]\n%s", ref.ID, mem.Content)
        
    case RefTypeRepoMap:
        // Load cached repo map
        repoMap, _ := ex.refStore.GetContent(ref.ID)
        return fmt.Sprintf("@ref:%s [EXPANDED]\n%s", ref.ID, repoMap)
        
    // ... other types
    }
}
```

**Deliverable**: Full expansion system

---

### Phase 5: Reference Management Tools

**Goal**: Allow agent to manage references

#### 5.1 Create Reference Tools
```go
// New tools:

// 1. reference_search - find references
{
  "name": "reference_search",
  "description": "Search available references (files, memories, etc.)",
  "input_schema": {
    "query": {"type": "string"},
    "types": {"type": "array", "items": {"enum": ["file", "memory", "repo-map"]}},
    "limit": {"type": "integer", "default": 10}
  }
}

// 2. reference_expand - request full expansion of a reference
{
  "name": "reference_expand",
  "description": "Get full content of a reference",
  "input_schema": {
    "ref_id": {"type": "string"},
    "mode": {"enum": ["full", "summary", "partial"]},
    "params": {"type": "object"} // e.g., {"lines": "45-120"}
  }
}

// 3. reference_create - manually create reference
{
  "name": "reference_create",
  "description": "Create a reference for future use",
  "input_schema": {
    "type": {"enum": ["file", "web", "memory"]},
    "path_or_url": {"type": "string"},
    "summary": {"type": "string"}
  }
}
```

#### 5.2 Agent Workflow
```
User: "Fix the bug in agent.go"

Agent (internal):
1. Prompt optimizer suggests: EXPAND_FULL for agent.go
2. Sees code, identifies issue
3. Responds with fix

Later...
User: "What was that pattern you mentioned earlier?"

Agent (internal):
1. Optimizer suggests: KEEP_REF for agent.go (not needed now)
2. EXPAND_FULL for memory about the pattern
3. Retrieves from memory, answers question
```

---

## Token Savings Analysis

### Example: "Fix bug in agent.go"

#### Traditional Approach
```
System:
- Identity: 3000 tokens
- Soul: 1500 tokens
- User: 800 tokens
- Tools: 2000 tokens
- Repo map: 8000 tokens
- agent.go: 4200 tokens
- agent_test.go: 2100 tokens
- Recent memories (5): 2000 tokens
Total: 23,600 tokens
```

#### Reference-Based Approach

**Pass 1 (Haiku optimizer)**:
```
User: "Fix bug in agent.go"
References (summary only): 500 tokens
Haiku cost: ~$0.00004 (500 tokens × $0.80/MTok)
```

**Pass 2 (Opus/Sonnet)**:
```
Expanded prompt:
- @ref:identity [kept]: 10 tokens
- @ref:soul [kept]: 10 tokens
- @ref:user [summary]: 50 tokens
- @ref:tools [list]: 300 tokens
- @ref:repo-map [kept]: 10 tokens
- @ref:file:agent.go [FULL]: 4200 tokens
- @ref:file:agent_test.go [kept]: 10 tokens
- @ref:memory:pattern [FULL]: 340 tokens
Total: 4,930 tokens
Main model cost: 4930 tokens × $15/MTok = $0.074
```

**Total cost**: $0.07404
**Traditional cost**: 23,600 × $15/MTok = $0.354
**Savings**: 79% reduction ($0.28 saved per request)

Over 100 requests: **$28 saved**

---

## Benefits

1. **Massive Token Savings**: 70-80% reduction in typical prompts
2. **Intelligent Optimization**: Haiku decides what's needed (cheap model, smart decision)
3. **On-Demand Expansion**: Agent can request more context if needed
4. **Better Caching**: Stable references → better prompt caching
5. **Future-Proof**: Easy to add new reference types (web pages, APIs, etc.)
6. **Explainability**: Can see why Haiku chose to expand/keep each reference
7. **Cost Optimization**: Haiku ($0.80/MTok) makes decisions, Opus ($15/MTok) gets optimized prompt

---

## Migration Path

1. **Phase 1** (Week 1): Build reference store, auto-create refs
2. **Phase 2** (Week 2): Update prompt builder to use references
3. **Phase 3** (Week 3): Integrate Haiku optimizer
4. **Phase 4** (Week 4): Build expander, test end-to-end
5. **Phase 5** (Week 5): Add reference management tools, polish

**Estimated effort**: 4-5 weeks for full implementation

---

## Next Steps

Want me to:
1. Start with Phase 1 (reference store)?
2. Build a proof-of-concept with just file references?
3. Create the Haiku optimizer first to test the concept?
