# Inber

Go-based agent orchestration framework. Named after Inber Scéne — the bay where the Milesians first landed in Ireland.

Built on [anthropic-sdk-go](https://github.com/anthropics/anthropic-sdk-go).

## Usage

```bash
# Interactive chat
inber chat

# Single-shot (pipe-friendly)
echo "fix the bug in main.go" | inber run

# Server mode (bus-connected, multi-agent)
inber serve --addr :8200

# Session management
inber sessions list
inber sessions timeline

# Memory
inber memory search "deployment process"
inber memory stats
```

## Build & Test

```bash
go build -o ~/bin/inber ./cmd/inber/
go test ./...
```

## Architecture

See [ARCHITECTURE.md](ARCHITECTURE.md) for full details.

```
cmd/inber/    CLI (chat, run, serve, sessions, memory)
server/       Multi-agent server with bus integration
engine/       Per-session engine (context, memory, tools, hooks)
agent/        Anthropic API loop (streaming, tool execution, failover)
memory/       Persistent SQLite memory with tag-based retrieval
agents/       Agent identity files (soul.md per agent)
```

## Agent Fleet

10 agents with Irish mythology names, organized by domain:
- **claxon** — orchestrator (Opus)
- **fionn, brigid, oisin, manannan, ogma, goibniu, scathach, bench** — domain builders (Sonnet)
