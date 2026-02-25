# AGENTS.md — inber

You are the dev agent for **inber** — a Go-based agent orchestration framework replacing OpenClaw.

## Architecture

- **`agent/`** — Core agent loop: sends messages to Claude, handles tool calls, loops until final response
  - `agent.go` — Agent struct, Run() loop, hooks system
  - `models.go` — Model registry, pricing, API key helper
- **`context/`** — Tag-based context system: everything competes for context space via tagged chunks
  - `store.go` — In-memory chunk store with tags, expiration, token estimates
  - `builder.go` — Budget-aware context builder, filters/prioritizes chunks
  - `chunker.go` — Splits large content into tagged chunks
  - `tagger.go` — Auto-tags content (errors, code, file paths, identity)
  - `files.go` — File loader with gitignore support
  - `repomap.go` — AST-based repo structure parser (Go function signatures, types, structs, interfaces)
  - `recency.go` — Recently modified files detection (git or mtime)
  - `autoload.go` — Automatic context loading orchestrator (identity, repo map, recent files, project docs)
- **`tools/`** — Built-in tools: shell, read_file, write_file, edit_file, list_files
- **`session/`** — JSONL session logging with full request payloads, tool calls, thinking, costs
- **`cmd/inber/`** — CLI REPL
  - `main.go` — REPL loop
  - `config.go` — CLI flags, env loading
  - `display.go` — Pretty terminal output (colors, tool call display, thinking)

## Key Design Decisions

- **Claude-only** — uses `anthropic-sdk-go` directly, no multi-provider abstraction
- **Tag-based context** — everything (files, messages, memory) is a tagged chunk competing for context space
- **Model decoupled from agent** — model chosen per-task via `Run(ctx, model, messages)`
- **Token efficiency first** — tags over relevance scores, size-aware budgeting
- **Hooks for observation** — OnRequest, OnThinking, OnToolCall, OnToolResult

## Dependencies

- `github.com/anthropics/anthropic-sdk-go` — Anthropic API client
- `github.com/joho/godotenv` — .env file loading

## Environment

- `.env` file in project root with `ANTHROPIC_API_KEY`
- `.env` is gitignored

## Testing

```bash
# Unit tests (no API key needed)
go test ./context/ -v

# Agent tests (needs ANTHROPIC_API_KEY)
export $(cat .env | xargs) && go test ./agent/ -v -timeout=120s

# All tests
export $(cat .env | xargs) && go test ./... -v
```

## Building

```bash
go build -o inber ./cmd/inber/
```

## Running

```bash
./inber                                    # default model (sonnet)
./inber --model claude-opus-4-6            # use opus
./inber --thinking 4096                    # enable extended thinking
./inber --list-models                      # show available models
```

## Roadmap

1. ~~Agent loop + tools + logging~~ ✅
2. ~~Context system with auto-loading (identity, repo map, recent files)~~ ✅
3. Memory system — SQLite + embeddings + importance scoring
4. Multi-agent orchestration
5. Streaming responses
6. Config file system (replace CLI flags)

## Context System (Implemented)

The context system automatically builds useful context when the agent starts:

- **Agent identity** — System prompt loaded as a tagged chunk (`identity`, `always`)
- **Repo map** — AST-parsed Go codebase structure (functions, types, structs, interfaces) — not full file contents
- **Recent files** — Detects files modified in the last 24h via git or mtime, tagged as `high-priority`
- **Project docs** — Auto-loads `.openclaw/AGENTS.md`, `README.md`, etc.

Context is built per-message using tag-based prioritization with a 50k token budget. See `context/README.md` for details.

## After Every Task

1. Run tests — `go test ./...`
2. Build — `go build -o inber ./cmd/inber/`
3. `git add -A && git commit -m "<descriptive message>"`
4. `git push`
