# Auto-Reference Creation Demo

## Interactive Example

### Turn 1: User Reads a File
```
User: "Read the auto_references.go file"

Agent calls: read_file(path="memory/auto_references.go")

Tool returns: 
  package memory
  
  import (
      "encoding/json"
      ...
  )
  ...
  [330 lines total]

🔧 Auto-Reference Created:
Memory{
    ID:         "f4e8c3...",
    Content:    "",  // Empty - lazy loaded
    Summary:    "File memory/auto_references.go (330 lines, read at 14:45)",
    RefType:    "file",
    RefTarget:  "memory/auto_references.go",
    IsLazy:     true,
    Importance: 0.4,
    Tags:       ["file", "read-file", "auto_references.go", ".go"],
    Source:     "auto-reference",
}
```

### Turn 2: User Asks About Structure
```
User: "What packages does this project have?"

Agent calls: repo_map()

Tool returns:
  pkg agent
    agent.go
      type Agent struct
      func (a *Agent) Run()
      ...
  pkg memory
    auto_references.go
      type AutoReferenceManager struct
      func NewAutoReferenceManager()
      ...
  [8 packages total]

🔧 Auto-Reference Created:
Memory{
    ID:         "repo-map-20260301-144732",
    Content:    "pkg agent\n  agent.go\n...",  // Stored (ephemeral)
    Summary:    "Repository structure (8 packages, generated 14:47)",
    RefType:    "repo-map",
    IsLazy:     false,
    Importance: 0.3,
    ExpiresAt:  2026-03-01 14:57:32,  // Expires in 10min
    Tags:       ["repo-map", "structure", "code-introspection"],
    Source:     "auto-reference",
}
```

### Turn 3: User Queries Memory
```
User: "What files have I looked at recently?"

Agent calls: memory_search(query="files recently read")

Memory returns:
  1. File memory/auto_references.go (330 lines, read at 14:45)
     Tags: file, read-file, auto_references.go, .go
     Importance: 0.4

Agent response:
  "You recently read memory/auto_references.go (330 lines)"
```

### Turn 4: Agent Lazy-Loads Content
```
User: "What's the CreateFileReference function signature?"

Agent thinks: "I have a reference to auto_references.go, 
              but need the actual content..."

Context builder:
  - Finds Memory{RefType:"file", IsLazy:true}
  - Calls lazy loader: LoadContent("f4e8c3...")
  - Lazy loader reads file from disk
  - Returns fresh content (even if file was modified!)

Agent response:
  "The CreateFileReference function signature is:
   func (m *AutoReferenceManager) createFileReference(toolID, inputJSON string) error"
```

## Before vs After

### Before Auto-References
```
User: "Read auth.go"
  ↓
read_file returns 500 lines
  ↓
500 lines stored in conversation history
  ↓
User: "Read another file"
  ↓
Another 500 lines added
  ↓
Token count: 4000+ tokens in history
```

### After Auto-References
```
User: "Read auth.go"
  ↓
read_file returns 500 lines
  ↓
Reference created: "File auth.go (500 lines, read at 14:30)"
  ↓
User: "Read another file"
  ↓
Another reference created
  ↓
Token count: ~100 tokens (just summaries)
  ↓
If agent needs content: Lazy-load from disk on-demand
```

## Database View

### Auto-Created References in SQLite
```sql
sqlite> SELECT id, ref_type, ref_target, is_lazy, summary, expires_at 
        FROM memories 
        WHERE source = 'auto-reference'
        ORDER BY created_at DESC;

f4e8c3...  | file     | memory/auto_references.go | 1 | File memory/auto_references.go (330 lines, read at 14:45) | NULL
repo-map-  | repo-map | NULL                      | 0 | Repository structure (8 packages, generated 14:47)        | 1709316452
```

## Configuration

Users can customize auto-reference behavior:

```go
// In memory/auto_references.go
config := AutoReferenceConfig{
    CreateOnReadFile: true,   // Auto-create after read_file
    CreateOnRepoMap:  true,   // Auto-create after repo_map
    CreateOnRecent:   true,   // Auto-create after recent_files
    MinFileSize:      100,    // Skip tiny files
    ExpiresAfter:     10 * time.Minute,  // TTL for ephemeral refs
}
```

## Memory Search Example

```
User: "What Go files did I look at?"

memory_search(query="Go files", limit=10)
  ↓
Semantic search finds:
  1. File memory/auto_references.go (similarity: 0.92)
  2. File cmd/inber/engine.go (similarity: 0.87)
  3. File agent/agent.go (similarity: 0.81)
  ↓
Agent: "You've looked at:
  - memory/auto_references.go (330 lines)
  - cmd/inber/engine.go (650 lines)
  - agent/agent.go (420 lines)"
```

## Smart Features

### 1. Skips Small Files
```
User: "Read README.md" (20 bytes)
  ↓
File too small (< 100 bytes)
  ↓
No reference created (not worth the overhead)
```

### 2. Fresh Content Always
```
Before request:
  File content: "func old() {}"

Auto-reference created

Developer edits file:
  File content: "func new() {}"

Agent lazy-loads:
  Returns: "func new() {}"  ← Fresh!
```

### 3. Expired References Auto-Purged
```
14:47 - repo-map reference created (ExpiresAt: 14:57)
14:50 - User queries: sees repo-map in results
15:00 - User queries: repo-map excluded (expired)
```

## Diagnostics

Check what references were created:

```bash
$ sqlite3 .inber/memory.db
sqlite> SELECT 
    ref_type,
    COUNT(*) as count,
    AVG(tokens) as avg_tokens,
    SUM(CASE WHEN is_lazy THEN 1 ELSE 0 END) as lazy_count
FROM memories 
WHERE source = 'auto-reference'
GROUP BY ref_type;

file     | 12 | 245.3 | 12  (all lazy)
repo-map | 3  | 892.7 | 0   (stored)
recent   | 2  | 156.0 | 0   (stored)
```

## Performance Impact

**Overhead per tool call:**
- File stat: ~1ms
- Line count: ~5ms
- Database insert: ~10ms
- **Total: ~16ms** (negligible compared to LLM call ~2000ms)

**Token savings:**
- Before: 4000+ tokens for full file content
- After: ~50 tokens for reference stub
- **Savings: ~98%**

**Memory savings:**
- Before: ~10KB per file in DB
- After: ~200 bytes per reference
- **Savings: ~98%**
