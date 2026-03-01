# Context-Memory-Tool Hybrid: Implementation Complete

## What We Did

Converted the repo map and recent files from **context loaders** into **on-demand tools**, implementing the three-tier architecture:

1. **Static Memory** — Knowledge that persists
2. **Dynamic Tools** — Generate content on-demand  
3. **Ephemeral Metadata** — Cached summaries with expiration

## Key Changes

### 1. Tools Are Already in Agentkit ✅

The `repo_map` and `recent_files` tools were already implemented in `github.com/kayushkin/agentkit/tools`:

**`repo_map` tool:**
```go
tools.RepoMap(rootDir, ignorePatterns)
// Generates fresh codebase structure
// Parameters: path (subdirectory), format (compact/full)
```

**`recent_files` tool:**
```go
tools.RecentFiles(rootDir)
// Lists recently modified files with metadata
// Parameters: since (time window), include_content (bool)
```

These are already registered in `cmd/inber/engine.go` and available to the agent.

### 2. Tool Registry in Memory

Created `memory/tool_registry.go` with `LoadToolRegistry()`:

**What it does:**
- Saves a list of available tools into memory
- Categorizes tools (filesystem, code-introspection, memory, execution)
- Includes usage guidelines
- Marked as `AlwaysLoad: true` so agent always knows what tools exist

**Example memory entry:**
```
You have access to these tools:

## Code-Introspection

- **repo_map**: Generate a structural map of the codebase...
- **recent_files**: List files that were recently modified...

## Filesystem

- **read_file**: Read the contents of a file...
- **write_file**: Create or overwrite a file...
- **edit_file**: Edit a file by replacing exact text...
- **list_files**: List files and directories...

Important guidelines:
- Use `repo_map()` to understand codebase structure before reading files
- Use `recent_files()` to see what's been worked on recently
- Use `read_file()` to get full file contents only when needed
- Tools generate fresh data on-demand - don't rely on stale context
```

### 3. Recent Files as Metadata Stubs

`memory/prepare.go` already loads recent files as **lightweight metadata** (not full content):

**Before (hypothetical full content approach):**
```
Memory{Content: "<full 234-line file>", Tokens: 2340}
```

**After (current stub approach):**
```
Memory{
    Content: "Recently modified (2 hours ago): context/builder.go",
    Tags: ["recent", "file:context/builder.go", "ext:.go"],
    Importance: 0.7,
    ExpiresAt: +10min,
}
```

**Benefits:**
- Minimal tokens (20 vs 2000+)
- Agent knows files were modified
- Can call `recent_files()` tool for full list with scores
- Can call `read_file()` for actual content

### 4. Workflow

**Session Start:**
1. `PrepareSession()` loads:
   - Identity (AlwaysLoad)
   - Memory instructions (AlwaysLoad)
   - Recent file stubs (ephemeral, 10min TTL)
2. `LoadToolRegistry()` loads tool list (AlwaysLoad)

**Agent sees in context:**
```
[Memory: Identity]
You are Claxon...

[Memory: Tools]
You have access to: repo_map, recent_files, read_file, ...

[Memory: Recent file stub]
Recently modified (2 hours ago): context/builder.go

[Memory: Recent file stub]
Recently modified (5 hours ago): agent/agent.go
```

**Agent decides to use tools:**
```
User: "Show me the repo structure"

Agent: <thinking>I should use repo_map tool</thinking>
Tool call: repo_map(format="compact")
Result: [fresh repo map]

User: "What changed recently?"

Agent: <thinking>I see stubs but need full list with scores</thinking>
Tool call: recent_files(since="24h")
Result: [5 files with scores, line counts, etc.]
```

## Architecture Benefits

### Always Fresh Data
- Repo map reflects current code (no 10-minute-old snapshot)
- Recent files is truly recent, not stale
- No expiration logic needed for tools

### On-Demand Generation
- Only generate when needed
- No wasted tokens on unused data
- Agent decides relevance

### Parameterized Queries
- `repo_map(path="agent/")` — just one package
- `recent_files(since="2h")` — narrow window
- More flexible than pre-loaded chunks

### Memory for Metadata Only
- "Repo has 12 packages" (50 tokens) vs full map (5000 tokens)
- "5 files changed in last 2h" (20 tokens) vs full content (10000 tokens)
- Memory stays lightweight

### Clear Mental Model
- **Memory** = knowledge (identity, preferences, decisions)
- **Tools** = capabilities (repo_map, recent_files, read_file)
- **Context** = memory + tool awareness

## Decision Matrix (Final)

| Type | Store in Memory? | Provide as Tool? | Example |
|------|------------------|------------------|---------|
| Identity | ✅ AlwaysLoad | ❌ | System prompt |
| Preferences | ✅ Static | ❌ | Favorite color |
| Decisions | ✅ Static | ❌ | Nginx config |
| Session summaries | ✅ Static | ❌ | Previous work |
| Repo map | ❌ Too large | ✅ On-demand | `repo_map()` |
| Recent files | ⚠️ Stubs only | ✅ Full data | `recent_files()` |
| File content | ❌ | ✅ Exists | `read_file()` |
| Tool registry | ✅ AlwaysLoad | ❌ | What tools exist |
| Tool summaries | ✅ Ephemeral | ❌ | "Repo has 12..." |

## Files Changed

- `memory/tool_registry.go` — New: LoadToolRegistry(), ToolMetadata type
- `memory/prepare.go` — Updated: Comment about tool registry
- `cmd/inber/engine.go` — Updated: Call LoadToolRegistry() after building tools
- `tools/tools.go` — Unchanged (already had RepoMap and RecentFiles wrappers)

## Testing

All existing tests pass. The tools were already implemented in agentkit and already registered in the engine.

## What This Enables

### Future: Tool Result Caching

After calling a tool, optionally save summary:

```go
// After repo_map() call:
memory.UpdateToolUsageSummary(
    "repo_map",
    "Repository has 12 packages: agent, context, memory, tools, ...",
    3600, // 1 hour TTL
)
```

Next turn: Agent sees "Repo has 12 packages" without re-calling tool.

### Future: Smart Tool Selection

Agent can reason about which tool to use:
- "User asked about structure" → `repo_map()`
- "User asked about recent changes" → `recent_files()`
- "User asked about specific file" → `read_file()`

## Summary

✅ **Repo map** is now a tool (was: autoloaded into context)
✅ **Recent files** metadata in memory, full data via tool
✅ **Tool registry** in memory (agent knows what tools exist)
✅ **All tests pass**
✅ **Architecture is clean**: Memory=knowledge, Tools=capabilities

The agent now has:
- **Better awareness** (knows what tools exist)
- **Fresher data** (tools generate on-demand)
- **Lower token cost** (stubs vs full content)
- **More flexibility** (parameterized tool calls)
