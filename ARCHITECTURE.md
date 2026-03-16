# Inber Architecture

Go-based agent orchestration framework. Runs agents, manages sessions, spawns sub-agents, connects to the bus for all external I/O.

## Package Layout

```
cmd/inber/          CLI: chat, run, serve, sessions, memory
server/             Server: sessions, bus, queue, spawn, API, events, store
engine/             Engine: NewEngine → RunTurn → context/memory/tools/hooks
agent/              Agent loop: API call → tool_use → collect → repeat
  registry/         Agent config loading from agent-store
memory/             Persistent memory: SQLite store, search, auto-references, prepare
context/            File scanning, recency detection (used by memory)
session/            Session JSONL logging, conversation repair, truncation/stash
forge/              Git workspace management (separate repo, imported as library)
agents/             Agent identity: soul.md, _principles.md, _values.md, _user.md
```

## Key Types

**Server** (`server/server.go`) — Top-level process. Manages sessions, routes bus messages, runs queue, exposes HTTP API.

**Engine** (`engine/engine.go`) — Per-session. Loads context/memory, builds system prompt, calls Agent.Run(), handles hooks. `NewEngine(cfg) → RunTurn(input) → Close()`.

**Agent** (`agent/agent.go`) — Stateless API loop. Takes messages + tools, calls Anthropic streaming API, executes tool calls, returns TurnResult.

**Memory Store** (`memory/store.go`) — SQLite. Tag-based memories with importance scores, TTL, always-load flags. Auto-references track files read during tool calls.

## Message Flow

```
SI (dashboard) → Bus (inbound topic) → Server → Queue → Session → Engine → Agent → Anthropic API
                                                                                          ↓
SI (dashboard) ← Bus (outbound topic) ← Server ← onEvent hooks ← streaming deltas ←──────┘
```

All I/O flows through bus. Server does NOT expose HTTP for messaging — only for management API.

### Inbound
1. Bus delivers message on `inbound` topic via WebSocket
2. Server filters by `orchestrator == "inber"` (skips openclaw messages)
3. Resolves agent from message `agent` field
4. Enqueues work (serialized per session, concurrent across sessions)
5. Queue processor runs engine turn, streaming events published to bus

### Outbound
Server publishes to bus `outbound` topic with `orchestrator: "inber"`:
- `stream: "delta"` — text chunks
- `stream: "thinking"` — extended thinking
- `stream: "tool_call"` — tool invocations  
- `stream: "tool_result"` — tool outputs
- `stream: "done"` — final text + token stats

## Sessions

- Key format: `agent:<name>:main` (primary), `agent:<name>:main:sub:<id>` (spawned)
- Messages persisted to `~/.inber/server/sessions/<key>/messages.json`
- Engine also persists to `<workspace>/.inber/workspace/<agent>/messages.json`
- Session repair on resume: empty content → dangling tool_use → alternation → ID sanitization
- Queue: per-session serialization, configurable lane concurrency (default 4 main, 2 spawn)

## Context System

Each turn, the engine builds a system prompt from memory entries:
1. **Always-load** memories (identity, instructions, tool registry) — permanent
2. **High-importance** memories (decisions, architecture notes, user prefs)
3. **File references** from recent tool calls (auto-created, TTL-based)
4. **Recent files** from filesystem (git log or mtime scan)

Budget-based: 4K–6K tokens default, auto-truncates large memories (>500 tokens → preview + `memory_expand` tool).

### Prompt Caching Strategy

Anthropic caches based on prefix matching. Cache breakpoints:
1. Last system block (context memories)
2. Last tool definition
3. Second-to-last message in conversation

For best cache hit rates, system blocks should be **deterministically ordered** and **stable across turns**. Volatile content (file references, recent files) should come AFTER stable content (identity, instructions, architecture decisions).

## Agent Fleet

10 agents, Irish mythology themed, domain-based:

| Agent | Role | Domain |
|-------|------|--------|
| claxon | Orchestrator | All repos, spawn/merge |
| fionn | Builder | inber + agentkit |
| brigid | Builder | kayushkin.com + bookstack |
| oisin | Builder | si + bus + dashboard |
| manannan | Builder | downloadstack + videostack + mediastack |
| ogma | Builder | logstack |
| goibniu | Builder | forge |
| scathach | Builder | claxon-android |
| bench | Evaluator | agent-bench |
| bran | (shelved) | — |

Config in `agents.json` (projects, tools) + agent-store SQLite (model, system prompt, tools list). Agent-store is single source of truth for runtime config.

## Model Management

**model-store** — standalone service (port 8150). SQLite DB with models, credentials, health tracking.
- Multi-credential per provider with priority-based selection
- Health-based failover: tracks response times + errors, tries fallbacks on unhealthy
- OAuth auto-refresh via aiauth library
- Auto-syncs to OpenClaw's auth-profiles.json

## Spawn System

`Server.Spawn()` creates ephemeral workspace (via forge) → runs agent turn on spawn branch → commits. Orchestrator (claxon) reviews and explicitly merges/rejects.

Forge workspace API: `CreateWorkspace`, `CommitAll`, `MergeToMain`, `PushAll`, `Cleanup`.

## External Connections

```
Server ↔ Bus (ws://localhost:8100, token auth)
Server → Model APIs (Anthropic via model-store credentials)
Server → Forge (library call, SQLite DB)
Server → Agent-store (SQLite, shared with model-store)
```

## Systemd

`inber-server.service` — user service, auto-restart.
Env: `BUS_URL`, `BUS_TOKEN`, `ANTHROPIC_API_KEY`.
Binary: `~/bin/inber serve --addr :8200`.
