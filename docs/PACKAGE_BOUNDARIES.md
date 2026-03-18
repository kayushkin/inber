# Package Boundaries

Clear dependency rules for inber's modular architecture. 

## Dependency Rules

### ✅ Allowed Dependencies

**Top-level coordinator:**
- `engine/` → `agent/`, `conversation/`, `memory/`, `session/`, `tools/`

**Core abstractions (minimal dependencies):**
- `agent/` → external SDKs only (anthropic, openai)
- `memory/` → stdlib + database only 
- `bus/` → stdlib + websocket only

**Domain logic:**
- `conversation/` → `memory/`, `agent/`
- `session/` → `agent/`, external logging
- `tools/` → `agent/`, external tool kits

**Interface layer:**
- `cmd/` → any package (CLI needs access to everything)
- `server/` → any package (HTTP server needs coordination access)

### ❌ Prohibited Dependencies  

**Reverse dependencies:**
- `agent/` ← other packages (keep agent pure)
- `memory/` ← other packages (keep storage isolated)
- `conversation/` ← `engine/` (conversation shouldn't know about orchestration)

**Cross-domain coupling:**
- `session/` ↔ `memory/` (different concerns, should not directly couple)
- `tools/` → `memory/` (tools get data via engine/agent, not directly)
- `bus/` → any inber package (pure communication layer)

## Package Responsibilities

### `engine/`
**Role:** Orchestrator and coordinator
- Manages conversation turns and context budgets
- Coordinates between agent, memory, and tools  
- Handles display hooks and user interaction
- **Should import:** Everything it coordinates
- **Should not:** Implement domain logic directly

### `agent/`  
**Role:** Pure LLM abstraction
- Wraps LLM providers (Anthropic, OpenAI, etc.)
- Handles tool calling protocol
- Context window management
- **Should import:** Only external LLM SDKs
- **Should not:** Know about inber's memory, sessions, etc.

### `memory/`
**Role:** Persistent storage
- Memory embeddings and search
- Database operations
- Content tagging and importance scoring
- **Should import:** Only database and stdlib
- **Should not:** Know about conversations, agents, or tools

### `session/`  
**Role:** Conversation logging
- JSONL session files
- Session resume/checkpoint
- Timeline management  
- **Should import:** `agent/` for types, external logging
- **Should not:** Directly manipulate memory or tools

### `conversation/`
**Role:** Conversation management
- Message summarization and extraction
- Conversation repair
- Memory integration logic
- **Should import:** `memory/`, `agent/`  
- **Should not:** Know about orchestration or display

### `tools/`
**Role:** Tool integration
- Wraps external tool libraries
- Converts between tool interfaces
- **Should import:** `agent/` for types, external tool kits
- **Should not:** Access memory/session directly

### `bus/`
**Role:** Communication layer  
- WebSocket and HTTP messaging
- Pure transport abstraction
- **Should import:** Only stdlib networking
- **Should not:** Know about any inber domain logic

## Architectural Principles

1. **Information flows down:** High-level packages can import low-level ones, but not vice versa

2. **Agent is pure:** The `agent/` package should remain a thin wrapper over LLM APIs with no inber-specific logic

3. **Memory is isolated:** Storage concerns in `memory/` should be completely separate from conversation logic

4. **Tools are leaf nodes:** Tools should only depend on the agent interface, never reach into other systems

5. **Bus is transport only:** Communication layer should have no domain knowledge

6. **Engine orchestrates:** Only `engine/` should coordinate between different subsystems

## Dependency Graph

```
       cmd/          server/
         |             |
         v             v  
      engine/ ←→ (coordinates)
       /   |   \
      v    v    v
   agent/ memory/ conversation/
      ↑           ↗     ↑
      |         /      |  
   tools/    session/  |
                       |
                    bus/
```

## Validation

To check compliance:

```bash
# Check for prohibited imports
go mod why -m github.com/kayushkin/inber/memory  # shouldn't appear in tools/, agent/
go list -deps ./agent/... | grep kayushkin/inber  # should only see agent/
go list -deps ./memory/... | grep kayushkin/inber  # should only see memory/
```

These boundaries ensure that:
- Packages can be tested in isolation
- Future refactoring is easier 
- Dependencies remain explicit and minimal
- Each package has a single, clear responsibility