# Context-Memory-Tool Hybrid Architecture

## The Key Insight

You're absolutely right: **Some things shouldn't be in memory at all - they should be tools.**

### Three Categories of Context

1. **Static Memory** — Persists across sessions, stored in memory.db
   - Identity/system prompt
   - User preferences
   - Session summaries
   - Learned facts/decisions
   
2. **Dynamic Tools** — Generated on-demand when called
   - `repo_map()` — Generates fresh repo structure
   - `recent_files()` — Lists files modified since timestamp
   - `search_code(query)` — Searches codebase
   - `file_history(path)` — Git log for file
   
3. **Ephemeral Metadata** — Short-lived references in memory
   - "Repo map was generated 10 minutes ago" (expires in 1 hour)
   - "Recent files checked at 14:32" (expires in 10 minutes)
   - Points to tool calls, not actual content

---

## Proposed Architecture

### Memory: Static Knowledge Only

```go
type Memory struct {
    ID           string
    Content      string
    Tags         []string
    Importance   float64
    AlwaysLoad   bool      // Identity, critical config
    ExpiresAt    *time.Time // For ephemeral metadata
    Source       string    // "user", "agent", "system", "tool"
    // ... existing fields ...
}
```

**What goes in memory:**
- Identity: `Memory{Content: "You are Claxon...", AlwaysLoad: true}`
- Preference: `Memory{Content: "User prefers blue", Tags: ["preference"]}`
- Decision: `Memory{Content: "Deployment uses nginx...", Tags: ["deployment"]}`
- Tool metadata: `Memory{Content: "Repo map last generated at 14:30", ExpiresAt: +1h}`

**What does NOT go in memory:**
- Full repo map (20KB+) ❌
- Full file contents ❌
- Full git log ❌

### Tools: Dynamic Content Generation

New tools that replace context loaders:

```go
// 1. Repo Map Tool
{
    "name": "repo_map",
    "description": "Generate a structural map of the codebase",
    "input_schema": {
        "type": "object",
        "properties": {
            "path": {"type": "string", "description": "Subdirectory to map (optional)"},
            "format": {"type": "string", "enum": ["compact", "full"], "default": "compact"}
        }
    }
}

// 2. Recent Files Tool
{
    "name": "recent_files",
    "description": "List recently modified files with metadata",
    "input_schema": {
        "type": "object",
        "properties": {
            "since": {"type": "string", "description": "Time duration (e.g., '2h', '1d')"},
            "include_content": {"type": "boolean", "default": false}
        }
    }
}

// 3. Search Code Tool
{
    "name": "search_code",
    "description": "Search for code patterns in the repository",
    "input_schema": {
        "type": "object",
        "properties": {
            "query": {"type": "string", "description": "Search pattern"},
            "file_pattern": {"type": "string", "description": "File glob (e.g., '*.go')"}
        }
    }
}

// 4. File Metadata Tool
{
    "name": "file_metadata",
    "description": "Get detailed metadata about a file (size, history, dependencies)",
    "input_schema": {
        "type": "object",
        "properties": {
            "path": {"type": "string", "description": "File path"}
        }
    }
}
```

### Agent Workflow

**Session Start:**
1. Load static memories (identity, preferences, recent decisions)
2. Load tool metadata memories (what tools are available, when last used)
3. Agent sees in context:
   ```
   You have access to these tools:
   - repo_map: Generate codebase structure
   - recent_files: List recently modified files
   - search_code: Search repository
   - file_metadata: Get file details
   
   [Cached: Repo map last generated 15 minutes ago]
   ```

**Agent decides when to call tools:**
```
User: "What files were changed recently?"
Agent: I'll check that.
Tool call: recent_files(since="24h")
Result: [list of files with metadata]

User: "Show me the structure of the agent package"
Agent: Let me generate that.
Tool call: repo_map(path="agent/")
Result: [compact repo map]
```

**After tool use:**
- Optionally save tool result summary to memory:
  ```go
  memory.Save(Memory{
      Content: "Repo map showed 12 packages, main entry at cmd/inber/main.go",
      Tags: ["repo-structure", "architecture"],
      Source: "tool",
      ExpiresAt: +1hour, // Don't keep forever
  })
  ```

---

## Benefits of Tool-Based Approach

### 1. Always Fresh Data
- Repo map reflects current code, not 10-minute-old snapshot
- Recent files is truly recent, not stale
- No expiration logic needed

### 2. On-Demand Generation
- Only generate when needed
- No wasted tokens on unused data
- Agent decides relevance

### 3. Parameterized Queries
- `repo_map(path="agent/")` — just one package
- `recent_files(since="2h")` — narrow window
- More flexible than pre-loaded chunks

### 4. Memory for Metadata Only
- "Repo has 12 packages" (50 tokens) vs full map (5000 tokens)
- "5 files changed in last 2h" (20 tokens) vs full content (10000 tokens)
- Memory stays lightweight

### 5. Better Mental Model
- Memory = **knowledge**
- Tools = **capabilities**
- Context = memory + tool awareness

---

## Migration Plan

### Phase 1: Convert Repo Map to Tool ✅

1. Create `tools/repo_map.go`:
```go
type RepoMapTool struct {
    rootDir string
    ignorePatterns []string
}

func (t *RepoMapTool) Run(input map[string]interface{}) (string, error) {
    path := input["path"].(string)
    format := input["format"].(string) // "compact" or "full"
    
    fullPath := filepath.Join(t.rootDir, path)
    
    if format == "compact" {
        return context.BuildRepoMap(fullPath, t.ignorePatterns)
    } else {
        return context.BuildRepoMapVerbose(fullPath, t.ignorePatterns)
    }
}
```

2. Remove repo map from autoload:
```go
// OLD:
func AutoLoad(cfg Config) (*Store, error) {
    // ... load identity ...
    // ... load repo map ... ❌ REMOVE THIS
    // ... load recent files ...
}

// NEW:
func AutoLoad(cfg Config) (*Store, error) {
    // ... load identity only ...
    // Recent files become metadata stubs or also a tool
}
```

3. Register tool:
```go
tools.Register("repo_map", &RepoMapTool{
    rootDir: cfg.RootDir,
    ignorePatterns: cfg.IgnorePatterns,
})
```

### Phase 2: Convert Recent Files to Tool

Option A: **Tool-only** (pure on-demand)
```go
type RecentFilesTool struct {
    rootDir string
}

func (t *RecentFilesTool) Run(input) (string, error) {
    since := parseDuration(input["since"])
    files := context.FindRecentlyModified(t.rootDir, since)
    
    if input["include_content"] {
        return formatFilesWithContent(files)
    }
    return formatFilesMetadata(files)
}
```

Option B: **Hybrid** (metadata in memory, content via tool)
- Memory: `Memory{Content: "5 files modified in last 2h: agent.go, builder.go...", Tags: ["recent"], ExpiresAt: +10min}`
- Tool: `read_file(path)` to get full content when needed

### Phase 3: Memory as Tool Registry/Cache

Add memory entries that describe tool usage:

```go
// When session starts:
memory.Save(Memory{
    Content: "Tools available: repo_map, recent_files, search_code, file_metadata",
    Tags: ["tools", "capabilities"],
    AlwaysLoad: true,
})

// After first repo_map call:
memory.Save(Memory{
    Content: "Repository structure: 12 packages (agent, context, memory, tools, session, cmd/inber, ...)",
    Tags: ["repo-structure", "architecture"],
    Source: "tool",
    ExpiresAt: +1hour,
})

// This gives future turns awareness without re-running tool
```

---

## Tool vs Memory Decision Matrix

| Type | Store in Memory? | Provide as Tool? | Example |
|------|------------------|------------------|---------|
| Identity/system prompt | ✅ AlwaysLoad | ❌ | "You are Claxon..." |
| User preferences | ✅ Static | ❌ | "User prefers blue" |
| Session summaries | ✅ Static | ❌ | "Previous conversation about..." |
| Learned facts | ✅ Static | ❌ | "Deployment uses nginx" |
| Repo map | ❌ Too large | ✅ Generate on demand | `repo_map()` |
| Recent files content | ❌ Too large | ✅ Generate on demand | `recent_files()` |
| File content | ❌ Too large | ✅ Already exists | `read_file(path)` |
| Code search | ❌ Dynamic | ✅ Generate on demand | `search_code(query)` |
| Tool usage summary | ✅ Metadata only | ❌ | "Repo map last called 15m ago" |

---

## Implementation Steps

1. ✅ **Create tool interfaces** (already done in tools/)
2. **Build repo_map tool** (reuse context.BuildRepoMap)
3. **Build recent_files tool** (reuse context.FindRecentlyModified)
4. **Build search_code tool** (new: grep-based or AST search)
5. **Update autoload** to skip repo map/recent files
6. **Add tool awareness** to memory at session start
7. **Optionally: Cache tool results** as ephemeral memories

---

## Example Session Flow

**Turn 1: User asks about recent changes**
```
User: What files changed recently?

Context loaded:
- Identity (memory, AlwaysLoad)
- Tools available (memory)

Agent: <thinking>I should use recent_files tool</thinking>
Tool call: recent_files(since="24h", include_content=false)
Result: "5 files modified: context/builder.go (234 lines, 2h ago, score: 0.85), ..."

Agent: "In the last 24 hours, 5 files were modified. The most important is context/builder.go..."
```

**Turn 2: User asks about structure**
```
User: Show me the agent package structure

Context loaded:
- Identity (memory)
- Tools available (memory)
- Recent files summary (memory, from previous turn)

Agent: <thinking>I should use repo_map tool for just the agent package</thinking>
Tool call: repo_map(path="agent/", format="compact")
Result: "pkg agent\n*Agent.Run(...)\ntype Agent struct{...}"

Agent: "The agent package has..."
```

**Turn 3: Follow-up**
```
User: What's in agent.go?

Context loaded:
- Identity (memory)
- Tools (memory)
- Repo structure summary (memory, just saved): "Agent package has Run, New, struct with ID/Name fields"

Agent: <thinking>I have a summary but user wants details</thinking>
Tool call: read_file(path="agent/agent.go")
Result: [full file content]

Agent: "Here's the agent.go file..."
```

---

## Benefits Summary

✅ **Smaller memory DB** — Only static knowledge, not dynamic content
✅ **Always fresh** — Tools generate current state
✅ **On-demand** — Only generate when needed
✅ **Flexible** — Parameterized queries (path, time window, format)
✅ **Cacheable** — Save summaries as ephemeral memories
✅ **Clear separation** — Memory = knowledge, Tools = capabilities
✅ **Token efficient** — Metadata vs full content

---

## Next Steps

Want me to:
1. Implement `repo_map` tool first?
2. Implement `recent_files` tool?
3. Build the tool registry system in memory?
4. All of the above?
