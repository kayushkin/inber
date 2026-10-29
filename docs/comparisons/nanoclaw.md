# NanoClaw Comparison

**Last reviewed**: 2026-10-29 against nanocoai/nanoclaw@acc2b1c (2026-10-26).

**Project**: [NanoClaw](https://github.com/nanocoai/nanoclaw) (moved from `qwibitai/nanoclaw`; the old URL redirects)
**Language**: TypeScript (Node.js)
**Focus**: Lightweight personal AI assistant — a minimalist alternative to OpenClaw with container-based agent isolation
**Key Strengths**: OS-level isolation via Docker/Apple Container, tiny codebase (~35k tokens), fork-and-customize model, skills-as-branches contribution pattern

## Update, 2026-10-29: NanoClaw v2

The project is active and has shipped a v2 with a migration script (`migrate-v2.sh`). What changed from the v1 design described below:

- **No more file IPC.** Each session now has two SQLite files: the host writes `inbound.db`, the container's agent-runner (now on Bun) polls it and writes `outbound.db`, and the host polls that to deliver replies. Each file has one writer. Section 3 below describes the old design.
- **Routing model**: user → messaging group → agent group → session, with a choice per channel of a private agent, a shared agent with separate conversations, or one shared session across channels.
- **Other model runtimes** arrive as skills: `/add-codex` (ChatGPT subscription or API key), `/add-opencode`, `/add-cursor`, `/add-ollama-provider`. Claude through the Claude Agent SDK stays the default.
- **More channels**: WhatsApp, Telegram, Discord, Slack, Teams, iMessage, Matrix, Google Chat, Webex, Linear, GitHub, WeChat and email. Channels live on a long-lived `channels` branch and are copied into a fork by `/add-<channel>` skills.
- **Credential gateway**: still OneCLI's Agent Vault by default, now behind a swappable seam that also enforces per-agent policies and rate limits.
- **Agent templates** (`ncl groups create --template`) and an optional free account for a prebuilt hardened image and a managed Slack app per agent.

Script-gated scheduled tasks and per-group `CLAUDE.md` files remain. I did not check whether agents can still edit their own runner code (section 5).

## Architecture Overview (v1, as first studied)

NanoClaw is a single Node.js process with ~5 core source files. The architecture is a simple linear pipeline:

```
Channels (WhatsApp/Telegram/Slack/Discord/Gmail)
    → SQLite (message queue)
    → Polling loop
    → Docker Container (Claude Agent SDK)
    → Response routing back to channel
```

Each agent invocation spawns a fresh Docker container running Claude Code via the Agent SDK. Responses stream back via stdout sentinel markers. Channels are not plugins — they're code that gets merged into your fork via Claude Code skills stored on `skill/*` branches.

Per-group isolation: each chat group gets its own `CLAUDE.md` memory file, filesystem directory, IPC namespace, and container sandbox. The "main channel" (user's self-chat) has elevated privileges.

## What NanoClaw Does Well

### 1. Container Isolation as First-Class Architecture ⭐️

The central design decision. Agents run inside Docker containers with controlled volume mounts. Bash commands execute inside the sandbox, not on the host. The `.env` file is shadowed with `/dev/null` to prevent credential leakage. API keys never enter the container — they're injected at the proxy level by OneCLI's Agent Vault.

This is a fundamentally different security model from application-level permission checks. The container IS the sandbox — no allowlists to misconfigure.

**Inber connection**: Inber uses the forge library for git worktree isolation of spawned agents (`server/forge_iface.go`), which separates workspaces but not processes. Tools run in the inber process. On the credential side inber now strips live secrets from every request it sends to a model provider (`redact/`, `agent/redaction.go`), but the secrets are still in its own environment and reachable by its shell tool. Container-level sandboxing would be a stronger guarantee for untrusted agent operations.

### 2. Fork-and-Customize Over Configuration

NanoClaw explicitly rejects the framework pattern. There are no config files — customization means code changes. The codebase is small enough (~35k tokens) that Claude Code can read and modify the entire thing. The contribution model uses skills-as-branches: contributors submit Claude Code skills that transform the user's fork, not feature PRs.

This inverts the typical framework relationship. Instead of "configure the framework," it's "the framework IS your codebase."

### 3. File-Based IPC with Atomic Writes

Communication between host and container agents uses JSON files written to `/workspace/ipc/` directories. All writes use temp-file-then-rename for atomicity. An MCP server inside the container provides structured tools (`send_message`, `schedule_task`, etc.) that write to these IPC directories. The host watches for new files.

Simple, debuggable, crash-resistant. No message bus, no WebSocket, no gRPC.

### 4. Script-Gated Scheduled Tasks

Scheduled tasks can include a bash script that runs before waking the agent. The script outputs `{"wakeAgent": boolean, "data": any}`. If `wakeAgent` is false, the agent is never invoked — saving API costs. Useful for conditional tasks like "only alert me if the build is broken."

### 5. Agent Self-Modification

The agent-runner source is copied to a per-group writable directory and mounted into the container. Agents can modify their own runner code to add tools or change behavior. Changes persist across container restarts for that group but don't affect other groups.

## What Inber Should Adopt

### 1. Container Isolation Option (MEDIUM PRIORITY)

Inber's forge system manages git worktrees for workspace isolation, but process isolation would be valuable for untrusted or spawned sub-agents:

- Spawn agents in Docker containers instead of in-process
- Mount workspace read-only by default, specific directories writable
- Shadow sensitive files (`.env`, credentials) in container mounts
- Credential injection at proxy level, not environment variables

### 2. Script-Gated Task Execution (LOW PRIORITY)

The pre-check pattern (run a script, only invoke agent if needed) would reduce costs for inber's scheduler integration:

- Before waking an agent for a scheduled task, run a lightweight check
- Pass check results as enriched context if the agent is needed
- Skip the API call entirely if the condition isn't met

### 3. Per-Group Memory Files (LOW PRIORITY)

NanoClaw gives each chat group its own `CLAUDE.md` memory file. Inber's memory is per-workspace (per repo). For multi-project agents, per-project memory files could provide more targeted context.

## What's Different

| Aspect | NanoClaw | Inber |
|--------|----------|-------|
| **Model** | Fork-and-customize | Framework with config |
| **Isolation** | Docker containers per invocation | In-process, forge worktrees for workspace |
| **Agent runtime** | Claude Agent SDK inside container; Codex, OpenCode, Cursor, Ollama as add-ons | Own loop in-process: Anthropic SDK, plus an OpenAI loop (`engine/turn_openai.go`) that can use a ChatGPT subscription (`agent/codex_subscription.go`) |
| **Channels** | Installed per fork by skills from a `channels` branch | None built in; separate adapter services publish to a NATS bus that inber subscribes to (`bus/`, `server/bus.go`) |
| **Message bus** | Two SQLite files per session, polled (v1: SQLite + file IPC) | NATS JetStream; also HTTP, including llm-bridge routes (`server/api_bridge.go`) |
| **Memory** | CLAUDE.md files per group | The memory-store SQLite library (`memory/`), with tag and embedding search; agent definitions come from agent-store |
| **Multi-agent** | Agent teams via Claude Code experimental flag | Named agents with defined identities |
| **Codebase** | A handful of host files plus the container runner | About 6,400 lines of non-test Go in `engine/` and 30,500 in the whole repo (2026-10-29) |
| **Target user** | Personal assistant, single user | Multi-agent orchestration, server-based |

## Key Takeaway

NanoClaw validates that container isolation is the right answer for agent sandboxing — application-level permissions are inherently leaky. The fork-and-customize model is interesting but only works because the codebase is tiny. For inber, the actionable insight is: **consider container-based execution for spawned sub-agents**, especially for agents with shell access on untrusted codebases. The script-gated task pattern is also worth adopting for the scheduler integration.
