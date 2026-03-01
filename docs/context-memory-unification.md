# Context + Memory Unification Analysis

## Current Architecture

### context.Store (In-Memory, Per-Session)
```go
type Chunk struct {
    ID        string
    Text      string
    Tokens    int
    Tags      []string    // "identity", "error-log", "file:main.go"
    Source    string      // "user", "assistant", "tool-result", "system"
    CreatedAt time.Time
    ExpiresAt *time.Time  // TTL
}
```
- Lives in RAM only
- Recreated every session from files/repo-map/recency
- Builder filters by tags + budget
- No persistence, no search

### memory.Memory (SQLite, Cross-Session)
```go
type Memory struct {
    ID           string
    Content      string
    Summary      string     // for compacted memories
    OriginalID   string     // lineage pointer
    Tags         []string
    Importance   float64    // 0-1 scoring
    AccessCount  int
    LastAccessed time.Time
    CreatedAt    time.Time
    Source       string
    Embedding    []float64  // TF-IDF (soon: real embeddings)
}
```
- Persists to SQLite
- Semantic search via embeddings
- Importance scoring + decay
- Access tracking

---

## The Duplication Problem

Both systems:
- Store tagged text chunks
- Track creation time
- Have a "Source" field
- Support retrieval by tags
- Estimate tokens

**Current workflow:**
1. Engine builds context.Store from files/repo/recent
2. Engine loads high-importance memories → inserts into context.Store
3. Builder filters context.Store by tags + budget → builds prompt
4. On session end, saveSessionSummary() creates a new Memory

**The redundancy:** We're loading memories into chunks, then filtering chunks. Why not just query memories directly?

---

## Proposed Unified System

### Single Source of Truth: `memory.Store`

**Everything becomes a memory:**
- Repo map → `Memory{Tags: ["repo-map", "go"], Source: "system", Importance: 0.3}`
- Recent file → `Memory{Tags: ["file:foo.go", "recent"], Source: "system", Importance: 0.4}`
- Identity → `Memory{Tags: ["identity"], Source: "system", Importance: 1.0}`
- Session summary → `Memory{Tags: ["session-summary"], Source: "agent", Importance: 0.6}`
- User preference → `Memory{Tags: ["preference"], Source: "user", Importance: 0.8}`

**New fields to add:**
```go
type Memory struct {
    // ... existing fields ...
    
    // Priority controls whether this always loads (like "identity")
    AlwaysLoad   bool      // replaces Chunk's implicit "identity" tag check
    
    // TTL for ephemeral memories (repo maps, recent files)
    ExpiresAt    *time.Time
    
    // For large content, store reference instead of full text
    IsReference  bool       // if true, Content is a query to retrieve full text
    RefType      string     // "file", "repo-map", "recent-commits"
    RefQuery     string     // path, git query, etc.
}
```

---

## Benefits of Merging

### 1. **Unified Retrieval Interface**
Instead of:
```go
// Current: Load from multiple sources
store := context.NewStore()
context.AutoLoad(cfg)               // files, repo, recent
memory.LoadIntoContext(memStore, store, 10, 0.7)  // high-importance memories
chunks := builder.Build(messageTags)
```

We get:
```go
// Unified: One query
memories := memStore.BuildContext(BuildContextRequest{
    Tags:         messageTags,
    TokenBudget:  32000,
    MinImportance: 0.5,
    IncludeAlwaysLoad: true,
})
```

### 2. **Fewer Tools**
Current tools:
- `memory_search` - search persistent memories
- `memory_save` - save new memory
- `memory_expand` - get full memory by ID
- `memory_forget` - mark memory as forgotten

After merge, we could simplify to:
- `memory_query` - unified search (replaces search + expand)
- `memory_save` - same
- `memory_forget` - same

Or even better: **context becomes queryable** via memory_query, so the model can say:
```
"I see from context there's a repo-map reference. Let me query: memory_query('repo-map', 'package agent')"
```

### 3. **Automatic Stashing**
When content is too large for budget:
- Current: Builder silently drops chunks
- Proposed: `Memory{IsReference: true, RefType: "file", RefQuery: "path/to/file.go"}`
  - Stored as stub: "File contents available via memory_query('file', 'path/to/file.go')"
  - Model can retrieve on demand

### 4. **Smart Importance Scoring**
- Files with recent commits: `Importance = 0.6 + 0.2*recency_score`
- Identity: `Importance = 1.0, AlwaysLoad = true`
- Repo map: `Importance = 0.3` (background info)
- Active conversation: `Importance = 0.8`

The builder would naturally prioritize recent/important content.

### 5. **Repo Map as Lazy-Loaded Memory**
```go
// Don't store full repo map inline
Memory{
    ID:       "repo-map:2026-03-01",
    Content:  "Repository structure (32 packages, 145 files). Query for details.",
    IsReference: true,
    RefType:  "repo-map",
    RefQuery: "",  // empty = full map
    Tags:     []string{"repo-map", "structure"},
    Importance: 0.3,
}

// Model asks: "What's in package agent?"
// We query: repoMap.Search("package agent") → return relevant chunk
```

---

## Issues to Solve

### Issue 1: **Repo maps are huge (~10k+ lines)**
**Current:** Loaded into context.Store, competes for tokens  
**Proposed:** Store as `IsReference = true`, provide `memory_query` tool that searches the repo map on demand

**Implementation:**
```go
func (m *Memory) Retrieve() (string, error) {
    if !m.IsReference {
        return m.Content, nil
    }
    switch m.RefType {
    case "file":
        return os.ReadFile(m.RefQuery)
    case "repo-map":
        return repoMap.Search(m.RefQuery)  // AST search
    case "recent-files":
        return recency.ListFiles(since: m.RefQuery)
    }
}
```

### Issue 2: **Freshness vs Persistence**
**Current:** context.Store rebuilt every session (always fresh)  
**Proposed:** Memory.ExpiresAt for ephemeral content

**Solution:**
```go
// On session start:
memory.RefreshEphemeral()  // re-scans files, updates ExpiresAt

// Repo map memory
Memory{
    ID:        "repo-map:current",
    ExpiresAt: time.Now().Add(1 * time.Hour),  // refresh hourly
    IsReference: true,
    RefType:   "repo-map",
}

// Recent file memory
Memory{
    ID:        "recent:foo.go",
    ExpiresAt: time.Now().Add(10 * time.Minute),
    Tags:      []string{"recent", "file:foo.go"},
}
```

### Issue 3: **Tag Matching Logic**
**Current:** Builder has special logic for tag matching  
**Proposed:** Move to SQL query

**SQL Query:**
```sql
-- Get memories matching tags, budget-aware, importance-sorted
WITH tagged AS (
  SELECT m.*, COUNT(mt.tag) as match_count
  FROM memories m
  LEFT JOIN memory_tags mt ON m.id = mt.memory_id
  WHERE mt.tag IN (?, ?, ?)  -- user's message tags
    OR m.always_load = 1
    AND (m.expires_at IS NULL OR m.expires_at > ?)
  GROUP BY m.id
)
SELECT * FROM tagged
WHERE importance >= ?
ORDER BY 
  always_load DESC,
  match_count DESC,
  importance DESC,
  last_accessed DESC
```

Then iterate, sum tokens until budget is hit.

### Issue 4: **In-Memory Performance**
**Current:** context.Store is fast (no disk I/O)  
**Proposed:** SQLite queries add latency

**Mitigation:**
- Use `:memory:` database for ephemeral memories (repo map, recent files)
- Main `.inber/memory.db` for persistent memories
- On session start: `ATTACH DATABASE ':memory:' AS ephemeral`
- Union queries across both DBs

Or simpler: keep a `map[string]Memory` cache in memory, sync to disk on changes.

### Issue 5: **Builder Logic**
**Current:** `context/builder.go` has size-aware filtering, test file exclusion  
**Proposed:** Move logic into Memory retrieval

**Solution:**
```go
type BuildContextRequest struct {
    Tags            []string
    TokenBudget     int
    MinImportance   float64
    ExcludeTags     []string  // ["test", "archived"]
    IncludeAlwaysLoad bool
    MaxChunkSize    int       // skip memories > this size
}

func (s *Store) BuildContext(req BuildContextRequest) ([]Memory, int) {
    // Query with filters
    // Iterate, skip oversized chunks
    // Track running token total
    // Return when budget hit
}
```

---

## Migration Path

### Phase 1: Add Memory Fields
```go
type Memory struct {
    // ... existing ...
    AlwaysLoad   bool
    ExpiresAt    *time.Time
    IsReference  bool
    RefType      string
    RefQuery     string
    Tokens       int  // pre-computed, like Chunk.Tokens
}
```

### Phase 2: Populate Memories on Session Start
```go
func PrepareSession(repoRoot string, memStore *memory.Store) {
    // Load identity (permanent)
    memStore.Save(Memory{
        ID: "identity",
        Content: identityText,
        Tags: []string{"identity"},
        AlwaysLoad: true,
        Importance: 1.0,
    })
    
    // Load repo map (ephemeral, 1hr TTL)
    repoMapText := context.BuildRepoMap(repoRoot)
    memStore.Save(Memory{
        ID: "repo-map:current",
        Content: "Repo structure available. Query with memory_query('repo-map', 'package name')",
        IsReference: true,
        RefType: "repo-map",
        RefQuery: repoMapText,  // or store path to file
        ExpiresAt: time.Now().Add(1*time.Hour),
        Tags: []string{"repo-map", "structure"},
        Importance: 0.3,
    })
    
    // Load recent files (ephemeral, 10min TTL)
    recentFiles := context.FindRecentlyModified(repoRoot, 24*time.Hour)
    for _, f := range recentFiles {
        memStore.Save(Memory{
            ID: "recent:" + f.RelativePath,
            Content: fmt.Sprintf("Recently modified: %s", f.RelativePath),
            IsReference: true,
            RefType: "file",
            RefQuery: f.Path,
            Tags: []string{"recent", "file:" + f.RelativePath},
            ExpiresAt: time.Now().Add(10*time.Minute),
            Importance: 0.6,
        })
    }
}
```

### Phase 3: Update Builder to Query Memory
```go
// OLD: context/builder.go
chunks := store.ListByTags(tags)
// filter by budget

// NEW: memory-backed builder
memories := memStore.BuildContext(BuildContextRequest{
    Tags: messageTags,
    TokenBudget: 32000,
    MinImportance: 0.5,
    ExcludeTags: []string{"test"},
})
```

### Phase 4: Add memory_query Tool
```go
func MemoryQueryTool(store *memory.Store) agent.Tool {
    return agent.Tool{
        Name: "memory_query",
        Description: "Query memories, files, or repo structure by type and query string",
        InputSchema: schema.Props([]string{"ref_type"}, map[string]any{
            "ref_type": schema.Str("Type: 'memory', 'file', 'repo-map', 'recent'"),
            "query": schema.Str("Search query or file path"),
            "limit": schema.Integer("Max results (default: 10)"),
        }),
        Run: func(ctx context.Context, raw string) (string, error) {
            // Parse input
            // If ref_type == "repo-map": search AST
            // If ref_type == "file": read file
            // If ref_type == "memory": semantic search
            // Return results
        },
    }
}
```

### Phase 5: Deprecate context.Store
- Remove `context/store.go`
- Move `context/builder.go` → `memory/builder.go`
- Update `autoload.go` to populate memory store instead

---

## Tool Simplification

### Before (5 tools)
- `memory_search` - search persistent memories
- `memory_save` - save memory
- `memory_expand` - get full memory
- `memory_forget` - forget memory
- (implicit: read_file for repo context)

### After (3-4 tools)
- `memory_query` - unified retrieval (search + expand + file + repo-map)
- `memory_save` - save memory
- `memory_forget` - forget memory
- (optional) `memory_context` - show what's currently loaded

**memory_query examples:**
```json
{"ref_type": "memory", "query": "how to handle errors", "limit": 5}
{"ref_type": "file", "query": "agent/agent.go"}
{"ref_type": "repo-map", "query": "package memory"}
{"ref_type": "recent", "query": "last 24 hours"}
```

---

## Open Questions

1. **Do we keep separate DBs?**
   - Option A: Everything in one `memory.db` (simpler)
   - Option B: Ephemeral in-memory DB, persistent in file DB (faster)

2. **How to handle compaction with references?**
   - Compacted memory might reference 10 files
   - Store as JSON array in RefQuery? `["file1.go", "file2.go"]`

3. **What happens to context/ package code?**
   - `chunker.go` → memory insert helper
   - `builder.go` → move to memory/builder.go
   - `repomap.go` → keep, but return Memory instead of Chunk
   - `files.go`, `recency.go` → keep, populate memory store
   - `tagger.go` → keep, used when creating memories

4. **Should AlwaysLoad be a tag instead?**
   ```go
   // Instead of AlwaysLoad bool field:
   Tags: []string{"identity", "always-load"}
   ```
   More flexible, but then we need to check tags in queries.

5. **Importance scoring strategy?**
   - Identity: 1.0
   - User preferences: 0.8
   - Session summaries: 0.6
   - Recent files: 0.5-0.7 (decay over time)
   - Repo map: 0.3
   - Test files: 0.2

---

## Recommendation

**YES, merge them.** The benefits are huge:

✅ Simpler architecture (one store, not two)  
✅ Fewer tools for the model to reason about  
✅ Automatic stashing of large content  
✅ Unified importance scoring  
✅ Memory becomes the universal storage layer  
✅ Context becomes a *view* of memory (filtered query)

**Start with:**
1. Add `AlwaysLoad`, `ExpiresAt`, `IsReference`, `RefType`, `RefQuery`, `Tokens` to Memory
2. Update autoload to populate memory store with repo/files/identity
3. Create `memory_query` tool that handles references
4. Test with a simple session
5. If it works well, deprecate context.Store

**Biggest risk:** SQLite query performance. Mitigate with in-memory cache or hybrid approach.
