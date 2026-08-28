# Inber

Go-based agent orchestration framework. Named after Inber Scéne — the bay where the Milesians first landed in Ireland.

Built on [anthropic-sdk-go](https://github.com/anthropics/anthropic-sdk-go).

## Usage

```bash
# Start the server (always-on daemon, port 8200)
inber-server
inber-server --addr :9000
inber-server --config server.json
```

The CLI is a separate thin client: [inber-cli](../inber-cli).

## Build & Test

```bash
go build -o ~/bin/inber-server ./cmd/inber-server/
go test ./...
```

## Architecture

See [ARCHITECTURE.md](ARCHITECTURE.md) for full details.

```
cmd/inber-server/  Server entry point (flag-based, no cobra)
server/            Multi-agent HTTP server with bus integration
engine/            Per-session engine (context, memory, tools, hooks)
agent/             Anthropic API loop (streaming, tool execution, failover)
  registry/        Multi-agent config loading and spawn management
tools/             Built-in tools (shell, files, deploy, MCP adapter)
memory/            Thin wrapper around memory-store (SQLite-backed)
session/           Session logging, workspace persistence, cost tracking
```

## Agent Fleet

Agents are defined externally in [agent-store](../agent-store) (`~/.config/agent-store/agents.db`),
not in this repo. Irish mythology names, organized by domain:
- **claxon** — orchestrator (Opus)
- **fionn, brigid, oisin, manannan, ogma, goibniu, scathach, bench** — domain builders (Sonnet)
