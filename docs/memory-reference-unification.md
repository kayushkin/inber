# Memory & Reference Unification Design

## The Core Insight

You're absolutely right: **references and memories are fundamentally the same thing** - they're both:
- Content that can be retrieved
- Summarizable
- Scored by relevance/importance
- Tagged for categorization
- Searchable semantically
- Expandable on-demand

The only difference is **what they reference**:
- Memory references: Agent thoughts, decisions, learned facts
- File references: Code files, docs
- Identity references: Config files (identity.md, soul.md)
- Repo references: Code structure
- Tool references: Tool schemas
- Web references: URLs (future)

## Current Memory Structure

```go
type Memory struct {
    ID           string
    Content      string      // The actual text
    Summary      string      // Compressed version
    Tags         []string    // Categorization
    Importance   float64     // 0-1 relevance score
    Embedding    []float64   // Semantic search vector
    Source       string      // "user", "agent", "system"
    
    // Context unification fields (just added)
    AlwaysLoad   bool        // Always include in context
    ExpiresAt    *time.Time  // Expiration for ephemeral content
    Tokens       int         // Pre-computed token count
}
```

## Proposed Unified Structure

### Option 1: Add RefType to Memory (Simpler)

**Make Memory the universal reference type** by adding one field:

```go
type Memory struct {
    ID           string
    Content      string      // Full content OR reference stub
    Summary      string      // Brief description
    Tags         []string
    Importance   float64
    Embedding    []float64
    Source       string
    
    // Unification fields
    AlwaysLoad   bool
    ExpiresAt    *time.Time
    Tokens       int
    
    // NEW: Reference fields
    RefType      string      // "memory", "file", "identity", "repo-map", "tools", "web"
    RefTarget    string      // Path/URL/ID being referenced
    RefMetadata  string      // JSON blob for type-specific metadata
    ExpandModes  string      // JSON array of available expansion modes
}
```

**Schema changes**:
```sql
ALTER TABLE memories ADD COLUMN ref_type TEXT DEFAULT 'memory';
ALTER TABLE memories ADD COLUMN ref_target TEXT;
ALTER TABLE memories ADD COLUMN ref_metadata TEXT; -- JSON
ALTER TABLE memories ADD COLUMN expand_modes TEXT; -- JSON array

CREATE INDEX idx_ref_type ON memories(ref_type);
CREATE INDEX idx_ref_target ON memories(ref_target);
```

**Benefits**:
- ✅ Unified storage (one table, one query system)
- ✅ Unified search (semantic search works for ALL reference types)
- ✅ Existing memory tools work for references too
- ✅ Simpler architecture (no separate reference/memory stores)

**Queries**:
```go
// Search for ANY relevant content (memories + files + repo map)
results := memStore.Search("how does the engine work?", 10)
// Returns mix of:
// - Memory: "Decided to use event-driven engine..."
// - File ref: cmd/inber/engine.go
// - Repo map ref: package structure

// Filter by type
fileRefs := memStore.SearchByType(query, "file", 5)
memories := memStore.SearchByType(query, "memory", 5)

// Get always-load content
alwaysLoad := memStore.GetAlwaysLoad()
// Returns: identity, soul, user, tool registry
```

---

### Option 2: Rename to Reference Store (More Explicit)

Rename `memory.Store` → `reference.Store`, `Memory` → `Reference`, and update all the types:

```go
// reference/reference.go (renamed from memory/memory.go)
type Reference struct {
    ID           string
    Content      string      // Full content OR stub
    Summary      string
    Tags         []string
    Importance   float64
    Embedding    []float64
    Source       string      // "user", "agent", "system", "file", "repo"
    
    AlwaysLoad   bool
    ExpiresAt    *time.Time
    Tokens       int
    
    // Reference specifics
    Type         RefType     // memory, file, identity, repo-map, tools, web
    Target       string      // Path/URL/ID
    Metadata     map[string]any
    ExpandModes  []string
}

type RefType string
const (
    RefTypeMemory   RefType = "memory"
    RefTypeFile     RefType = "file"
    RefTypeIdentity RefType = "identity"
    RefTypeRepoMap  RefType = "repo-map"
    RefTypeTools    RefType = "tools"
    RefTypeWeb      RefType = "web"
)

type Store struct {
    db       *sql.DB
    embedder *Embedder
}
```

**Benefits**:
- ✅ Clearer naming (references are the superset)
- ✅ More explicit about unified nature
- ✅ Easier to reason about ("everything is a reference")

**Drawbacks**:
- ❌ Breaking change (rename everything)
- ❌ Confusing migration ("where did memory.Store go?")

---

## Recommendation: Option 1 (Add RefType to Memory)

**Why**: Simpler migration, backwards compatible, same benefits.

The name "Memory" is actually fine - it's the agent's **memory of everything**: thoughts, files it's seen, repo structure, tools available, etc. We're just expanding what "memory" means.

---

## Implementation Plan

### Phase 1: Extend Memory with Reference Fields

#### 1.1 Update Memory struct
```go
// memory/memory.go
type Memory struct {
    // ... existing fields ...
    
    // Reference fields
    RefType      string  // "memory" (default), "file", "identity", etc.
    RefTarget    string  // file path, URL, or empty for pure memories
    RefMetadata  string  // JSON blob for type-specific data
    ExpandModes  string  // JSON array: ["full", "summary", "lines:N-M"]
}
```

#### 1.2 Update schema
```sql
-- Migration: Add reference fields
ALTER TABLE memories ADD COLUMN ref_type TEXT DEFAULT 'memory';
ALTER TABLE memories ADD COLUMN ref_target TEXT;
ALTER TABLE memories ADD COLUMN ref_metadata TEXT;
ALTER TABLE memories ADD COLUMN expand_modes TEXT;

CREATE INDEX idx_ref_type ON memories(ref_type);
CREATE INDEX idx_ref_target ON memories(ref_target);
```

#### 1.3 Update Save/Get methods
```go
func (s *Store) Save(mem Memory) error {
    // Default ref_type to "memory" if empty
    if mem.RefType == "" {
        mem.RefType = "memory"
    }
    
    // Serialize metadata and expand_modes to JSON
    metadataJSON, _ := json.Marshal(mem.RefMetadata)
    modesJSON, _ := json.Marshal(mem.ExpandModes)
    
    _, err := s.db.Exec(`
        INSERT OR REPLACE INTO memories 
        (id, content, summary, ..., ref_type, ref_target, ref_metadata, expand_modes)
        VALUES (?, ?, ?, ..., ?, ?, ?, ?)
    `, mem.ID, mem.Content, mem.Summary, ..., mem.RefType, mem.RefTarget, metadataJSON, modesJSON)
    
    return err
}

func (s *Store) Get(id string) (Memory, error) {
    var mem Memory
    var metadataJSON, modesJSON string
    
    err := s.db.QueryRow(`
        SELECT id, content, summary, ..., ref_type, ref_target, ref_metadata, expand_modes
        FROM memories WHERE id = ?
    `, id).Scan(&mem.ID, &mem.Content, ..., &mem.RefType, &mem.RefTarget, &metadataJSON, &modesJSON)
    
    json.Unmarshal([]byte(metadataJSON), &mem.RefMetadata)
    json.Unmarshal([]byte(modesJSON), &mem.ExpandModes)
    
    return mem, err
}
```

---

### Phase 2: Add Reference Creation Helpers

Create convenience functions for each reference type:

```go
// memory/references.go (new file)
package memory

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "time"
)

// NewFileReference creates a reference to a file
func NewFileReference(path string, summary string) Memory {
    info, _ := os.Stat(path)
    
    metadata := map[string]any{
        "path":     path,
        "size":     info.Size(),
        "modified": info.ModTime(),
    }
    
    content, _ := os.ReadFile(path)
    tokens := len(content) / 4 // rough estimate
    
    return Memory{
        ID:          fmt.Sprintf("file:%s", path),
        Content:     fmt.Sprintf("File: %s\n%s", path, summary),
        Summary:     summary,
        Tags:        []string{"file", filepath.Ext(path), filepath.Dir(path)},
        Importance:  0.3, // Files default to low importance
        Source:      "system",
        AlwaysLoad:  false,
        ExpiresAt:   timePtr(time.Now().Add(1 * time.Hour)), // Refresh hourly
        Tokens:      tokens,
        RefType:     "file",
        RefTarget:   path,
        RefMetadata: toJSON(metadata),
        ExpandModes: toJSON([]string{"full", "summary", "lines:N-M"}),
        CreatedAt:   time.Now(),
        LastAccessed: time.Now(),
    }
}

// NewMemoryReference creates a pure memory (agent thought/decision)
func NewMemoryReference(content string, tags []string, importance float64) Memory {
    return Memory{
        ID:          generateID(),
        Content:     content,
        Summary:     extractSummary(content, 100), // First 100 chars
        Tags:        tags,
        Importance:  importance,
        Source:      "agent",
        AlwaysLoad:  false,
        Tokens:      len(content) / 4,
        RefType:     "memory",
        RefTarget:   "",
        ExpandModes: toJSON([]string{"full", "summary"}),
        CreatedAt:   time.Now(),
        LastAccessed: time.Now(),
    }
}

// NewIdentityReference creates a reference to identity.md
func NewIdentityReference(path string, content string) Memory {
    return Memory{
        ID:          "identity",
        Content:     content,
        Summary:     "Claxon identity: helpful coding assistant",
        Tags:        []string{"identity", "config", "system"},
        Importance:  1.0,
        Source:      "system",
        AlwaysLoad:  true,
        Tokens:      len(content) / 4,
        RefType:     "identity",
        RefTarget:   path,
        ExpandModes: toJSON([]string{"full", "summary"}),
        CreatedAt:   time.Now(),
        LastAccessed: time.Now(),
    }
}

// NewRepoMapReference creates a reference to repository structure
func NewRepoMapReference(scope string, repoMap string) Memory {
    metadata := map[string]any{
        "scope":    scope,
        "packages": countPackages(repoMap),
        "files":    countFiles(repoMap),
    }
    
    return Memory{
        ID:          fmt.Sprintf("repo-map:%s", scope),
        Content:     repoMap,
        Summary:     fmt.Sprintf("Repository structure: %s", scope),
        Tags:        []string{"repo-map", "structure", "code"},
        Importance:  0.3,
        Source:      "system",
        AlwaysLoad:  false,
        ExpiresAt:   timePtr(time.Now().Add(30 * time.Minute)), // Expire after 30min
        Tokens:      len(repoMap) / 4,
        RefType:     "repo-map",
        RefTarget:   scope,
        RefMetadata: toJSON(metadata),
        ExpandModes: toJSON([]string{"full", "package:NAME", "summary"}),
        CreatedAt:   time.Now(),
        LastAccessed: time.Now(),
    }
}

// NewToolRegistryReference creates a reference to available tools
func NewToolRegistryReference(tools []string, schemas string) Memory {
    metadata := map[string]any{
        "tool_count": len(tools),
        "tools":      tools,
    }
    
    return Memory{
        ID:          "tools:registry",
        Content:     schemas,
        Summary:     fmt.Sprintf("%d tools available: %s", len(tools), strings.Join(tools, ", ")),
        Tags:        []string{"tools", "registry", "capabilities"},
        Importance:  0.5,
        Source:      "system",
        AlwaysLoad:  true,
        Tokens:      len(schemas) / 4,
        RefType:     "tools",
        RefTarget:   "registry",
        RefMetadata: toJSON(metadata),
        ExpandModes: toJSON([]string{"full", "list", "tool:NAME"}),
        CreatedAt:   time.Now(),
        LastAccessed: time.Now(),
    }
}

// Helper functions
func toJSON(v any) string {
    b, _ := json.Marshal(v)
    return string(b)
}

func timePtr(t time.Time) *time.Time {
    return &t
}

func extractSummary(text string, maxLen int) string {
    if len(text) <= maxLen {
        return text
    }
    return text[:maxLen] + "..."
}
```

---

### Phase 3: Update Search to Handle Reference Types

```go
// memory/memory.go
func (s *Store) Search(query string, limit int) ([]Memory, error) {
    // Existing semantic search, but now returns all types
    return s.semanticSearch(query, "", limit)
}

// NEW: Search by specific reference type
func (s *Store) SearchByType(query string, refType string, limit int) ([]Memory, error) {
    return s.semanticSearch(query, refType, limit)
}

func (s *Store) semanticSearch(query string, refType string, limit int) ([]Memory, error) {
    qEmbed, err := s.embedder.Embed(query)
    if err != nil {
        return nil, err
    }
    
    // Build WHERE clause
    where := "WHERE (expires_at IS NULL OR expires_at > ?)"
    args := []any{time.Now().Unix()}
    
    if refType != "" {
        where += " AND ref_type = ?"
        args = append(args, refType)
    }
    
    rows, err := s.db.Query(`
        SELECT id, content, summary, ref_type, ref_target, importance, embedding
        FROM memories
        `+where+`
        ORDER BY importance DESC
        LIMIT ?
    `, append(args, limit*3)...)
    
    // Score by cosine similarity
    var results []Memory
    for rows.Next() {
        var mem Memory
        var embBlob []byte
        rows.Scan(&mem.ID, &mem.Content, &mem.Summary, &mem.RefType, &mem.RefTarget, &mem.Importance, &embBlob)
        
        embed := deserializeEmbedding(embBlob)
        similarity := cosineSimilarity(qEmbed, embed)
        
        mem.score = similarity * mem.Importance
        results = append(results, mem)
    }
    
    // Sort by score, return top N
    sort.Slice(results, func(i, j int) bool {
        return results[i].score > results[j].score
    })
    
    if len(results) > limit {
        results = results[:limit]
    }
    
    return results, nil
}

// NEW: Get all always-load references
func (s *Store) GetAlwaysLoad() ([]Memory, error) {
    rows, err := s.db.Query(`
        SELECT id, content, summary, ref_type, ref_target, tokens
        FROM memories
        WHERE always_load = 1
        ORDER BY importance DESC
    `)
    
    var results []Memory
    for rows.Next() {
        var mem Memory
        rows.Scan(&mem.ID, &mem.Content, &mem.Summary, &mem.RefType, &mem.RefTarget, &mem.Tokens)
        results = append(results, mem)
    }
    
    return results, nil
}

// NEW: Get references by type
func (s *Store) GetByType(refType string) ([]Memory, error) {
    rows, err := s.db.Query(`
        SELECT id, content, summary, ref_type, ref_target, importance, tokens
        FROM memories
        WHERE ref_type = ?
        AND (expires_at IS NULL OR expires_at > ?)
        ORDER BY importance DESC
    `, refType, time.Now().Unix())
    
    var results []Memory
    for rows.Next() {
        var mem Memory
        rows.Scan(&mem.ID, &mem.Content, &mem.Summary, &mem.RefType, &mem.RefTarget, &mem.Importance, &mem.Tokens)
        results = append(results, mem)
    }
    
    return results, nil
}
```

---

### Phase 4: Auto-Create References in Tools

Hook into existing tool execution to auto-create references:

```go
// tools/tools.go (adapter layer)
func (t *toolWrapper) Run(input map[string]any) (string, error) {
    result, err := t.tool.Run(input)
    
    // Auto-create reference after successful tool execution
    if err == nil && t.memStore != nil {
        switch t.tool.Name() {
        case "read_file":
            path := input["path"].(string)
            summary := extractFileSummary(result)
            ref := memory.NewFileReference(path, summary)
            t.memStore.Save(ref)
            
        case "repo_map":
            path := input["path"].(string)
            ref := memory.NewRepoMapReference(path, result)
            t.memStore.Save(ref)
        }
    }
    
    return result, err
}
```

---

### Phase 5: Rename Tools (Breaking Change, But Clear)

Rename memory tools to reference tools:

**Old**:
- `memory_search` → Search memories
- `memory_save` → Save memory
- `memory_expand` → Get full memory
- `memory_forget` → Forget memory

**New**:
- `reference_search` → Search references (memories, files, etc.)
- `memory_save` → Save memory (keep same name for pure memories)
- `reference_expand` → Get full reference content
- `reference_forget` → Forget reference

**Or keep backward compatibility**:
- `memory_search` → alias for `reference_search(type="memory")`
- Add new `reference_search` with `type` parameter

---

## Example Usage

### Auto-Creating References

```go
// Session start:
engine.MemStore.Save(memory.NewIdentityReference(".inber/identity.md", identityContent))
engine.MemStore.Save(memory.NewToolRegistryReference(toolNames, toolSchemas))

// After read_file("cmd/inber/engine.go"):
ref := memory.NewFileReference("cmd/inber/engine.go", "Main engine orchestration logic")
engine.MemStore.Save(ref)

// After memory.Save() with high importance:
mem := memory.NewMemoryReference("Decided to use SQLite for unified store", 
    []string{"decision", "architecture"}, 0.85)
engine.MemStore.Save(mem)

// After repo_map():
ref := memory.NewRepoMapReference(".", repoMapContent)
engine.MemStore.Save(ref)
```

### Searching Across All References

```go
// User: "How does the engine work?"

// Search returns mix of:
results := memStore.Search("engine work", 10)

// Results:
// 1. Memory: "Decided to use event-driven engine with hooks" (ref_type: memory)
// 2. File: cmd/inber/engine.go (ref_type: file)
// 3. Repo map: package structure showing agent/engine (ref_type: repo-map)

// Filter by type:
fileRefs := memStore.SearchByType("engine", "file", 5)
memories := memStore.SearchByType("engine", "memory", 5)
```

### Building Reference Prompt

```go
// Get always-load content
alwaysLoad := memStore.GetAlwaysLoad()
// Returns: identity, soul, user, tool registry

// Get relevant references
relevant := memStore.Search(userMessage, 20)

// Build reference blocks
var refBlocks []ReferenceBlock
for _, ref := range append(alwaysLoad, relevant...) {
    refBlocks = append(refBlocks, ReferenceBlock{
        RefID:      ref.ID,
        Type:       ref.RefType,
        Summary:    ref.Summary,
        SizeTokens: ref.Tokens,
        Modes:      parseExpandModes(ref.ExpandModes),
    })
}

// Send to Haiku optimizer...
```

---

## Benefits of Unified Approach

### 1. **Simpler Architecture**
- One store instead of two
- One search API for everything
- Unified semantic search across all content types

### 2. **Better Search Results**
Mix of memories + files + repo structure in one query:

```
User: "Why did we decide to use SQLite?"

Results:
1. Memory: "Decided to use SQLite because..." (ref_type: memory)
2. File: memory/memory.go (ref_type: file)
3. Memory: "SQLite migration added fields..." (ref_type: memory)
```

### 3. **Consistent API**
```go
// Same API for all reference types:
memStore.Save(fileRef)
memStore.Save(memory)
memStore.Save(repoMapRef)

// Same search:
memStore.Search("engine", 10) // returns all types
```

### 4. **Easier Prompt Optimization**
Haiku sees all available references in one list, makes holistic decisions about what to expand.

### 5. **Natural Tool Integration**
```go
// Tools automatically create references:
result := tools.ReadFile("agent.go")
// → ref:file:agent.go created

// Agent can search/expand later:
refs := memStore.SearchByType("agent", "file", 5)
```

---

## Migration Path

### Non-Breaking Migration

1. **Add new columns** (backwards compatible)
   ```sql
   ALTER TABLE memories ADD COLUMN ref_type TEXT DEFAULT 'memory';
   ALTER TABLE memories ADD COLUMN ref_target TEXT;
   -- etc.
   ```

2. **Existing memories** automatically get `ref_type='memory'`, `ref_target=NULL`

3. **New references** created with appropriate ref_type

4. **Tools renamed gradually**:
   - Add `reference_search`, keep `memory_search` as alias
   - Deprecate old names over time

---

## Next Steps

Want me to:

1. **Start with Phase 1** (add reference fields to Memory)?
2. **Build the helper functions** (NewFileReference, etc.)?
3. **Create a test** showing unified search across memories + files?

What feels right?
