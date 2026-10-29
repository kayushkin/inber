# Hermes Agent Comparison

**Last reviewed**: 2026-10-29 against NousResearch/hermes-agent@67d47601 (2026-10-29).

**Project**: [Hermes Agent](https://github.com/NousResearch/hermes-agent) by Nous Research. Not to be confused with Facebook's Hermes JavaScript engine, which an earlier version of this page described by mistake.
**Language**: Python (agent core, gateway, tools), TypeScript (Ink terminal UI, Electron desktop app, web dashboard)
**Focus**: A personal agent that runs one agent core behind a CLI, a terminal UI, a desktop app, about twenty chat platforms and an OpenAI-compatible HTTP server, and that keeps memory and writes its own skills across sessions.
**Key Strengths**: Many providers including subscription sign-ins, self-written and self-pruned skills, a frozen-at-start memory snapshot that keeps the prompt cache intact, pluggable terminal backends (local, Docker, SSH, Modal, Daytona, Vercel Sandbox), built-in cron with delivery to chat, and a large messaging gateway.

## Architecture Overview

Each former large file is now a facade plus `<stem>_<topic>.py` siblings (`AGENTS.md`, "Facade + siblings layout"). `run_agent.py` holds the `AIAgent` class; the turn itself runs in `agent/conversation_loop.py` and the `agent/turn_*.py` phases.

```
CLI / TUI / desktop / chat platforms / HTTP API (gateway/platforms/api_server.py)
            |
            v
     gateway/run.py  (sessions, profiles, delivery)  --  cron/scheduler.py (60s tick)
            |
            v
  AIAgent (run_agent.py) -> agent/conversation_loop.py
      |-- agent/transports/  anthropic | chat_completions | codex (Responses) | bedrock | codex_app_server
      |-- tools/registry.py + toolsets.py  (terminal, files, browser, skills, memory, delegate_task, cronjob_manage, MCP)
      |-- memory: MEMORY.md + USER.md snapshot, optional provider plugin
      |-- agent/context_compressor.py  (aux model summarises the middle)
      '-- agent/background_review.py   (forked agent decides what to save after the turn)
```

## What Hermes Agent Does Well

### 1. One loop, many provider wire formats

`agent/conversation_loop.py` drives a user turn: model call, tool dispatch, retries, fallbacks, compression and post-turn hooks. A transport (`agent/transports/base.py`) owns only the format change for one `api_mode`; client setup, streaming, credentials and retries stay on `AIAgent`. Transports exist for Anthropic Messages, chat completions, the Codex Responses API, Bedrock, and the Codex app-server, where Codex runs the loop and Hermes exposes a subset of its tools back to it over stdio MCP (`agent/transports/codex_app_server.py`, `agent/transports/hermes_tools_mcp_server.py`). About forty provider profiles live in `plugins/model-providers/`. The turn cap `agent.max_turns` defaults to unlimited (`hermes_cli/config_defaults.py`), with an optional wall-clock budget.

**Inber connection**: Inber has two paths: Anthropic through anthropic-sdk-go (`engine/engine.go` `RunTurn`), and an OpenAI-compatible chat-completions client (`agent/openai.go`, `engine/turn_openai.go`). Hermes's split, where the transport only converts formats, is a clean line to copy if inber grows a third path. Inber's limits are tighter by default and broader in kind: `MaxTurns`, `MaxInputTokens`, `MaxCost` and `MaxDuration` (`engine/engine_types.go`).

### 2. Subscription sign-ins and credential pools

Hermes signs in to more than API keys. `hermes_cli/auth_codex.py` runs a ChatGPT/Codex device-code login and keeps its own tokens in Hermes's `auth.json`, apart from the Codex CLI, so the two apps do not rotate each other's refresh tokens. `agent/anthropic_credentials.py` resolves `ANTHROPIC_TOKEN` / `CLAUDE_CODE_OAUTH_TOKEN`, then an API key, then Hermes's own OAuth grants, and last borrows Claude Code's credentials file. There are also Nous Portal, Qwen, xAI, MiniMax and OpenRouter logins (`hermes_cli/auth_*.py`). `agent/credential_pool.py` rotates several credentials per provider with per-model cooldowns.

**Inber connection**: Inber can post through the Codex CLI's ChatGPT subscription to the Responses endpoint (`agent/codex_subscription.go`), but it reads the Codex CLI's token rather than owning a login. Hermes's note on refresh-token rotation applies directly: two programs sharing one refresh token can sign each other out.

### 3. Tools grouped into toolsets, with a high bar for core tools

Each tool file calls `registry.register()` at import with its schema, handler, toolset and an availability check (`tools/registry.py`). `toolsets.py` names the core list (`_HERMES_CORE_TOOLS`: terminal, file tools, browser, web search, skills, memory, `session_search`, `delegate_task`, `cronjob_manage`, kanban tools, `computer_use`) and derives smaller sets such as a coding set. Because every tool goes on every call, the project steers new features into skills, plugins or MCP servers first (`AGENTS.md`, "Footprint ladder"), and can defer rarely used tools behind a search tool (`tools/tool_search.py`).

**Inber connection**: Inber takes tools from tool-store (`agent/registry/tools.go`) plus an MCP client (`tools/mcp/`). Hermes's rule that a new core tool must beat "terminal plus a skill" is a useful test for tool-store additions too.

### 4. Skills the agent writes and prunes itself

Skills are `SKILL.md` folders. `tools/skill_manager_tool.py` lets the agent create and edit skills under the Hermes home. After a turn, `agent/background_review.py` may fork the agent in a background thread, replay the turn and ask whether any skill or memory should be saved; the fork shares the parent's cached prompt so it costs little. `agent/curator.py` runs when the agent is idle and archives, pins or merges skills, and never deletes. Skills from outside pass a regex scan and a trust policy before install (`tools/skills_guard.py`, `tools/skills_hub*.py`).

**Inber connection**: Inber has no skills system; grep finds no skill code in its Go source. In the wider system around inber, skills live in a separate skill-store service. The background review, a cheap fork after the turn that decides what to keep, is the idea most worth borrowing, whether it writes skills or memory.

### 5. Memory that does not break the prompt cache

`tools/memory_tool.py` keeps `MEMORY.md` (agent notes) and `USER.md` (a profile of the user). Both enter the system prompt as a snapshot at session start; writes during the session go to disk but do not change the prompt, so the cached prefix holds. `agent/memory_provider.py` lets one external provider plugin add recall (in tree: `plugins/memory/byterover`, `plugins/memory/retaindb`; Honcho is now a catalog plugin, `plugin-catalog/hermes-honcho-plugin.yaml`). `tools/session_search_tool.py` searches past sessions with SQLite FTS5 and no model call.

**Inber connection**: Inber's memory is the memory-store module, SQLite with embeddings (`memory/memory.go`, `memory/tools.go`). Inber has no separate user profile. Inber already keeps its system prompt stable for the cache in a different way: only always-load and tag-matched memories go into system blocks, and everything that changes per turn goes into the turn's user message (`engine/turn_prompt.go`, `BuildSystemPrompt`). Hermes goes further by not refreshing recall at all mid-session.

### 6. Compression and delegation

`agent/context_compressor.py` has a cheap auxiliary model summarise the middle turns while the head and a token-budgeted tail stay, and it prunes old tool output first. It starts at 50% of the window by default, raised to 75% for windows under 512K (`hermes_cli/config_defaults.py`). `tools/delegate_tool.py` starts child agents with a fresh conversation, the parent's toolsets minus a blocked list (`tools/delegate_tool_toolsets.py`), and returns only a summary. The default is flat: `max_spawn_depth` 1 and up to 10 children at once.

**Inber connection**: Inber compacts with `conversation/summarize.go`, `stash.go` and `manage_tool_pruning.go`, and spawns subagents with `MaxSpawnDepth` default 2 (`server/config.go`). The two designs are close; Hermes defaults one level shallower.

### 7. Messaging gateway, cron, terminal backends

Chat adapters live in `plugins/platforms/` (Telegram, Discord, Slack, Signal, WhatsApp, Matrix, Mattermost, email, SMS, Teams, IRC and others), with the enum in `gateway/config.py`. `cron/scheduler.py` ticks every 60 seconds inside the gateway; jobs in `cron/jobs.py` carry a prompt, script or skill and a `deliver` target. The terminal tool runs each command in a fresh `bash -c` with a saved environment (`tools/environments/base.py`) on local, Docker, SSH, Modal, Daytona or Vercel Sandbox (`tools/environments/`). Risky commands go through an approval gate with pattern checks, an optional auxiliary-model risk check and a "yolo" switch (`tools/approval*.py`).

**Inber connection**: Inber has none of these itself. Chat channels and cron live in other services, and inber reaches a scheduler only as a tool-store connection. Inber has no OS sandbox. Its guard modes observe, assist and autonomous (`guard/guard.go`, enforced in `engine/build_hooks.go`) are coarser than Hermes's gate, and assist mode refuses approval because no approver exists.

### 8. The OpenAI-compatible API server

`gateway/platforms/api_server.py` is a gateway platform built on aiohttp, on `127.0.0.1:8642` by default, behind a bearer `API_SERVER_KEY`. It serves `/v1/chat/completions`, `/v1/responses`, `/v1/models`, `/v1/capabilities`, `/v1/runs` (with events, steer, stop and approval), `/api/sessions` and `/health`. `/v1/responses` (`gateway/platforms/api_server_openai_routes.py`) takes `input`, `instructions`, `stream`, and either `previous_response_id` or a named `conversation`, not both; a SQLite store keeps the chain across restarts (`api_server_response_store.py`). Hermes runs its own tools on the server side and streams tool start and finish events. The llm-bridge-hermes wrapper drives Hermes through this endpoint.

**Inber connection**: Inber's equivalent is the llm-bridge-compatible routes in `server/api_bridge.go` plus the NATS bus (`bus/`). Hermes's `/v1/capabilities` feature table lets a client learn what a server supports without guessing; inber's bridge routes have nothing like it.

## What Inber Could Adopt

1. **A post-turn review fork (HIGH).** After a turn, fork with the same cached prompt and ask what should go to memory. It fits memory-store as it is and needs no skills system (`agent/background_review.py`).
2. **A capabilities route (MEDIUM).** Add a route to `server/api_bridge.go` that lists the features and endpoints this server supports, as `/v1/capabilities` does, so bridge clients stop inferring them.
3. **Memory frozen per session (LOW).** Inber already moves per-turn context out of the system prompt (`engine/turn_prompt.go`). Freezing the tag-matched memories too, as Hermes does (`tools/memory_tool.py`), would be the remaining step; measure cache misses before doing it.
4. **Transport as pure format change (LOW).** If inber adds a provider beyond Anthropic and chat completions, keep credentials, retries and streaming in one place and let each path only convert formats (`agent/transports/base.py`).
5. **Own the subscription login (LOW).** Keep inber's own Codex token copy rather than reading the Codex CLI's, so refreshes do not collide (`hermes_cli/auth_codex.py`).

## What's Different

| Aspect | Hermes Agent | Inber |
|--------|--------------|-------|
| **Language** | Python, TypeScript UIs | Go |
| **Main provider path** | Many transports, ~40 provider profiles | Anthropic SDK, plus OpenAI-compatible chat completions |
| **Subscription sign-in** | Own OAuth for Codex, Anthropic, Nous, Qwen and others | Borrows the Codex CLI's ChatGPT token |
| **Agent config** | `config.yaml` per profile | agent-store (`agent/registry/config.go`) |
| **Tools** | In-process registry and toolsets, plus MCP | tool-store plus MCP client |
| **Skills** | Agent writes, reviews and archives its own | None in inber |
| **Memory** | `MEMORY.md` + `USER.md` snapshot, optional provider plugin, FTS session search | memory-store, SQLite with embeddings |
| **Subagents** | `delegate_task`, depth 1 by default | spawn, depth 2 by default |
| **Safety** | Command approval gate, terminal backends incl. containers | Guard modes, egress secret redaction (`agent/redaction.go`, `redact/`), no OS sandbox |
| **Limits** | Unlimited turns by default, optional time budget | Turns, input tokens, cost, duration |
| **Chat and cron** | Built in | In other services |
| **HTTP surface** | OpenAI-compatible `/v1/responses`, `/v1/chat/completions`, runs, sessions | llm-bridge routes and NATS bus |

## Key Takeaway

Hermes is a whole personal-agent product in one Python tree: chat platforms, cron, sandboxes, desktop app, and an agent that grows its own skills. Inber is a narrower Go engine that leans on separate stores and services for those jobs. The part of Hermes that carries over best is not its breadth but two habits: never change the prompt mid-session, so the cache holds, and spend a cheap forked call after each turn deciding what is worth keeping.
