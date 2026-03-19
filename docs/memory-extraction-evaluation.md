# Memory System Extraction Evaluation

**Date**: March 18, 2026  
**Task**: Evaluate feasibility of extracting inber's memory system as standalone Go module

## Summary

✅ **FEASIBLE** - The memory system is well-architected for extraction with minimal changes needed.

## Current Architecture

The memory package is already well-modularized with clean separation of concerns:

```
memory/
├── store.go           # MemoryStore interface (clean abstraction)
├── memory.go          # SQLite implementation 
├── search.go          # Semantic search logic
├── embedding.go       # TF-IDF embeddings (no external deps)
├── compaction.go      # Memory compaction/summarization
├── sessions.go        # Session tracking
├── management.go      # Memory lifecycle (forget, decay)
├── builder.go         # Context building for agent sessions
├── prepare.go         # Session preparation logic
├── recency.go         # Recent file detection
├── scan.go            # DB scanning utilities
├── util.go            # General utilities
├── tool_registry.go   # Tool metadata storage
└── tools.go           # ⚠️ Agent tool definitions (problematic)
```

## Interface Quality

The `MemoryStore` interface in `store.go` is well-designed and comprehensive:

- ✅ Core memory CRUD operations
- ✅ Semantic search with similarity ranking
- ✅ Memory management (decay, compaction, forgetting)
- ✅ Context building for token-budgeted agent sessions
- ✅ Session preparation and tracking
- ✅ Tool registry and usage tracking
- ✅ Clean lifecycle management

## Dependencies Analysis

### External Dependencies (OK for standalone library)
- `modernc.org/sqlite` - Pure Go SQLite (good choice, no CGO)
- `github.com/google/uuid` - Standard UUID generation
- Standard library only (database/sql, encoding/json, etc.)

### Internal Dependencies (Problematic)
- `memory/tools.go` imports:
  - `github.com/anthropics/anthropic-sdk-go` - Anthropic client types
  - `github.com/kayushkin/inber/agent` - Agent tool interface

## Extraction Strategy

### Option 1: Extract Core Memory System (Recommended)
Extract everything **except** `tools.go` as a standalone module:

```go
module github.com/inbernos/memory-store

// Clean dependencies:
require (
    modernc.org/sqlite v1.x.x
    github.com/google/uuid v1.x.x
)
```

**What moves:**
- All memory storage, search, compaction logic
- Session tracking and management  
- Context building and preparation
- Tool registry (metadata only, not tool implementations)
- Complete MemoryStore interface

**What stays in inber:**
- `memory/tools.go` → `agent/memory_tools.go` (tool definitions)
- Integration code that bridges memory store to agent runtime

### Option 2: Extract with Generic Tool Interface
Create a generic tool interface and extract everything:

```go
// Generic tool interface
type Tool interface {
    Name() string
    Description() string
    Execute(params map[string]any) (string, error)
}

// Generic tool builder function type
type ToolBuilder func(store MemoryStore) Tool
```

### Option 3: Keep as Internal Package (Status Quo)
Continue current architecture but improve documentation.

## Benefits of Extraction

1. **Reusability**: Other agent frameworks could use inber's memory system
2. **Testing**: Easier to unit test in isolation
3. **Versioning**: Independent versioning and release cycle
4. **Focus**: Clear boundary between memory storage and agent logic
5. **Innovation**: Community could contribute memory backends (Redis, PostgreSQL, etc.)

## Challenges

1. **Tool Integration**: Need to decide how tools integrate with extracted library
2. **Configuration**: Session preparation logic is inber-specific (git repos, file paths)
3. **Testing**: Current tests are integration-style with inber context
4. **Migration**: Existing inber installations would need migration path

## Recommendation

**Proceed with Option 1** - Extract core memory system without tools.

### Implementation Plan

1. **Create new module**: `github.com/inbernos/memory-store`
2. **Move files**: All except `tools.go` 
3. **Update imports**: Memory package → memory-store module
4. **Bridge tools**: Move `tools.go` content to `agent/memory_tools.go`
5. **Update tests**: Adapt integration tests to new module boundary
6. **Documentation**: Usage examples for other frameworks

### Timeline
- **Week 1**: Module setup and file migration
- **Week 2**: Test adaptation and CI setup  
- **Week 3**: Integration testing and documentation
- **Week 4**: Release v0.1.0-alpha

## Impact Assessment

**Positive Impact:**
- ✅ Makes inber's memory innovation available to broader ecosystem
- ✅ Cleaner architecture boundaries
- ✅ Easier testing and maintenance
- ✅ Potential for community contributions (Redis backend, etc.)

**Risk Assessment:**
- ⚠️ Moderate complexity migration 
- ⚠️ Need to maintain backward compatibility
- ⚠️ Additional maintenance overhead for separate module

**Overall**: Low risk, high potential benefit. The memory system is already well-architected for this extraction.

## Alternative Backend Ideas

Once extracted, the interface could support:

- **Redis**: For distributed agent deployments
- **PostgreSQL**: For enterprise installations with existing Postgres  
- **In-Memory**: For testing and ephemeral agents
- **Hybrid**: SQLite for persistence + Redis for caching
- **Cloud**: DynamoDB, Azure Cosmos, etc.

The current SQLite implementation would remain the default, battle-tested backend.