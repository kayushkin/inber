# Go Modules Analysis: Mono vs Multi-Module

## Current Structure

Inber is currently organized as a single Go module with these major packages:

- `agent/` - Core agent types and provider interfaces
- `memory/` - Conversation memory and storage 
- `session/` - Session lifecycle and state management
- `engine/` - Orchestration and execution engine
- `conversation/` - Message processing and summarization  
- `tools/` - Tool system and MCP integration
- `server/` - HTTP server and bus integration
- `cmd/` - CLI commands and main entry points
- `bus/` - Message bus for inter-component communication

## Dependency Analysis

### Current Import Graph

```
engine/ → agent, conversation, memory, session, tools
server/ → agent, bus, conversation, engine, memory, session  
cmd/    → agent, engine, memory, session, server, tools
conversation/ → agent, memory
session/ → agent, memory
tools/  → agent  
memory/ → agent
agent/  → (mostly self-contained)
bus/    → (independent)
```

### Key Findings

1. **Agent is the foundation** - Nearly every package imports `agent/` for core types
2. **Engine is the orchestrator** - Imports most other packages to coordinate execution
3. **Circular-ish dependencies** - While not strictly circular, many packages depend on shared foundations
4. **Server and CMD are consumers** - They import many packages but export little

## Multi-Module Evaluation

### Potential Module Boundaries

#### Option 1: Core/Engine/Server Split
- `inber-core` (agent, memory, conversation)
- `inber-engine` (engine, session, tools) 
- `inber-server` (server, cmd)

#### Option 2: Layer-Based Split  
- `inber-agent` (agent only)
- `inber-storage` (memory, session)
- `inber-engine` (engine, conversation, tools)
- `inber-server` (server, cmd, bus)

#### Option 3: Feature-Based Split
- `inber-agent` (agent, conversation)
- `inber-memory` (memory, session)  
- `inber-tools` (tools)
- `inber-runtime` (engine, server, cmd)

## Pros of Multi-Module

### ✅ Benefits

1. **Clearer boundaries** - Forces explicit interfaces between modules
2. **Independent versioning** - Different modules can evolve at different paces
3. **Reduced coupling** - Can't accidentally create tight dependencies 
4. **Testability** - Easier to test modules in isolation
5. **Reusability** - Other projects could use `inber-agent` or `inber-memory` independently
6. **Build optimization** - Only rebuild changed modules
7. **Documentation clarity** - Each module has focused purpose

### Real-World Examples
- **Kubernetes** - Split into 100+ modules for different components
- **Go standard library** - net/http, database/sql as separate concerns
- **gRPC-Go** - Split into grpc, status, codes, metadata modules

## Cons of Multi-Module  

### ❌ Drawbacks

1. **Version coordination hell** - Managing compatible versions across 3-4 modules
2. **Development friction** - Need to publish intermediate versions during development
3. **Circular dependency risk** - Current structure has implicit cycles that would break
4. **Go workspace complexity** - Need go.work or replace directives for local development
5. **Increased maintenance** - Multiple go.mod files, separate releases, more CI complexity
6. **Premature optimization** - Inber isn't large enough to warrant this complexity yet

### Specific Challenges for Inber

1. **Agent as foundation** - Nearly everything imports agent/, making it hard to split
2. **Engine as coordinator** - Engine needs to import most packages, limiting module boundaries  
3. **Shared types** - Many packages share common types (Message, Session, Tool)
4. **Development velocity** - Inber is still rapidly evolving; modules would slow iteration

## Recommendation: **Stay Mono-Module**

### Why Mono-Module Wins for Inber

1. **Size** - ~15k LOC is well within reasonable mono-module size (cf. Hugo: 100k+ LOC)
2. **Cohesion** - All packages work together toward single goal (AI agent runtime)
3. **Development speed** - No version coordination overhead during rapid iteration
4. **Simplicity** - One go.mod, one release, one version to track
5. **Tooling works** - `go test ./...`, `go build ./...` just work across everything

### When to Reconsider

Split into modules **IF/WHEN**:

1. **Size grows 5x+** (>75k LOC) - Then complexity overhead becomes worth it
2. **Clear reusable components emerge** - e.g., inber-memory becomes useful for other AI agents
3. **Different release cycles needed** - e.g., tools change frequently but agent core is stable  
4. **External contributors** - Multiple teams working on different modules
5. **Specific technical needs** - e.g., agent core needs to be CGO-free but tools can use CGO

## Current Focus: Better Package Design

Instead of modules, improve the mono-module structure:

1. **✅ Already done**: Clean provider interfaces, swappable stores
2. **🔄 In progress**: Reduce engine/ file count, simplify conversation package
3. **📋 Next**: Define clear package contracts, add package-level docs

### Package Contract Example

```go
// Package agent provides core AI agent types and provider interfaces.
// 
// This package should be imported by other inber packages but should NOT
// import any other inber packages except for shared types.
package agent
```

## Conclusion

**Stick with mono-module** for now. Inber is at the sweet spot where mono-module provides maximum development velocity with minimal complexity. Focus energy on **better package design** and **cleaner interfaces** within the single module.

Re-evaluate when Inber hits 50k+ LOC or when a specific component (like memory) becomes valuable as a standalone library.

---

*Analysis completed: March 17, 2026*  
*Next review: When codebase doubles in size or external usage patterns change*