# Context System

The context system provides automatic, tag-based context building for agents. It's designed to give agents the right information at the right time without overwhelming their context window.

## Architecture

### Core Components

- **`store.go`** — In-memory chunk store with tags, expiration, and token estimates
- **`builder.go`** — Budget-aware context builder that prioritizes chunks by tags
- **`chunker.go`** — Splits large content into manageable, tagged chunks
- **`tagger.go`** — Auto-tags content based on patterns (errors, code, files, identity)
- **`files.go`** — File loader with gitignore support
- **`repomap.go`** — AST-based repository structure parser for Go codebases
- **`recency.go`** — Detects recently modified files via git or mtime
- **`autoload.go`** — Orchestrates automatic context loading

## Auto-Loading

The `AutoLoad()` function builds initial context when an agent starts:

```go
cfg := context.DefaultAutoLoadConfig("/path/to/repo")
cfg.IdentityText = "You are a helpful coding assistant."

store, err := context.AutoLoad(cfg)
```

### What Gets Loaded

1. **Agent Identity** — System prompt, role description (tagged: `identity`, `always`, `system`)
2. **Repo Map** — Structural summary of the codebase (tagged: `repo-map`, `structure`, `code`, `always`)
3. **Recent Files** — Files modified in the last 24h (tagged: `recent`, `high-priority`, `files`, plus individual filenames)
4. **Project Context** — `.openclaw/AGENTS.md`, `README.md`, etc. (tagged by file purpose)

## Repo Map

The repo map uses Go's `go/parser` and `go/ast` packages to extract:

- Function signatures (with parameters and return types)
- Type definitions
- Struct fields
- Interface methods
- Package declarations
- Imports

**For non-Go files**, it shows just the filename and size.

### Example Repo Map Output

```
## main.go
package main

imports:
  "fmt"
  "os"

type Config struct {
  Host string
  Port int
}

func (c *Config) Validate() error

func main()
```

This gives the agent a complete picture of the codebase structure in a fraction of the tokens it would take to include full file contents.

## Recent Files Detection

The system detects recently modified files using:

1. **Git** (preferred) — `git log --since` to find recently committed files
2. **mtime** (fallback) — Filesystem modification times if git is unavailable

Recent files are tagged as `high-priority` so they're more likely to be included in context.

## Tag-Based Prioritization

The builder uses tags to prioritize chunks:

1. **Always-include** — `identity`, `always` tags
2. **Tag-matched** — Chunks with tags matching the user's message
3. **Recent conversation** — User/assistant messages, most recent first

### Smart Filtering

Large chunks need stronger tag matches to be included:

- **< 500 tokens** — Include if any tag matches
- **500-5000 tokens** — Need 2+ matching tags
- **> 5000 tokens** — Need 3+ matching tags

This keeps the context focused and efficient.

## Usage in Agent

The CLI integrates context building automatically:

```go
// At startup
contextStore, _ := context.AutoLoad(cfg)

// Before each agent turn
messageTags := context.AutoTag(userMessage, "user")
builder := context.NewBuilder(contextStore, 50000) // 50k token budget
chunks := builder.Build(messageTags)

// Build system prompt from chunks
systemPrompt := assembleChunks(chunks)
agent := agent.New(client, systemPrompt)
```

## Token Efficiency

The system is designed for token efficiency:

- **Repo map** — Structural summary instead of full files (10x-100x smaller)
- **Tag-based selection** — Only relevant chunks included
- **Size-aware filtering** — Large chunks need stronger relevance
- **Budget enforcement** — Hard limit on total context tokens

## Configuration

### AutoLoadConfig

```go
type AutoLoadConfig struct {
    RootDir         string        // Repository root
    IdentityFile    string        // Path to identity file (optional)
    IdentityText    string        // Direct identity text
    AgentName       string        // Agent name for identity chunk
    RepoMapEnabled  bool          // Build repo map?
    RecencyWindow   time.Duration // How far back for recent files
    IgnorePatterns  []string      // Patterns to ignore
}
```

### Defaults

```go
cfg := context.DefaultAutoLoadConfig("/path/to/repo")
// RootDir: as specified
// AgentName: "inber"
// RepoMapEnabled: true
// RecencyWindow: 24h
// IgnorePatterns: [*.log, *.tmp, .git/*, vendor/*, etc.]
```

## Testing

All components have comprehensive tests:

```bash
go test ./context/ -v
```

Tests cover:
- AST parsing of various Go constructs
- Tag-based prioritization
- Budget enforcement
- Auto-loading workflow
- Project context loading

## Design Philosophy

1. **Tags over embeddings** — Simple, fast, predictable
2. **Structure over content** — Show what exists, not full implementations
3. **Recency matters** — Recently modified files are often most relevant
4. **Budget-aware** — Never exceed token limits
5. **Automatic** — Zero configuration for common cases

## Future Enhancements

- [ ] Embeddings for semantic similarity (optional upgrade path)
- [ ] Importance scoring based on dependency graphs
- [ ] Cross-file reference tracking (which files import/use which)
- [ ] Cached repo maps (rebuild only on file changes)
- [ ] Support for more languages (Python, TypeScript, Rust, etc.)
