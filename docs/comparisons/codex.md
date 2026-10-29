# Codex Comparison

**Last reviewed**: 2026-10-29 against openai/codex@184c9c5 (2026-10-29).

**Project**: [Codex](https://github.com/openai/codex), OpenAI's coding agent for the terminal, IDE and desktop
**Language**: Rust (`codex-rs/`, about 200 workspace crates)
**Focus**: One coding agent, run by a person or a script, against OpenAI models over the Responses API
**Key Strengths**: OS-level sandboxes on three platforms, a full Responses API client (WebSocket, prewarm, encrypted reasoning replay, server-side compaction), a JSON-RPC app server that every front end shares, ChatGPT sign-in with token refresh

## Architecture Overview

The TUI and `codex exec` no longer drive the agent loop in-process. Both depend on
`codex-app-server-client` (`tui/Cargo.toml`, `exec/Cargo.toml`) and talk to the app server, which
owns `codex-core`. The app server speaks JSON-RPC over stdio or WebSocket
(`app-server-transport/src/lib.rs`), with its message types in `app-server-protocol/src/protocol/`
(`v1.rs` and `v2/`).

```
codex (cli/src/main.rs) ── tui / exec / review / login / mcp / sandbox / app-server
        │
        ▼
app-server  (JSON-RPC over stdio or WebSocket; app-server-protocol v2)
        │
        ▼
core  ── session/turn.rs ── client.rs (ModelClient) ── codex-api ── Responses API
  │          │                        WebSocket first, HTTP SSE fallback
  │          ├── tools/ (exec_command, write_stdin, apply_patch, MCP, multi-agent)
  │          ├── compact.rs / compact_remote_v2.rs
  │          └── agents_md.rs, memories/
  └── sandboxing ── Seatbelt (macOS) / bwrap + seccomp (Linux) / restricted token (Windows)
```

There is no `mcp-server` crate at this commit. `codex mcp` manages *external* MCP servers; the
client lives in `codex-mcp/` and `rmcp-client/`.

## What Codex Does Well

### 1. A full Responses API client

`core/src/client.rs` builds every request with `store: false`, `stream: true` and
`include: ["reasoning.encrypted_content"]`. Reasoning items stay in history and go back on later
turns, so the model keeps its earlier reasoning even though the server stores nothing. For
providers that are not OpenAI, it strips encrypted content from every item except reasoning and
compaction items. `prompt_cache_key` defaults to the session id (`PromptCacheKey::SessionDerived`),
and `codex-api/src/requests/headers.rs` sends `session-id` and `thread-id` headers. A comment in
`client.rs` says "ChatGPT derives cache affinity from the Responses session-id header." Subagents
add an `x-openai-subagent` header (`codex-api/src/endpoint/responses.rs`).

The OpenAI provider sets `supports_websockets: true` (`model-provider-info/src/lib.rs`). A turn
first sends a `response.create` with `generate=false` to open the socket and warm the server, then
sends the real request with `previous_response_id` and only the new items. If the socket fails,
the session falls back to HTTP for the rest of its life (`client.rs` module docs).

**Inber connection**: inber's subscription path (`agent/codex_subscription.go`) posts to the same
`https://chatgpt.com/backend-api/codex/responses` endpoint, with `store: false`,
`prompt_cache_key` set to the session id, and the same value as a `session-id` header. Its comment
records that the key alone hit the cache on 1 of 5 repeat calls and the header made it 5 of 5,
which matches the Codex comment. The gaps: inber converts chat-completion requests to Responses
and back (`agent/openai.go`, `engine/turn_openai.go`), drops reasoning items on the way back, so
each call reasons afresh, and it uses HTTP SSE only, with no WebSocket, no prewarm and no
`previous_response_id`.

### 2. Sign-in that owns its own tokens

The `login` crate runs a browser OAuth flow with PKCE (`login/src/server.rs`, `pkce.rs`) or a
device code (`device_code_auth.rs`), and stores the result in `$CODEX_HOME/auth.json` or the OS
keyring (`login/src/auth/storage.rs`). `login/src/auth/manager.rs` refreshes the ChatGPT token
when `last_refresh` is more than 8 days old (`TOKEN_REFRESH_INTERVAL`) and on a 401
(`UnauthorizedRecovery`). With a ChatGPT sign-in the base URL is
`https://chatgpt.com/backend-api/codex`; with an API key it is `https://api.openai.com/v1`.

**Inber connection**: inber reads the Codex CLI's auth file on each call, refuses any
`auth_mode` other than `"chatgpt"`, and leaves refresh to the CLI: one owner for the token, but
inber fails once the token expires if nobody has run the CLI.

### 3. Approval policies backed by real OS sandboxes

`AskForApproval` (`protocol/src/protocol.rs`) has four values: `untrusted`, `on-request` (the
default; `on-failure` is now an alias for it), `granular` (per-category allow or auto-reject)
and `never`. Sandbox modes are `read-only`, `workspace-write` and `danger-full-access`
(`protocol/src/config_types.rs`). The `sandboxing` crate enforces them:

- macOS: Seatbelt profiles (`sandboxing/src/seatbelt.rs` and the `.sbpl` files).
- Linux: `codex-linux-sandbox` applies `no_new_privs` and seccomp, and uses bubblewrap for the
  file system (`linux-sandbox/src/lib.rs`). Landlock remains only as a hidden opt-in,
  `--use-legacy-landlock` (`linux-sandbox/src/linux_run_main.rs`).
- Windows: a restricted token (`windows-sandbox-rs/src/token.rs`, `CreateRestrictedToken`) with
  ACL-based write and read limits. `codex sandbox` runs any command inside the same sandbox.

**Inber connection**: inber's modes (observe, assist, autonomous in `guard/guard.go`) decide
*which tools* may run; `engine/build_hooks.go` `buildToolRefusal` refuses the rest. Assist mode
refuses every call that needs approval, because no approver exists. Nothing limits what an
allowed tool does once it runs: tools run in-process or as subprocesses on the host. Codex
answers both questions: whether to run, and how far the process can reach.

### 4. Tools built for long-running shells

The shell tool is `exec_command` plus `write_stdin` (`core/src/tools/handlers/shell_spec.rs`,
`handlers/unified_exec/`), so the model can start a process, read part of its output, and type
into it later. `apply_patch` has its own crate and handler. Other tools include `view_image`,
`plan`, `request_user_input`, `request_permissions`, tool search and MCP resources (`handlers/`).

**Inber connection**: a handle the model can poll and write to would let inber agents drive
servers, REPLs and long builds.

### 5. Subagents with limits

`core/src/tools/handlers/multi_agents_spec.rs` defines `spawn_agent`, `send_input`,
`send_message`, `followup_task`, `resume_agent`, `wait_agent`, `list_agents`, `close_agent` and
`interrupt_agent` (a v1 and a v2 set). Defaults: at most 6 agent threads and depth 1
(`DEFAULT_AGENT_MAX_THREADS`, `DEFAULT_AGENT_MAX_DEPTH` in `core/src/config/mod.rs`).

**Inber connection**: inber has one `spawn_agent` tool (`agent/registry/spawn_tool.go`) with
`MaxSpawnDepth` (default 2, `server/config.go`); Codex adds a thread cap and more verbs.

### 6. Compaction on the server

When the provider declares `RemoteCompactionSupport::V2`
(`model-provider-info/src/capabilities.rs`), Codex sends `compaction_trigger` items over the
normal Responses endpoint and gets back a `Compaction` item with encrypted content
(`core/src/compact_remote_v2.rs`). It can run this in the background before the limit is hit
(`session/async_compaction.rs`). Other providers get local summary compaction
(`core/src/compact.rs`). The trigger is `model_auto_compact_token_limit`. I found no separate
`/responses/compact` path in `codex-api` at this commit.

**Inber connection**: inber compacts on the client (`conversation/summarize.go`, `stash.go`,
`manage_tool_pruning.go`). That works with any provider, but on the subscription path it pays for
a summary call that the server could do.

### 7. AGENTS.md and memories

`core/src/agents_md.rs` walks from the project root (found by `.git` by default) down to the
working directory and joins every `AGENTS.override.md` or `AGENTS.md` it finds, capped at 32 KiB.
The memories pipeline (`memories/README.md`, `core/src/memories/`) summarizes old sessions with a
model in the background, then a consolidation agent merges them into files under a memories root.

**Inber connection**: inber loads no AGENTS.md; an agent's identity and instructions come from
always-load memories (`engine/turn_prompt.go`). Its memory is the memory-store module (SQLite
with embeddings) wrapped by `memory/`, which recalls by search rather than by consolidated files.

## What Inber Could Adopt

1. **Replay encrypted reasoning on the subscription path (HIGH).** Ask for
   `reasoning.encrypted_content`, keep reasoning items in the session, and send them back. The
   chat-completion conversion is what drops them, so this means carrying them beside the
   chat-completion history.
2. **An OS sandbox for shell tools (HIGH).** Wrap subprocess tools in bubblewrap with seccomp on
   Linux, using Codex's read-only and workspace-write split as the model.
3. **An approver for assist mode (MEDIUM).** Route `NeedsApproval` to a person through the bridge
   instead of refusing. Codex's `granular` policy shows how to approve by category.
4. **Server-side compaction where offered (MEDIUM).** Use `compaction_trigger` on the subscription
   path; keep `conversation/summarize.go` for every other provider.
5. **Interactive shell sessions (MEDIUM).** An `exec_command` / `write_stdin` pair.
6. **WebSocket with prewarm (LOW).** Faster turns, but the endpoint is undocumented and HTTP works.

## What's Different

| Dimension | Codex | Inber |
|-----------|-------|-------|
| **Language** | Rust | Go |
| **Main model path** | OpenAI Responses API | Anthropic (`anthropic-sdk-go`, `engine/engine.go` `RunTurn`) |
| **OpenAI transport** | WebSocket first, HTTP SSE fallback | HTTP SSE only, via chat-completion conversion |
| **Reasoning across turns** | Encrypted reasoning replayed | Dropped; each call reasons afresh |
| **Token refresh** | Owns it (8-day and on-401) | Leaves it to the Codex CLI |
| **Safety** | Approval policy plus OS sandbox | Tool modes plus limits (`MaxTurns`, `MaxInputTokens`, `MaxCost`, `MaxDuration` in `engine/engine_types.go`); no sandbox |
| **Secret leaks** | Redacts secrets in generated memories | Redacts secrets in every outbound request (`agent/redaction.go`, `redact/`) |
| **Front ends** | TUI, exec, desktop, IDE over app-server JSON-RPC | Server with llm-bridge routes (`server/api_bridge.go`) |
| **Subagents** | Nine verbs, 6 threads, depth 1 | `spawn_agent`, depth 2 |
| **Compaction** | Server-side (v2) or local summary | Local summary, stash and tool pruning |
| **MCP** | Client (`codex-mcp`, `rmcp-client`) | Client (`tools/mcp/`) |
| **Project instructions** | AGENTS.md walk | Always-load memories |

## Key Takeaway

Codex is one agent built mainly for one provider (others work, with fewer features), and it uses that provider fully: encrypted reasoning,
WebSocket reuse, server compaction and cache headers. Inber already borrowed the cache headers and
the endpoint, and its own measurement agrees with Codex on why the `session-id` header matters. The
two things most worth taking next are reasoning replay, which changes answer quality on every
subscription call, and an OS sandbox, which inber has no answer for today. Codex's per-tool
approval policy also shows what inber's assist mode needs to become useful: someone to ask.
