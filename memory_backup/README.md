# Memory Package

Persistent, searchable memory across agent sessions.

## Overview

The memory package provides SQLite-backed persistent storage for agent memories with:
- **Semantic search** via TF-IDF embeddings (simple, no external dependencies)
- **Importance scoring** with decay over time
- **Access tracking** to identify frequently used memories
- **Tag-based categorization** reusing the context system's tagging
- **Soft deletion** (forget memories without losing them permanently)

## Storage

Memories are stored in SQLite at `.inber/memory.db` by default.

Each memory has:
- `id` — unique identifier
- `content` — the actual text
- `summary` — compressed version (nullable, for future compaction)
- `original_id` — pointer to parent if compacted (for expansion)
- `tags` — array of tags (e.g., "code", "preference", "fact")
- `importance` — float 0-1, how important this memory is
- `access_count` — how many times it's been retrieved
- `last_accessed` — timestamp of last retrieval
- `created_at` — when it was stored
- `source` — where it came from: "user", "agent", "reflection", "compaction", "system"
- `embedding` — 256-dimensional vector for semantic search

## Agent Tools

Agents can use these tools during conversation:

### `memory_search`
Search memories by semantic similarity to a query.
```json
{
  "query": "Python type hints",
  "limit": 10
}
```
Returns ranked results combining similarity, importance, and recency.

### `memory_save`
Store a new memory.
```json
{
  "content": "User prefers snake_case for Python variables",
  "tags": ["preference", "code-style"],
  "importance": 0.8,
  "source": "user"
}
```
Auto-generates embeddings.

### `memory_expand`
Retrieve full content of a memory by ID.
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000"
}
```
Returns complete metadata + content. Useful for expanding summaries.

### `memory_forget`
Soft-delete a memory (sets importance to 0, excluded from search).
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000"
}
```

## Integration with Context System

On session start, high-importance recent memories are automatically loaded as context chunks:

```go
import (
    "github.com/kayushkin/inber/memory"
    "github.com/kayushkin/inber/context"
)

// Open memory store
memStore, _ := memory.OpenOrCreate("/path/to/repo")
defer memStore.Close()

// Load top 20 memories with importance >= 0.6 into context
ctxStore := context.NewStore()
memory.LoadIntoContext(memStore, ctxStore, 20, 0.6)
```

## Embedding

Currently uses a simple TF-IDF style bag-of-words embedding:
- Tokenize text (lowercase, filter stop words)
- Hash tokens into 256 buckets
- L2-normalize the vector

This is fast, deterministic, and has no external dependencies. Can be swapped for real embeddings (OpenAI, sentence-transformers, etc.) later without changing the API.

## Importance & Decay

- New memories start at 0.5 importance (or agent-specified)
- Each access bumps importance by 1% (capped at 1.0)
- Daily decay job multiplies importance by 0.99 for memories not accessed in 24h
- Search ranking = `similarity × importance × recency_boost`
  - `recency_boost = 0.99^days_since_access`

## Planned Features (Not Implemented Yet)

- **Compaction** — LLM-driven summarization of old memories to save space
- **Reflection** — Auto-generate insights from patterns in memories
- **Real embeddings** — Use OpenAI/Cohere/local models for better semantic search
- **Memory clusters** — Group related memories for better retrieval

## Testing

```bash
go test ./memory/ -v
```

All memory tests use temporary SQLite databases and clean up after themselves.

## Example Usage

```go
package main

import (
    "github.com/kayushkin/inber/memory"
)

func main() {
    // Open store
    store, err := memory.NewStore(".inber/memory.db")
    if err != nil {
        panic(err)
    }
    defer store.Close()

    // Save a memory
    m := memory.Memory{
        ID:         "uuid-here",
        Content:    "The capital of France is Paris",
        Tags:       []string{"geography", "fact"},
        Importance: 0.8,
        Source:     "user",
    }
    store.Save(m)

    // Search
    results, _ := store.Search("France capital", 5)
    for _, mem := range results {
        println(mem.Content)
    }
}
```

## Notes

- Memory database path: `.inber/memory.db` (gitignored)
- Thread-safe for concurrent access
- Access tracking is synchronous (no race conditions)
- Forgotten memories remain in DB with importance=0 (can be revived if needed)
