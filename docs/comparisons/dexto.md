# Dexto Comparison

**Project**: [Dexto](https://github.com/truffle-ai/dexto)
**Language**: TypeScript (Node.js monorepo)
**Focus**: Agent harness / orchestration layer — config-driven agents via YAML with multi-interface runtime
**Key Strengths**: YAML-driven agent definitions, multi-interface (CLI/Web/REST/MCP/bots), pluggable storage (SQLite/PostgreSQL/Redis), sub-agent orchestration with signal bus, OpenTelemetry built-in

## Architecture Overview

Dexto is a monorepo (~20 packages) with a layered architecture. YAML defines the agent, `core` provides the runtime, tool packages provide capabilities, storage backends provide persistence, and interface packages provide access.

```
YAML Agent Config
    → core (DextoAgent: lifecycle, streaming, state machine)
        → llm/ (50+ models via provider adapters)
        → tools/ (MCP-first + modular tool packages)
        → memory/ (persistent across sessions)
        → session/ (create, resume, list, search)
    → orchestration/ (signal-bus, task-registry, condition-engine)
    → interfaces: CLI | Web UI | REST+SSE server | MCP server | Discord | Telegram
    → storage: PostgreSQL | SQLite | Redis | in-memory (pluggable)
```

Same agent runs as CLI, web UI, REST API server, MCP server, or embedded via SDK. The `--mode` flag switches between interfaces.

## What Dexto Does Well

### 1. YAML-Driven Agent Configuration ⭐️

Each agent is a single YAML file defining: LLM provider/model, MCP servers, system prompt (with static and dynamic contributors), memory settings, storage backends, permissions, tools, hooks, and telemetry. Environment variables are interpolated. Agents are data, not code.

**Inber connection**: Inber defines agents in agent-store SQLite with programmatic configuration. YAML is more accessible for non-developers and easier to version-control. A YAML export/import for agent-store would bridge the gap.

### 2. Multi-Interface Runtime

Same agent definition runs across 6 different interfaces without code changes. This is the "write once, run everywhere" promise for agents.

**Inber connection**: Inber achieves something similar via the server (HTTP API + SSE + NATS bus), with `si` adapting to Discord/Telegram/Slack. But Dexto's single-binary approach is simpler to deploy.

### 3. Pluggable Storage Tiers

Three independent storage tiers: cache (Redis/in-memory), database (PostgreSQL/SQLite/in-memory), blob (local filesystem). Production uses Redis + PostgreSQL; dev uses in-memory + SQLite. Configured per-agent in YAML.

**Inber connection**: Inber uses SQLite everywhere (agent-store, model-store, session DB). Pluggable storage isn't needed today but would matter if scaling beyond a single server.

### 4. Signal Bus for Sub-Agent Communication

The orchestration package provides a `signal-bus` for inter-agent communication and a `task-registry` for managing spawned agents. Sub-agents run ephemerally with auto-cleanup. Tool approvals propagate from sub-agent to parent.

**Inber connection**: Inber uses NATS JetStream for agent communication, which is more robust (persistent, distributed) but heavier than an in-process signal bus.

### 5. Bidirectional MCP

Dexto is both an MCP client (connects to MCP servers for tools) and can expose itself as an MCP server (for integration into Claude Code, Cursor, etc.). This makes it interoperable in both directions.

### 6. OpenTelemetry Built-In

Distributed tracing with OTLP export. Enables observability dashboards without custom logging.

**Inber connection**: Inber uses logstack for centralized logging. OpenTelemetry would provide richer observability (traces, spans, metrics) with industry-standard tooling.

## What Inber Should Adopt

### 1. Agent Config Export/Import as YAML (MEDIUM PRIORITY)

Add `inber agents export <name> > agent.yaml` and `inber agents import agent.yaml` to agent-store. Makes agent definitions portable, version-controllable, and shareable without database access.

### 2. OpenTelemetry Integration (LOW PRIORITY)

Replace or complement logstack with OpenTelemetry for richer observability. Traces per RunTurn, spans per phase (prepare, context, execute, postprocess), metrics for token usage and latency. Works with any OTLP-compatible backend (Jaeger, Grafana Tempo, etc.).

### 3. Approval Propagation for Sub-Agents (MEDIUM PRIORITY)

When a sub-agent needs approval (tool confirmation), propagate the request up to the parent agent or user. Dexto's pattern: sub-agent approval surfaces through the parent's interface. Inber could route approval requests through the bus to the original requestor.

## What's Different

| Aspect | Dexto | Inber |
|--------|-------|-------|
| **Agent definition** | YAML files | SQLite agent-store |
| **Agent identity** | Generic (coding-agent, explore-agent) | Named with Irish mythology identities |
| **Multi-agent** | In-process signal bus, sub-agent spawning | NATS JetStream across processes |
| **Interfaces** | CLI, Web, REST, MCP, Discord, Telegram, SDK | Server (HTTP+SSE), NATS, si adapters |
| **Storage** | Pluggable (PG/SQLite/Redis/in-memory) | SQLite everywhere |
| **Observability** | OpenTelemetry | logstack (custom JSONL) |
| **Model support** | 50+ via provider adapters | Anthropic-focused via model-store |
| **License** | Elastic 2.0 (restrictive) | Internal |
| **Deployment** | Single Node.js process | Ecosystem of Go services |

## Key Takeaway

Dexto's YAML-driven agent definitions and multi-interface runtime make it the most accessible agent framework for getting started. For inber, the main takeaways are: **YAML export/import for agent configs** (portability), **OpenTelemetry for observability** (industry-standard tracing), and **approval propagation for sub-agents** (safety UX). The fundamental architectural difference — single-process vs distributed services — means inber is more robust for production multi-agent workloads, but Dexto is easier to prototype with.

## Harness-watch — 2026-06-02: turn-loop rebuild (PR 796)

[PR 796](https://github.com/truffle-ai/dexto/pull/796) ("Rebuild runtime storage,
skills, and turn execution architecture") is a substantial rewrite with several
inber-relevant primitives.

### 1. Checkpointable TurnDriver with serializable state

Dexto replaces the AI SDK's internal model→tool→model loop with an explicit
`TurnDriver` (`prepareNextModelStep` / `runNextModelStep` / `executeToolCalls` /
`decideNextStep` / `checkpoint` / `finish` / `fail` / `dispose`). Each step is one
model request plus explicit post-model tool execution, and the driver exposes a
serializable, Zod-validated `TurnDriverState` (`parseTurnDriverState`) so a turn
can be checkpointed and resumed at safe boundaries by a host.

**What inber should consider:** refactor inber's `RunTurn` into an explicit step
driver that emits a serializable checkpoint after each model step/tool batch, so a
crashed or killed agent (cf. the autoworker process leak) can resume mid-turn from
memory-store instead of replaying the whole turn.

### 2. Durable, idempotent tool-execution records (deterministic IDs + setIfAbsent)

A `ToolExecutionStore` with `createToolExecutionId()` and a `setIfAbsent(...)`
primitive (across memory/SQLite/Postgres backends) makes tool execution and
approval writes durable and idempotent: a replayed/resumed step re-issuing the
same tool call is detected and not double-executed, and tool-call/tool-result
pairing is preserved even on failure, denial, or cancellation.

**What inber should consider:** give inber's tool layer a deterministic
tool-execution id (hash of session+step+tool+args) persisted via an idempotent
`setIfAbsent` in the prehook path, so resumed turns and retried bus deliveries
never double-run a side-effecting tool, and every tool call always has a paired
terminal result row.

### 3. Split busy-session input into `steer` (active-turn) vs `follow-up` (next-turn) queues

Dexto splits the single message queue into a durable `steerQueue` (inject into the
currently running turn) and `followUpQueue` (defer to the next turn), surfaced as
`steer()`/`followUp()` APIs and `/api/steer/{sessionId}` / `/api/follow-up/{sessionId}`
routes; composers map Enter→send/steer based on processing state and
Alt+Enter→queue follow-up. "Change what the agent is doing now" and "here's the
next task" become first-class distinct concepts.

**What inber should consider:** model mid-turn steering and next-turn follow-up as
two distinct durable queues on inber sessions (over the NATS bus / si adapters)
instead of one undifferentiated inbound stream, so a user can redirect an
in-flight agent without conflating it with the next request.

**Refinement (2026-06-13, [PR 839](https://github.com/truffle-ai/dexto/pull/839)):** steer
messages drained from the queue *before the first model request* are now held until the
**next model boundary** rather than folded into the opening request. The takeaway for
inber: steer injection points are the gaps *between* model calls, not arbitrary moments —
a steer that arrives before any model turn has run has no boundary to attach to yet, so it
defers to the first real one. Apply steers only at model-call boundaries so they never
corrupt an in-flight assistant message or a partially-built first request.

### 4. TOCTOU file-safety guard at the approval boundary

`write_file`/`edit_file` now hash the content previewed at *approval* time and
reject execution if the file changed before the actual write (plus workspace-handle
path normalization rejecting escapes) — closing the time-of-check/time-of-use gap
where a user approves a diff but the on-disk file has since changed.

**What inber should consider:** in the PreToolUse prehook, capture a content/file
hash of what the user actually approved for write/edit tools and re-verify it at
execution time, denying if the target changed since approval — a concrete TOCTOU
defense for the permission gate.

### 5. Core-provided ToolPresentation snapshots (note the edge-presentation tension)

PR 796 adds `ToolPresentation` / `ToolPresentationSnapshotV1` so tool headers,
arg/result summaries, chips, and approval actions are produced by *core* and
shipped to clients, rather than each interface parsing tool names/args. This
tensions with inber's "presentation belongs at the edge" directive.

**What inber should consider:** bridge-ui parses tool names client-side today; if
that drifts across clients, have the harness emit a *versioned, transparent*
presentation hint blob (structured data, not formatted strings) that bridge-ui
renders uniformly — keeping the edge in control while eliminating per-client
tool-name parsing drift. (The per-session model auth profiles / ChatGPT-Login in
[PR 804](https://github.com/truffle-ai/dexto/pull/804) are largely redundant with
inber's model-store, except the runtime auth-profile switch projected into the
model call — minor.)

## Harness-watch — 2026-06-04: `interaction:blocked` as a distinct event from `llm:response`

[PR 811](https://github.com/truffle-ai/dexto/pull/811) introduces a dedicated
`interaction:blocked` event that fires *before* a real LLM call when a policy
blocks the interaction, and tightens `llm:response` to represent **only** actual
model output (required provider/model/token/finish-reason fields). The blocked
event flows through the same agent/session event buses consumed by TUI, web, and
A2A subscribers — so a synthetic "this was denied" message and a genuine model
turn are no longer the same event shape on the wire.

**What inber should consider:** this maps directly onto inber's tracked
`user_message` dual-emit / `TurnsView` dedup problem (the prompt and policy-block
synthetics currently ride the same stream and get reconciled at render). Adopt
dexto's split at the *event contract* level: emit policy-blocked interactions as a
distinct typed event (carry the prehook verdict + reason), and reserve the
assistant/response event for real model output with required usage fields. Stream
subscribers (bridge-ui TurnsView, kanban curators reading session state) then never
have to guess whether a message came from the permission prehook or the model —
removing the render-time dedup entirely instead of patching it.

## Harness-watch — 2026-06-17: media as a first-class retention class + provider-edge attachment projection

Two context/attachment fixes that each carry a transferable design point for inber's
text-centric truncation + transparent-bridge model.

**1. Age-tiered media retention ([PR 837](https://github.com/truffle-ai/dexto/pull/837)).**
Fixes a context-manager bug where images/media returned *by tool results* were dropped before
LLM formatting, so the model couldn't analyse media a tool produced. The fix rehydrates recent
tool-returned media within the existing media-retention window, while keeping *older* tool
media compacted/excluded (blob-backed resources preserved) so large tool outputs don't inflate
later prompts. The idea worth extracting is the **age-tiered media policy**: media is a
first-class retention class with its own recency window — rehydrate-when-recent,
compact-when-old — not flat text.
- **What inber should consider:** give inber's context manager / `smart-truncation` an explicit
  media-retention class — keep tool-returned images/media rehydrated within a recency window
  and reference-compact (blob pointer) older media — instead of letting media fall through the
  text-oriented truncation path and silently disappear before the model sees it. Folds into the
  structured-eviction value/lifecycle tagging in `smart-truncation.md` (media =
  high-value-when-recent, evict-to-blob-pointer when old) and the CWL paper thread.

**2. Text-doc inlining at the provider-adapter edge ([PR 848](https://github.com/truffle-ai/dexto/pull/848)).**
Providers/models don't consistently accept *text* documents (e.g. markdown attachments) as
native file parts. At the formatter boundary (the Vercel provider-adapter), dexto now inlines
text-like local uploads as plain text into the message, while preserving non-text and remote
files as native file parts — UI/history attachment behaviour stays intact. The design point:
the provider bridge is the correct place to project an attachment into whatever shape *that*
provider accepts, because file-part support is a per-provider capability the canonical message
shouldn't assume.
- **What inber should consider:** ensure inber's provider bridges (`llm-bridge-anthropic`/
  `-openai`/`-google`) own the projection of attachment/document parts into each provider's
  accepted shape — inline text-like docs as content where native document parts aren't
  supported, keep binary/remote as file parts — so a text attachment in the canonical
  `msg.Conversation` never silently drops because a given provider lacks document-part support.
  This is a per-provider capability decision at the bridge edge, consistent with "presentation/
  projection belongs at the edge" (note: *projection* at the edge, not lossy transform — the
  attachment still reaches the model, just in a provider-accepted shape).

## Harness-watch — 2026-07-15: keep provider-readable tool media out of the model process heap ([PR 889](https://github.com/truffle-ai/dexto/pull/889))

A distinct axis from the 06-17 media work: that entry was about **context tokens**
(which media to keep in prompt history); this one is about **process memory**. When a
tool returns a large blob-backed resource, dexto's history builder used to `read()` it —
downloading the bytes and base64-expanding them **into the model process heap** — even
when the media would only ever be sent to the provider as a reference or elided as a
placeholder. The fix asks the artifact store for a **provider-readable URL first**: if the
store can hand back an `https://` URL for an image, it passes that URL straight to the
provider; if the media is being placeholdered anyway (binary type, or filtered by
`allowedMediaTypes`), it emits a compact text placeholder from *metadata only*
(mime/size/name) and never touches the bytes. Only stores without provider-readable URLs
fall back to the download-and-inline path. The design point: **history assembly should
resolve media to the cheapest faithful representation the provider will accept — a remote
URL — and must not pay heap for bytes it is about to reference or drop.**
- **What inber should consider:** this is the missing footnote to inber's own MCP-media
  OOM (`project_mcp_descoping`: browser MCPs cost ~890MB/session and OOM'd the box) and to
  the `msg.Conversation` assembly in `llm-bridge-server`. inber's provider bridges
  (`-anthropic`/`-openai`/`-google`) all accept image/document parts *by URL*, yet the
  path that materializes tool-returned media into a conversation should be audited for the
  same trap: does it read+base64 a large tool/MCP artifact into the server heap when it
  could pass the artifact-store URL through, or (when the part will be truncated/placeholdered
  by the media-retention class) render a metadata-only placeholder without ever loading the
  bytes? This is squarely the "layers are transparent — no lossy transform, pass the
  reference through" rule at the media edge, and it caps a real memory-footprint failure
  mode, not just token count. Cheap audit, potentially large RSS win on media-heavy sessions.
