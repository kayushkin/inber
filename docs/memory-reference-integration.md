# Memory-Reference Integration: Implementation Log

**Date:** 2026-03-01  
**Status:** Phase 1 Complete ✅

---

## 🎯 Goal

Unify the Memory and Reference systems so that:
- Files, repo maps, identities are stored as **lazy-loaded references**
- Content is read from disk **on-demand** (always fresh, never stale)
- Memory DB stores only **metadata** (summaries, tags, importance)
- Search works across all types (memories, files, repo structures)

---

## ✅ Phase 1: Add Reference Fields to Memory (COMPLETE)

### Changes Made

**1. Extended Memory struct** (`memory/memory.go`):
```go
type Memory struct {
    // ... existing fields ...
    
    // Reference fields for lazy loading
    RefType      string     // "memory", "file", "identity", "repo-map", "tools", "web"
    RefTarget    string     // file path, URL, or empty for pure memories
    IsLazy       bool       // if true, load content on-demand instead of from DB
}
```

**2. Database migration** (`memory/memory.go`):
```sql
ALTER TABLE memories ADD COLUMN ref_type TEXT NOT NULL DEFAULT 'memory';
ALTER TABLE memories ADD COLUMN ref_target TEXT;
ALTER TABLE memories ADD COLUMN is_lazy INTEGER NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS idx_ref_type ON memories(ref_type);
```

**3. Lazy loading logic** (`memory/lazy_loader.go`):
- `loadLazyContent()` - dispatcher based on RefType
- `loadFileContent()` - reads from disk
- `loadIdentityContent()` - reads identity files
- Called automatically in `Store.Get()` when `IsLazy == true`

**4. Updated Save/Get methods**:
- `Save()` now stores ref_type, ref_target, is_lazy
- `Get()` triggers lazy loading after DB read
- Content is **always fresh** (read from disk on each Get)

**5. Tests** (`memory/lazy_loader_test.go`):
- ✅ Lazy load file references
- ✅ Lazy load identity files
- ✅ Stale data test (file changes on disk → new content on next Get)
- ✅ Missing file error handling
- ✅ Non-lazy memories still work (content in DB)

---

## 📊 Benefits Demonstrated

### No More Stale Data
```go
// Save reference (no content stored)
store.Save(Memory{
    RefType:   "file",
    RefTarget: "agent.go",
    IsLazy:    true,
})

// Edit agent.go on disk
os.WriteFile("agent.go", []byte("new content"), 0644)

// Get memory → reads fresh content from disk
mem, _ := store.Get("agent-file")
// mem.Content == "new content" ✅
```

### No DB Bloat
```go
// Before: 4K lines stored in DB
store.Save(Memory{
    Content: readFile("builder.go"), // 4000 lines in DB
})

// After: Only metadata stored
store.Save(Memory{
    Summary:   "Context builder (234 lines)",
    RefType:   "file",
    RefTarget: "context/builder.go",
    IsLazy:    true,
    Tokens:    950,
})
// DB size: 200 bytes vs 120KB
```

---

## 🧪 Test Results

```bash
$ go test ./memory -run TestLazy -v
=== RUN   TestLazyLoading
=== RUN   TestLazyLoading/LazyLoadFile
=== RUN   TestLazyLoading/LazyLoadIdentity
=== RUN   TestLazyLoading/NonLazyMemory
--- PASS: TestLazyLoading (0.36s)
=== RUN   TestLazyLoadingStaleData
--- PASS: TestLazyLoadingStaleData (0.33s)
=== RUN   TestLazyLoadMissingFile
--- PASS: TestLazyLoadMissingFile (0.30s)
PASS

$ go test ./memory/...
ok  	github.com/kayushkin/inber/memory	3.847s
```

All tests pass, including existing unified field tests.

---

## 📝 Next Steps

### Phase 2: Auto-Reference Creation
Create references automatically:
- After `read_file()` → create file reference
- After `memory.Save()` with high importance → already a memory
- On session start → create identity/soul/user references
- After `repo_map()` tool call → create repo-map reference

### Phase 3: Reference-Aware Search
Update `memory.Search()` to:
- Return references by default (small summaries)
- Add `expand: bool` parameter to force full content loading
- Filter by `ref_type` for targeted searches

### Phase 4: Tool Integration
Create unified retrieval tool:
```go
// Old: 5 separate tools
memory_search(query)
memory_expand(id)
read_file(path)
repo_map()
// ...

// New: 1 unified tool
reference_query(
    query: "agent implementation",
    types: ["file", "memory"],
    expand: false  // returns summaries by default
)
```

### Phase 5: Prompt Builder
Update context builder to:
- Build prompts from references (not full content)
- Let model decide what to expand via tool calls
- 70-80% token reduction on large contexts

---

## 🔍 Design Decisions

### Why Lazy Loading?
**Problem:** Storing file content in DB creates staleness issues.  
**Solution:** Store only metadata, read from disk on-demand.

### Why Unified Table?
**Problem:** Separate reference/memory stores duplicate search logic.  
**Solution:** Single table with `ref_type` field, unified search API.

### Why Read on Every Get()?
**Problem:** Caching file content still creates staleness windows.  
**Solution:** Read from disk is fast (<1ms for most files), always fresh.

---

## 📊 Metrics

- **Files changed:** 3 (memory.go, lazy_loader.go, lazy_loader_test.go)
- **Lines added:** ~250
- **Tests added:** 3 test cases, 5 subtests
- **DB migration:** 3 new columns + 1 index
- **Breaking changes:** None (backward compatible)

---

## 🎉 Summary

Phase 1 complete! The foundation for reference-based memory is in place:
- ✅ Memory struct extended with reference fields
- ✅ Database schema migrated
- ✅ Lazy loading implemented and tested
- ✅ Stale data problem solved
- ✅ All tests passing

Next: Auto-create references after tool calls and on session start.
