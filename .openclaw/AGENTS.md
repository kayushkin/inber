# AGENTS.md — inber

You are the dev agent for **inber** — a Go-based agent orchestration framework replacing OpenClaw.

## Architecture

- **`agent/`** — Core agent loop, hooks system, multi-agent registry
  - `agent.go` — Agent struct, Run() loop, hooks
  - `models.go` — Model registry, pricing, API key helper
  - `hooks.go` — Lifecycle event system with gatekeeper support
  - `registry/` — Multi-agent registry, config loading, tool scoping
- **`context/`** — Tag-based context system
  - `store.go` — In-memory chunk store with tags, expiration, token estimates
  - `builder.go` — Budget-aware context builder, filters/prioritizes chunks
  - `chunker.go` — Splits large content into tagged chunks
  - `tagger.go` — Auto-tags content (errors, code, file paths, identity)
  - `files.go` — File loader with gitignore support
  - `repomap.go` — AST-based repo structure parser (function signatures, types, structs, interfaces)
  - `recency.go` — Recently modified files detection (git or mtime)
  - `autoload.go` — Automatic context loading orchestrator
- **`memory/`** — Persistent memory across sessions
  - `memory.go` — SQLite storage, importance scoring, decay, search ranking
  - `embedding.go` — TF-IDF bag-of-words embeddings (256-dim, swappable)
  - `tools.go` — Agent-facing tools: memory_search, memory_save, memory_expand, memory_forget
  - `context.go` — Auto-loads high-importance memories into context on session start
- **`tools/`** — Built-in tools: shell, read_file, write_file, edit_file, list_files
- **`session/`** — JSONL session logging with full request payloads, tool calls, thinking, costs
- **`cmd/inber/`** — CLI REPL
  - `main.go` — REPL loop
  - `config.go` — CLI flags, env loading
  - `display.go` — Pretty terminal output (colors, tool calls, thinking)
- **`agents/`** — Agent definitions (markdown identity files + agents.json config)
- **`docs/`** — Design docs (multi-agent, hooks, usage guides)

## Key Design Decisions

- **Claude-only** — uses `anthropic-sdk-go` directly, no multi-provider abstraction
- **Tag-based context** — everything (files, messages, memory) is a tagged chunk competing for context space
- **Model decoupled from agent** — model chosen per-task via `Run(ctx, model, messages)`
- **Token efficiency first** — tags over relevance scores, size-aware budgeting
- **Markdown for identity, JSON for config** — agent definitions split between human-readable .md and machine-parsed .json
- **Memory is persistent, context is per-session** — memory/ survives across sessions, context/ is rebuilt each time

## Dependencies

- `github.com/anthropics/anthropic-sdk-go` — Anthropic API client
- `github.com/joho/godotenv` — .env file loading
- `github.com/mattn/go-sqlite3` — SQLite driver (memory system)
- `github.com/google/uuid` — UUID generation

## Environment

- `.env` file in project root with `ANTHROPIC_API_KEY`
- `.env` is gitignored
- Memory DB at `.inber/memory.db`

## Testing

```bash
# Unit tests (no API key needed)
go test ./context/ -v
go test ./memory/ -v

# Agent tests (needs ANTHROPIC_API_KEY)
export $(cat .env | xargs) && go test ./agent/ -v -timeout=120s

# All tests
export $(cat .env | xargs) && go test ./... -v
```

## Building

```bash
go build -o inber ./cmd/inber/
```

**Always build and run tests before pushing.** Every commit must:
1. `go build ./cmd/inber/` — must compile cleanly
2. `go test ./...` — all tests must pass
3. Only then `git push`

If tests fail, fix them before pushing. No exceptions.

## Running

```bash
./inber                                    # default model (sonnet)
./inber --model claude-opus-4-6            # use opus
./inber --thinking 4096                    # enable extended thinking
./inber --list-models                      # show available models
```

## Roadmap

### Done
1. ~~Agent loop + tools + logging~~ ✅
2. ~~Context system — auto-loading, repo maps (go/ast), recent files, identity~~ ✅
3. ~~Multi-agent registry — markdown identity + JSON config, tool scoping, session isolation~~ ✅
4. ~~Lifecycle hooks — 8 event types, gatekeeper mode, scheduled triggers~~ ✅
5. ~~Memory system — SQLite, TF-IDF search, importance/decay, agent tools~~ ✅
6. ~~Pretty display — ANSI colors, tool call visualization, thinking blocks~~ ✅
7. ~~Extended thinking support — budget config, thinking extraction/display/logging~~ ✅

### Next
8. **Memory triggers & auto-save** — automatic memory creation on session end, user requests, decisions, errors. System prompt instructions for when to use memory_save.
9. **Streaming responses** — stream tokens as they arrive instead of waiting for full response
10. **Sub-agent spawning tool** — orchestrator agent can spawn sub-agents as a tool call
11. **Compaction** — LLM-driven summarization of old memories, with lineage pointers for expand
12. **Reflection** — auto-generate higher-level insights from recent memories (Generative Agents pattern)
13. **Real embeddings** — swap TF-IDF for actual embedding model (local or API)
14. **Config file system** — replace CLI flags with project config
15. **Conversation pruning** — token-budget-aware message trimming using context builder
16. **Pure-Go SQLite** — swap go-sqlite3 (CGO) for modernc.org/sqlite for static binaries

### Next (continued)
17. **Message routing & session attachment** — when a new inbound message arrives, an evaluator agent (or heuristic) determines which active session it belongs to. If it matches an existing session, inject it into that session's context before the next turn. If it's a new conversation, create a new session. If multiple sessions could match, use recency + topic similarity to pick.
18. **Message queue** — inbound messages that can't be immediately processed (agent is busy, rate limited, needs routing decision) go into a queue. Queue is persistent (SQLite or file-backed). Messages are dequeued in priority order. Supports: priority levels, delay/schedule, dead-letter for failed deliveries, back-pressure when agents are saturated.
19. **In-flight message handling** — handle messages that arrive while an agent is mid-turn. Options: buffer and inject at next turn boundary, interrupt current turn, or queue for after current turn completes. Configurable per-agent.

### Future
- Web UI for session viewing / memory browsing
- MCP (Model Context Protocol) tool integration
- Token usage tracking and cost dashboards
- Cross-agent memory sharing with access control

## After Every Task

1. Run tests — `go test ./...`
2. Build — `go build -o inber ./cmd/inber/`
3. `git add -A && git commit -m "<descriptive message>"`
4. `git push`
