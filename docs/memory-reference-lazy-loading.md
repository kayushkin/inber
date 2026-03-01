# Memory Reference Architecture: Lazy Loading

## The Problem with Storing Content

**Current design stores full content in the database**:
- File reference: Stores entire file content in `content` field
- Identity reference: Stores full identity.md in `content` field
- Repo map: Stores entire map in `content` field

**Problems**:
- ❌ **Stale data**: File changes, but DB still has old content
- ❌ **Bloat**: Storing 4K lines of code in SQLite when we can just read the file
- ❌ **Duplicate storage**: File exists on disk AND in database
- ❌ **Sync issues**: Have to update DB every time file changes

---

## Solution: Lazy Loading with Pointers

**Store only metadata, load content on-demand**:

```go
type Memory struct {
    ID           string
    Content      string      // ONLY for pure memories (ref_type='memory')
    Summary      string      // Brief description (always stored)
    Tags         []string
    Importance   float64
    Embedding    []float64   // Embedding of summary (not full content)
    
    // Reference fields
    RefType      string      // "memory", "file", "identity", "repo-map", "tools", "web"
    RefTarget    string      // Path/URL to retrieve content from
    RefMetadata  string      // JSON: file info, line ranges, etc.
    ExpandModes  string      // JSON: ["full", "summary", "lines:N-M"]
    
    // Content retrieval
    IsLazy       bool        // If true, content loaded on-demand
}
```

### Storage Strategy by Type

| RefType | Content Field | RefTarget | Lazy Load |
|---------|--------------|-----------|-----------|
| `memory` | Full thought/decision | Empty | ❌ No (store in DB) |
| `file` | Empty or summary | File path | ✅ Yes (read from disk) |
| `identity` | Empty | `.inber/identity.md` | ✅ Yes (read from disk) |
| `soul` | Empty | `.inber/soul.md` | ✅ Yes (read from disk) |
| `user` | Empty | `.inber/user.md` | ✅ Yes (read from disk) |
| `repo-map` | Empty | Scope path | ✅ Yes (generate fresh) |
| `tools` | Empty | "registry" | ✅ Yes (build from tools) |
| `web` | Empty or cached | URL | ✅ Yes (HTTP fetch) |

---

## Architecture

### 1. Storage (Metadata Only)

```go
// memory/memory.go

// Save only stores metadata for lazy references
func (s *Store) Save(mem Memory) error {
    // For lazy references, don't store content in DB
    contentToStore := mem.Content
    if mem.IsLazy && mem.RefType != "memory" {
        contentToStore = "" // Don't store content for lazy refs
    }
    
    // Always store summary (used for search)
    if mem.Summary == "" && mem.RefType != "memory" {
        mem.Summary = generateSummary(mem.RefTarget, mem.RefType)
    }
    
    // Embed summary, not full content (for lazy refs)
    embedText := mem.Summary
    if mem.RefType == "memory" {
        embedText = mem.Content // Embed full content for pure memories
    }
    
    embedding, _ := s.embedder.Embed(embedText)
    
    _, err := s.db.Exec(`
        INSERT OR REPLACE INTO memories 
        (id, content, summary, ref_type, ref_target, ref_metadata, 
         expand_modes, is_lazy, embedding, ...)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ...)
    `, mem.ID, contentToStore, mem.Summary, mem.RefType, mem.RefTarget,
       mem.RefMetadata, mem.ExpandModes, mem.IsLazy, serializeEmbedding(embedding), ...)
    
    return err
}
```

### 2. Retrieval (Lazy Load Content)

```go
// memory/memory.go

// Get retrieves metadata, optionally loads content
func (s *Store) Get(id string, expand bool) (Memory, error) {
    var mem Memory
    
    err := s.db.QueryRow(`
        SELECT id, content, summary, ref_type, ref_target, 
               ref_metadata, expand_modes, is_lazy, ...
        FROM memories WHERE id = ?
    `, id).Scan(&mem.ID, &mem.Content, &mem.Summary, &mem.RefType, 
                &mem.RefTarget, &mem.RefMetadata, &mem.ExpandModes, 
                &mem.IsLazy, ...)
    
    if err != nil {
        return Memory{}, err
    }
    
    // If expand requested and this is a lazy reference, load content
    if expand && mem.IsLazy {
        content, err := s.loadContent(mem)
        if err != nil {
            return mem, fmt.Errorf("load content: %w", err)
        }
        mem.Content = content
    }
    
    return mem, nil
}

// loadContent loads actual content based on reference type
func (s *Store) loadContent(mem Memory) (string, error) {
    switch mem.RefType {
    case "memory":
        // Already stored in DB
        return mem.Content, nil
        
    case "file":
        // Read from disk
        content, err := os.ReadFile(mem.RefTarget)
        if err != nil {
            return "", fmt.Errorf("read file %s: %w", mem.RefTarget, err)
        }
        return string(content), nil
        
    case "identity", "soul", "user":
        // Read config file
        content, err := os.ReadFile(mem.RefTarget)
        if err != nil {
            return "", fmt.Errorf("read config %s: %w", mem.RefTarget, err)
        }
        return string(content), nil
        
    case "repo-map":
        // Generate fresh repo map
        repoMap, err := s.generateRepoMap(mem.RefTarget)
        if err != nil {
            return "", fmt.Errorf("generate repo map: %w", err)
        }
        return repoMap, nil
        
    case "tools":
        // Build tool registry from current tools
        registry, err := s.buildToolRegistry()
        if err != nil {
            return "", fmt.Errorf("build tool registry: %w", err)
        }
        return registry, nil
        
    case "web":
        // Fetch from URL (with caching)
        content, err := s.fetchURL(mem.RefTarget)
        if err != nil {
            return "", fmt.Errorf("fetch url %s: %w", mem.RefTarget, err)
        }
        return content, nil
        
    default:
        return "", fmt.Errorf("unknown ref_type: %s", mem.RefType)
    }
}
```

### 3. Partial Expansion (Lines, Sections)

```go
// memory/memory.go

// Expand loads content with specific expansion mode
func (s *Store) Expand(id string, mode string, params map[string]any) (string, error) {
    mem, err := s.Get(id, false) // Don't auto-expand
    if err != nil {
        return "", err
    }
    
    switch mode {
    case "KEEP_REF":
        // Just return summary
        return fmt.Sprintf("@ref:%s [%s]", mem.ID, mem.Summary), nil
        
    case "EXPAND_SUMMARY":
        // Return summary only
        return fmt.Sprintf("@ref:%s\n%s", mem.ID, mem.Summary), nil
        
    case "EXPAND_FULL":
        // Load full content
        content, err := s.loadContent(mem)
        if err != nil {
            return "", err
        }
        return fmt.Sprintf("@ref:%s [EXPANDED]\n%s", mem.ID, content), nil
        
    case "EXPAND_PARTIAL":
        // Load specific section
        return s.expandPartial(mem, params)
        
    default:
        return "", fmt.Errorf("unknown expand mode: %s", mode)
    }
}

func (s *Store) expandPartial(mem Memory, params map[string]any) (string, error) {
    switch mem.RefType {
    case "file":
        // Expand specific lines
        if lineRange, ok := params["lines"].(string); ok {
            // Parse "45-120"
            start, end := parseLineRange(lineRange)
            lines, err := readFileLines(mem.RefTarget, start, end)
            if err != nil {
                return "", err
            }
            return fmt.Sprintf("@ref:%s [lines %s]\n%s", mem.ID, lineRange, lines), nil
        }
        
    case "repo-map":
        // Expand specific package
        if pkg, ok := params["package"].(string); ok {
            fullMap, _ := s.loadContent(mem)
            pkgMap := extractPackage(fullMap, pkg)
            return fmt.Sprintf("@ref:%s [package %s]\n%s", mem.ID, pkg, pkgMap), nil
        }
        
    case "tools":
        // Expand specific tool
        if tool, ok := params["tool"].(string); ok {
            fullRegistry, _ := s.loadContent(mem)
            toolSchema := extractTool(fullRegistry, tool)
            return fmt.Sprintf("@ref:%s [tool %s]\n%s", mem.ID, tool, toolSchema), nil
        }
    }
    
    // Fallback to full expansion
    content, err := s.loadContent(mem)
    return content, err
}
```

---

## Reference Creation Helpers

### File Reference (Lazy)

```go
// memory/references.go

func NewFileReference(path string) Memory {
    info, _ := os.Stat(path)
    
    // Extract summary without reading full file
    summary := extractFileSummary(path) // Read first 200 chars or first comment
    
    metadata := map[string]any{
        "path":     path,
        "size":     info.Size(),
        "modified": info.ModTime(),
        "ext":      filepath.Ext(path),
    }
    
    return Memory{
        ID:          fmt.Sprintf("file:%s", path),
        Content:     "",           // DON'T STORE CONTENT
        Summary:     summary,      // Store brief summary
        Tags:        []string{"file", filepath.Ext(path), "code"},
        Importance:  0.3,
        Source:      "system",
        RefType:     "file",
        RefTarget:   path,         // Path to read from
        RefMetadata: toJSON(metadata),
        ExpandModes: toJSON([]string{"full", "lines:N-M", "summary"}),
        IsLazy:      true,         // Load on-demand
        ExpiresAt:   nil,          // Files don't expire (just read fresh)
        CreatedAt:   time.Now(),
        LastAccessed: time.Now(),
    }
}

func extractFileSummary(path string) string {
    // Read just first 200 bytes
    f, err := os.Open(path)
    if err != nil {
        return fmt.Sprintf("File: %s", path)
    }
    defer f.Close()
    
    buf := make([]byte, 200)
    n, _ := f.Read(buf)
    
    // Extract package comment or first line
    content := string(buf[:n])
    if strings.HasPrefix(content, "//") {
        lines := strings.Split(content, "\n")
        return lines[0] // First comment line
    }
    
    return fmt.Sprintf("File: %s (%s)", filepath.Base(path), filepath.Ext(path))
}
```

### Identity Reference (Lazy)

```go
func NewIdentityReference(path string) Memory {
    // Don't read content, just store path
    summary := "Claxon identity: helpful coding assistant, direct communication"
    
    return Memory{
        ID:          "identity",
        Content:     "",           // DON'T STORE
        Summary:     summary,
        Tags:        []string{"identity", "config", "system"},
        Importance:  1.0,
        Source:      "system",
        AlwaysLoad:  true,
        RefType:     "identity",
        RefTarget:   path,         // Read from here when expanded
        ExpandModes: toJSON([]string{"full", "summary"}),
        IsLazy:      true,
        CreatedAt:   time.Now(),
        LastAccessed: time.Now(),
    }
}
```

### Repo Map Reference (Lazy, Ephemeral)

```go
func NewRepoMapReference(scope string) Memory {
    summary := fmt.Sprintf("Repository structure: %s", scope)
    
    metadata := map[string]any{
        "scope": scope,
        "last_generated": time.Now(),
    }
    
    return Memory{
        ID:          fmt.Sprintf("repo-map:%s", scope),
        Content:     "",           // DON'T STORE (generate fresh)
        Summary:     summary,
        Tags:        []string{"repo-map", "structure", "code"},
        Importance:  0.3,
        Source:      "system",
        RefType:     "repo-map",
        RefTarget:   scope,        // Generate for this scope
        RefMetadata: toJSON(metadata),
        ExpandModes: toJSON([]string{"full", "package:NAME", "summary"}),
        IsLazy:      true,
        ExpiresAt:   timePtr(time.Now().Add(30 * time.Minute)), // Re-check after 30min
        CreatedAt:   time.Now(),
        LastAccessed: time.Now(),
    }
}
```

### Pure Memory (Stored in DB)

```go
func NewMemoryReference(content string, tags []string, importance float64) Memory {
    return Memory{
        ID:          generateID(),
        Content:     content,      // STORE in DB (it's a thought, not a file)
        Summary:     extractSummary(content, 100),
        Tags:        tags,
        Importance:  importance,
        Source:      "agent",
        RefType:     "memory",
        RefTarget:   "",           // No external target
        ExpandModes: toJSON([]string{"full", "summary"}),
        IsLazy:      false,        // Already in DB
        CreatedAt:   time.Now(),
        LastAccessed: time.Now(),
    }
}
```

---

## Search: Still Works on Summaries

```go
// memory/memory.go

func (s *Store) Search(query string, limit int) ([]Memory, error) {
    // Embed query
    qEmbed, err := s.embedder.Embed(query)
    if err != nil {
        return nil, err
    }
    
    // Search on summary embeddings (not full content)
    rows, err := s.db.Query(`
        SELECT id, content, summary, ref_type, ref_target, 
               importance, embedding, is_lazy
        FROM memories
        WHERE (expires_at IS NULL OR expires_at > ?)
        ORDER BY importance DESC
        LIMIT ?
    `, time.Now().Unix(), limit*3)
    
    var results []Memory
    for rows.Next() {
        var mem Memory
        var embBlob []byte
        rows.Scan(&mem.ID, &mem.Content, &mem.Summary, &mem.RefType, 
                  &mem.RefTarget, &mem.Importance, &embBlob, &mem.IsLazy)
        
        // Score by similarity to summary embedding
        embed := deserializeEmbedding(embBlob)
        similarity := cosineSimilarity(qEmbed, embed)
        mem.score = similarity * mem.Importance
        
        results = append(results, mem)
    }
    
    // Sort and limit
    sort.Slice(results, func(i, j int) bool {
        return results[i].score > results[j].score
    })
    
    if len(results) > limit {
        results = results[:limit]
    }
    
    // DON'T auto-expand content here (just return metadata)
    return results, nil
}
```

**Key**: Search uses summary embeddings, so it's fast. Content only loaded when explicitly expanded.

---

## Expansion on Retrieval

```go
// Agent workflow:

// 1. Search returns metadata only
results := memStore.Search("how does engine work", 10)
// Returns: [{ID: "file:engine.go", Summary: "Main orchestration", Content: ""}]

// 2. Haiku decides to expand file:engine.go
expander := reference.NewExpander(memStore)
expandedContent := memStore.Expand("file:engine.go", "EXPAND_FULL", nil)
// NOW reads from disk: os.ReadFile("cmd/inber/engine.go")

// 3. Main model sees expanded content
prompt := buildPrompt(expandedContent)
```

---

## Schema Updates

```sql
-- Add is_lazy column
ALTER TABLE memories ADD COLUMN is_lazy INTEGER DEFAULT 0;

CREATE INDEX idx_is_lazy ON memories(is_lazy);
```

---

## Benefits

### 1. **Always Fresh**
```go
// Every time identity is expanded, reads from disk
mem := memStore.Get("identity", expand=true)
// Reads .inber/identity.md NOW, not cached version
```

### 2. **No Bloat**
```sql
-- Database only stores metadata
SELECT id, summary, ref_type, ref_target FROM memories;

-- Results:
-- file:engine.go | "Main orchestration" | file | cmd/inber/engine.go
-- identity       | "Claxon assistant"   | identity | .inber/identity.md
-- memory:abc123  | "Decided to use..."  | memory | (full content in DB)
```

### 3. **No Sync Issues**
```
1. User edits engine.go
2. Next expansion reads fresh content from disk
3. No DB update needed
```

### 4. **Selective Loading**
```go
// Only load what Haiku requested
decisions := optimizer.OptimizePrompt(userMsg, references)
// Haiku says: expand engine.go, keep identity as ref

for _, decision := range decisions {
    if decision.Mode == "EXPAND_FULL" {
        content := memStore.Expand(decision.RefID, "EXPAND_FULL", nil)
        // Only reads engine.go from disk, not identity
    }
}
```

---

## Edge Cases

### 1. **File Deleted**
```go
func (s *Store) loadContent(mem Memory) (string, error) {
    if mem.RefType == "file" {
        content, err := os.ReadFile(mem.RefTarget)
        if os.IsNotExist(err) {
            // Mark reference as stale
            s.db.Exec("UPDATE memories SET expires_at = 0 WHERE id = ?", mem.ID)
            return "", fmt.Errorf("file not found: %s", mem.RefTarget)
        }
        return string(content), nil
    }
}
```

### 2. **Repo Map Stale**
```go
// ExpiresAt set to 30 minutes
// Next search will skip expired references
rows := s.db.Query(`
    SELECT ... FROM memories 
    WHERE expires_at IS NULL OR expires_at > ?
`, time.Now().Unix())

// Manual refresh:
newRepoMap := tools.RepoMap(".")
ref := NewRepoMapReference(".")
memStore.Save(ref) // Updates timestamp
```

### 3. **Large File**
```go
func (s *Store) Expand(id string, mode string, params map[string]any) (string, error) {
    // For huge files, support line ranges
    if mode == "EXPAND_PARTIAL" {
        if lineRange, ok := params["lines"].(string); ok {
            // Only read specific lines, not entire file
            return readFileLines(mem.RefTarget, start, end)
        }
    }
}
```

---

## Migration

### Step 1: Add Schema
```sql
ALTER TABLE memories ADD COLUMN is_lazy INTEGER DEFAULT 0;
CREATE INDEX idx_is_lazy ON memories(is_lazy);
```

### Step 2: Update Save Logic
```go
func (s *Store) Save(mem Memory) error {
    // Don't store content for lazy refs
    contentToStore := mem.Content
    if mem.IsLazy && mem.RefType != "memory" {
        contentToStore = ""
    }
    // ... rest of save
}
```

### Step 3: Update Get Logic
```go
func (s *Store) Get(id string, expand bool) (Memory, error) {
    // Load from DB
    // If expand && is_lazy, call loadContent()
}
```

### Step 4: Create References
```go
// Session start
memStore.Save(NewIdentityReference(".inber/identity.md"))
memStore.Save(NewFileReference("cmd/inber/engine.go"))

// Auto-create after read_file
ref := NewFileReference(path)
memStore.Save(ref)
```

---

## Example Flow

### User: "What's my identity?"

```go
// 1. Search (returns metadata only)
results := memStore.Search("identity", 5)
// [{ID: "identity", Summary: "Claxon assistant", Content: "", RefTarget: ".inber/identity.md"}]

// 2. Haiku optimizer
optimizer.OptimizePrompt("What's my identity?", results)
// Decision: EXPAND_FULL for identity

// 3. Expand (reads from disk NOW)
content := memStore.Expand("identity", "EXPAND_FULL", nil)
// → os.ReadFile(".inber/identity.md")
// → Returns fresh content

// 4. Send to main model
prompt := buildPrompt(content)
```

**Result**: Always reads fresh `.inber/identity.md` from disk, never stale.

---

## Next Steps

Want me to:
1. **Implement lazy loading** (update Save/Get methods)?
2. **Create reference helpers** with IsLazy=true?
3. **Build the loadContent dispatcher**?
