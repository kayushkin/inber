# Memory Reference System

The memory reference system enables lazy-loading of content from various sources, preventing stale data and reducing database size.

## Overview

Memories can be **stored references** (content in DB) or **lazy references** (content loaded on-demand from source).

```go
type Memory struct {
    // ... other fields ...
    
    // Reference fields
    RefType    string     // Type of reference (see below)
    RefTarget  string     // Source location (file path, URL, etc.)
    IsLazy     bool       // If true, load content on-demand instead of from DB
}
```

## Reference Types

### 1. `memory` (Default)
Standard memories with content stored in the database.

- **RefType**: `"memory"`
- **RefTarget**: empty
- **IsLazy**: `false`
- **Use case**: User notes, agent reflections, decisions, learned facts

```go
Memory{
    ID:         "user-preference-123",
    Content:    "User prefers tabs over spaces",
    RefType:    "memory",
    IsLazy:     false,
    Importance: 0.8,
}
```

### 2. `file`
References to files on disk. Content is read fresh on every access (no stale data).

- **RefType**: `"file"`
- **RefTarget**: absolute or relative file path
- **IsLazy**: `true`
- **Use case**: Source files, documentation, configuration

```go
Memory{
    ID:         "file-ref-123",
    Content:    "", // Empty - loaded on access
    Summary:    "main.go (152 lines)",
    RefType:    "file",
    RefTarget:  "/path/to/project/main.go",
    IsLazy:     true,
    Importance: 0.6,
}
```

### 3. `identity`
Agent identity/configuration files (soul.md, user.md, etc.).

- **RefType**: `"identity"`
- **RefTarget**: path to identity file
- **IsLazy**: `true`
- **AlwaysLoad**: `true` (loaded in every session)
- **Use case**: Agent personality, user preferences, core instructions

```go
Memory{
    ID:         "identity-config",
    Summary:    "Agent identity and core values",
    RefType:    "identity",
    RefTarget:  ".inber/identity.md",
    IsLazy:     true,
    AlwaysLoad: true,
    Importance: 1.0,
}
```

### 4. `conversation`
Full conversation history (created by summarizer before compacting).

- **RefType**: `"memory"` (alias for conversation type)
- **IsLazy**: `true` (don't auto-load, but available via `memory_expand`)
- **Tags**: `["conversation", "history", "session:ID"]`
- **Use case**: Long-term conversation archives

```go
Memory{
    ID:         "conversation-summary:session123:abc",
    Content:    "[User]\nHello\n[Assistant]\nHi there!",
    Summary:    "Full conversation (15 turns, ~800 tokens)",
    Tags:       []string{"conversation", "history", "session:session123"},
    RefType:    "memory",
    IsLazy:     true,
    Importance: 0.4,
    Source:     "summarization",
}
```

### 5. `stash`
Stashed code snippets, clipboard content, or saved selections.

- **RefType**: `"stash"`
- **IsLazy**: `false` (content stored in DB)
- **Use case**: Code snippets to reference later, clipboard saves

```go
Memory{
    ID:         "stash-clipboard-xyz",
    Content:    "func calculateTotal(items []Item) float64 { ... }",
    Summary:    "Stashed code snippet from clipboard",
    Tags:       []string{"stash", "clipboard", "code", "go"},
    RefType:    "stash",
    IsLazy:     false,
    Importance: 0.6,
}
```

### 6. `repo-map`
Repository structure maps (generated on-demand, not stored).

- **RefType**: `"repo-map"`
- **IsLazy**: `false` (stored temporarily with expiration)
- **ExpiresAt**: set to TTL (e.g., 10 minutes)
- **Use case**: Caching repo structure for the session

```go
Memory{
    ID:         "repo-map-20260301-143022",
    Content:    "pkg memory\n  - memory.go\n  - tools.go\n...",
    Summary:    "Repository structure (15 packages)",
    RefType:    "repo-map",
    IsLazy:     false,
    ExpiresAt:  &tenMinutesFromNow,
    Importance: 0.3,
}
```

### 7. `tools`
Tool registry information (generated on-demand).

- **RefType**: `"tools"`
- **IsLazy**: should not be lazy-loaded (error if attempted)
- **Use case**: Storing tool availability metadata

### 8. `web`
Web content references (not yet implemented).

- **RefType**: `"web"`
- **RefTarget**: URL
- **IsLazy**: `true` (would fetch on access)
- **Status**: Placeholder for future implementation

## Usage

### Creating References

#### Auto-References (Recommended)
The `AutoReferenceManager` automatically creates file references after tool calls:

```go
mgr := memory.NewAutoReferenceManager(store, repoRoot, config)

// Hook into tool execution
mgr.OnToolResult(toolID, "read_file", inputJSON, output)
// Creates lazy file reference automatically
```

#### Manual References
```go
// File reference
fileRef := memory.Memory{
    ID:         uuid.New().String(),
    Summary:    "Important configuration file",
    Tags:       []string{"config", "file"},
    RefType:    "file",
    RefTarget:  "/path/to/config.yaml",
    IsLazy:     true,
    Importance: 0.7,
}
store.Save(fileRef)
```

### Expanding References

Via tool (for agent use):
```go
expandTool := memory.ExpandTool(store)
result, err := expandTool.Run(ctx, `{"id": "file-ref-123"}`)
// Returns full file content with metadata
```

Via direct retrieval:
```go
mem, err := store.Get("file-ref-123")
// Content is automatically loaded if IsLazy=true
fmt.Println(mem.Content) // Fresh content from disk
```

## Key Features

### No Stale Data
Lazy references are **always read fresh** from their source:
- File changes are immediately visible on next access
- No manual refresh needed
- No cache invalidation complexity

### Space Efficiency
- File references store only metadata (~200 bytes)
- Actual file content (potentially megabytes) stays on disk
- Database stays small and fast

### Flexible Expiration
```go
// Temporary reference (expires after 10 minutes)
expiresAt := time.Now().Add(10 * time.Minute)
mem.ExpiresAt = &expiresAt
```

Expired memories are automatically excluded from `Search()` and `ListRecent()`.

### Always-Load Priority
```go
mem.AlwaysLoad = true // Loaded in every session
```

Useful for identity, core instructions, and essential context.

## Best Practices

1. **Use lazy references for files** — Always set `IsLazy: true` for file references
2. **Store conversations in DB** — Content is generated, not on disk, so `IsLazy: true` but `RefTarget: ""`
3. **Set appropriate importance** — Identity (1.0), config (0.8), temp files (0.3)
4. **Tag consistently** — Use `file`, `conversation`, `stash`, etc. for filtering
5. **Expire ephemeral content** — Repo maps, recent file lists should have TTL

## Migration Notes

The reference system was introduced in the context-memory unification work (commits 1c206a7-f81aea1). Features that worked:

- ✅ Lazy file loading
- ✅ Reference types and coherent schema
- ✅ Auto-reference creation after tool calls
- ✅ Conversation summarization with lazy storage
- ✅ Stale data protection

Features that were removed (went overboard):

- ❌ Smart tagging with AST analysis
- ❌ Importance scoring algorithms
- ❌ Context-based summarization (replaced by conversation summarizer)

## Testing

Comprehensive tests in `reference_expansion_test.go`:
- File reference expansion
- Conversation reference expansion
- Identity reference expansion
- Stale data protection
- Reference type coherence
- Stashed content handling

Run tests:
```bash
go test ./memory -run TestReference -v
```

## See Also

- `lazy_loader.go` — Lazy loading implementation
- `auto_references.go` — Automatic reference creation
- `tools.go` — Memory tools including `memory_expand`
- `../cmd/inber/summarize.go` — Conversation summarization
