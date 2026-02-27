# Context System

Inber uses a **tag-based context system** where all content (files, messages, memories) is broken into tagged chunks that compete for limited context space.

## Architecture

```
Input Sources          Context Store              Context Builder
───────────────       ───────────────            ───────────────
                      
Files (.go, .md)  ──► Tagged Chunks ──► Budget-aware Selection ──► Final Context
Messages          ──►  (with TTL)   ──►   (filtered, sorted)   ──►  (to LLM)
Memories          ──►               ──►                         ──►
Recent changes    ──►               ──►                         ──►
```

## Core Concepts

### 1. Tagged Chunks

Everything in inber becomes a **chunk** with:
- **Content**: The actual text
- **Tags**: Categories for filtering (e.g., `["code", "golang", "agent"]`)
- **Priority**: Importance score (0.0-1.0)
- **Size estimate**: Token count (estimated as chars/4)
- **Expiration**: Optional TTL for temporary content

**Example:**
```go
store.Add(context.Chunk{
    Content:  "package main\n\nfunc main() { ... }",
    Tags:     []string{"code", "golang", "main"},
    Priority: 0.8,
})
```

### 2. Token Budget

Each agent has a **context budget** (defined in `agents.json`) that limits how much context can fit in a single turn.

**Example budgets:**
- Coder: 50,000 tokens
- Researcher: 30,000 tokens
- Orchestrator: 20,000 tokens

The context builder ensures the total token count stays within budget by:
1. Filtering by tags (only relevant content)
2. Sorting by priority (most important first)
3. Including chunks until budget is exhausted

### 3. Tag Filtering

Tags act as a **multi-dimensional filter** for context selection:

- `["code", "golang"]` → Load Go source files
- `["errors", "debugging"]` → Load error logs and debug info
- `["identity"]` → Load agent identity/system prompt
- `["recent"]` → Load recently modified files

**Combining filters:** You can request multiple tag sets and the builder will include chunks matching ANY of the tag combinations.

### 4. Priority Ordering

Within matching tags, chunks are ordered by priority:

- **1.0** - Critical (identity, user directives)
- **0.8** - High (recent changes, active files)
- **0.6** - Medium (related files, relevant memories)
- **0.4** - Low (documentation, auxiliary files)
- **0.2** - Background (historical context)

## Automatic Context Loading

Inber's **autoload** system populates context automatically at session start:

### 1. Identity
- Agent's system prompt (markdown file)
- Priority: 1.0
- Tags: `["identity"]`

### 2. Recent Files
- Files modified in last 7 days (git or mtime)
- Priority: 0.8
- Tags: `["recent", "code"]`

### 3. Repo Map
- AST-based structure map (function signatures, types)
- Priority: 0.6
- Tags: `["structure", "code"]`

### 4. Memories
- High-importance memories (score > 0.7)
- Priority: Based on memory importance
- Tags: From memory tags

### 5. Error Context
- Recent error messages
- Priority: 0.9
- Tags: `["errors", "debugging"]`

## Context Builder API

The context builder provides flexible context assembly:

```go
builder := context.NewBuilder(store)

// Build context with budget and filters
result := builder.WithBudget(50000).
    WithTags("code", "golang").
    WithTags("recent").
    Build()

// Use result in LLM call
messages := result.ToMessages()
```

### Builder Methods

- **WithBudget(tokens)**: Set token limit
- **WithTags(tags...)**: Add tag filter (OR logic across multiple WithTags calls)
- **WithRequired(tag)**: Ensure specific content is included
- **WithExclude(tags...)**: Exclude chunks with certain tags
- **Build()**: Execute and return result

### Result Fields

```go
type Result struct {
    Chunks       []Chunk  // Selected chunks
    TotalTokens  int      // Actual token count
    Budget       int      // Target budget
    Dropped      int      // Number of chunks that didn't fit
}
```

## Chunking Strategy

Large files are automatically chunked to fit in context:

### Code Files
- Split on function boundaries
- Each function = one chunk
- Preserve imports and package declaration

### Markdown Files
- Split on headers
- Each section = one chunk

### Generic Files
- Split on blank lines or fixed size (4000 chars)
- Maintain logical boundaries

## Tag Conventions

### Automatic Tags

The **auto-tagger** adds tags based on content:

- **Language detection**: `golang`, `python`, `javascript`
- **File types**: `code`, `markdown`, `config`, `test`
- **Error patterns**: `error`, `panic`, `failed`
- **Path-based**: `agent`, `context`, `memory` (from directory)

### Custom Tags

You can add custom tags when loading content:

```go
store.AddFile("config.yaml", "config", "yaml", "production")
```

## Memory Integration

Memories loaded into context are:
1. Searched based on relevance to current task
2. Filtered by importance (> 0.7 for auto-load)
3. Added as chunks with priority = importance score
4. Tagged with memory tags for filtering

## Best Practices

### For Agent Designers

1. **Set appropriate budgets**: More budget = more context but slower/costlier
2. **Use specific tags**: Generic tags pull too much irrelevant content
3. **Prioritize wisely**: Reserve 1.0 for truly critical content
4. **Expire temporary content**: Use TTL for transient messages

### For Tool Implementations

1. **Tag tool outputs**: Error messages → `["error"]`, file reads → `["code", "recent"]`
2. **Estimate tokens accurately**: Use `len(content) / 4` as baseline
3. **Set priorities based on urgency**: Errors = high, documentation = low

### For Agent Runtime

1. **Refresh context between turns**: Rebuild to incorporate new files/memories
2. **Prune old messages**: Don't let conversation history dominate budget
3. **Monitor dropped chunks**: If many chunks are dropped, increase budget or refine tags

## Example: Coder Agent Context

A typical coder agent turn might include:

```
┌─────────────────────────────────────────┐
│ Context Budget: 50,000 tokens           │
├─────────────────────────────────────────┤
│ Identity (priority 1.0)        2,000    │
│ Recent files (priority 0.8)   15,000    │
│ Repo map (priority 0.6)       10,000    │
│ Relevant memories (0.7-0.9)   5,000     │
│ Error logs (priority 0.9)      3,000    │
│ Conversation history          15,000    │
├─────────────────────────────────────────┤
│ Total:                        50,000    │
│ Dropped chunks:               12         │
└─────────────────────────────────────────┘
```

## Future Enhancements

- **Dynamic budgeting**: Adjust budget based on task complexity
- **Semantic chunking**: Use embeddings to group related content
- **Context caching**: Reuse expensive computations (repo maps, embeddings)
- **Cross-agent context sharing**: Allow agents to share context stores
