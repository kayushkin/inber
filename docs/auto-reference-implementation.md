# Auto-Reference Creation Implementation

**Date:** 2026-03-01  
**Status:** ✅ Complete  

## Summary

Successfully implemented automatic reference creation that triggers after tool calls. When the user reads a file or generates a repo map, inber now automatically creates a memory reference that can be lazy-loaded later.

## What Changed

### 1. Updated Memory Fields (Already Done in Step 1)
- `RefType` - Type of reference ("file", "repo-map", "recent", "memory")
- `RefTarget` - Target path/identifier for lazy loading
- `IsLazy` - Whether content should be loaded on-demand

### 2. Created Auto-Reference Manager (`memory/auto_references.go`)

**Key Functions:**
- `OnToolResult(toolID, name, inputJSON, output string)` - Hook called after tool execution
- `createFileReference()` - Creates lazy file reference after `read_file`
- `createRepoMapReference()` - Creates ephemeral repo map reference after `repo_map`
- `createRecentFilesReference()` - Creates recent files reference after `recent_files`

**Configuration:**
```go
type AutoReferenceConfig struct {
    CreateOnReadFile bool         // Auto-create after read_file
    CreateOnRepoMap  bool         // Auto-create after repo_map
    CreateOnRecent   bool         // Auto-create after recent_files
    MinFileSize      int          // Don't reference tiny files (default: 100 bytes)
    ExpiresAfter     time.Duration // TTL for ephemeral refs (default: 10min)
}
```

### 3. Integrated Into Engine (`cmd/inber/engine.go`)

**Engine Struct:**
- Added `autoRefMgr *memory.AutoReferenceManager`
- Added `toolInputsCache map[string]string` to track tool inputs

**Hook Integration:**
- `OnToolCall` now caches tool input JSON
- `OnToolResult` calls `autoRefMgr.OnToolResult()` for successful tool calls
- Errors are logged but don't break the turn

### 4. Fixed Database Query Functions (`memory/memory.go`)

Updated `Search()` and `ListRecent()` to include new columns:
- Added `ref_type`, `ref_target`, `is_lazy` to SELECT statements
- Added corresponding `refTarget sql.NullString` variable
- Set `m.RefTarget = refTarget.String` after scanning

## How It Works

### Flow Diagram
```
User: "Read memory/auto_references.go"
  ↓
Agent calls: read_file(path="memory/auto_references.go")
  ↓
OnToolCall hook: caches input JSON
  ↓
Tool executes: returns file content
  ↓
OnToolResult hook: 
  - Parses input JSON to extract path
  - Creates Memory{RefType:"file", RefTarget:"memory/auto_references.go", IsLazy:true}
  - Saves to database
  ↓
Next turn: Agent sees "File memory/auto_references.go (230 lines, read at 14:32)" in context
  ↓
If needed: Lazy loader reads file from disk on-demand
```

### Example References Created

**File Reference (lazy):**
```go
Memory{
    ID:         "8e3c7...",
    Content:    "",  // Empty - loaded on demand
    Summary:    "File memory/auto_references.go (230 lines, read at 14:32)",
    RefType:    "file",
    RefTarget:  "memory/auto_references.go",
    IsLazy:     true,
    Importance: 0.4,
    Tags:       []string{"file", "read-file", "auto_references.go", ".go"},
}
```

**Repo Map Reference (ephemeral):**
```go
Memory{
    ID:         "repo-map-20260301-143215",
    Content:    "pkg memory\n  auto_references.go\n...",
    Summary:    "Repository structure (8 packages, generated 14:32)",
    RefType:    "repo-map",
    IsLazy:     false,  // Content stored (can't be regenerated identically)
    Importance: 0.3,
    ExpiresAt:  time.Now().Add(10*time.Minute),
    Tags:       []string{"repo-map", "structure", "code-introspection"},
}
```

## Benefits

1. **No Stale Data** - File references lazy-load from disk, always fresh
2. **Lower Memory Usage** - Database stores metadata only, not 10KB+ file contents
3. **Automatic** - No user intervention needed
4. **Queryable** - References appear in semantic search results
5. **Efficient** - Small files (< 100 bytes) are skipped

## Testing

Created `memory/auto_references_test.go` with 3 test cases:
- ✅ `CreateFileReference` - Verifies file reference creation
- ✅ `CreateRepoMapReference` - Verifies repo map reference creation
- ✅ `SkipSmallFiles` - Verifies small files are skipped

All tests pass:
```bash
$ go test ./memory -run TestAutoReferences -v
=== RUN   TestAutoReferences
=== RUN   TestAutoReferences/CreateFileReference
=== RUN   TestAutoReferences/CreateRepoMapReference
=== RUN   TestAutoReferences/SkipSmallFiles
--- PASS: TestAutoReferences (0.31s)
PASS
ok      github.com/kayushkin/inber/memory       0.308s
```

## Next Steps (Optional)

Future enhancements could include:
1. **Expand on Session Start** - Auto-create identity/soul/user references
2. **Memory Save References** - Create refs for high-importance memories (>0.7)
3. **Web References** - Cache web pages after web_search tool calls
4. **Deduplication** - Prevent duplicate file references in same session

## Files Modified

- `memory/auto_references.go` - Created new file (330 lines)
- `memory/auto_references_test.go` - Created new file (178 lines)
- `memory/memory.go` - Updated `Search()` and `ListRecent()` queries
- `cmd/inber/engine.go` - Integrated auto-reference manager into hooks

## Commit Message

```
Add auto-reference creation after tool calls

- Create file references after read_file (lazy-loaded from disk)
- Create repo-map references after repo_map (ephemeral, 10min TTL)
- Create recent-file references after recent_files
- Skip files smaller than 100 bytes
- Integrate into OnToolResult hook
- Add comprehensive tests

Fixes issue where file content was stored in memory and went stale.
Now creates lightweight references that load fresh content on-demand.
```
