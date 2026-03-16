# Inber Architecture

Multi-agent orchestrator for LLM-powered coding agents. Supports CLI interactive chat,
headless runs, and a server mode with bus-based message routing.

## Package Map

```
cmd/inber/       CLI entry point (cobra commands: chat, run, server, etc.)
engine/          Core turn loop — builds prompts, calls LLM, executes tools
  engine.go      Engine struct, NewEngine (init: agent config, memory, tools, session)
  turn.go        RunTurn (the main loop), runOpenAITurn (OpenAI-compat path)
server/          Multi-session server with bus integration
  server.go      Server struct, Run/Stream, session management, queue
  bus.go         BusClient — WebSocket subscribe + HTTP publish to message bus
agent/           LLM client abstraction, tool definitions, message conversion
  registry/      Agent config store (agent-store), spawn management
context/         Token-budgeted context assembly
  builder.go     Builder — tag-based chunk selection within token budget
  store.go       In-memory chunk store
conversation/    Message repair, stashing, extraction, summarization
memory/          SQLite-backed semantic memory (references, auto-indexing)
session/         Session logging, workspace persistence, cost tracking
deploy/          Deployment hooks
```

## Key Types

### `engine.Engine`
Central runtime. Holds:
- `modelClient` — unified Anthropic/OpenAI client
- `Agent` — tool-equipped LLM agent
- `MemStore` — semantic memory (SQLite)
- `ContextStore` — token-budgeted context chunks
- `Session` — turn logger + cost tracker
- `Messages` — conversation history (`[]anthropic.MessageParam`)
- `agentTools` — registered tools (shell, file ops, memory, spawn, etc.)
- `workflowHooks` — auto-commit, auto-format after tool calls
- `forgeHook` — project build/preview tracking

### `engine.EngineConfig`
Configuration passed to `NewEngine`: model, agent name, thinking budget,
tool injection, max turns/tokens, context injectors, etc.

### `server.Server`
Multi-agent session manager:
- `sessions` — `sync.Map` of `sessionKey → *Session`
- `queue` — lane-based concurrency limiter (main / subagent lanes)
- `store` — SQLite persistence for sessions & requests
- `bus` — `BusClient` for inbound/outbound message routing
- `events` — event publisher for dashboard

### `server.BusClient`
WebSocket subscriber + HTTP publisher. Connects to a message bus for real-time
chat routing between adapters (Discord, Telegram, etc.) and agent sessions.

### `context.Builder`
Token-budget-aware context assembler:
1. Always-include chunks (identity, always tags)
2. Tag-matched chunks (scored by match count, size-aware)
3. Recent conversation chunks
Deduplicates by ID and content similarity.

### `context.Chunk`
```go
type Chunk struct {
    ID        string
    Text      string
    Tags      []string
    Source    string
    Tokens    int
    CreatedAt time.Time
}
```

### `agent.Tool`
```go
type Tool struct {
    Name        string
    Description string
    InputSchema json.RawMessage
    Run         func(ctx context.Context, input string) (string, error)
}
```

### `agent.TurnResult`
```go
type TurnResult struct {
    Text                string
    InputTokens         int
    OutputTokens        int
    CacheReadTokens     int
    CacheCreationTokens int
    ToolCalls           int
}
```

## Data Flow

### CLI Mode (`inber chat` / `inber run`)

```
User Input
    │
    ▼
Engine.RunTurn(input)
    │
    ├─ 1. Stash large user messages (>threshold → memory store)
    ├─ 2. Append to Messages[]
    ├─ 3. Summarize/prune if conversation too long
    ├─ 4. BuildSystemPrompt (memory + context + injectors)
    ├─ 5. Select model (failover-aware)
    │
    ├─ 6a. Anthropic path: Agent.Run(ctx, model, &messages)
    │       └─ Streaming API call → tool_use → execute → loop
    │
    ├─ 6b. OpenAI path: runOpenAITurn(ctx, systemBlocks)
    │       └─ ChatCompletion → tool_calls → execute → loop
    │
    ├─ 7. Background memory extraction (async goroutine)
    ├─ 8. Stash large assistant responses
    ├─ 9. Save messages snapshot (workspace JSON)
    └─ 10. Track tokens/cost in model-store
```

### Server Mode (`inber server`)

```
Bus (WebSocket)                    HTTP API
    │                                  │
    ▼                                  ▼
Server.ListenBus()              Server.Run/Stream()
    │                                  │
    ▼                                  │
handleBusMessage()                     │
    │                                  │
    ├─ openclaw? → proxyToOpenClaw()   │
    │                                  │
    └─ inber? ─────────────────────────┤
                                       │
                                       ▼
                              Server.run(ctx, req, onEvent)
                                       │
                    ┌──────────────────┤
                    │                  │
                    ▼                  ▼
              Session busy?      Queue.Enqueue()
              → inject()              │
                                      ▼
                               getOrCreateSession()
                                      │
                                      ▼
                               session.turn(input)
                                      │
                                      ▼
                               Engine.RunTurn(input)
                                      │
                                      ▼
                               PublishOutbound (deltas, done)
```

### Message Bus Protocol

- **Inbound** (`inbound` topic): `InboundMessage{Text, Author, Agent, Channel, Orchestrator}`
- **Outbound** (`outbound` topic): `OutboundMessage{Text, Agent, Channel, Stream, StreamID, Meta}`
- Stream types: `status`, `delta`, `thinking`, `tool_call`, `tool_result`, `done`
- Messages for `orchestrator != "inber"` are skipped (multi-orchestrator bus)

## Session Management

- **CLI**: Single session per workspace, persisted as `messages.json`
- **Server**: Multiple concurrent sessions keyed by `agent:<name>:main`
- **Queue lanes**: `main` (default 4 concurrent) and `subagent` (default 8)
- **Injection**: Messages sent to a busy session are injected mid-turn (agent sees them between tool calls)
- **Repair on load**: Empty content, dangling tool_use, alternation violations are auto-repaired

## Context Budget System

The `context.Builder` assembles the system prompt within a token budget:

1. **Always-include**: chunks tagged `identity` or `always` (agent personality, core rules)
2. **Tag-matched**: chunks whose tags overlap with the user message's tags, scored by match count. Size thresholds: <500 tokens = 1 tag match, 500-5000 = 2+, >5000 = 3+
3. **Conversation**: recent user/assistant messages by recency

Deduplication by ID and content similarity prevents bloat.

## Memory System

SQLite-backed (`memory.Store`):
- **References**: auto-created after tool calls (file reads, shell output)
- **PrepareSession**: loads identity + recent files into memory at startup
- **Auto-extraction**: background goroutine extracts memories from completed turns
- **Stashing**: large messages (>threshold tokens) are stashed to memory and replaced with pointers

## Model Support

- **Anthropic** (native): streaming, extended thinking, cache control
- **OpenAI-compatible**: via `agent.ModelClient` — converts tools/messages between formats
- **model-store**: SQLite registry of providers, credentials, usage tracking, OAuth refresh
- **Failover**: `selectModel()` picks healthy model based on recent error/latency data

## Agent Registry

Agents are defined in agent-store (external SQLite). Each agent has:
- Model, system prompt, tool allowlist, workspace path
- Limits: max turns, max input tokens, max response time
- Spawn capability: orchestrator agents can spawn sub-agents via registry

## Workflow Hooks

Post-tool-call automation:
- **Auto-commit**: commit after file writes (configurable)
- **Auto-format**: run formatters after code changes
- **Forge integration**: project detection, build/preview tracking
