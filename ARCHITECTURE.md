# Inber Architecture

Multi-agent orchestrator for LLM-powered coding agents. Runs as an always-on server
with bus-based message routing. The CLI lives in a separate repo ([inber-cli](../inber-cli)).

## Package Map

```
cmd/inber-server/  Server entry point (flag-based, starts HTTP server + bus listener)
engine/            Core turn loop — builds prompts, calls LLM, executes tools
  engine.go      Engine struct, NewEngine (init: agent config, memory, tools, session)
  turn.go        RunTurn (the main loop), runOpenAITurn (OpenAI-compat path)
  turn_prompt.go BuildSystemPrompt — token-budgeted context assembly
  turn_context.go contextBudget — scales context by turn state and message complexity
server/          Multi-session server with bus integration
  server.go      Server struct, Run/Stream, session management, queue
  bus.go         BusClient — WebSocket subscribe + HTTP publish to message bus
agent/           LLM client abstraction, tool definitions, message conversion
  registry/      Agent config store (agent-store), spawn management
tools/           Built-in tool implementations (shell, files, deploy, MCP adapter)
conversation/    Message repair, stashing, extraction, summarization
memory/          Thin wrapper re-exporting agent-store/memory (SQLite-backed)
session/         Session logging, workspace persistence, cost tracking
deploy/          Agent seed data (agents-seed.db)
```

## Key Types

### `engine.Engine`
Central runtime. Holds:
- `modelClient` — unified Anthropic/OpenAI client
- `Agent` — tool-equipped LLM agent
- `MemStore` — semantic memory (via the memory-store SQLite package)
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
- `bus` — `*bus.Client` for inbound/outbound message routing
- `busManager` — `*server.BusManager`, routes an inbound bus message to an agent
- `events` — event publisher for dashboard

### `bus.Client`
NATS subscriber and publisher. Connects to the message bus for real-time chat
routing between adapters (Discord, Telegram, etc.) and agent sessions.

Named `server.BusClient` until `127e030` moved it into its own `bus/` package as
`bus.Client`. It spoke WebSocket-in / HTTP-out at that point; it has since been
rewritten onto NATS, so both halves of the old sentence here were wrong.

### `agent.Tool`
```go
type Tool struct {
    Name        string
    Description string
    InputSchema anthropic.ToolInputSchemaParam
    Run         func(ctx context.Context, input string) (string, error)
}
```

### `agent.TurnResult`
```go
type TurnResult struct {
    Text                string
    Thinking            string // extended thinking output
    ToolCalls           int
    InputTokens         int
    OutputTokens        int
    CacheCreationTokens int
    CacheReadTokens     int
}
```

## Data Flow

### Turn Execution (Engine.RunTurn)

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

### Server Request Flow

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

Context assembly is handled by the engine (`turn_prompt.go` / `turn_context.go`), not a separate package.
`BuildSystemPrompt()` assembles the system prompt within a token budget that scales by turn state:

1. **Always-include**: identity and core rules from memory
2. **Tag-matched**: memory entries scored by relevance to the user message
3. **Budget scaling**: first turn gets a small budget (4K tokens), error recovery gets up to 50K,
   complex messages scale proportionally via `contextBudget()`

## Memory System

Thin wrapper around `agent-store/memory` (SQLite-backed). Inber's `memory/` package re-exports types
and constructors; the implementation lives in the external `agent-store` library.

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
