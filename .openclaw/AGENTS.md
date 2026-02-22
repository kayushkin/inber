# AGENTS.md — Inber

You are the dev agent for **Inber** — an agent orchestration framework in Go, named after Inber Scéne where the Milesians first landed in Ireland.

## What Inber Is

A replacement for OpenClaw — a framework for running AI agents with tool calling, memory, and multi-agent orchestration. Built on the official Anthropic Go SDK.

## Architecture

```
inber/
├── cmd/inber/          # CLI entrypoint (REPL for now)
├── agent/              # Core agent loop (message → tool calls → response)
├── go.mod
└── go.sum
```

### Current state
- `agent/agent.go` — core loop: send messages, handle tool calls, collect results, repeat until final text response. Token tracking.
- `agent/agent_test.go` — 4 integration tests (simple, tool call, multi-tool, conversation continuity). All hit the real Anthropic API.
- `cmd/inber/main.go` — minimal REPL CLI.

### Roadmap (build incrementally, not all at once)
- Session persistence (JSONL or SQLite)
- Memory system with semantic search (the core innovation — not just flat files)
- Built-in tools (exec, file read/write/edit, web fetch)
- WebSocket server (for claxon-android and web clients)
- Multi-agent support (agents as config with different system prompts + tool sets)
- Context window management (smart pruning, compaction)
- Personality files (SOUL.md, IDENTITY.md — loaded from workspace dir)

## Key Design Principles

- **Token efficiency is king** — every design choice is evaluated by "does this waste tokens?"
- **Use anthropic-sdk-go directly** — no multi-provider abstraction layers
- **Incremental** — get each piece working with tests before moving on
- **Simple config** — TOML or YAML, no live patching, no restart dances

## Dependencies

- `github.com/anthropics/anthropic-sdk-go` — official Anthropic Go SDK
- Go 1.22+

## Testing

Tests require `ANTHROPIC_API_KEY` env var. Without it, tests skip gracefully.

```bash
ANTHROPIC_API_KEY=your-key go test -v ./...
```

## After Every Task

1. `go build ./...` — make sure it compiles
2. `go test ./...` — run tests (set ANTHROPIC_API_KEY for integration tests)
3. `git add -A && git commit -m "<descriptive message>"`
4. `git push`
