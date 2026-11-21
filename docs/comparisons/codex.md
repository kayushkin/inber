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

## 2026-11-09: one recovery path for context-window errors, whatever the transport

[#60482](https://github.com/openai/codex/pull/60482) sends `context_length_exceeded` from HTTP 400,
status-less events and SSE through one shared parser, so manual compaction can retry with less
history, and it ends a stream as soon as it reports the failure. inber also has one recovery path
(prune to half the window, retry once), but the retry ignores how the first call was made. A
streamed call that fails on context length is retried through `a.provider.Complete`
(`agent/agent_run.go:252`), so the retried reply sends no `delta` events and runs with no stream
idle bound. Its text reaches the chat only in the turn's final `done` event.

- **What inber should consider:** retry through the same streaming branch the call started on.
  Filed as todo `25ef6454-54c4-4c7c-a652-d6850782b330`.

## 2026-11-13: a dedup cache must forget what compaction removed

[#61390](https://github.com/openai/codex/pull/61390) fixes a pair of mechanisms that each worked
alone: compaction dropped context the client had injected, and deduplication then refused to
resend it because the value had not changed. Codex now tracks injected items by history id and
restores the live ones after compaction. inber has the same pair. Its read cache
(`agent/read_cache.go:111`) answers a re-read with "already in context … read earlier this turn",
and it is scoped to the turn on the premise that this turn's reads are still in the transcript.
They need not be. The mid-turn prune (`agent/agent_run.go:135`, from `engine/build.go:119`) ages
messages by counting user-role messages (`conversation/manage.go:75-82`), and each tool_result
counts, so a long tool loop summarizes its own reads after 3 round trips and drops them after 10.
A throwaway test against `PruneConversation` on 2026-11-13 turned a 3400-byte read from 11 round
trips back into `[read_files: dropped - too old, 3400 bytes]`, while the cache still held it.

- **What inber should consider:** after any prune that changed the slice, clear or narrow the read
  cache, or keep the pruner off the current turn's results. Filed as todo
  `dbbd2104-8df7-4def-bb99-c661ec07c84f`; the todo lists the choices without picking one.

## 2026-11-14: a spend budget is checked before each new piece of work, not once per turn

Codex's rollout-budget series checks the budget again at every point where new spending can
start: [#61578](https://github.com/openai/codex/pull/61578) after a tool call gets its execution
slot, and again just before the handler or MCP handoff runs;
[#61582](https://github.com/openai/codex/pull/61582) before every model request, hook and
compaction, through one gate shared by the whole tree of threads. A response that has already
been paid for is kept even when its bill uses up the budget. Work already running may finish, and
no new work starts. [#61581](https://github.com/openai/codex/pull/61581) records the stop as a
neutral `developer` marker, so the transcript does not blame the user for the interruption.
inber checks `max_cost` only at the top of `RunTurn` (`engine/engine.go:270`), and adds the
turn's cost only after the turn ends (`engine/turn_postprocess.go:87`). The hook that runs before
each API call (`engine/build_hooks.go:38`) checks tokens, tool calls and response time, but not
cost. So one turn of up to `MaxAPICallsPerTurn = 50` round trips can spend any amount, and on a
session's first turn the cap does nothing. (Found by reading the code at baf1332; not reproduced
by a run.)

- **What inber should consider:** price the turn so far inside `buildLimitCheck` from the usage
  fields `TurnResult` already carries, and decide whether hitting the cap mid-turn should force a
  summary call (more spending past the cap), stop at once, or stop and leave a marker as codex does.
  Filed as todo `764feeb0-a9bb-4568-98c8-f4dbe80f7807`, which leaves that choice open, along
  with whether a child's spend counts against its parent's cap.

## 2026-11-16: the date goes into the context, and which day it is matters

[#61897](https://github.com/openai/codex/pull/61897) adds an opt-in `utc_environment_date`
feature. Codex already tells the model the current date in its environment context, as
`<current_date>` in local time; the flag sends it as `<current_date_utc>` instead, and records
the time basis in the environment snapshot so that a switch between the two sends an update even
when the date itself has not changed. The point is that "today" depends on the clock you read it
from, and the model should be told which clock that is.

- **What inber should consider:** inber does not tell the model the date at all. Nothing in
  `engine/`, `agent/`, `conversation/` or `memory/` reads the clock into the prompt, the volatile
  context holds only memories, fleet status, injectors and queued notes
  (`engine/volatile_context.go:5-10`), and the only injectors are fleet status and session status,
  for the orchestrator alone (`server/session_context.go:14-21`). So an inber agent asked for
  "tomorrow", a due date or a `due_at` works the day out from its training cutoff. The volatile
  context is the place for it, since a per-turn date there leaves the cached prefix alone. The open
  choice is the clock: this host runs on UTC, while its operator is on Pacific time, which is a day
  ahead of UTC from 5 pm Pacific. Filed as todo `388f84a5-3461-4897-b831-a3343656ff9f`, with that choice left open.

## 2026-11-20: drop the echoed tool list; keep pinned history outside the compaction budget

[#62863](https://github.com/openai/codex/pull/62863) sends `x-codex-exclude: tools` on every
Responses request, so the stream's `response.*` events stop repeating the request's tool
definitions. That is bytes on the wire, not tokens. inber's subscription route sends no such
header (`agent/codex_subscription.go:129-136`) and gets the tool list back in the events it reads
up to `response.completed` (`:335`); with the sideband fields, six tools are about 11.5 KB.

[#62677](https://github.com/openai/codex/pull/62677) and
[#62654](https://github.com/openai/codex/pull/62654): an item marked as pinned survives compaction
whole and in order, outside the token budget the rest of history shares, and its retention is set
separately for the thread itself and for spawned children. inber's compaction has no pin: it keeps
the last N turns and summarizes everything older (`conversation/summarize.go:39-47`), so the
user's original request survives only as the summarizer chose to restate it.

- **What inber should consider:** add the header after one probe on the subscription route,
  checking that `response.completed` still carries usage and output. Consider a pin flag on
  messages that compaction copies through, starting with the first user message of a session.
  Neither is a defect.
