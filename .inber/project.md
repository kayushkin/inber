# Inber Project Context

**Project:** inber — Go-based agent orchestration server  
**Repo:** github.com/kayushkin/inber  
**Language:** Go 1.24+

## Architecture

- **cmd/inber-server/** — Server entry point
- **server/** — HTTP API, bus integration, session management
- **engine/** — Core turn loop, context building, tool execution
- **agent/** — LLM client abstraction, tool definitions
- **agent/registry/** — Agent config loading from agent-store
- **tools/** — Built-in tools (shell, files, deploy, MCP)
- **memory/** — Thin wrapper around agent-store/memory (SQLite-backed)
- **session/** — JSONL logging, session tracking, cost tracking
- **conversation/** — Message repair, stashing, extraction

The CLI is a separate thin HTTP client: [inber-cli](../inber-cli).

## Build & Test Commands

```bash
# Build
go build -o inber-server ./cmd/inber-server/

# Test everything
go test ./...

# Run
./inber-server                    # default port :8200
./inber-server --addr :9000       # custom port
./inber-server --config gw.json   # custom config
```

## Pre-Push Checklist

**Every commit MUST:**
1. Build cleanly: `go build ./cmd/inber-server/`
2. Pass all tests: `go test ./...`
3. Only then: `git push`

## Deployment

This is a systemd service. "Deployment" means:
1. Build binary: `go build -o inber-server ./cmd/inber-server/`
2. Move to PATH: `mv inber-server ~/bin/`
3. Restart service: `systemctl --user restart inber`

## Dependencies

- `github.com/anthropics/anthropic-sdk-go` — Claude API
- `github.com/kayushkin/agent-store` — Agent identity and config (SQLite)
- `github.com/kayushkin/model-store` — Model registry, auth, usage tracking
- `github.com/kayushkin/bus` — NATS message bus client
- `github.com/kayushkin/forge` — Workspace management
- `modernc.org/sqlite` — Pure Go SQLite (no CGO)

## Environment Setup

Required env vars (or in `.env`):
```bash
NATS_URL=nats://localhost:4222   # bus connection
```

Optional:
```bash
OPENCLAW_URL=http://localhost:18789
OPENCLAW_TOKEN=...
BUS_TOKEN=...
```

## Project-Specific Conventions

- Agent definitions are in agent-store (`~/.config/agent-store/agents.db`), not in this repo
- Context building is in engine/turn_prompt.go and turn_context.go
- Session logs in `logs/` (gitignored)
- Server data in `~/.inber/server/`
