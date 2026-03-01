# Context-Memory Unification Progress

## Goal
Merge `context.Store` (in-memory, per-session chunks) with `memory.Store` (SQLite, persistent memories) into a single unified system where memory becomes the source of truth for all context.

## Status: Step 4 Complete ✅ - SYSTEM NOW LIVE!

### Step 1: Add Memory Fields (DONE)

**Added fields to Memory struct:**
```go
type Memory struct {
    // ... existing fields ...
    AlwaysLoad   bool       // if true, always include (e.g., identity)
    ExpiresAt    *time.Time // optional expiration (recent files)
    Tokens       int        // pre-computed token count
}
```

**Schema changes:**
- Added `always_load INTEGER DEFAULT 0`
- Added `expires_at INTEGER`
- Added `tokens INTEGER DEFAULT 0`
- Added indexes: `idx_always_load`, `idx_expires_at`

**Query updates:**
- `Search()` excludes expired memories: `WHERE expires_at IS NULL OR expires_at > NOW()`
- `ListRecent()` excludes expired memories
- `Save()` auto-computes `Tokens` if not set: `len(content) / 4`
- `Get()` loads all new fields

**Tests:**
- Created `memory/unified_test.go` with 4 test cases
- All tests pass ✅
- Verified expired memories excluded from search
- Verified AlwaysLoad, ExpiresAt, Tokens persist correctly

**Commit:** `187ce8b` - "Add context unification fields to Memory"

---

### Step 2: Load Identity & Recent Files into Memory (DONE)

**Created `memory.PrepareSession()`:**
```go
func (s *Store) PrepareSession(cfg PrepareSessionConfig) error
```

**Loads into memory:**
1. **Identity** - Agent identity/system prompt
   - AlwaysLoad: true
   - Importance: 1.0
   - Tags: ["identity", "always-load"]
   - Never expires

2. **Memory instructions** - How to use memory tools
   - AlwaysLoad: true
   - Importance: 0.9
   - Tags: ["instructions", "memory", "always-load"]
   - Never expires

3. **Recent files** - Lightweight file references
   - Content: "Recently modified (2h ago): path/to/file"
   - Importance: 0.5-0.7 (based on recency)
   - Tags: ["recent", "file:path", "ext:.go"]
   - ExpiresAt: 10 minutes (configurable)

**PrepareSessionConfig:**
- `RootDir` - repository root
- `IdentityFile` / `IdentityText` - identity source
- `RecencyWindow` - how far back to look (default: 24h)
- `RecentFilesTTL` - how long file refs live (default: 10min)

**Recent file importance scoring:**
- Modified < 1h ago: importance 0.7
- Modified < 6h ago: importance 0.6
- Else: importance 0.5

**Tests:**
- Created `memory/prepare_test.go` with 6 test cases
- Verified identity loading (from text and file)
- Verified memory instructions loaded
- Verified recent files have correct TTL and importance
- Verified old files (>24h) excluded
- All tests pass ✅

**Commit:** `1c206a7` - "Step 2: Add PrepareSession to load identity + recent files into memory"

---

## Next Steps

### Step 2: Load Identity & Recent Files into Memory

Create `PrepareSession()` function:

```go
func PrepareSession(repoRoot string, memStore *memory.Store) {
    // Load identity (permanent)
    identity := loadIdentityFile(repoRoot)
    memStore.Save(memory.Memory{
        ID: "identity",
        Content: identity,
        Tags: []string{"identity"},
        AlwaysLoad: true,
        Importance: 1.0,
        Source: "system",
    })
    
    // Load recent files (ephemeral, 10min TTL)
    recent := context.FindRecentlyModified(repoRoot, 24*time.Hour)
    for _, f := range recent {
        memStore.Save(memory.Memory{
            ID: "recent:" + f.RelativePath,
            Content: fmt.Sprintf("Recently modified: %s", f.RelativePath),
            Tags: []string{"recent", filepath.Ext(f.Path)},
            ExpiresAt: time.Now().Add(10*time.Minute),
            Importance: 0.5,
            Source: "system",
        })
    }
}
```

---

### Step 3: Create BuildContext() Query (DONE)

**Created `memory.BuildContext()`:**
```go
func (s *Store) BuildContext(req BuildContextRequest) ([]Memory, int, error)
```

**BuildContextRequest config:**
- `Tags` - tags to match (from message/query)
- `TokenBudget` - max tokens to include (default: 32000)
- `MinImportance` - threshold filter (default: 0.0)
- `ExcludeTags` - tags to exclude (e.g., "test", "archived")
- `IncludeAlwaysLoad` - include always-load memories (default: true)
- `MaxChunkSize` - skip memories larger than this (default: 0 = no limit)

**Priority order:**
1. AlwaysLoad memories (identity, instructions) - always first
2. Tag-matched memories (more matches = higher priority)
3. High importance memories
4. Recently accessed memories

**Score calculation:**
- Base: importance (0-1)
- Tag bonus: +0.3 per matching tag
- Recency bonus: +0.2 if accessed <1 day, +0.1 if <7 days
- Tie-breaker: smaller memories first (more likely to fit budget)

**Features:**
- Auto-excludes expired memories (WHERE expires_at > NOW())
- Budget enforcement: stops when budget hit
- AlwaysLoad memories can exceed budget (but still included)
- Size-aware filtering via MaxChunkSize

**Helper methods:**
- `scanMemory()` - reusable SQL row scanner
- `loadTags()` - loads tags for a memory ID
- `calculateScore()` - computes relevance score

**Tests:**
- Created `memory/builder_test.go` with 6 test cases
- Tag matching and priority ✅
- Token budget enforcement ✅
- Tag exclusion filtering ✅
- Importance threshold ✅
- AlwaysLoad priority ✅
- MaxChunkSize filtering ✅

**Commit:** `c9943b3` - "Step 3: Add BuildContext() for memory-backed prompt building"

---

### Step 4: Update Engine to Use Memory-Backed Context

Replace in `cmd/inber/engine.go`:
    // Priority: AlwaysLoad > match_count > importance
    // Track running token total, stop when budget hit
}
```

---

### Step 4: Update Engine to Use Memory-Backed Context (DONE)

**MAJOR MILESTONE: The system is now live and using memory-backed context!**

**Engine changes:**
- Replaced `context.AutoLoad()` with `memory.PrepareSession()`
- `BuildSystemPrompt()` now uses `memory.BuildContext()` 
- Falls back to old context.Store if memory unavailable (backward compat)
- Logging now shows "done (N memories)" instead of "done (N chunks)"

**Migration support:**
- Added `runMigrations()` to handle existing databases
- Automatically adds new columns to old DBs: `always_load`, `expires_at`, `tokens`
- Schema creation split: base table first, migrations add new columns
- Prevents "no such column" errors on upgrade

**How it works:**
1. **Session start:** `PrepareSession()` loads identity + memory instructions + recent files
2. **Per message:** `BuildContext()` queries memories matching message tags
3. **Priority:** AlwaysLoad > tag matches > importance > recency
4. **Budget:** Respects 50k token limit, stops when budget hit
5. **Output:** Returns `[]Memory` converted to `[]NamedBlock` for system prompt

**Testing:**
- ✅ Fresh install works
- ✅ Existing database migration succeeds
- ✅ Recent files detection works
- ✅ Memory recall works (tested: "what is my name?")
- ✅ Git recent files: "what files have I been working on?" works perfectly
- ✅ All tests pass

**Commit:** `5a7141b` - "Step 4: Wire memory-backed context into engine"

---

### Step 5: Remove Repo Map from Autoload (ALREADY DONE)

**Status:** Repo map is already available as a tool via `tools.RepoMap()` ✅

- ✅ Don't load repo map into memory
- ✅ Don't generate it at session start  
- ✅ Model calls `repo_map` tool when needed

This was already the design - repo maps are generated fresh on demand via the tool.

---

### Step 6: Deprecate context.Store (OPTIONAL - NOT URGENT)

**Status:** Can be done later as cleanup

The old `context/` package is still around for backward compatibility:
- `context/store.go` - still exists but not used in main path
- `context/builder.go` - not used (replaced by `memory/builder.go`)
- `context/repomap.go` - still needed for the `repo_map` tool
- `context/files.go`, `context/recency.go` - still used by `memory.PrepareSession()`

**Cleanup options:**
1. Leave it - it's not hurting anything, provides fallback
2. Remove unused files (store.go, builder.go) but keep loaders
3. Move loaders into memory/ package for full unification

**Recommendation:** Leave it for now. Focus on new features. Clean up later if needed.

---

## Design Decisions (Confirmed with Slava)

1. **SQLite I/O is fine** - don't over-optimize with in-memory caching
2. **Repo map NOT in memory** - always generated fresh via tool
3. **Only store lightweight refs** - identity + recent file stubs, not full content

## Benefits After Completion

- ✅ Unified retrieval: one query instead of multiple loaders
- ✅ Fewer tools: memory becomes queryable
- ✅ Smart importance scoring across all content types
- ✅ Repo map on-demand instead of always-loaded
- ✅ Simpler architecture: memory is the source of truth

## Open Questions

1. Should we keep `context/` package for loaders (files, recency)? 
   - Yes, but rename to something like `loaders/` or fold into `memory/`
2. How to handle large files?
   - Store stub with size: "Large file (1234 lines). Use read_file to retrieve."
3. Importance scoring for different content types?
   - Identity: 1.0
   - User preferences: 0.8
   - Recent files: 0.5-0.7 (decay over time)
   - Session summaries: 0.6
