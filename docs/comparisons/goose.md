# Goose Comparison

**GitHub:** github.com/block/goose
**Language:** Rust
**Focus:** AI agent framework with CLI and desktop interfaces, extensible via MCP (Model Context Protocol)

## Architecture Overview

Goose is a comprehensive AI agent framework built in Rust with a multi-tier architecture:

```
┌─ UI Layer ──────────────────────────────────────┐
│  CLI (goose-cli) | Desktop (Electron) | Custom  │
└─────────────────┼─────────────────────────────────┘
                  │
┌─ Server Layer ──┼──────────────────────────────────┐
│  goose-server (REST API) + Agent Client Protocol  │
└─────────────────┼─────────────────────────────────┘
                  │
┌─ Core Layer ────┼──────────────────────────────────┐
│  Provider System | Extension Manager | Config      │
└─────────────────────────────────────────────────────┘
```

## Extension/Plugin Architecture

**MCP-Centric Design:**
- Uses Model Context Protocol (MCP) as the primary extension mechanism
- Extensions can be: builtin (Rust), stdio processes, HTTP servers, or inline Python
- Clean separation: extensions provide tools, resources, and instructions to the core

**Extension Types:**
```rust
enum ExtensionConfig {
    Builtin { name, timeout },
    Platform { name },  // In-process Rust extensions
    Stdio { cmd, args, envs },
    StreamableHttp { uri, headers },
    InlinePython { code, dependencies },
}
```

**Tool Management:**
- Tools are automatically prefixed (`extension__tool_name`) unless marked as "unprefixed"
- Extensions declare which tools they provide via MCP
- Runtime discovery and registration of tools
- Built-in tool validation and parameter checking

## Provider System

**Clean Abstraction:**
```rust
#[async_trait]
pub trait Provider {
    async fn stream(&self, model_config, session_id, system, messages, tools) -> MessageStream;
    async fn complete(&self, ...) -> (Message, ProviderUsage);
    fn get_model_config(&self) -> ModelConfig;
    // ... other methods
}
```

**Dual Provider Support:**
- **Declarative Providers:** JSON configuration files for OpenAI-compatible APIs
- **Code Providers:** Full Rust implementations for complex providers (Anthropic, etc.)
- Provider metadata includes model capabilities, context limits, cost information

## Key Architectural Strengths

### 1. **Separation of Concerns**
- Core logic, providers, extensions, and UIs are cleanly separated
- Each crate has a single responsibility
- Clear interfaces between layers

### 2. **Extension Modularity**
- MCP standard enables language-agnostic extensions
- Extensions are self-contained (declare their own tools, resources, instructions)
- Dynamic loading and unloading of extensions
- Built-in security model for extension execution

### 3. **Multi-Interface Support**
- CLI, desktop app, and custom UIs all use the same core
- Server mode enables web/mobile/IDE integrations
- Agent Client Protocol (ACP) for rich programmatic access

### 4. **Recipe System**
- YAML-based workflow definitions
- Parameter substitution and templating
- Sub-recipes and subagent composition for complex workflows
- Shareable and distributable workflow packages

## What Inber Should Adopt

### 1. **Provider Interface Design**
Goose's `Provider` trait is excellent:
- Clean async interface with streaming support
- Metadata-driven provider discovery
- Support for both declarative (JSON) and code-based providers
- Usage tracking and model capability detection

**Inber Action:** Extract a similar `Provider` interface in `agent/provider.go` that wraps the current Anthropic SDK coupling.

### 2. **Extension Architecture**
The MCP-based extension system is superior to inber's current approach:
- **Self-describing:** Extensions declare their tools, not the core
- **Multi-transport:** stdio, HTTP, in-process
- **Standard protocol:** MCP is becoming an industry standard
- **Security model:** Clear boundaries and permission system

**Inber Action:** 
- Evaluate MCP adoption for inber's tool system
- Move towards self-describing tools vs. hardcoded tool registration
- Consider stdio and HTTP tool execution models

### 3. **Configuration Structure**
Goose's config hierarchy is clean:
```
- Provider configs (declarative JSON + secrets)
- Extension configs (with environment substitution) 
- Recipe configs (workflow definitions)
- Global settings
```

**Inber Action:** Simplify inber's config system with similar separation.

### 4. **Crate Organization**
```
goose/              # Core logic
goose-cli/          # CLI interface
goose-server/       # HTTP API server  
goose-acp/          # Agent Client Protocol
goose-mcp/          # MCP client implementations
```

**Inber Action:** Consider breaking inber into similar focused modules.

## What Goose Does Differently

### 1. **Recipe-First Workflows**
- Complex multi-step workflows are first-class citizens
- YAML configuration for reproducible agent behaviors
- Sub-recipe composition and parallel execution

*Inber equivalent:* Could add recipe/workflow system to inber.

### 2. **Multi-Model Orchestration**
- Lead/worker model patterns built-in
- Fast model fallbacks
- Model capability-aware tool routing

*Inber potential:* Inber could benefit from multi-model support.

### 3. **Rich Session Management**  
- Session persistence and resumption
- Session-specific extensions and configurations
- Cross-session subagent spawning

*Inber gap:* Inber has simpler session model.

## What Inber Does Better

### 1. **Memory Architecture**
Inber's memory system (conversation summarization, pruning, stashing) is more sophisticated than Goose's basic session persistence.

### 2. **Context Management**
Inber's smart truncation and context loading is more advanced than Goose's approach.

### 3. **Simplicity**
Inber's current model is simpler and more focused, while Goose tries to be everything to everyone.

## Recommended Adoptions for Inber

**High Priority:**
1. **Provider interface extraction** - Decouple from Anthropic SDK
2. **Self-describing tools** - Move towards MCP-style tool registration
3. **Declarative provider configs** - Support JSON-based provider definitions

**Medium Priority:**
4. **Extension stdio/HTTP support** - Beyond just in-process tools
5. **Recipe system** - For workflow standardization
6. **Module reorganization** - Break into focused packages

**Low Priority:**
7. **Multi-model support** - Lead/worker patterns
8. **Desktop interface** - Beyond CLI

The key insight: **Goose's architecture is more modular and extensible**, while **inber's core agent logic is more sophisticated**. Inber should adopt Goose's modularity patterns while preserving its advanced memory and context management.

---

## Harness-watch — 2026-05-09: Projects as backend sources, system-prompt injection

[PR 8739](https://github.com/block/goose/pull/8739) (merged 2026-05-07) graduates "projects" from a Tauri-only frontend IPC concept into a first-class ACP `sources` entity served by `goose serve`, with two design choices worth pulling out:

- **System-prompt injection at the agent layer.** Project instructions previously came from the desktop client and were prepended to every user turn. Now the project source is read server-side and injected into the *system prompt* once per conversation. This is a direct prompt-cache hit-rate win — the cacheable prefix stops being invalidated by a per-turn prepended payload.
- **Storage as `.md` + YAML frontmatter under `Paths::data_dir()/projects/`.** Project definitions live as plain files with structured metadata (name, description, instructions, working dirs), making them human-editable and version-controllable. Skills become project-scoped via the same working-dir scan.

## Harness-watch — 2026-06-02: blocking Stop hooks, keyed prompt fragments, live context window

> **[Verified 2026-08-01 — §2's two defect claims are REFUTED; §1 and §3 are feature ideas, not defects.]**
> *"Compose the prompt from named fragments"* — already done: `engine/turn_prompt.go:113,166`
> build `sessionMod.NamedBlock{ID, Text}`. What is missing is a runtime API to set/clear one
> by key, which is a feature. (Minor real gap: `buildSystemBlocks` drops the ID at
> `turn_prompt.go:193`, but Anthropic has no per-block key to carry it anyway.)
> *"Drive truncation from the live window, not a static per-model constant"* — already
> single-sourced from model-store: `agent/models.go:101 ContextWindow: requestableContextWindow(model)`
> → `engine/build.go:114` → used at `agent/agent_run.go:120-121`. The remaining constants are a
> documented fallback for a *missing* registry row (`agent/models.go:25`) and a real client
> ceiling (`agent/models.go:46`). No provider in inber's path advertises a live window, so
> there is nothing to prefer over the registry.

### 1. Honor blocking Stop hook decisions, capped by a consecutive-block counter

[PR 9468](https://github.com/block/goose/pull/9468) routes Stop hooks through the
same blocking-decision path as PreToolUse: a Stop hook returning
`{"decision":"block","reason":...}` at turn-end now feeds the reason back to the
agent as a hidden user message and the loop *continues* (within `max_turns`)
instead of ending. To prevent infinite re-engagement it adds
`GOOSE_STOP_HOOK_BLOCK_CAP` — once consecutive Stop denials exceed it, goose
overrides the block, warns, and ends the turn (mirrors Claude Code's
`CLAUDE_CODE_STOP_HOOK_BLOCK_CAP`). The novel piece is a turn-end policy gate
that can re-engage the agent with a reason, bounded by a counter.

**What inber should consider:** add a Stop-style turn-end gate to the PreToolUse
prehook layer — a policy can return block+reason at turn completion, re-injected
as hidden context to resume the loop, guarded by an env-tunable consecutive-block
cap so a misbehaving policy can't trap the agent in an infinite turn loop.

### 2. Keyed runtime system-prompt fragments + trust the live context window

[PR 9478](https://github.com/block/goose/pull/9478) adds an ACP method to set the
session system prompt at runtime, supporting both base-prompt replacement and
**keyed** additional-instruction fragments (each named so it can be individually
added/cleared) — composable, addressable system-prompt segments, not one opaque
blob. [PR 9455](https://github.com/block/goose/pull/9455) fixes `AcpProvider`
reporting a static `context_limit` even when the wrapped downstream server
advertises its real context size via `usage_update`; it now tracks the latest
advertised size in an `AtomicU64` and uses that, falling back to static config
only when nothing was advertised — so truncation/compaction decisions use the
live model's real window.

**What inber should consider:** compose inber's system prompt from named,
individually add/clear-able fragments (identity card, project `INBER.md`, tool
inventory) so sources can be swapped without rebuilding the whole prefix; and
when a provider advertises its real context window, drive truncation/memory-
compaction from that live value rather than a static per-model constant
(single-source-of-truth).

### 3. Skill token-count CLI (observability)

[PR 9326](https://github.com/block/goose/pull/9326) adds a `goose skills` command
that enumerates discovered skills and reports the token count each contributes.

**What inber should consider:** surface per-source token cost (skill-store /
SKILL.md entries, project context blocks, tool inventory) in inber's CLI tool
surface, so the user can see what's consuming the system-prompt/context budget —
useful for tuning what stays cacheable. (The related
`GOOSE_MAX_TOOL_RESPONSE_SIZE` configurable truncation in
[PR 9256](https://github.com/block/goose/pull/9256) is already covered by inber's
`docs/smart-truncation.md`.)

**What inber should consider:** Inber has at least two surfaces today that prepend per-turn rather than inject into the system prompt — the conversation summary header and the project-context block built by `engine/turn_prompt.go`. Whatever is *stable for the duration of a session* (project-level `INBER.md`, agent identity card, tool inventory description) belongs in the system prompt where it'll cache, not in the user turn where each new turn pushes it past the cache breakpoint. The goose pattern also argues for promoting any "project" concept inber adopts (today closest to forge worktree slots + agent-store config) to a server-side source rather than something the chat frontend owns. Worth a section in `docs/cache-optimization.md` cross-referencing `reference-based-prompt-architecture.md` — the two notes already converge on this thesis.

> **[Verified 2026-08-01 — REFUTED. This is the most overstated passage in the file; do not action it.]**
> *"The project-context block built by `engine/turn_prompt.go`"* — **no such block exists.**
> `grep -rn "INBER.md" --include=*.go` returns nothing. What `turn_prompt.go:129-152` assembles
> into the user turn is `VolatileContext`: fleet status, `fileref:`/`recent:`/`file:` memories,
> task-plan/scratchpad injectors, source ref. Every one of those is genuinely per-turn volatile,
> and putting them in the user message is **deliberate and correct for caching** — see
> `turn_prompt.go:122-124` and `agent/agent_run.go:86-88` ("keeps it AFTER BP3, preventing cache
> busting"). The passage recommends undoing a correct optimization.
> *"Agent identity card, tool inventory belong in the system prompt"* — they already are
> (`turn_prompt.go:126` + the doc comment at `:76-78`), with the breakpoint on the last stable
> block (`turn_prompt.go:217,223`) and a byte-identity reuse path (`:202-212`).
> *"The conversation summary header"* — real, but not per-turn: `conversation/summarize.go:104`
> inserts it **once**, at compaction, after which it is part of the cacheable prefix. One
> invalidation per compaction is inherent to compacting at all.
> One genuine (tiny) defect found nearby, and it is not the one alleged:
> `server/session_context.go:25-26` claims the injector's output is "injected into the system
> prompt"; it actually goes to `VolatileContext` in the user turn. Lying comment — **fixed
> 2026-08-01 in this pass.**


## Harness-watch — 2026-06-05: bound sub-tasks by turn budget the agent can see, not a wallclock timeout it can't

> **[Verified 2026-08-01 — PARTLY: two of the three named subsystems are not inber's, but the `spawn_agent` half holds.]**
> "autoworker / scoper / dispatcher sub-sessions" are llm-bridge/kanban concepts —
> `grep -rni "autoworker|scoper"` returns zero in inber. Read the entry as being about
> `spawn_agent` only.
> That half is confirmed: the sole knob is wall-clock — `server/spawn_tools.go:100-103`
> (`timeout_seconds`), `server/spawn.go:23` (`defaultSpawnTimeout = 5m`), enforced by
> `context.WithTimeout` at `spawn.go:298`. `SpawnRequest` (`spawn.go:14-21`) has no `max_turns`.
> Worse than the passage says: the child is created with `RunRequest{}` (`spawn.go:219`), so the
> `MaxTurns = 25` default at `engine/engine_new.go:406-408` never applies — a child gets a turn
> limit only if its agent-store config sets one, otherwise **unlimited turns**. And the limit is
> surfaced only *after* exhaustion, as a stop reason (`engine/build_hooks.go:50-54`); nothing
> puts the remaining allowance in the child's prompt (`spawn.go:302` passes `req.Task` verbatim).

[PR 9571](https://github.com/block/goose/pull/9571) removes the 5-minute
`CHECK_TIMEOUT_SECS` wall-clock cap on review subprocesses and replaces it with
`--max-turns`, extending the turn cap to the main per-file reviewer passes (which
previously had none) and — the key move — **adding a "## Turn budget" section to
the subagent's prompt so it sees its remaining allowance.** The stated rationale:
a wall-clock timeout is "opaque to subagents and could kill in-progress work with
no findings," whereas a turn count is a budget the agent can reason about and
adapt to (e.g. wrap up and emit partial findings before it runs out).

**What inber should consider:** inber's autoworker / scoper / dispatcher
sub-sessions and any `spawn_agent` child today are bounded mainly by wall-clock /
external watchdogs. Swap (or pair) those for an agent-visible **turn budget**:
pass a max-turns limit into the child and surface "N turns remaining" in its
prompt, so a hard kill becomes a planning constraint and the child emits partial
results instead of dying silently with nothing. This composes with the
turn-limit-based review timeout goose already replaced, and with the kanban
task-completion-loop's dispatcher closure logic — a turn budget is a cleaner
"this sub-card is over its allowance" signal than a timestamp.

## Harness-watch — 2026-06-09: org-managed security gates that read *strictly from process env*, so persisted config can't impersonate them

> **[Verified 2026-08-01 — REFUTED as a defect: the named surfaces are not inber's.]**
> "PreToolUse prehook gating" and "auto-allow for unattended autoworkers" live in
> llm-bridge-server, not here — `grep -rni "PreToolUse|prehook|auto_allow|permission"` across
> inber's Go returns one unrelated hit (`engine/openclaw_feed.go:187`). inber's actual toggle
> is `guard.Mode` (`guard/guard.go:34-51`), enforced at `guard/guard.go:138-156` via
> `engine/build_hooks.go:93-99`, set per session in `server/session_creation.go`. There is no
> org-managed layer, so nothing can impersonate one — this is a greenfield feature, not a bug.
> If it is ever built, note the collision: `guard.Unset` is documented at `guard/guard.go:38-45`
> as meaning *full access*, so an env-override layer must decide how a silent config interacts
> with an enforced floor.

[PR 9612](https://github.com/block/goose/pull/9612) replaces goose's old
`DEFAULT_SECURITY_*` "seed-once" defaults with runtime **override** env vars
(`SECURITY_PROMPT_ENABLED_OVERRIDE`, `SECURITY_COMMAND_CLASSIFIER_ENABLED_OVERRIDE`)
that take precedence over user config and turn prompt-injection / command-injection
detection on by default for internal users — kept on, with no way for the user to
disable it. The mechanically interesting bit: a single `get_override` helper reads
the value **strictly from `std::env::var`, never the config store**, so a value a
user persisted into their own config can't masquerade as the org-managed setting.
When the env vars are unset, behaviour is unchanged and the user's own settings
apply. ([PR 9690](https://github.com/block/goose/pull/9690) then re-tunes the
detector's confidence thresholds.) This is the same *non-overridable policy layer*
shape as codex's org-level managed permission allowlists (agentic-design-patterns,
06-06 entry), generalised from "what tools are allowed" to "is a safety feature on".

**What inber should consider:** inber's permission/security toggles (PreToolUse
prehook gating, auto-allow for unattended autoworkers, any future injection
detection) are configured per-agent/per-session. For settings that must be
org-enforced rather than agent-selectable — e.g. "unattended workers may auto-allow
file writes but NOT network egress", or "injection screening is always on" — read the
enforcing value from a source the agent/session config **cannot write to** (process
env / a root-owned file), with a strict precedence: env-override > agent-store config
> default. The anti-spoofing rule is the point: a session must not be able to persist a
value that impersonates the org-managed one. Pairs with the `feedback_audit_deployed_env`
lesson — deployed env is already the source of truth for "what's actually enforced".

## Harness-watch — 2026-06-11: a provider-neutral *thinking-effort* enum that maps onto each provider's native reasoning knob

> **[Verified 2026-08-01 — PARTLY: the premise is wrong, but there is a real fail-silent bug underneath. Filed.]**
> *Premise wrong:* inber does not "route across providers (claudecode, jig, forgecode)" — inber
> **is** one harness and stamps `Harness: "inber"` on every session it serves
> (`server/api_bridge.go:110,156,324`). Harness routing is llm-bridge-server's job. What inber
> actually routes across is Anthropic vs. OpenAI-compatible model clients
> (`engine/turn_execute.go:29-30`, `agent/clients.go:92-99`).
> *Real defect:* `server/api_bridge.go:657` already defines an abstract `Effort`
> ("high"/"medium"/"low"), but `handleBridgeConfig` maps it **only** onto the Anthropic thinking
> budget (`api_bridge.go:688-703` → `SetThinkingBudget` → `agent/agent_run.go:77-82`). The
> OpenAI path (`engine/turn_openai.go:65`) never reads it — `reasoning_effort` appears nowhere
> in the repo. So on an OpenAI-served session, `effort:"high"` is accepted with a 200 and
> silently does nothing. That is worse than the "no-op where there is no knob" the passage
> endorses, because OpenAI *does* have the knob.

[PR 9743](https://github.com/block/goose/pull/9743) adds **canonical thinking modes** —
a small fixed enum of reasoning-effort levels (low/medium/high-style) that goose defines
once and then translates per-provider onto whatever that provider actually exposes:
Anthropic's thinking-token budget, OpenAI's `reasoning_effort`, and others that have no
knob at all collapse to a no-op. [PR 9711](https://github.com/block/goose/pull/9711) then
surfaces the same level as an **ACP config option**, so a client (or recipe) sets one
abstract dial and goose maps it down at the provider boundary. The point is that
"how hard to think" becomes a portable, first-class session setting that survives a
model swap, instead of a provider-specific magic number leaking into every call site.

**What inber should consider:** inber routes across providers (claudecode, jig, forgecode,
…) and each has a different reasoning control — Anthropic thinking budget vs. OpenAI
effort vs. none. Define one canonical effort enum at the bridge layer and translate it in
each harness adapter (the same place that already maps model ids), so a kanban card / task
can request `effort: high` once and have it mean the right thing regardless of which
provider the dispatcher picks — and degrade to a no-op rather than erroring on providers
with no knob. Keeps the routing decision (which model) orthogonal to the effort decision
(how hard it thinks), and stops per-provider reasoning params from being hard-coded at
spawn sites. Lives naturally next to the model-store mapping (`reference_model_store`).

## Harness-watch — 2026-06-15: unified cross-tool security telemetry schema + pattern-detector calibration

> **[Verified 2026-08-01 — REFUTED, and wrong in the opposite direction to the claim.]**
> The passage says inber's gates "each log in its own shape". In fact **none of them log at
> all.** `guard.CheckTool` (`guard/guard.go:138-156`) returns a verdict with no logging, and its
> only consumer renders it to the model as a refusal string and drops it
> (`engine/build_hooks.go:95-98`). No event, no counter, no logstack write. The other named
> surfaces (prehook, herald, autoworker) are other repos.
> The real nearby gap is narrower and worth having: **a denied tool call leaves no audit trail
> anywhere.** A minimal `logger.WithComponent("guard")` emission on `Denied`/`NeedsApproval` is
> under an hour; the canonical cross-service schema is a different, larger question.
> The calibration half is advice about future pattern detectors — inapplicable today, since
> `isReadOnly`/`isDangerous` are tool-**name** allowlists with no regex anywhere in `guard/`.

[PR 9713](https://github.com/block/goose/pull/9713) makes goose emit its security
findings (prompt-injection, data-exfil egress, adversary ALLOW/BLOCK, tool-execution
user decisions) under a **single standardized OTLP schema** shared across goose's
sibling agent tools (Sandpit, BuilderBot): `security.event_type / .action /
.confidence / .threat_type`, plus `session.user / .host / .agent_type` as correlation
pivots (cached once per process via `OnceLock`, propagated to every log record in a
reply-stream span). The point is operational: with consistent field names, *one* query
surfaces threats across every tool instead of per-tool bespoke parsing. Separately,
[PR 9690](https://github.com/block/goose/pull/9690) is a calibration lesson — the
pattern detectors fired at Medium/0.60 on routine input (any single `\uXXXX`, any
nested `$(...)`), flooding anyone running a threshold ≤0.60; the fix tightens patterns
to require a stronger signal (3+ consecutive unicode escapes, not one) so a hit is
actually meaningful.

**What inber should consider:** inber's security surface is *spread across services* —
the PreToolUse prehook (`project_permission_prompt_followups`), autoworker auto-allow,
herald — each logging in its own shape. Define one canonical security-event schema
(`event_type / action / confidence / threat_type` + `session_id / agent_type` pivots)
emitted by every gate, so a single healthcheck/observability query can answer "what got
blocked/flagged across all bridge sessions today" without per-service grepping. And take
the calibration lesson directly: any pattern-based deny rule should be tuned against
real traffic and require a strong signal before it fires at a blocking confidence —
a detector that cries wolf on `$(dirname …)` gets globally disabled and protects nothing.

## Harness-watch — 2026-06-17: sharpen the delegate contract on both ends — typed reference-context *in*, structured status *out*

Two `summon` (delegate) PRs tighten goose's subagent contract in a way inber's spawn/dispatcher
path doesn't yet document. Both are the "names/enums tell the truth, presentation at the edge"
doctrine applied to delegation.

**1. Typed `context` distinct from `instructions` ([PR 9518](https://github.com/block/goose/pull/9518)).**
Delegates are "blind" — a child sees only `instructions` (the directive) plus `source`, so
parents today cram reference material (file contents, constraints, prior findings) into
`instructions`, conflating *what to do* with *what to know*. The PR adds an optional `context`
param injected into the child's **system prompt** under a `# Reference Context` heading,
keeping task-direction and background-knowledge as distinct typed inputs.
- **What inber should consider:** give inber's `spawn_agent`/delegate path a typed `context`
  input separate from the task instruction, injected into the child's system-prompt prefix
  (where it caches) under a stable heading — so parents stop conflating "what to know" with
  "what to do," and background material doesn't invalidate the child's cacheable prefix the
  way an inlined instruction blob does. Composes with the existing "pass a durable plan-file
  path" idea: children read large reference *by path*, inline small `context`.

**2. Structured result envelope instead of prose ([PR 9521](https://github.com/block/goose/pull/9521)).**
`load(task_id)` now returns structured `CallToolResult.meta` — `task_status`
(`completed`/`failed`/`panicked`/`cancelled`), `turns_taken`, `duration_secs` — instead of
signalling completion only via markdown (`✓ Completed`). A parent can finally distinguish
"extension failed to connect" vs "hit turn limit" vs "model refused" vs "panicked" without
parsing prose, so it can make a sound retry/escalation decision.
- **What inber should consider:** have inber's subagent/delegate collection path return a typed
  result envelope — granular `status` enum (completed/failed/turn-limit/refused/panicked/
  cancelled), `turns_taken`, `duration` — as structured metadata, not markdown, so the kanban
  task-completion-loop dispatcher decides retry/escalation/close on a field instead of
  string-matching prose. Directly aligns with `feedback_status_enum_granularity` (keep the
  enum granular, map to pills at the edge) and is the *outbound* twin of codex #28375
  (error-precedence forwarded to the parent, agentic-design-patterns 06-17).

## Harness-watch — 2026-06-22: non-blocking peek at a running delegate (wait/kill is not the only choice)

**`summon` peek mode ([PR 9519](https://github.com/block/goose/pull/9519)).** `load(task_id)` on an
async delegate was binary: block until it finishes, or `load(cancel)` to kill it. The PR adds
`load(task_id, peek: true)` — a **non-blocking, non-destructive status read** of a still-running
child: its description, elapsed wall-time, turns-taken, how long it's been idle, and the count of
buffered (un-collected) tool-call outputs. The parent can inspect progress without committing to
wait-or-kill. This is the *mid-flight* complement to the 06-17 structured result envelope (#9521),
which only typed the *terminal* outcome.

**What inber should consider:**
- Give inber's spawn/collect path and the kanban task-completion-loop **dispatcher a non-blocking
  peek** on a running subagent (turns-taken, idle duration, buffered-output count) so it can make
  revive / keep-waiting / escalate decisions **without blocking the 5-min curator tick or killing
  the child**. This directly addresses `feedback_polling_loops` (no leading sleeps) — a peek is a
  cheap point-in-time read, not a wait — and pairs with the agent-visible turn-budget idea
  (goose 06-05) and the typed status envelope (06-17): peek reports `turns_taken` against the
  budget the child already knows. An idle-too-long peek is a concrete signal a worker is stuck at
  awaiting_permission, the exact failure mode `project_autoworker_leak` fixed reactively.

## Harness-watch — 2026-07-01: a volatile block in `system` still busts the *message* cache — put it after the last message breakpoint

**[PR #10030](https://github.com/block/goose/pull/10030) — keep turn-context out of the Anthropic prefix cache.**
goose injects a per-turn "turn-context" block (current time to the minute, cwd, turn budget,
compaction status) onto the request before every Anthropic call. Because it sat *upstream* of the
message `cache_control` breakpoints and changed every minute, its bytes were inside the cached
prefix and the hash missed **every** call — goose paid cache *creation* (1.25× input) instead of
*reads* (0.10×) on the whole conversation, turn after turn. The fix relocates the volatile block to
the **tail of the messages, after the last breakpoint**. The load-bearing detail: a trailing
`system` block is *not* enough — Anthropic hashes `tools → system → messages`, so a changing
`system` entry still precedes the message breakpoints and re-creates the message cache. Their
isolation test nails it: volatile-in-prefix → `read 0 / creation 1946`; volatile-as-trailing-system
→ `read 2162 / creation 1946` (message cache still lost); volatile-as-message-tail →
`read 3885 / creation 0` (fully preserved).

**What inber should consider:** inber has the *exact* structure goose's rejected fix has. `engine/turn_prompt.go`
places `cache_control` on the last stable block before `__CACHE_BOUNDARY__` and keeps the volatile
blocks (fleet status, recent files, context injectors) at the **tail of the `system` array** — the
"trailing system block" case. Since inber also carries message-side breakpoints (BP3, the latest
user message — see `cache-optimization.md`), that volatile system tail sits before the message
breakpoints in the `tools → system → messages` hash and **re-creates the message cache every turn**,
silently, exactly as goose measured. Action: move inber's per-turn volatile blocks out of `system`
and append them as the **last message** (after the final `cache_control`), leaving only truly stable
content in `system`; then re-run the `cache-optimization.md` before/after harness on a multi-turn
session and confirm `creation → 0` on the conversation prefix. This is the missing enforcement half
of that doc's own thesis — the doc keeps dynamic content out of the *system* prefix but doesn't
account for volatile system content invalidating the *message* prefix downstream.

> **[Verified against the code 2026-08-01 — this entry is STALE. Do not action it.]**
> inber has goose's **accepted** fix, not the rejected one. The migration this entry asks
> for has already happened. `engine/turn_prompt.go:122-124` puts only stable content in the
> system array and collects the volatile parts (fleet status, volatile memories, context
> injectors, `sourceRef`) into `e.Turn.VolatileContext` at `turn_prompt.go:149`;
> `agent/agent_run.go:85-115` then injects that string into the **last user message, after
> the final `tool_result`** — precisely goose's `volatile-as-message-tail` case.
> `buildSystemBlocks` says so itself at `turn_prompt.go:178-179`: "All system blocks are now
> stable (volatile content moved to user message injection)."
> Two leftovers found while checking, both harmless and neither the alleged defect:
> `cacheBoundaryID` (`turn_prompt.go:56`) is retained but skipped as a "legacy marker"
> (`turn_prompt.go:187`), and `buildDynamicBlocks` (`turn_prompt.go:229`) has no caller.
> The one thing still worth a targeted test is ordering, not placement: `markLastContentBlock`
> (`agent/agent.go:492`) marks the last content block of a breakpoint message while the
> volatile block is appended after the last `tool_result`, so on a tool-result-only user
> message the two can land on the same block.

## Harness-watch — 2026-07-13: *signed* thinking is an exactly-once replay contract — dedupe it, don't strip it (and never guess the cause from a generic error string)

[goose #10083](https://github.com/block/goose/pull/10083) fixes a recurring Anthropic 400 by adding a `dedupe_signed_thinking` fixer to the shared `fix_messages` pipeline in `goose-provider-types/src/conversation.rs`, positioned **immediately after `merge_consecutive_messages`** (load-bearing — the merge is what creates the duplicates). `is_signed_thinking` = a `Thinking` block with a non-empty signature, or any `RedactedThinking`; a conversation-wide `seen` set drops exact (text + signature) repeats, keeping the first. The contract, verbatim from the diff's doc comment: **"Signed blocks must be replayed exactly; unsigned reasoning summaries need not"** — and unsigned reasoning is deliberately *left alone*, since providers like Kimi/DeepSeek require it echoed on every tool-call message. Duplicates arise two ways: intra-message (a standalone thinking message merged into a tool-call message that re-embedded it) and **cross-message (one provider turn split into several tool-call messages interleaved with tool results, each carrying a copy of the turn's signed thinking)**. Because the fixer sits in the provider-neutral repair pipeline upstream of every formatter, one change covers direct Anthropic, Bedrock, Databricks and Vertex.

The design idea is the asymmetry most harnesses get wrong: *signature present ⇒ exactly-once verbatim replay; signature absent ⇒ echo freely.* The signature is also what makes exact-match dedupe **provably safe** — two blocks with an identical signature can only be the same turn's thinking copied onto split messages, never two genuinely distinct thoughts. And it establishes the **conversation-repair pipeline as the right home for provider-contract normalization**, rather than each provider's formatter.

**What inber should consider:** this is directly load-bearing, because inber runs the exact configuration that produces the bug and its current remedy is a sledgehammer. `agent/clients.go:103` sends `interleaved-thinking-2025-05-14` — thinking interleaved with `tool_use` across split assistant messages, i.e. goose's **cross-message** duplicate cause precisely. (inber's `RepairAlternation` merges consecutive *user* messages but inserts a `[continued]` placeholder between consecutive assistants, so the intra-merge cause is absent; the cross-message one is not.) inber's only thinking fixer, `conversation/repair.go:207 RepairThinkingSignatures`, **blanket-strips every `OfThinking` / `OfRedactedThinking` block** and substitutes a `"[thinking redacted]"` text block. It fires from two places, and both are wrong in a different way:

- `engine/turn_execute.go:51` strips-and-retries when `apiutil.IsThinkingSignatureError(err)` — and that predicate is literally **`msg == "Error"`** (`internal/apiutil/apiutil.go:12`), a string-equality guess against the word "Error". So *any* error whose message happens to be exactly `"Error"` destroys all thinking in the conversation and retries. inber is guessing at the cause and applying the maximally destructive remedy — when the actual common cause in this configuration is a *duplicate*, for which the correct fix is dedupe-keep-first.
- `server/session_creation.go:102` strips **unconditionally on every resume**, so a resumed session forfeits interleaved thinking's continuity across tool calls — which is the entire point of the beta header inber enables one file over.

The stated rationale (signatures are credential-bound) is real, but only for the *credential-rotation* path it was written for; it is being applied as a general-purpose repair. Recommended: **(a)** narrow `RepairThinkingSignatures` to the credential-rotation case it actually addresses, and stop invoking it on generic-error retry and on every resume; **(b)** add a `DedupeSignedThinking` fixer to inber's repair pipeline (which already *is* a fixer chain — `RepairEmptyContent` → `RepairDanglingToolUse` → `RepairAlternation` → … ), ordered after any merge step, keying on (text + signature) and keeping the first; **(c)** adopt the signed/unsigned split as an explicit contract — inber today has no notion of it. Note this interlocks with the cline budget-projection entry in `agentic-design-patterns.md` (07-13), which independently arrives at the same rule from the compaction side: drop thinking when feeding a *summarizer*, preserve it verbatim for the *provider request*.

## Harness-watch — 2026-07-16: a fixed-window safety classifier is padding-bypassable — chunk with overlap, take the max score, fail closed on oversize

[goose #10416](https://github.com/block/goose/pull/10416) fixes a real evasion of goose's command-injection classifier (bashcat-distilbert), which has a hard 512-token input window: anything past the window is silently ignored, so a benign prefix can push the malicious command out of view and the model returns SAFE (measured on the live endpoint: SAFE 0.0006 → INJECTION 0.9997 once fixed). The fix (`command_chunker.rs`) splits the command into **overlapping** windows sized to a worst-case byte budget (bytes as a token proxy, so no window can exceed the limit), classifies them **concurrently**, and takes the **max** injection score. Two fail-closed details: a partial-window failure never discards a detection — a clean-but-incomplete result falls back to pattern scanning rather than being trusted — and oversized commands (beyond `MAX_WINDOWS`) are flagged *before* detections run, so the fail-closed signal survives truncation. Logs record counts only, never command content.

The general lesson: **any fixed-context safety check — a classifier, a regex over the first N bytes, or an LLM screening prompt with a token cap — is bypassable by padding the dangerous span past the window unless you scan the whole input.** Overlap matters because a naive split lets a payload straddle a boundary; max-over-windows is the safe aggregation; the truncation and failure paths must fail *closed*.

**What inber should consider:** inber's guard/permission layer (the PreToolUse prehook in bridge-server, the `permission_store` rules) matches commands against patterns — a regex like the `^curl\b` allow or any `rm -rf` deny (see `project_permission_store_live_rules_open`) is evaluated against the command string as given. Confirm those matches scan the **whole** argument, not a truncated head, and that oversized / unparse-able commands fail *closed* (deny or prompt) rather than falling through to allow. If inber ever adds an LLM- or classifier-based command screener, adopt goose's shape directly: overlapping worst-case-sized windows, max score, pattern-scan fallback on partial failure, flag-don't-drop on oversize. This is the concrete mechanism behind the `feedback_disarm_dont_document` and open permission-store concerns — a screen that only sees the first N bytes is a screen an attacker pads around.

## Harness-watch — 2026-07-20: a raw JSON-Schema tool definition is not portable — normalize `oneOf`→`anyOf` before it reaches an OpenAI-compatible backend

[goose #10571](https://github.com/block/goose/pull/10571) fixes a 400 that killed every request in a session: schemars (Rust's schema generator) emits `oneOf` of `const`s for enums with per-variant docs, and Moonshot's/Kimi's server-side schema validator only follows `anyOf` in its `$ref` termination check — so a `$ref → oneOf` is misreported as infinite recursion and rejected. The fix in `validate_tool_schemas` rewrites every `oneOf` to `anyOf`, recursing into nested `$defs`/subschemas. The rewrite is provably safe as a widening: anything valid under `oneOf` is valid under `anyOf`, so no legal argument is newly rejected. The load-bearing detail is that this is a *provider-portability* normalization, not a goose-specific patch — **OpenAI structured outputs also accepts `anyOf` and rejects `oneOf`**, so the same rewrite is the correct shape for any strict OpenAI-compatible validator, and it belongs in the one place tool schemas are projected toward that provider.

The general rule: a tool's JSON Schema is authored once (often by a codegen that emits `oneOf`, `$ref`, `format`, or other keywords a given backend won't traverse) but is consumed by N provider validators with different strictness. The bridge that projects a canonical tool definition into a provider's function-call shape must **normalize the schema to that provider's accepted dialect**, not forward it verbatim.

**What inber should consider:** inber has this exact bug latent today. `agent/openai_conversion.go:11 ConvertAnthropicToolsToOpenAI` marshals each tool's Anthropic `InputSchema` straight into the OpenAI `Parameters` map (`json.Marshal(t.InputSchema)` → `json.Unmarshal` → `Parameters: schemaMap`) with **zero normalization** — whatever `oneOf`/`$ref`/unsupported keyword an MCP tool declares is passed through unchanged to every OpenAI-compatible backend inber routes to (Kimi/Moonshot and OpenAI structured outputs among them). An MCP server whose tool schema uses `oneOf` (common: enums-with-docs, tagged unions) will 400 the whole turn against a strict validator, and inber currently has no path that would catch or rewrite it. Fix: add a schema-normalization pass in `ConvertAnthropicToolsToOpenAI` (and the equivalent Google projection) that recursively rewrites `oneOf`→`anyOf` and strips/downgrades keywords the destination provider is known to reject — a widening rewrite, applied at the bridge edge, keyed off the destination provider, never mutating the canonical `InputSchema`. This is the tool-schema analogue of the 07-18 modality-projection rule (`agentic-design-patterns.md`): *project the canonical artifact into what the destination will actually accept, at the edge, as data — don't forward it raw and don't assume every backend reads the same dialect.*

> **[Verified against the code 2026-08-01 — DONE, and it was LATENT anyway. Do not action it.]**
> The fix landed in `e6804c7` ("agent: normalize oneOf->anyOf in OpenAI tool-schema
> projection"). `agent/openai_conversion.go:26` now calls `normalizeSchemaForOpenAI` between
> the unmarshal and the `Parameters:` assignment; the function at `openai_conversion.go:56`
> rewrites `oneOf`→`anyOf` recursively through `$defs`/properties/items and merges into any
> pre-existing `anyOf`. Covered by `agent/openai_schema_normalize_test.go`, including an
> assertion that the source `InputSchema` is not mutated.
> Two corrections to the passage's framing, both worth keeping:
> (1) **There is no "equivalent Google projection" to also fix.** `agent/clients.go:93` routes
> `"google"` through `NewOpenAIClient`, so Google already shares the normalized path — one
> site, not two.
> (2) **The MCP premise was never live.** `grep -rn "inber/tools/mcp" --include=*.go` returns
> zero importers outside `tools/mcp/` itself — nothing constructs `MCPToolRegistry`, so no
> MCP-authored schema has ever reached this path. The fix was correct to make as portability
> hardening, but the "will 400 the whole turn" urgency was hypothetical.


## Harness-watch — 2026-07-26: a compaction summary should be a typed, section-ordered, user-templatable artifact — not one opaque prose blob

> **[Verified 2026-08-01 — PARTLY: the code reading is right, but the safety argument rests on a function that does not exist.]**
> Confirmed: `conversation/summary_generation.go:27-38` is the freeform prompt with exactly the
> five loose focus areas quoted; the join is `summary_generation.go:76` (**not** `summarize.go:88`,
> which is the memory-archive `memStore.Save` block). No typed contract, no section ordering, no
> template. Point (3) is also right — `conversation/summarize.go:99` estimates the *retained*
> summary, so inber does not have goose's 2.3× overstatement bug; do not "fix" it toward the raw
> usage number.
> **Wrong, and load-bearing:** the passage claims a forgiving-parse fallback "is already inber's
> error path (`generateSummary` err → `mechanicalSummary`)". **`mechanicalSummary` does not exist
> anywhere in the tree** (zero hits, tests included). The real path is
> `conversation/summarize.go:64-65`, which returns the error and **aborts compaction entirely**.
> So the "the discipline already fits" argument is false — the verbatim fallback has to be built,
> and that is exactly the safety property that made goose's change adoptable. Budget for it.

[goose #10471](https://github.com/block/goose/pull/10471) replaces goose's freeform prose compaction summary with a structured contract. The compaction prompt now asks the model for an `<analysis>` scratchpad plus a single ```json block matching `StructuredSummary` (`context_mgmt/structured.rs`): **nine named sections — user intent, technical concepts, files + key code, errors and fixes, problem solving, user messages, pending tasks, current work, next step — each ordered most-important-first.** The parsed object is rendered to markdown by a **minijinja template the user can override** at `~/.config/goose/prompts/compaction_summary.md`, so changing *what survives compaction* is a config edit, not a code rebuild; unknown JSON fields are preserved so custom prompt+template pairs can carry extra sections. Parsing is deliberately forgiving — brace-balanced candidates are tried after each `</analysis>`/```json fence or at response start, wrong-shaped fields are stringified rather than rejected, and **any parse failure falls back to keeping the raw response verbatim, i.e. exactly the old behavior** (a strict widening, never worse). A blind-judge probe eval (30 replayed conversations, Haiku 4.5 + Sonnet 4.6) held summary fidelity at parity while improving decision-recall and retaining **24–48% fewer context tokens** post-compaction. A second fix in the same PR: goose's post-compaction context baseline had been using the summarizer call's *raw output token count*, overstating retained context ~2.3×; it now bills the raw output but sets the session baseline to the *estimated tokens of the actually-retained conversation*.

The general lesson: the summary a compaction step emits is a **data structure, not a paragraph.** Typing it into ordered sections lets you (a) control and prioritize what survives, (b) trim least-important sections under pressure instead of doing prompt surgery, and (c) render it through a swappable template. The forgiving-parse-with-verbatim-fallback discipline is what makes it safe to adopt: a malformed model response is never worse than the prose blob you have today.

**What inber should consider:** inber's summarizer is exactly the opaque-blob shape goose just left. `conversation/summary_generation.go:generateSummary` prompts for a freeform bulleted summary against five loose focus areas ("main topics / key decisions / important info / project status / next steps") and joins the model's text into one string (`summarize.go:88`); there is no typed contract, no per-section importance ordering, and no way to trim or template what is retained. Adopt goose's shape: (1) prompt for a typed `StructuredSummary` (inber can reuse goose's nine sections nearly verbatim — they're generic agentic-session fields), parse it forgivingly, and **on any parse failure fall back to the current prose join** — that fallback is already inber's error path (`generateSummary` err → `mechanicalSummary`), so the discipline fits. (2) Render the parsed summary through a template so *what survives compaction* becomes tunable per role without touching Go. (3) One thing inber already gets right and should keep: `result.SummaryTokens = memory.EstimateTokens(summary)` estimates the *retained* summary text, not the API's raw output-token count — inber does **not** have goose's 2.3× baseline overstatement bug, so don't "fix" it toward the raw usage number. This complements, from the summary-structure side, the compaction rules already in `agentic-design-patterns.md` (07-13 budget projection, 07-13 drop-thinking-when-summarizing).

## Harness-watch — 2026-07-30: freeze the whole turn-context, not just the clock — plus a *message* needs its own identity, and a delegate inherits *runtime* state rather than config

> **[Verified 2026-08-01 — the message-identity, delegate-inheritance and progress-hook entries HOLD; the schema-normalizer entry is overstated. All line citations below have drifted; current ones given.]**
> **Message identity — CONFIRMED, filed.** `bus/messages/chat.go:40` declares `MessageID` and is
> the only occurrence in the whole bus repo: zero writers. Zero occurrences of `MessageID` /
> `message_id` anywhere in inber. Every producer goes through `messages.NewChatDelta(...)`
> (`server/bus.go:58,88,105,138,174`, `server/events.go:46,53,65,71`, and two more) and never
> sets it. One nuance in inber's favour that the passage misses: the OpenAI path **already**
> propagates the provider id (`agent/openai_conversion.go:193,232` write `ID: resp.ID`); only the
> Anthropic path drops it (`agent/agent_run.go:199` returns `resp` with no `resp.ID` read).
> **Delegate inherits config, not runtime — CONFIRMED.** `engine/engine.go:314-317` (`SetModel`,
> called from `server/api_bridge.go:685`) and `engine/turn_execute.go:23` (`e.Model = modelUsed`)
> both mutate the parent's live model, while spawn resolves statically:
> `server/spawn.go:170` → `session_creation.go:114 Model: ac.Model`, overridden only by an
> explicit `req.Model` (`spawn.go:174-176`). Nothing reads the parent's live `Model` on the spawn
> path and nothing logs the divergence.
> **Progress hooks — CONFIRMED.** `engine/display.go:12-18` is `DisplayHooks` with `OnThinking`,
> `OnTextDelta`, `OnToolCall`, `OnToolResult`, `OnStatus` — no progress channel. Shell output is
> fully buffered: `tool-store/tools/shell.go:63 cmd.CombinedOutput()`, so a ten-minute build shows
> nothing until it exits. This is an absent feature, not a wrong result.
> **Schema normalizer — OVERSTATED.** "Re-derived per API call" is wrong: the normalizer is
> called from inside the converter (`agent/openai_conversion.go:26`), and the converter runs at
> `engine/turn_openai.go:32`, **above** the tool-call loop that opens at `turn_openai.go:56`. So
> it is once per *turn*, not per API call — 1/N of the claimed cost, on a turn that already
> marshals the whole conversation. The const-union collapse is genuinely absent
> (`agent/chain.go:24` is the only `"enum"` in non-test Go) and is a decision-free sub-hour fix,
> but its real payload is **latent**: `tools/mcp` has zero non-test importers, so no codegen
> schema reaches this path today.

### 1. A multi-step turn is N API calls sharing one prefix — freeze every volatile input at turn start

[goose #10734](https://github.com/block/goose/pull/10734) is the direct follow-on to the 07-01
entry above. `compose_moim` called `chrono::Local::now()` itself, and compaction info and
`turns_taken` were recomputed per tool-loop iteration, so goose's `<turn_context>` block changed
bytes between the 1st and Nth call *of the same turn* — and because it sits ahead of the growing
tool-result tail, every intra-turn call missed the prefix cache. The fix captures `turn_start`,
`turn_start_compaction_info` and `turn_start_turns_taken` once before the loop and threads them
into every injection. The sharpened rule: it is not enough to keep volatile content out of the
stable prefix; anything injected at a **fixed position** must be frozen at turn start, because a
turn is not one request. Note goose had to freeze *three* fields — the clock was merely the
obvious one.

inber already holds this shape structurally, and has no timestamp in the prompt at all:
`BuildSystemPrompt` runs exactly once per turn (`engine/turn_prepare.go:69`), and
`agent/agent_run.go:119` clears the agent's `VolatileContext` after the first injection so the
tool loop cannot re-inject. But checking that turned up **two real defects in the same code path**:

- **`engine/lifecycle.go:111` is dead every turn.** `pruneIfNeeded()` appends the
  cross-zone-superseded-files note with `e.Turn.VolatileContext += "\n" + note`, but it runs from
  `prepareInput` at `engine/engine.go:224`, and `buildTurnContext` at `engine/engine.go:225` then
  **assigns** `e.Turn.VolatileContext = "[Context]\n" + …` (`engine/turn_prompt.go:154`, or `= ""`
  at :156). Whenever `e.MemStore != nil` — always, in production — the note is unconditionally
  clobbered. The model is therefore never told that frozen-zone reads are stale, which is the exact
  hazard `CrossZoneDedup` exists to report. Fix is one of: reorder, or make :154 append.
- **The thinking-signature repair path double-injects.** `agent/agent_run.go:117` mutates
  `(*messages)[lastIdx].Content` **in place** on `e.Messages`, so the first injection is already
  persisted; `agent_run.go:119` then clears only the *Agent's* copy, never `e.Turn.VolatileContext`.
  `engine/turn_execute.go:52` rebuilds the agent and re-runs, `engine/build.go:32` re-reads the
  still-set engine field, and a second `[Context]` block lands in the same user message —
  duplicated context plus a guaranteed cache miss on the retry. Clear both copies together.

### 2. `oneOf`-of-`const` is a 9× token tax, and inber normalizes schemas in the wrong place

[goose #10577](https://github.com/block/goose/pull/10577) extends the 07-20 entry above.
`schemars` renders a documented Rust unit enum as `$defs: {X: {oneOf: [{const:"list"}, …]}}` plus a
`$ref`, which costs ~9× the tokens of a flat `enum` *and* trips Moonshot's `walle` validator into
rejecting `$ref → oneOf` as infinite recursion, 400-ing the turn. `collapse_const_unions` rewrites
it to `{type:"string", enum:[…]}`, firing only when **every** member is a bare string const
(nullable `anyOf:[{$ref},{null}]`, data-carrying variants and draft-04-or-earlier dialects are left
alone). Measured: 12.5% schema shrink on `computercontroller`, 10–12% fewer input tokens per call,
identical accuracy. It runs **once at tool registration**, provider-agnostic.

**What inber should consider:** inber built the sibling of this fix and put it in the worse place.
`agent/openai_conversion.go:56-78` (`normalizeSchemaForOpenAI`) widens `oneOf`→`anyOf` for the same
`walle` failure, but runs from `engine/turn_openai.go:29` — **per API call, per tool, marshal +
unmarshal + full recursive walk** — and only on the OpenAI path. Two moves: hoist normalization to
registration time and cache the result per tool, as goose did; and add const-union collapsing to
that normalizer, because inber's real exposure is not its own schemas (hand-written
`Properties: map[string]any{}`, exactly one flat `"enum"` in the tree at `agent/chain.go:24`) but
**MCP passthrough** — `tools/mcp/adapter.go:46` forwards third-party `InputSchema` verbatim, and any
`schemars`-based MCP server hands inber precisely the `$ref → oneOf → const` shape. The widening
inber has fixes the 400 but not the 9× bloat. Copy goose's guardrail verbatim: collapse only when
every branch is a bare string const.

### 3. A tool-call id is not a message identity

[goose #10716](https://github.com/block/goose/pull/10716) gives every `Message` a `message_id` —
provider-supplied where available, else generated — applied at a handful of chokepoints so
streaming chunks, persisted history, ACP `ContentChunk.message_id`, tool requests and per-message
usage all agree. The load-bearing bug is invisible from the title: history enrichment located a
message by **`tool_call_id`, which is reused across turns**, so title and usage enrichment updated
the wrong historical message. The rule: an id minted by a *subsystem* (a tool call, a content
block) is not a valid identity for a *message*, and a provider-native id must be preserved rather
than shadowed, so live events and replayed events collapse to the same key.

**What inber should consider:** inber's wire format already has the field and inber never fills it.
`ChatDelta` declares a `MessageID` field with a `json:"message_id,omitempty"` tag, commented
"assistant message ID from CC" (`messages/chat.go:40` in the sibling `bus` repo)
and there is not one non-test writer of it in the tree, so every delta published from
`server/events.go` ships it empty — while the sibling harness layer treats it as load-bearing
(`llm-bridge-server/internal/harness/manager.go` exists to "mint canonical bridge MessageIDs and
reconcile them"). inber is the harness giving that reconciler nothing, which is the structural
precondition for the `user_message` dual-emit render bug: a consumer with no stable per-message key
is *forced* into adjacency and count heuristics. Concretely: preserve `resp.ID` from the Anthropic
response — `agent/agent_run.go:196` has the `*anthropic.Message` in hand and drops its id today —
set it once per assistant message rather than per delta, and stamp the same value on the persisted
session-log record so the live stream and `session/resume.go` replay key identically. Then audit for
goose's specific trap: inber threads tool ids widely and `sanitizeToolID` **rewrites** them, so a
tool id is neither turn-unique nor stable across the Anthropic/OpenAI conversion and must never
become a correlation key for a message.

### 4. A delegate's provider is inherited runtime state, not a config lookup

[goose #10754](https://github.com/block/goose/pull/10754) makes `summon` reuse the parent Agent's
**installed provider instance** when a delegate resolves to the same provider, keeping registry
construction only for genuinely different providers. Small diff, general rule: re-resolving a
delegate's provider from config silently discards whatever the parent session actually converged
on — a model switch, an escalation, an injected or test provider, provider-managed context.

**What inber should consider:** inber has both halves of this defect. The parent's model *does*
change at runtime — `engine/engine.go:268` `SetModel` (driven from `server/api_bridge.go:647`) and
`engine/turn_execute.go:27` `e.Model = modelUsed`, which persistently mutates the engine's model as
a side effect of per-turn selection. But spawn resolves the child from **static config**:
`server/spawn.go:87-93` reads `GetAgentConfig(req.Agent)` and only overrides on an explicit
`req.Model`, and `server/session_creation.go:48` uses `Model: ac.Model`. So a parent that escalated
to a stronger model, or that the user switched mid-session, spawns children on the agent-store
default — silently, with no log line. Second half: each engine builds a fresh `ModelClient`
(`agent/clients.go:26`), so every spawn re-resolves credentials from auth-store and stands up
another HTTP client and connection pool; given the autoworker leak history, N children = N clients
is worth avoiding on its own. Mirror goose: pass the parent's live `e.Model` as the child's default
with config as *fallback*, and hand a same-provider-same-model child the parent's existing client.

## Harness-watch — 2026-07-31: stream shell output on a best-effort side channel that provably cannot change the tool result

[#10808](https://github.com/block/goose/pull/10808) makes shell output visible while the command is
still running. `collect_tagged_lines` gained a `tokio::select!` over the merged stdout/stderr stream
plus a 150 ms flush interval, feeding a `ShellOutputBatcher` — 16 KB batches, a 256 KB live cap, the
first line emitted immediately so the user sees something at once, and exactly one terminal
`truncated: true` notification when the cap is hit. Chunks go out as a `goose/developer_shell_output`
custom notification on a **best-effort `try_send`** emitter, with the reason stated in the code: do
not let a slow notification consumer delay tool execution. The invariant is named by the regression
test — `full_live_notification_channel_does_not_change_final_output` — so a saturated or absent
display channel leaves the bytes entering the transcript **byte-identical**.

**What inber should consider:** `engine/display.go:12-18` has `OnToolCall`, `OnToolResult`,
`OnStatus` and `OnTextDelta` — a delta channel for *model text* but no mid-tool progress event — and
`tool-store/tools/shell.go:57` uses `cmd.CombinedOutput()`, which buffers to exit, so a ten-minute
build shows nothing until it finishes. Adding `OnToolProgress` is cheap; the two rules worth copying
verbatim are the best-effort send (a blocked UI consumer must never stall the tool) and a test
asserting the final tool result is unchanged, because the moment streaming and the transcript share
a buffer, display back-pressure starts silently editing what the model sees. This is the display
half of the shell/cancel contract written up in `cline.md` (07-31 §1), where the more urgent finding
is that inber's interrupt endpoint cannot actually stop a running command.

## Harness-watch — 2026-08-01: deny is the absorbing element in a multi-inspector merge, a path comparison must run on canonicalized paths, and a tool result is structured content rather than a string

> **[Verified 2026-08-01 — the tool-result entry holds, its own "latent" caveat included. One correction: it bundles two fixes that are not coupled.]**
> Confirmed: `agent/agent.go:30` types a tool `Run` as returning `(string, error)`, so a tool
> result is structurally a string. `tools/mcp` has zero non-test importers, so the media half is
> **latent** exactly as the entry says — that self-assessment is correct, unlike several older
> entries in this file.
> The drop happens one level earlier than described: `tools/mcp/client.go:394-399` decodes the
> result into an anonymous struct whose `Content` elements carry **only** `Type` and `Text`, so
> image/resource/audio blocks are discarded at `json.Unmarshal`, before the `Type == "text"`
> filter at `:405-410` ever runs. Same outcome, stricter mechanism.
> **Not coupled, and worth separating:** `isError` is ignored entirely (zero occurrences of
> `isError`/`IsError` in `tools/mcp/`), so an MCP tool **failure returns as ordinary output with a
> nil error** — a fail-silent. That is a ~5-line fix (decode the field, return an error or an
> error-marked result) and does not depend on what `Run`'s signature eventually becomes. Widening
> the tool-result type is the genuinely large cross-module change (every tool in inber plus the
> separate `tool-store` module) and is correctly deferred.
>
> **[2026-08-03 — the `isError` half is SHIPPED. The separation this block asked for was right.]**
> `CallTool` decodes `isError` and returns an error carrying the server's own text, which is the
> only description of what went wrong. Tests in `tools/mcp/tool_error_test.go` cover the failure,
> an ordinary result (the control), an explicit `isError:false`, and a failure with no content;
> the failure cases were sabotage-verified red. The adapter passes `(string, error)` straight to
> `agent.Tool.Run`, so the error reaches the model's tool_result as an error rather than as
> output. **The media half is untouched and still correctly deferred** — do not re-file it as a
> defect, and note the drop happens at `json.Unmarshal`, one level earlier than the `Type ==
> "text"` filter, as the correction above says.

Three from this window; the first two are written up cross-cuttingly in
`agentic-design-patterns.md` (2026-08-01, §1 and "Also in-window"), so only the goose-specific
detail is repeated here.

**[#10612](https://github.com/block/goose/pull/10612) — denied tool request precedence.**
`apply_inspection_results_to_permissions` merges verdicts from N independent inspectors (security
scanners, LLM judges, static rules) into `approved` / `needs_approval` / `denied`. On a
`RequireApproval` verdict it checked only whether the request was already in `needs_approval` before
pushing it there — never whether it was already in `denied` — so a request an earlier inspector had
**refused outright** was re-added to `needs_approval` and became reachable by a user click. The new
tests pin the real invariant, `denial_dominates_regardless_of_inspection_result_order`, over both
permutations. A merge implemented as a sequence of *"if not in bucket X, push to bucket Y"* is not a
lattice; it is a race between inspectors, and the winner is whoever ran last.

**[#10545](https://github.com/block/goose/pull/10545) — contain subdirectory hint discovery.**
`SubdirectoryHintTracker::load_new_hints` decided containment with `dir.starts_with(working_dir)` on
as-written paths, so a path reached through a symlink, a `..` segment or a differently-spelled prefix
either escaped containment or failed it spuriously — and the tracker then loaded (or refused to
load) `AGENTS.md`-style hint files from outside the workspace. Now both sides canonicalize first, an
uncanonicalizable working dir bails, and an uncanonicalizable entry is skipped rather than admitted.
The tests moved from hardcoded `/home/user/project` strings to real `TempDir`s — the old ones never
touched a filesystem, which is why they could not have caught it.

**[#10340](https://github.com/block/goose/pull/10340) — forward images and MCP embedded-resource
blobs.** Anthropic/Google message formatting flattened every MCP tool result to a joined text
string, keeping only `as_text()` blocks, so any `ContentBlock::Image` and any `BlobResourceContents`
(a screenshot, a PDF) was **silently dropped** — a screenshot tool returned nothing to the model.
The fix emits typed `image`/`document` blocks with `source: {type: base64, media_type, data}` when
media is present and keeps the plain-string form when it is not, so no existing behaviour shifts.
The guardrail is a hard allowlist (`image/jpeg|png|gif|webp` plus `application/pdf`); everything
else, `image/svg+xml` included, falls through to a `[Image: <mime>]` **text marker** rather than
being handed to a provider that will 400 the turn.

**What inber should consider:** the media half is latent, not live, and worth recording before it
becomes live. `agent.Tool.Run` returns `(string, error)`, so an inber tool result is *structurally* a
string and an image cannot be forwarded at all — and `tools/mcp/client.go:406-410` accumulates only
`content.Type == "text"`, dropping every image/resource/audio block, while never reading `isError`
so an MCP tool failure is reported to the model as a success. That package still has zero importers
outside itself, so this is a design constraint to settle **before** MCP is wired, not a defect today:
widening the tool-result type is a change to every tool in the tree, and doing it after MCP lands
means doing it under a deadline. Take the allowlist-with-text-fallback shape when it happens — a
pass-through hands the provider bytes it will reject, and a silent drop is what #10340 was fixing.

Two more, dismissed as inber-irrelevant but noted so the next sweep does not re-derive them:
[#10487](https://github.com/block/goose/pull/10487) (incremental streaming render; inber's
`OnTextDelta` is a pure pass-through at `engine/build_hooks.go:127-131`, no accumulate-and-rescan
buffer exists) and [#10409](https://github.com/block/goose/pull/10409) (`to_string_pretty` →
`to_string` on model-facing tool schemas — inber already uses plain `json.Marshal` on that path).
The principle in #10409 is still worth stating: **model-facing serialization has no human reader, so
pretty-printing there is a pure token tax**, and the regression test should pin losslessness rather
than a byte count.

## Harness-watch — 2026-08-02

**⚠️ The 2026-07-30 §1 entry above is STALE — both defects it names are fixed.** Verified against
the current tree, not inherited: `engine/lifecycle.go:169` now calls `e.queueVolatileNote(note)`
instead of `+=`-ing onto `e.Turn.VolatileContext`, and `engine/volatile_context.go:33-48` folds the
queue in *after* the prompt build (`engine/turn_prepare.go:107-108`), so the cross-zone note is no
longer clobbered; `engine/build.go:40` `takeVolatileContext()` clears the engine's copy
(`volatile_context.go:58-60`), so the thinking-signature retry at `engine/turn_execute.go:47-49`
cannot double-inject.

**That entry's clean bill of health, however, was half wrong**, and the correction is the substantive
find of this window. "inber has no timestamp in the prompt at all" is true of the *rendered* prompt
and false of the *inputs to prefix assembly* — a wall clock in memory-store's scorer
(`builder.go:368-374`), the turn counter (`engine/turn_context.go:8-39`) and the user's own message
text (`engine/turn_prompt.go:81`) jointly decide the order and membership of the memories that make
up inber's BP2-cached system prefix. Written up in full in `agentic-design-patterns.md` (2026-08-02
§1), because it generalizes past goose.

Two more from this window, both dismissed for inber but recorded so the next sweep does not
re-derive them:

- **[#10409](https://github.com/block/goose/pull/10409)** — already dismissed in the 2026-08-01 entry
  above, and the dismissal is confirmed at source: the change is one line in
  `crates/goose/src/providers/toolshim.rs:882` (`to_string_pretty` → `to_string`) on the path that
  renders schemas **into prompt text** for models lacking native tool calling. inber has no toolshim
  and stringifies no schema into prompt text; both wire paths marshal compactly via SDKs. The tree's
  only `json.MarshalIndent` of a schema is `session/prompts_write.go:145`, which writes a debug
  artifact on turn 1 and never reaches the wire.
- **[#10577](https://github.com/block/goose/pull/10577)** — fully covered by the 2026-07-30 §2 entry
  above (the 9× figure, the guardrail, and the correct prescription). Do not re-file.

What auditing #10409's question in inber *did* find is an inber-authored bloat about ten times
larger than the one goose fixed: `AddChainAndSidebandFields` grows the 9-tool registry block from
**6,566 B to 17,834 B (+172%)**, exactly 1,252 B × 9 of byte-identical boilerplate. Detail and the
caching caveat are in `agentic-design-patterns.md` (2026-08-02, "Also in-window"). goose's placement
lesson applies one layer higher than the 07-30 entry recommends: `AddChainAndSidebandFields` rebuilds
the same maps on **every request**, from both `agent/agent_run.go:26` and `engine/turn_openai.go:32`,
where goose normalizes once at registration.

## Harness-watch — 2026-08-03: goose reverted the chunker this doc told inber to copy — a max over N windows is a false-positive multiplier; and a tool you disabled comes back when the session forks

### 1. ⚠️ The 2026-07-16 entry's prescription is withdrawn upstream. The threat model stands; the aggregation does not.

[goose #10870](https://github.com/block/goose/pull/10870) (merged 2026-08-01) is a clean
`git revert` of [#10416](https://github.com/block/goose/pull/10416) — the overlapping-window
command-classifier chunker written up at `goose.md:540` above and recommended there as a shape to
"adopt directly". It deletes `command_chunker.rs` (156 lines) and restores
`classification_client.rs`, `scanner.rs` and `security/mod.rs` to their pre-#10416 state. The
stated reason, verbatim: *"The overlapping-window chunking of command-classifier input has
significantly increased false positives for large commands, so we're reverting it while the
approach is reconsidered."* A follow-up is promised that chunks "in a different way that's a
little less disruptive".

**What the 07-16 entry got right, and what it missed.** The evasion is real and unretracted — a
512-token window can be padded past, and the measured SAFE 0.0006 → INJECTION 0.9997 flip stands.
The general lesson stands too: a fixed-context safety check that sees only the first N bytes is a
check an attacker pads around. What does not stand is **max-over-windows as the aggregation**.
Taking the maximum score across N windows is a monotone OR over N noisy detectors: if one window
false-positives with probability p, an N-window command false-positives with 1−(1−p)^N. The
screen's false-positive rate therefore *rises with input length*, and overlap makes N larger than a
naive split would for the same bytes — so the change widened the false-positive surface fastest on
exactly the inputs (large commands) it was added to protect. The 07-16 entry recorded the
true-positive demo and the three fail-closed details and never asked what the aggregation does to
the other error. That is the whole of the correction.

**What inber should consider:**

- **Keep the threat model, drop the aggregation.** If inber ever adds a classifier or
  LLM-screening gate ahead of shell, scanning the whole input is still required, but the
  per-window threshold has to be calibrated for N — raised as N grows so the *whole-command*
  false-positive rate stays fixed — or a single window's score must not be sufficient on its own.
  A screen whose deny rate grows with command length is an availability failure, and in an
  unattended session a false deny is a stalled job, not a prompt.
- **A safety gate needs both error rates measured before it ships.** goose merged this on a
  reproduced evasion and reverted it 16 days later on false positives; that sequence is what
  measuring only the true-positive side produces.
- **Correction to the 07-16 homework as well.** That entry asked inber to "confirm those matches
  scan the *whole* argument, not a truncated head". There is nothing in this repo to confirm:
  `guard/` contains no `regexp`, no `strings.Contains` and no `HasPrefix` on a command string, and
  the classification is a switch over **tool names** — `isReadOnly` at `guard/guard.go:319-326`,
  `isDangerous` at `:328-334`. inber's gate never looks at the command bytes at all, so it is
  neither padding-bypassable nor length-sensitive; the pattern screen the 07-16 entry was aiming
  at lives in bridge-server's `permission_store`, outside this repo. That makes this whole section
  advice for a gate inber has not built, and the name-only classification is the more pressing
  fact about the gate it has (see the open todo on the six unclassified tools).

### 2. A disabled tool is a session-lifetime fact in inber, and a session's life is shorter than the operator thinks

[goose #10223](https://github.com/block/goose/pull/10223) fixes a user disabling the **Developer**
extension — toggle off in settings, `enabled: false` in `config.yaml` — and still getting `shell`,
`edit`, `write`, `tree` and `read_image` loaded into every new chat, with shell commands executing.
The fix is three lines: skip a builtin when the config explicitly disables it. The sentence in the
PR body is the reusable part — *"Any mitigation built on disabling Developer is silently void."*

inber's version of the question is not "does config-load honour the flag", because **inber has no
config field for it at all**. `disabled_tools` reaches an engine through exactly one door:
`POST /sessions/{id}/config` (`server/api_bridge.go:671`, handled at `:721-722`) calls
`Engine.SetDisabledTools` (`engine/engine.go:363-370`), which stores the set in
`engine.disabledToolNames` (`engine/engine.go:94`) and re-derives the wire set at
`applyDisabledTools` (`engine/engine.go:427`). Those five lines are the complete set of references
to that field in the tree. It is not on `EngineConfig`, not on `RunRequest`, not on the stored
agent config, and nothing serializes it. So the set is engine-memory state with a session's
lifetime, and two ordinary events end that lifetime while the operator believes the tool is still
off:

- **A fork or a spawn.** `server/session_forking.go:47` and `server/spawn.go:224` both build the
  child through `createSession(..., RunRequest{}, ...)`, so the child engine starts with an empty
  `disabledToolNames` and gets the parent's full tool set. `spawn_agent` is in every session's tool
  set (`server/agent_tools.go:10-13`), which means a session that has had a tool taken away can
  reach it again by spawning a child.
- **A restart.** Nothing persists the set, so a revived session comes back with everything enabled.

**What a fix would have to decide, and this entry does not.** Whether `disabled_tools` is a *safety
boundary* or a *context/behaviour knob*. The `ConfigRequest` doc comment (`server/api_bridge.go:656-667`)
argues only about nil-versus-empty and never claims the former; if it is only a knob, losing it on
fork is a UX bug and persisting it is enough. If it is a boundary, then inheritance has to be
decided too — a child that inherits the parent's denials is the safe default and also the one that
makes a delegate less capable than its parent for reasons the delegate cannot see — and it has to
be reconciled with the `mode`/guard path rather than bolted beside it. Related but distinct: the
open todo on the three zero-`RunRequest` call sites covers fields that *exist* on `RunRequest` and
are dropped; this is a setting that has no field to drop.

### Also in-window, worth a line each

- **codex [#36641](https://github.com/openai/codex/pull/36641)** parses a provider-only usage field
  (`codex_rollout_budget_units`) into `TokenUsage` and deliberately keeps it out of the serialized
  protocol, JSON schema and TypeScript types — a provider's private accounting is recorded where the
  cost model can read it and never promoted to a contract other code may depend on. Nothing to do in
  inber: the four Anthropic counters are already logged and priced (`session/timeline_cost.go`), and
  the arithmetic bug under them was closed by `09848b9`/`961f6dd`.
- **Negative results from this window's permission audit, so the next sweep does not re-derive them.**
  goose #10612's "allow beat deny" has **no analogue in inber**: there is no allow-list/deny-list pair
  to order, `CheckTool` (`guard/guard.go:165-188`) is one `switch` over the mode calling exactly one
  classifier per branch, and the classifiers are `switch`es over string literals
  (`guard/guard.go:319-334`) — no map iteration, nothing order-dependent. There is likewise **no deny
  cache to poison**: `CheckTool` is stateless w.r.t. verdicts, so there is no "always allow" entry a
  denial could be overwritten by. The one place two lists meet, `applyDisabledTools`
  (`engine/engine.go:427-444`), applies the deny filter after the allow list by code order, and a
  disabled tool is absent from `toolMap` (`agent/agent_run.go:44`) so it cannot execute even if the
  model emits it. Deny wins, structurally.
- **`guard/` is no longer a stub, and any note saying otherwise is stale.** Live callers on the hot
  path: `engine/build.go:105`, `engine/build_hooks.go:89-102`, `agent/chain.go:388` and `:437`,
  `agent/sideband.go:228`. `harness-control-matrix.md:35` already records the correction. Still dead
  *inside* the package: `RecordToolCall` (`guard/guard.go:209`) and `IsRepeating` (`:305`), which is
  the standing "repetition detector has never run" todo. Still genuine stubs: `trace`
  (`trace/trace.go:105,113`), `codeindex` (`codeindex/codeindex.go:50,58,68,77`), `checkpoint`
  (every method returns `ErrNotImplemented`).
- **Already filed, found again by this audit, recorded so it is not filed twice:** the Assist
  denylist failing open — `isDangerous` (`guard/guard.go:328-334`) names four tools, so every
  unclassified tool is allowed with no approval. The audit widened the count well past the six the
  open todo names: `scheduler`, `web_fetch`, `browser`, `task_plan`, `scratchpad`, `memory_save`,
  `memory_forget`, `end_turn` (`tools/tools.go:33-45`), plus `spawn_agent`, `steer_agent`,
  `agents_status` and the workspace tools injected into **every** server session
  (`server/agent_tools.go:10-31`), plus `sideband:done|note|split` (`agent/sideband.go:27-30`). Two
  of those reach a shell in Assist: `scheduler` (jobs the scheduler runs as `sh -c`) and
  `sideband:done` → `engine/build_sideband.go:44` `RunBuildCheck` → `bash -c`, the second while
  `agent/sideband.go:191-199` explicitly justifies gating the sideband *because* it reaches `bash -c`.
  That belongs on the existing todo as a count correction, not as a new one.

### Held back from the queue this pass, and why

The cache-stability audit run against [#10734](https://github.com/block/goose/pull/10734) turned up
seven findings; three slots were spent, so these are recorded here instead. **Findings 1–4 are one
defect and it is already filed** — the open todo "the BP2-cached system prefix is rebuilt from a
clock, a turn counter and the user's message text". What is new is that it was *measured* rather
than argued: a probe against a real memory-store DB varied only the user message across six turns
and the prefix hash changed **every time**, including one pair (turns 1 and 2) holding the identical
eleven memories at the identical token count and differing only in **order**. Three distinct inputs
drive it — `AutoTag(userMessage)` scoring at `memory-store/builder.go:397` (`+0.3` per matched tag,
larger than most importance spreads), the 4000–50000 budget ladder at `engine/turn_context.go:8-37`
feeding the cut-off at `builder.go:180`, and a wall clock at `builder.go:400-405` with 24h/7d cliffs,
which is goose #10734's own bug one layer up. Two feedback loops make it self-sustaining: memory
extraction after every turn (`engine/turn_postprocess.go:34-45`) writes a row with `last_accessed =
now` that takes the `+0.2` recency bonus and lands near the front, and `memory_expand` *reading* a
memory writes `importance*1.01` and `last_accessed = now` (`memory-store/access.go:8-13`), so the
model reorders its own cached prefix by reading.

Two more, verified and unfiled:

- **`LastStablePrefix` reuse cannot prevent a cache miss, and its comment says otherwise.**
  `engine/turn_prompt.go:200-220` hashes `stableTexts`, which at `:194` is built from *every* block,
  while `cacheIdx` is always `len-1` — so the hash covers 100% of the system content and the reuse
  branch fires only when the blocks are already byte-identical, which is exactly when rebuilding
  them would produce the same bytes anyway. The guard at `:208`
  (`cacheIdx+1 < len(systemBlocks)`) is unreachable for the same reason. The comment at `:200-201`
  claims it "guarantee[s] byte-identical prefix"; it guarantees nothing the code did not already
  have. Left unfiled because deleting it is decision-free only if nobody intends to fix finding 1 by
  narrowing `cacheIdx` — which is the obvious fix, and would make this mechanism start working.
- **`server/api_oneshot.go:82-112` sets no `cache_control` at all** and reports the counters at
  `:130-134`, where they are structurally always 0/0. Every one-shot call pays full price, including
  repeated classifier calls that share a system prompt and schema.

And one **ruled out**, recorded so it is not re-derived: tool definitions are *not* emitted from an
unsorted Go map on any live path — `tools/interface.go:60-76` sorts by name and says why,
`tools/tools.go:83-91` and `memory/tools.go:210-217` return fixed slices, and
`server/spawn_tools.go:61-73` uses `sortedAgentNames`. The unsorted-map pattern does exist at
`tools/mcp/adapter.go:75-86` and `tools/mcp/client.go:433-439`, but `MCPToolRegistry` still has no
caller outside `tools/mcp/`, so it is a trap for whoever wires MCP, not a cost today. Likewise
**no timestamp reaches a system block**: `sourceRef` is `sync.Once`-cached
(`engine/turn_prompt.go:22-51`) and fleet status, context injectors and volatile memories all route
to `e.Turn.VolatileContext` (`:128-152`), which is injected after BP2 and — checked against the
2026-08-02 correction — is written into the message array once, not recomputed on replay.

## Harness-watch — 2026-08-11: a thinking signature is bound to the *model*, not just the credential — and inber's failover swaps the model under a live conversation

[goose #10007](https://github.com/block/goose/pull/10007) fixes an unrecoverable Anthropic 400 —
``messages.N.content.M: `thinking` or `redacted_thinking` blocks in the latest assistant message
cannot be modified`` — with a cause this doc has not recorded before. A signature is issued by, and
valid only for, **the model that produced it**. Switch model or thinking-effort mid-conversation
(goose's case: two `set_config_option` calls) and the stored history still replays the *previous*
model's signed blocks against the new one, which Anthropic rejects direct and through Bedrock,
Vertex and Databricks. Observed in the wild at message index 3, so not a compaction artifact. The
fix is targeted rather than blunt: when serializing history, drop signed thinking blocks whose
originating model — read from `message.metadata.inference` — differs from the model this request
targets. Text and tool content still go. When provenance is unknown (older rows, no metadata) the
old behaviour stands, so single-model conversations are untouched.

The generalization: **credential rotation is not the only thing that invalidates a signature, and
provenance is what lets you drop the right blocks instead of all of them.** A harness that stores
history as a bare provider message array has no way to answer "which model wrote this?", so its only
available remedy is the sledgehammer.

**What inber should consider:** inber is the configuration this bug needs, and it produces the model
switch *by itself*. `engine/failover.go:21-60 selectModel` runs at the top of every turn
(`engine/turn_execute.go:18`) and, for any session that did not pass `--model`
(`modelExplicitlySet`, `:30-33`), silently swaps to a fallback the moment model-store health says the
preferred model is unhealthy — `meta-harness.md:108` already records inber's model as "selected per
turn". The history it carries into that new model holds the old one's signed thinking for any
session with extended thinking on.

⛔ **Correcting this doc's own 2026-07-13 entry while citing it.** That entry says
"`agent/clients.go:103` sends `interleaved-thinking-2025-05-14`". Two things are off, measured
this pass: the header is at **`agent/clients.go:119`**, and it is sent **only on the OAuth branch**
— `newAnthropicClient` splits on a `sk-ant-oat01-` key prefix (`:114`), and the API-key branch
(`:126-130`) sends `prompt-caching-2024-07-31` alone. So the cross-message duplicate cause that
entry describes, and the interleaving that spreads one turn's signed thinking across several
assistant messages, are **OAuth-only**. The signature-versus-model problem here is wider than that
branch — any thinking-enabled session produces signed blocks — but it is at its worst on OAuth,
where there are more of them per turn.

Three consequences, in order of how much they cost:

- **inber's guard will not fire.** The strip-and-retry at `engine/turn_execute.go:45-50` is gated on
  `apiutil.IsThinkingSignatureError`, which is `msg == "Error"` in full
  (`internal/apiutil/apiutil.go:6-13`). The 400 goose quotes is a long, specific sentence, not the
  word `Error`, so the predicate returns false and the turn dies. This half is already filed as todo
  `cf3b6b4c`; what is new is a second, automatic way to reach it that nobody chose.
- **The failure is then recorded against the wrong model.** `recordModelHealth`
  (`turn_execute.go:58`) marks the turn's outcome as evidence about `modelUsed` — the model inber
  just failed *over to*, which did nothing wrong. So one poisoned history marks the fallback
  unhealthy too, and the next turn fails over again, walking the chain down.
- **inber cannot implement goose's fix as written.** `Engine.Messages` is a bare
  `[]anthropic.MessageParam` (`engine/engine.go:67`); there is no per-message record of which model
  produced it, and `resp.Model` is never persisted at all (`harness-control-matrix.md:81` already
  says so, from the audit side). Provenance has to exist before a targeted drop can.

**What a fix would have to decide**, and this is a real choice rather than an oversight: whether
inber gains per-message model provenance (a wrapper type or a parallel array — it changes the type
that every conversation repair, prune and summarize function takes, so the blast radius is the whole
`conversation/` package), or whether failover simply refuses to swap models when the history carries
signed thinking, keeping the sledgehammer for the credential case it was written for. The second is
far cheaper and costs availability exactly when a model is down. Do not fold this into `cf3b6b4c`'s
dedupe work without saying which was chosen.

## Harness-watch — 2026-08-12: a path that cannot enforce a declared tool policy must refuse, and a durable transcript is a second sanitizer chokepoint

Three small permission/injection fixes landed this week whose *shapes* are worth naming even where
inber has no counterpart.

- [#11128 — enforce review check tool policy](https://github.com/block/goose/pull/11128). The legacy
  single-prompt review path *advertised* per-check tool allowlists but delegated through Summon
  **with the parent session's full developer toolset in Auto mode**, so repository-controlled check
  content could reach capabilities outside the declared policy. The fix refuses the run outright
  when a check declares a `tools` policy the path cannot enforce — including an *explicitly empty*
  list, which is the case a permissive reading would silently treat as "no restriction." The rule:
  **a path that cannot enforce a declared capability policy fails closed, it does not downgrade to
  the ambient toolset.**
  *Not a defect in inber* — subagent construction already has this shape:
  `agent/registry/registry.go:219-226` registers exactly `cfg.Tools` and returns an error on an
  unknown tool name rather than substituting an ambient set. Worth recording because open todo
  `83e084f8` ("an empty tool allowlist means ALL tools") is the *permissive* reading of the same
  empty-list case, one layer up, and #11128 is upstream evidence for which way to resolve it.
- [#10609 — sanitize nested tool responses](https://github.com/block/goose/pull/10609). Extends
  Unicode-tag sanitization to tool-response text, resources and errors — but the architecturally new
  half is that it also sanitizes the direct `Vec<MessageContentBlock>` deserialization used by
  **SQLite session reloads**, "preventing persisted history from bypassing the control." Ingress
  sanitization alone is insufficient once history is durable: the reload path is a second chokepoint,
  and content written before the sanitizer existed re-enters through it forever. This sharpens open
  todo `657601a9` (a pruning marker is plain text, so any tool output can forge one): whatever
  sentinel or block type that todo picks, the resume path needs the same check, not just the write
  path.
- [#10455 — match extension owners exactly](https://github.com/block/goose/pull/10455). Stored
  permissions were removed by namespace **prefix** match, so resetting extension `foo` also deleted
  `foobar`'s rules. The canonical "a prefix is not an identity" bug. No inber counterpart — guard
  state is per-session (`session/guard_state.go`), not a namespaced rule store — but it is the same
  failure the repo directive about joining on ids rather than names exists to prevent.

**What inber should consider:** nothing here is a live defect. The one concrete carry-over is to
resolve `83e084f8` in the fail-closed direction and to give `657601a9` a resume-path check, since
both now have upstream precedent rather than only a preference behind them.

## Harness-watch — 2026-08-14: a cache write is a bet that the prefix recurs — which qualifies this file's own advice about `/oneshot`

[goose #11179](https://github.com/block/goose/pull/11179) stamps `disable_prompt_cache` on the
fast-model config behind `complete_fast` — compaction, session naming, tool-pair summaries,
orchestrator routing — and every breakpoint site honours it: the Anthropic format skips the tools,
system and last-two-user-message breakpoints, the three `apply_chat_payload_breakpoints` callers
(Databricks-Claude, OpenRouter, LiteLLM) skip theirs, Bedrock skips `cachePoint`. These calls
summarize a payload that never recurs, so the entry written can never be read back and only bills
the 1.25× write rate. Measured against `api.anthropic.com`: the compaction request drops from 2
`cache_control` markers and 11,633 cache-creation tokens to 0 and 0, while the next main-loop
request keeps all four breakpoints and an 87.8% read share.

⚠️ **This qualifies the unfiled note at `:1017-1019` of this file.** That note records
`server/api_oneshot.go` setting no `cache_control` as a cost — *"Every one-shot call pays full
price, including repeated classifier calls that share a system prompt and schema."* The missing
precondition is recurrence: a write only pays off if the prefix comes back, and for a one-shot call
carrying a transcript it is a guaranteed 1.25× loss. Re-verified 2026-08-14 that inber does not have
goose's bug — `conversation/summary_generation.go:62-71`, `conversation/extract.go:81-87` and
`server/oneshot_schema.go:34-43` stamp nothing.

**What inber should consider:** if `/oneshot` ever gets caching, make it a declared field on
`OneShotRequest` — the caller asserts its prefix recurs — rather than a stamp the handler applies to
everything, since that endpoint serves both repeated classifier calls and transcript-bearing
one-shots. The same rule pins `generateSummary` closed: it embeds the whole conversation in a fresh
user prompt every call, so it can never earn a read. Full write-up, including how this bears on open
todo `8754300f` (the force-summary call that withholds tools yet still marks its breakpoints), in
`agentic-design-patterns.md` under 2026-08-14 §1.

## Harness-watch — 2026-08-15: the third use of `manages_own_context()` arrived, and goose stopped writing it as an `if`

`agentic-design-patterns.md:4676-4680` closed the 2026-08-14 sweep by fencing
[goose #11203](https://github.com/block/goose/pull/11203) with a standing instruction: the
`provider.manages_own_context()` flag "is becoming a real cross-cutting capability bit upstream …
**Watch for a third use.**" It arrived the next day.

[goose #11094](https://github.com/block/goose/pull/11094) makes `/clear` and `/compact` return an
error when the active provider owns the conversation, because "when a provider owns the
authoritative conversation context, Goose cannot clear or compact that context by changing its local
transcript." That is the third call site — after the compaction gate and the extension-spawn filter
— and it is where goose changed technique. The legacy agent gets an ordinary check, but the state
machine gets something else: it **does not add `CompactionOperation` at all** for a context-owning
provider, so proactive and reactive local compaction cannot run because the operation is not
installed, not because a flag was consulted. Unhandled slash commands then fall through to provider
inference, letting the provider implement `/clear` itself. [#11139](https://github.com/block/goose/pull/11139)
and [#11216](https://github.com/block/goose/pull/11216) are what made that possible: the unrolled
agent loop became a state machine generic over session and effect, with a `StateMachineRuntime`
abstracting session loading, effect persistence and usage accounting, and then moved into its own
`goose-agent` crate.

The general lesson is about capability predicates, not about context ownership. A predicate checked
at each site leaks — you find the missed site when a user reports orphaned processes, which is
literally how #11203 was found. A predicate that decides **which operations exist** cannot leak,
because there is no second site to forget.

**What inber should consider:** nothing to fix — inber has no provider that runs its own agent loop,
and its one predicate of this shape is clean. `e.Guard == nil` short-circuits the gate in
`buildToolRefusal` (`engine/build_hooks.go:89-92`), and it is wired at **both** dispatch sites
(`engine/build.go:105` for the Anthropic path, `engine/turn_openai.go:120` for the OpenAI one), so
the leak this pattern warns about has not happened. The carry-over is a shape to reach for the next
time a capability bit is added, and one honest caveat about applying it here: inber's turn is a
straight-line function (`engine/engine.go:245`) with its two semantic retries written as inline
`if err != nil` branches (`agent/agent_run.go:205-222`, `engine/turn_execute.go:44-50`). Getting
goose's guarantee would mean restructuring that loop, which is a far larger change than the
guarantee is currently worth — record the technique, do not start the refactor for it.

## Harness-watch — 2026-08-18: on an adaptive-thinking model, omitting `thinking` buys thinking — and inber only ever omits it

[goose #11177](https://github.com/aaif-goose/goose/pull/11177) sends `thinking: {"type":"disabled"}`
explicitly to every model the registry marks `thinking_mode = adaptive`, instead of leaving the
field out. The reason is a billing one and goose states it plainly: **"Claude 5 models run adaptive
thinking, billed as invisible output tokens, when the thinking field is omitted."** A user with
`GOOSE_THINKING_EFFORT=off` — or with the setting untouched — was paying for reasoning that
`thinking.display` then defaulted to hiding. Measured on `claude-opus-5` with thinking off: output
tokens fell from **157 to 53** once the disable was sent. Two model classes are carved out, and the
carve-outs are the substance: always-on models such as `claude-fable-5` **reject** an explicit
disable, and pre-adaptive models such as `claude-haiku-4-5` already read omission as disabled. So
the rule is not "always send disabled" — it is **the registry has to say which of the three
behaviours a model has, and the request builder has to read it.**

**inber has one branch where three are needed.** `agent/agent_run.go:82-88`:

```go
if a.thinkingBudget > 0 {
    params.Thinking = anthropic.ThinkingConfigParamUnion{
        OfEnabled: &anthropic.ThinkingConfigEnabledParam{BudgetTokens: a.thinkingBudget},
    }
}
```

There is no `else`. A budget of zero — the documented way to turn thinking off
(`agent/agent.go:176`, "Set to 0 to disable"), the default for every agent whose registry config
omits `thinking` (`agent/registry/registry.go:215`), and what `engine/build.go:85-87` passes on — omits
the field, which on an adaptive model means thinking **on** and billed. This is live rather than
hypothetical: **`claude-opus-5`, `claude-sonnet-5` and `claude-fable-5` are all `enabled: true` in
model-store today** (`GET :8155/api/models`, measured this sweep), and an agent's model is a plain
config string (`agent/registry/config.go`), so pointing one at `claude-opus-5` is a one-line edit.
inber's defaults are safe only by accident — `agent/models.go:164` and `engine/lifecycle.go:92` name
Sonnet 4-family ids, which are pre-adaptive. **Filed as a todo.**

**What inber should consider:** the fix is blocked on a decision that is not inber's to take alone,
and must not be taken by accident. inber reads model metadata from **model-store**
(`agent/models.go:52` `GetModelInfo` → `Store.ResolveModel`), and that record carries a context
window and two prices and **no thinking mode** — so there is nowhere honest to read the answer from
today. The two options are (a) add a thinking-mode field to model-store, the owning registry, and
have `agent_run.go` branch on it, or (b) hardcode a model-id list in inber, which this box's
single-source-of-truth directive forbids and which would silently mis-handle every model added
after the list was written. Whoever takes it also has to decide what a *zero* budget means on an
always-on model like `claude-fable-5`, where the explicit disable is rejected outright: refuse the
request, or send nothing and log that the setting cannot be honoured. Note the cost asymmetry that
makes this worth doing rather than watching — the failure is silent in both directions, since the
tokens are invisible in the response and show up only on the bill.

## Harness-watch — 2026-08-19: a progress event's stream identity comes from the call that started the work — inber substitutes the literal `"main"` for a session id the sender supplied

[goose #10772](https://github.com/block/goose/pull/10772) fixes `SummonClient` broadcasting every
subagent notification to every active stream: it opened one notification stream per outer tool call
but kept subscribers in a shared `Vec<mpsc::Sender>` and fanned out to all of them. The PR body
names the consequence exactly — **"Concurrent delegate calls therefore accumulated identical
activity and could resolve the same fallback subagent session in ACP clients."** The remedy replaces
the shared subscriber list with a per-task `NotificationSink` that is either `Buffer` (nobody
attached yet) or `Emitter` (bound to the one outer tool call). The principle worth keeping: **a
progress event's stream identity must come from the call that started the work, never from a shared
or default channel** — and a *fallback* identity is the dangerous case, because it silently merges
rather than failing.

**inber gets the spawn-forwarder half right and the bus half wrong.** The forwarder is correct and
deliberately so: `server/spawn.go:76-100` labels every forwarded event with `session_key: childKey`
and `parent_key`, and resolves the parent stream per event rather than once, so two concurrent
children of one parent do not merge. The bus path throws that identity away and substitutes a
constant.

`ChatInbound` carries the field — `../bus/messages/chat.go:23`,
`SessionID string \`json:"session_id,omitempty"\` // logical session for conversation continuity`.
`server/bus.go:84` is `sessionID := "main"` under the comment *"use "main" for now, spawns will get
their own"*, and `msg.SessionID` is never read anywhere in `server/`. That literal is then the
`session_id` on the processing ack (`bus.go:88`), on every delta through
`busDeltaFor(agent, sessionID, ev)` (`bus.go:132`), and on the terminal `done`. The `RunRequest`
built two lines below (`bus.go:94-99`) sets four fields and **no `SessionKey`** — so
`server/session_forking.go:237-238` resolves the empty key to `mainSessionKey(agentName)`. `msg.Model`
and `msg.Effort` are dropped in the same statement, though `RunRequest.Model` exists
(`server/server.go:219`) and `server/api_bridge.go:701-712` already knows how to parse effort.

Two failures fall out, and they are different. **Inbound, conversations merge**: two bus clients
addressing one agent under different `session_id`s — a Discord thread and the scheduler — are routed
into the same `agent:<name>:main` session and their turns interleave in one transcript. The sender
said which conversation it was in and inber discarded it, which is the single-source-of-truth rule
inverted: the authoritative field is populated and the code prefers a literal. **Outbound, streams
cannot be demultiplexed**: every delta published carries `session_id: "main"`, and `ChatDelta` has no
turn-scoped discriminator to recover it — `MessageID` (`server/bus_delta.go:26`) groups deltas into
one assistant bubble, not into a session. That is goose's "resolve the same fallback subagent
session", verbatim.

The spawn-result path repeats the constant and contains its own refutation.
`server/spawn_delivery.go:95` publishes the completion outbound as
`PublishOutbound(parent.AgentName, "main", summary)` and `:112` sets `sessionID := "main"` for the
idle-parent turn — while `:141`, in the same function, correctly passes `SessionKey: parentKey` to
`g.run`. So the turn executes on the right session and its entire delta stream is published under
the agent's *main* session id. With `MaxSpawnDepth` defaulting to 2 a parent is routinely itself a
spawn session, so this lands a child's progress in whatever real conversation that agent is having.
**Filed as a todo.**

**What inber should consider:** the mechanical fix is to carry `msg.SessionID` into
`RunRequest.SessionKey` and use the *resolved* session key as the bus `session_id` at all five sites,
but the decision underneath it is not mechanical and should not be made by whoever writes the patch.
(a) It is **wire-visible** — anything subscribed on `session_id == "main"` today stops matching, and
`si`, the dash chat surface and llm-bridge-adapter are all downstream. (b) It needs an answer for an
*absent* `SessionID`, and "fall back to main" is the behaviour that caused this; the alternative is
to mint a per-connection key so two anonymous senders still cannot merge. (c) `msg.Model` and
`msg.Effort` are the same bug with a smaller blast radius and should be decided in the same pass
rather than left as the next sweep's finding — note that a per-request model override arriving over
an unauthenticated bus is a capability question, not just a plumbing one.

### Also in-window, checked and not worth a finding

- **[goose #11216](https://github.com/block/goose/pull/11216)** (`goose-agent` crate) and
  **[#11139](https://github.com/block/goose/pull/11139)** (generic state machine) — the unrolled-loop
  argument is already this file's 2026-08-15 entry, including the caveat that inber's straight-line
  turn (`engine/engine.go:245`) would need restructuring. #11216 is a crate move on top of it.
- **[#11283](https://github.com/block/goose/pull/11283)** — adds `error.code` and an
  `n_prompt_tokens > n_ctx` check *on top of* the substring fallback rather than replacing it.
  Confirms the open todo about `agent/agent.go:23-32`; no new material.
- **[#11234](https://github.com/block/goose/pull/11234)**, **[#11125](https://github.com/block/goose/pull/11125)**,
  **[#11310](https://github.com/block/goose/pull/11310)** — the recipe/minijinja arc has no inber
  surface at all: no `recipe` concept and no `text/template` import in `engine/`, `agent/` or
  `server/`.
- **[#11263](https://github.com/block/goose/pull/11263)** (`annotations` on replayed `output_text`) —
  inber's OpenAI path is Chat Completions only (`agent/openai.go`, `openai_conversion.go`); there is
  no Responses-API replay to be unfaithful.
- **Counter-precedent worth recording, not a bug.** #10772's second half buffers subagent
  notifications until the matching `load` consumes them. That is the *opposite* of inber's
  documented choice at `server/spawn.go:44-50`, where a child's events are dropped when the parent
  has no current writer ("there is nobody to deliver to, and no queue that would hold the event
  until there is"). Both start from the same premise — the subagent outlives the tool call that
  started it — and reach different ends. inber's line is defensible because *results* are never lost
  (`deliverResult` injects or triggers a turn), only progress; but the comment states the drop as
  though no alternative existed, and one now ships upstream.

## Harness-watch — 2026-08-20: the model a compaction runs on and the client it runs through are two answers that must be produced together — inber takes them from two fields, and failover rewrites only one

[goose #11255](https://github.com/block/goose/pull/11255) moves compaction and tool-result
summarization off `GOOSE_FAST_MODEL` and onto the main session model, keeping the fast model for
session naming, tool-call labels and orchestrator routing. The reasoning is an argument about *which*
auxiliary calls deserve the discount: the compaction summary is the foundation for every token
generated after it, compaction runs rarely, it fires when the context is at its largest, and it was
already falling back to the main model on Anthropic — so the discount bought inconsistency and
summary-quality risk for almost no saving. [#11207](https://github.com/block/goose/pull/11207) lands
the other half: it makes the main-model fallback an explicit choice for every `complete_fast` caller
rather than an ambient default, and turns thinking off for labels, because reasoning-capable models
were spending tokens deliberating over short labels whose reasoning output was discarded. Together:
**every auxiliary LLM call declares its own model and its own thinking policy, and neither is
inherited by accident.**

**inber does not have goose's bug and has a worse one in the same place.** It has the knob and never
turns it: `conversation/summarize_config.go:9-10` declares `Model string // Model to use for
summarization (empty = same as agent)`, no caller anywhere assigns it, and `conversation/summarize.go:58-61`
therefore always falls through to the model the caller passed. Compaction runs on the session model,
which is the answer #11255 arrives at. The defect is where that session model comes from.
`engine/lifecycle.go:90-97` reads the model from `e.Model` and the client from `e.Client`. Those two
fields are written by different code at different times and nothing keeps them consistent. `e.Client`
is assigned once, at construction (`engine/engine_new.go:589`), and never again on any live path.
`e.Model` is rewritten at the top of every turn: `engine/turn_execute.go:22-23` is
`modelUsed := e.resolveModelClient(selected)` then `e.Model = modelUsed`, and `resolveModelClient`
(`engine/model_client.go:39-43`) installs the new provider's client into `e.modelClient` and leaves
`e.Client` alone. The engine's own comment at `engine/model_client.go:10-16` states the invariant this
violates — "the string and the installed client must describe the same thing or every one of those
readers is told something untrue" — and then names only `e.modelClient` as the installed client.
`e.Client` is a second installed client nobody updates, and compaction is its reader.

The nil-client half of this is **already filed** as todo `36de2cf9` (`Engine.Client` is nil for the
whole session on an OpenAI-compatible model; compaction panics, memory extraction panics into a
`recover`, the registry builds subagents on nil) and is not re-filed here. The half that todo does not
cover is the cross-provider failover, which needs no misconfiguration at all: a session that starts on
Anthropic has a non-nil `e.Client`, and `fallbackChain` (`engine/failover.go:63-77`) is
`modelStore.FailoverChain()` ordered by priority — measured live this sweep, that chain crosses from
`anthropic` into `openai` at `gpt-5` (priority 30) directly after the four Anthropic models at
10/15/20/25. After one health-driven failover, `e.Model` is `gpt-5` and `e.Client` is still the
Anthropic client, so every later compaction posts `Model: "gpt-5"` to `api.anthropic.com`. The call
site is `_ = e.summarizeIfNeeded(ctx)` (`engine/turn_prepare.go:66`), so the error is discarded and
the only trace is one `Log.Warn` at `engine/lifecycle.go:106`. **The session then never compacts again
for as long as it holds the failover model**, and context grows against pruning alone — the same
"permanently unresumable" shape #11204 fixed upstream, produced here by inber's own failover.

**What inber should consider:** give `SummarizeConversation` the client and the model together. **What
a fix must decide** is #11255's actual question, which inber has never answered: compaction is
Anthropic-only machinery invoked from a provider-neutral turn loop, so "which model compacts" and "can
this session compact at all" are the same question. Three answers cost differently — (a) make the
summarizer provider-neutral, so a session compacts through its own client; (b) keep it Anthropic-only
and give it a **declared** Anthropic model rather than inheriting `e.Model`, which is
`SummarizeConfig.Model`, the field that already exists and has never been set, and which #11255 argues
should be the *strong* model; (c) refuse to compact on a non-Anthropic session and say so, letting
context grow until pruning alone holds it. Whoever picks (b) is also picking a second billed model per
session and should say so.

One thing for the same pass, from #11207 rather than #11255: inber's two sidecar call sites build
`anthropic.MessageNewParams` by hand and set no thinking policy at all —
`conversation/summary_generation.go:62-71` and `conversation/extract.go:81-87` each set `Model`,
`MaxTokens` and `Messages` and nothing else. Per this file's 2026-08-18 entry, omitting the `thinking`
field on an adaptive model buys thinking and bills it as invisible output tokens, so both pay for
reasoning on a summary and on a memory-extraction JSON blob the moment either is pointed at an
adaptive model. #11207 is upstream precedent that the auxiliary call is exactly where this is worth
turning off explicitly.

## Harness-watch — 2026-08-20: bound the handoff artifact at the sink that costs, and decide the retry by which errors *cannot* be about the prompt

[goose #11204](https://github.com/block/goose/pull/11204) fixes an ACP fallback handoff that replayed
the entire prior conversation as one unbounded text block: long sessions produced a prompt the agent
rejected outright, and because the same memo was rebuilt on every restore, the session became
permanently unresumable. The bounding scheme is detailed — `min(context_limit × 0.30, 64k) −
current_prompt_tokens`, newest-first selection rendered chronologically with an explicit
`[N earlier messages omitted]` marker, the five newest tool exchanges kept intact and older responses
redacted, matched **by tool-call id so parallel and batched calls are protected individually rather
than by message position**, and selection working on request/response *units* so a tight budget cannot
orphan half an exchange.

The transferable half is sharper than any of that. From the PR: the budget is only an estimate of what
the agent will accept, so the bare prompt is retained as a fallback — if the first send with a memo
fails it is retried once without the memo, and **no error text is matched**; errors that cannot be
about the prompt keep the retry rather than spending it (an `AuthRequired` code, and the ACP server's
structured `credits_exhausted` reason). That is the inversion: **you cannot reliably recognize a
too-long-prompt error, so stop trying — retry the smaller thing by default and enumerate only the
errors that are provably about something else.** A carve-out list is short, stable and falsifiable; a
match list is long, provider-specific and silently incomplete.

**inber has the retry and gates it on precisely the match #11204 deleted.** `agent/agent_run.go:205-218`
prunes to half the context window and retries once, but only when `isContextLengthError(apiErr)` is
true, and that predicate (`agent/agent.go:23-32`) is four `strings.Contains` calls against
`"prompt is too long"`, `"context_length_exceeded"`, `"maximum context length"` and `"too many tokens"`.
Any provider that words it differently — and inber routes to openai, google, openrouter, ollama and a
catch-all (`agent/clients.go:92-100`) — gets no retry and a dead turn. This file's 2026-08-19 sweep
recorded [#11283](https://github.com/block/goose/pull/11283) against the same predicate and concluded
"no new material" because #11283 only added `error.code` *on top of* the substring fallback. #11204 is
the new material: it removes the match entirely and inverts the polarity of the list.

**And inber's one genuine handoff artifact is unbounded at the only sink where size costs anything.**
`server/spawn.go:322` sets `summary = result.Text` — the child agent's entire final assistant message.
That string is truncated at three sinks and not the fourth: `server/spawn.go:342` writes
`truncate(summary, 1000)` to the request row, `:406` sends `truncate(summary, 300)` on the event,
`server/spawn_delivery.go:212-215` cuts it to 500 before queueing it into the agent's *main* session —
and `server/spawn_delivery.go:54-67` interpolates `result.Summary` raw into the message injected into
the **parent's live conversation**, which is the one copy that enters a prompt and is re-sent on every
subsequent turn until compaction reaches it. The three bounded sinks are a log row, a notification and
a pending-message note; the unbounded one is context. The bound is inverted with respect to cost.

**What inber should consider:** two things, separable.

- Replace the `isContextLengthError` gate with #11204's polarity. **What a fix must decide:** inber's
  retry is *destructive* — `a.BeforeRequest(ctx, *messages, a.contextWindow/2)` halves the conversation
  and `*messages = pruned` assigns it back, so retrying on an error that was never about the prompt
  costs the user half their transcript. goose can retry freely because its fallback merely drops a memo
  it can rebuild. Adopting the inversion here requires either making the prune non-destructive on the
  retry path (build the shortened request without assigning it back until it succeeds) or accepting
  that a mis-triaged error destroys context instead of merely failing the turn. Do not port the
  polarity flip without picking one.
- Bound `deliverResult`'s injected message. **What a fix must decide:** whether the bound is a byte
  count like the other three — in which case say why 300, 500 and 1000 are three different numbers for
  one string — or whether the parent should get a pointer instead, since `saveSpawnToMemory`
  (`server/spawn_delivery.go:167-172`) already writes the full text to memory and `updateMainSession`
  (`:219`) already tells the model "Full details available via memory_search". The parent's own copy is
  the one place that promise is not kept.

## Harness-watch — 2026-08-20: the 2026-08-18 thinking-mode decision has an upstream answer — a default-off capability flag on the model record, gated on the flag and never on a provider name

This file's 2026-08-18 entry ends on an open decision it declined to take: inber omits the `thinking`
field whenever `thinkingBudget == 0` (`agent/agent_run.go:82-88`), which on an adaptive model means
thinking *on* and billed; the fix needs a per-model thinking mode; model-store's record carries a
context window and two prices and nothing else, so "there is nowhere honest to read the answer from
today"; and the two options were (a) add a field to model-store or (b) hardcode a model-id list, which
this box's single-source-of-truth directive forbids.

[goose #10439](https://github.com/block/goose/pull/10439) is the same problem solved, and it picks (a)
with a shape worth copying. goose was injecting `clear_thinking: false` — a Z.AI/GLM property, not part
of Anthropic's `thinking` schema — into every request built by the shared `anthropic` formatter, which
also serves Anthropic, minimax and custom Anthropic-compatible gateways, producing
`HTTP 400 thinking.enabled.clear_thinking: Extra inputs are not permitted`. The fix adds
`emit_clear_thinking: bool` to `DeclarativeProviderConfig`, **defaulting to false**, sets it `true` in
`zai.json` alone, and threads it into the formatter. The PR gives the reason in one line: gated on the
flag rather than a provider-name check, so custom providers are covered without enumeration. A name
check is a list that is wrong the moment the next provider appears; a default-off flag is wrong only
for a provider that forgot to opt in, which is the safe direction.

`agentic-design-patterns.md:1570` already records the 2026-06-30 precedent — a context-budget decision
moved out of a hardcoded model-name match and into a **model-record capability flag** — so this is the
second instance of the technique upstream and the first that lands on the exact field inber is missing.

**What inber should consider:** take option (a) with #10439's defaults — model-store's `Model` gains a
thinking-mode field it owns, and `agent/agent_run.go` branches on what it reads back. **What remains
undecided, and is not settled by #10439:** inber needs **three** states where goose needed two —
omit-means-disabled (pre-adaptive), omit-means-enabled (adaptive), and rejects-an-explicit-disable
(always-on) — so a boolean is not enough, and the default has to be the pre-adaptive one to leave
today's Sonnet-4-family defaults untouched. The third state still needs the answer the 08-18 entry
asked for: what a zero budget *means* on a model that will not accept a disable — refuse the request,
or send nothing and log that the setting cannot be honoured.

### Also in-window, checked and not worth a finding

- **[#10285](https://github.com/block/goose/pull/10285)** (canonicalize mangled tool names before
  permission inspection) — no inber counterpart, checked directly. goose's bug was two names for one
  call: `PermissionInspector` read the raw `tool_call.name` while `resolve_tool` recovered a mangled
  one, so a `never_allow` on `db__drop_table` missed `db.drop_table` and dispatch ran it. inber has one
  name: `agent/chain.go:301` receives `name`, hands that same string to the gate and to `toolMap[name]`,
  and nothing anywhere recovers, prefixes or rewrites a tool *name* (`toolid.Sanitize` at
  `agent/openai_conversion.go:222` rewrites tool **ids**). A mangled name fails the map lookup. The
  prefixed-MCP-tool evasion cannot happen today for the reason this file already records:
  `MCPToolRegistry` has no caller outside `tools/mcp/`.
- **[#11369](https://github.com/block/goose/pull/11369)** (a config migration must not widen a
  user-configured tool allowlist) — inber has no config-migration path that rewrites agent tool lists;
  `agent/registry/registry.go:219-226` registers `cfg.Tools` as written and errors on an unknown name.
- **[#11362](https://github.com/block/goose/pull/11362)** (fail closed on an invalid ACP permission
  mode) — the general rule is this file's 2026-08-12 entry (#11128). The specific shape does not
  transfer: inber's guard mode has a deliberate documented zero value (`guard/guard.go:42-55`, with the
  reasoning for why Observe must *not* be the zero value) and is set from a parsed request field, not
  from a config file that can be malformed.
- **[#11367](https://github.com/block/goose/pull/11367)** (contain `REVIEW.md` discovery) — same fix as
  #10545, covered 2026-08-01 here and at `agentic-design-patterns.md:2466-2468`.
- **[#11191](https://github.com/block/goose/pull/11191)** (a model-controlled memory category becomes a
  filesystem path) — inber's memory is SQLite through memory-store; no `filepath.Join` in `memory/` or
  `tools/` takes a model-supplied component.
- **[#11228](https://github.com/block/goose/pull/11228)**, **[#11365](https://github.com/block/goose/pull/11365)**
  — renderer-driven recursive directory scans and buffered retry-command stdout. No Electron renderer,
  no retry-command feature.
- **[#10364](https://github.com/block/goose/pull/10364)** (stdio MCP children inheriting
  `PR_SET_PDEATHSIG` from a short-lived worker) — real and Linux-specific, but inber spawns no
  long-lived stdio MCP children on any wired path.
- **[#11339](https://github.com/block/goose/pull/11339)**, **[#11375](https://github.com/block/goose/pull/11375)**,
  **[#10990](https://github.com/block/goose/pull/10990)**, **[#11233](https://github.com/block/goose/pull/11233)**,
  **[#9650](https://github.com/block/goose/pull/9650)** — per-session extension merge, ACP audience
  annotations, Pi transcript import, two builtin skill files, and `GOOSE_SUBAGENT_*` precedence over
  LLM-injected delegate params. No ACP surface, no importer, no skills registry, and inber's spawn takes
  no model or provider from the model at all (`server/spawn_tools.go`), so there is no precedence order
  to get wrong.
- **[#11112](https://github.com/block/goose/pull/11112)** (`working_dir` in Stop hook context) — inber's
  hooks are in-process Go closures (`agent/agent.go:43-60`) taking typed arguments, so a missing field
  is a compile error rather than a silent allow.
- **[#10354](https://github.com/block/goose/pull/10354)** (`thoughtsTokenCount` folded into
  `output_tokens` for Google) — one line only: inber's Google traffic goes through the OpenAI-compatible
  path (`agent/clients.go:92-95`), which reads `completion_tokens` and already includes reasoning. inber
  does not have this bug and would acquire it if a native Gemini client is ever added.

## Harness-watch — 2026-08-21: a fail-open gate records the same thing whether it ran or not — and inber's post-tool hook is never reached by a failed call, so the entry it was supposed to delete is still there

### 1. `policy_evaluated` — "nothing decided, so allow" is not "policy ran and said allow"

[goose #11120](https://github.com/block/goose/pull/11120) adds `PreToolUseResult`, an observation-only
event emitted after the PreToolUse chain resolves and before the call executes or is denied. It carries
the final decision **and a `policy_evaluated` bit**, true only *"when at least one matching `PreToolUse`
hook exited 0 or returned a decision"* — a hook that exits non-zero without a decision, fails to spawn,
or times out does not count. goose's hook execution is fail-open, so until this bit existed the record
could not tell an allow that a policy considered from an allow that happened because nothing ran. The
PR also pins a `tool_call_id` stable across `PreToolUse` / `PreToolUseResult` / `PostToolUse` /
`PostToolUseFailure`, because *"tool name plus input"* cannot correlate a call that repeats. This is the
upstream implementation of the ask already recorded at `agentic-design-patterns.md:1490(b)` — persist
`decision_source` on the approval event — one level finer, because it also records whether anything
decided at all.

**What inber should consider:** `guard.CheckTool` (`guard/guard.go:165`) returns `Allowed` from its
`default:` arm for both Autonomous **and** Unset — the mode nobody named — and the engine records
nothing about which arm answered. A session whose mode failed to parse and a session deliberately run
autonomous produce byte-identical turn logs. The cheap version is not a new event: it is one field on
whatever the guard's caller already writes, distinguishing "Autonomous allowed this" from "no mode was
set, so the default allowed this".

### 2. One knob doing two jobs — and inber's is the transcript on disk

[goose #11391](https://github.com/block/goose/pull/11391) splits a read limit that had been serving two
purposes. Its predecessor bounded confined source-file reads by reusing `max_tool_response_size()` — the
user-facing knob for how much tool output the *model* should see — so lowering a presentation budget
stopped legitimate recipes loading, and raising it silently widened an allocation ceiling. The fix is a
typed two-armed `ReadLimit { Characters(n), Bytes(n) }`: characters for tool responses, because that is
what a context budget actually measures, and an independent `MAX_SOURCE_FILE_BYTES` for trusted source
files.

inber has the same conflation on a different pair. `session/truncate.go`'s `TruncateConfig` is read by
**both** `Session.TruncateToolResult` (`session/session_logging.go:64`), which is wired to
`hooks.ModifyToolResult` and therefore decides what the *model* sees, and `Session.LogToolResult`
(`session/session_logging.go:77`), which decides what the *session JSONL on disk* records. One config,
`s.truncateCfg`, and at the `main`/`agent` role that is a 1000-token threshold cut to 500 head + 200 tail
(`session/truncate.go:145-150`), so a 4 KB tool result is cut to ~2.8 KB in the transcript as well as in
the context. The file's own comment (`session/session_logging.go:84-95`) argues carefully against keeping
an in-memory copy of the untruncated output — a good argument, and a different question from whether the
*append-only log* should hold it. The full bytes exist nowhere afterwards.

**And the knob has a second problem, found while checking the first: three of its four settings are
unreachable.** `TruncateConfigForRole(role string)` (`session/truncate.go:141`) switches on `"main"`,
`"agent"`, `"project"`, `"run"` — but its one production caller passes an **agent name**, not a role:
`sessionMod.TruncateConfigForRole(e.AgentName)` (`engine/engine_new.go:600`). The ten agents configured
on this host are `bran`, `fionn`, `logstack`, `oisin`, `orchestrator`, `party`, `researcher`, `scathach`,
`task-manager`, `worker`. None of them is spelled `main`, `agent`, `project` or `run`, so **every session
on this host takes the `default:` arm** and gets `DefaultTruncateConfig()` — 1000/500/200. The `project`
config (3000/1500/500, *"needs more context"*) and the `run` config (5000/2000/1000, *"expects large
output"*) have never been selected by anything. `worker` and `researcher`, the two agents whose whole job
is producing large tool output, are truncated exactly as aggressively as the orchestrator. This is the
shape `session/truncate.go:20-27` already complains about in its own words — *"A config flag an operator
can read and set, wired to nothing, is a promise the code does not keep"* — one file over from where that
sentence is written. Not filed as a todo: the queue cap for this sweep was reached, and the fix is
entangled with the decision below.

**What inber should consider:** #11391's shape — two named limits, not one. **What a fix must decide:**
whether the JSONL is a context artifact or an audit artifact. If resume is its only reader, today's
behaviour is correct and the log should say so; if a human or a later analysis reads it, the context
budget has no business truncating it. That is a real choice about what the file is for, and it should be
made deliberately rather than inherited from a shared struct field.

### 3. A normalization rule implemented on one of two ingestion paths

[goose #11317](https://github.com/block/goose/pull/11317) is worth reading for its honesty about
severity. `Conversation::push` already coalesced consecutive thinking deltas by chunk id; `collect_stream`
— *"the default path every `Provider::complete()` call goes through"* — only had a coalescing arm for
`Text`, so `Thinking` deltas fell to the wildcard arm and persisted one content block per streamed chunk.
The PR states plainly that this is *"a storage/serialization defect, not a correctness or token-cost
one"*, and then gives the number that makes it matter anyway: **12,632 fragmented content items collapsed
to 249, a 50x reduction in `content_json` bytes.** The merge rule is the reusable part — append while the
previous block is unsigned or shares the incoming signature, and **never merge two distinctly-signed
blocks**, which is what keeps #10007's thinking-signature provenance (recorded at `goose.md:1032-1050`)
working.

**What inber should consider:** the transferable claim is not about thinking blocks, it is that a
normalization implemented at one of two ingestion points holds only for the path someone remembered.
inber has exactly that shape in `agent/agent_run.go`'s streamed accumulation versus `session/resume.go`'s
replay, and the sweep below found a live instance of the same shape in the tool-hook pair.

## Harness-watch — 2026-08-31: the prompt-cache TTL became a setting, and the half worth copying is the clamp that takes it away again

[goose #11576](https://github.com/block/goose/pull/11576) adds `GOOSE_CACHE_TTL`, accepting
`5m` or `1h` and rejecting anything else, stamping `{"type":"ephemeral","ttl":"1h"}` at all four
Anthropic breakpoints — system prompt, last tool spec, last two user messages. The field is
**omitted entirely when unconfigured**, so the default request stays byte-identical to what it
was before the feature existed. goose's motivating number: a 58k-token session idle for 7
minutes cost **$0.36** re-writing its prefix against **~$0.03** to read it back.

The reusable part is not the setting. Even for a user who opts into `1h`, three surfaces are
clamped back to `5m`: `goose run`, subagents (`TaskConfig::new`), and scheduled recipes, because
they "finish in one burst and cannot recover the 2× write premium of 1h cache". That completes
the rule this file wrote on 2026-08-14 — *a cache write is a bet that the prefix recurs*. The
missing clause: **a longer TTL is a bigger stake on the same bet, so the surfaces least likely
to win it are exactly the ones that must not be allowed to raise.**

inber takes the 5-minute default everywhere — `agent/agent.go:549`, `agent/agent_run.go:36`,
`engine/turn_prompt.go:218` and `:224` all stamp a bare
`anthropic.NewCacheControlEphemeralParam()`.

**Measured on this host before recommending anything** (full table in
`agentic-design-patterns.md` under 2026-08-31 §4): across the 165 consecutive request pairs in
`~/.inber/server/server.db`, the cache read:write ratio falls **3.73 → 2.02 → 1.09** across the
under-5m, 5m-to-1h and over-1h gap bands, and **39 pairs (24%) sit in the middle band** — where
a 5m entry has expired and a 1h entry would still be live. Caveat that bounds it: 99 of the 100
sessions hold a single request, so all 165 pairs come from `agent:claxon:main`.

**What inber should consider:** if the TTL becomes settable, copy the clamp as well as the
setting — but derive inber's clamp list from inber's own data, because it is not goose's. goose
forces subagents to 5m; inber's sub-agent requests carry **7,987,841 cache reads against
610,797 writes, a 13.1 ratio, the best in the table**, because one inber `requests` row is a
whole multi-call turn rather than one exchange. The surfaces here that genuinely cannot win the
bet are the ones the 2026-08-14 entry already named and re-confirmed stamp nothing —
`conversation/summary_generation.go:62-71`, `conversation/extract.go:81-87`,
`server/oneshot_schema.go:34-43`.

Also this window: [#11673](https://github.com/block/goose/pull/11673) adds `--system` to
`goose session` for parity with `goose run`. No inber surface.

## Harness-watch — 2026-09-02: the usage-bearing path nobody instrumented is the error path

Three items this window, and the first corrects the premise this sweep started from.

### 1. #11636 is not the resume fix it looked like

[goose #11636](https://github.com/block/goose/pull/11636) preserves thinking and
redacted-thinking blocks "across turns", which reads like inber's open resume gap. It is
not. It is an **outbound-conversion** fix at the Kotlin/Python binding boundary: thinking
blocks reached callers only as stream chunks, so a caller assembling assistant history from
a `ProviderCompletion` had nothing to replay. It adds `MessageContent::from_goose_content`
and a `content: Vec<MessageContent>` field. The transferable part is the invariant —
**signatures and redacted data are copied verbatim and never decoded**.

Thinking-block preservation in inber, measured this pass rather than assumed:

| Path | File:line | Preserved? |
|---|---|---|
| Stream accumulation | `agent/agent_run.go:175` `accumulated.Accumulate(event)` | **Yes** — SDK handles `SignatureDelta` |
| Into history | `agent/agent.go:412` `append(*messages, resp.ToParam())` | **Yes**, verbatim. inber constructs a `ThinkingBlockParam` **nowhere**, so it can never fabricate a signature |
| Persist | `engine/lifecycle.go:260` → `messages.json` | **Yes** — round-tripped against SDK v1.35.0, signature intact and byte-identical |
| CLI resume | `engine/engine_new.go:164,176-180` | **Yes** — that repair chain omits `RepairThinkingSignatures` |
| Server resume | `server/session_creation.go:151` | **No** — unconditional strip. Todo `cf3b6b4c` |
| Stream-error salvage | `agent/agent.go:397-402` | **No** — rebuilt from `deliveredText`, text blocks only. Lossy but safe; no orphan signature |

**What inber should consider:** nothing new from #11636 itself. The table is the point — the
strip is one line on one path, not a representational gap, and the surrounding pipeline is
already correct.

### 2. A failed API call's tokens are never counted — the 2026-07-30 fix landed on the text half only

`agent/agent.go:391-409`: when `executeAPICall` returns an error, the branch salvages
`deliveredText(resp)` into history and **returns at `:405`**. `processResponse`
(`agent/agent_run.go:263-299`) is the only writer of `result.InputTokens`,
`CacheCreationTokens`, `CacheReadTokens` and `result.APICalls`, and it sits after that
return, so it is unreachable on any failed call.

The tokens are real and in hand: `executeAPICall` sets `partial = &accumulated`
(`agent_run.go:195`) and the SDK's `MessageStartEvent` handling means `partial.Usage`
already holds the input and cache-write counts — the prompt was processed and the 1.25×
cache write was already paid before the stream died.

**Cost:** every mid-stream failure is free in inber's books — session total, `requests` row,
and `buildLimitCheck` (`engine/build_hooks.go:41`), which compares `MaxInputTokens` against
`e.Tokens.Input + result.InputTokens`. A flapping connection can retry indefinitely without
the input-token cap ever binding.

⚠️ **`docs/comparisons/claude-code.md:259` filed this on 2026-07-30 as one defect with two
halves.** The history half shipped — `partial`, `deliveredText` and
`incompleteResponseNotice` all exist now — and the usage half did not. That entry still
describes `resp` as staying `nil`, which is stale; read it against this.

**Distinct from open todo `5a565d77`**, which is the *engine* gate discarding spend already
recorded from earlier round trips. This is the layer below: the failing call's own tokens
were never written into `result` at all, so fixing the gate alone still loses them.

**What a fix must decide:** whether a failed call's usage counts against the session's
*budget* or only its *bill*. Not the same question — a mid-stream failure that is retried
and succeeds bills twice for one logical step, so charging both to `MaxInputTokens` makes a
flaky network eat a budget that produced no work, while charging neither is what ships
today. Also open: whether `result.APICalls` gains a failed entry, given it feeds
`ToolsWithheld` cost analysis, which assumes every element is a completed call.

### 3. Credentials from a command, with proactive and 401-reactive refresh

[goose #11657](https://github.com/block/goose/pull/11657) lets a provider's `auth` block name
a command, a refresh interval and a timeout, re-running it on the interval and immediately on
HTTP 401, "so short-lived credentials refresh automatically instead of requiring a restart".

inber resolves once and holds forever. `agent/clients.go:46-51` calls
`auth.ResolveKey(model.Provider)` and hands the string to `newClientFromKey`;
`engine/model_client.go:34-36` returns early when the selected model is unchanged, so on the
steady-state path `ResolveKey` never runs again. `createModelClient` has two callers, both at
engine construction, and `server/session_creation.go:24` serves cached sessions without
rebuilding an engine. Grepping `agent/` and `engine/` for `401`, `StatusUnauthorized` or
`refresh` returns **nothing**.

**Cost:** `mc.IsOAuth` (`agent/clients.go:89`) is set for `sk-ant-oat01-` tokens, which are
short-lived. A long-running server session outlives its access token and from that moment
every turn 401s with no refresh and no reclassification; the only recovery is tearing the
session down. `MaxDuration` sessions and the bus chat surface are exactly that case.
Distinct from `agentic-design-patterns.md:2990-2996`, which is about the *first* resolution
swallowing its error — this is about there never being a second.

**What a fix must decide:** who owns the refresh. auth-store already does server-side token
refresh and is the source of truth, which argues inber should re-resolve on a trigger (401,
or an interval) rather than caching a string; owning it in inber duplicates logic the vault
already has. Separately: whether a 401 invalidates the client and retries the same turn, or
fails the turn and lets the next one rebuild.

### 4. The JSONL resume reconstruction has no production caller

`session/resume.go:30 LoadMessages` and `:179 LoadMessagesFromDir` are called from nothing
but tests — no importer of `inber/session` anywhere under `~/repos`. The real resume is
`(*Workspace).LoadMessages` (`session/workspace.go:102`), which reads `messages.json`
directly with **no JSONL fallback**, and its caller `engine/engine_new.go:164-175` hard-errors
rather than degrading.

⚠️ This corrects a claim filed here on 2026-09-01. `agentic-design-patterns.md:9077-9083`
says `session.Entry` having no signature field means "when that snapshot is missing or
corrupt the reconstruction silently drops the reasoning". There is no such reconstruction in
production. The representational gap in `session.Entry` (`session/session.go:21-37`) is real;
its stated consequence is not, and the comment at `engine/lifecycle.go:251` cites
`LoadMessagesFromDir` as though it were live.

**What a fix must decide:** whether `session.jsonl` is a resume source at all. If it is,
`LoadMessagesFromDir` needs a caller and `Entry` needs a signature field. If it is not —
which is what the code does today — both functions should go, and `lifecycle.go:251` needs a
different justification for writing the snapshot.

### Also checked this window, nothing to import

- **[#11685](https://github.com/block/goose/pull/11685)** makes tool approval a
  suspend/resume in the state machine: the decision is persisted into conversation history
  *before* the machine resumes, and the machine is recreated over the same session, so
  approval survives disconnects and reloads. inber's `guard.Config.ApprovalFunc`
  (`guard/guard.go:90`) is a synchronous `func(tool, input) bool` — a shape that can only
  block a goroutine. Nothing sets it (`engine/build_hooks.go:85-88`), so this is the shape to
  copy *if* Assist mode is ever wired, not a defect today.
- **[#11492](https://github.com/block/goose/pull/11492)** replaces regex extraction of shell
  commands from model text with AST parsing, closing seven evasions. Good rule — *parse,
  don't pattern-match, when the parse result is a security decision* — but inber has no
  toolshim; tool calls arrive as structured `tool_use` blocks.
- **[#11637](https://github.com/block/goose/pull/11637)**, **[#11750](https://github.com/block/goose/pull/11750)** —
  already dismissed at `agentic-design-patterns.md:9001-9008`; and inber's OpenAI path does
  not stream (`OpenAIRequest.Stream` is declared and never set).
- **[#11628](https://github.com/block/goose/pull/11628)**, **[#11472](https://github.com/block/goose/pull/11472)** —
  covered at `agentic-design-patterns.md:3364-3395`, owned by todo `0d052752`.
- **[#11602](https://github.com/block/goose/pull/11602)**, **[#11275](https://github.com/block/goose/pull/11275)**,
  **[#11440](https://github.com/block/goose/pull/11440)**, **[#11117](https://github.com/block/goose/pull/11117)**,
  **[#11629](https://github.com/block/goose/pull/11629)**, **[#11630](https://github.com/block/goose/pull/11630)**,
  **[#11632](https://github.com/block/goose/pull/11632)** — verified no gap, no surface, or
  already recorded. `prepareTools` (`agent/agent_run.go:18-50`) builds `params` and `toolMap`
  from one index-aligned pass, so a dispatch mismatch cannot arise.

## Harness-watch — 2026-09-03 (#11429, #11469): a cache breakpoint that does not move is a breakpoint the growing end of the turn never reaches

Ten commits screened from the 2026-08-27 → 2026-09-03 window. Eight are already
covered here or in `agentic-design-patterns.md` — the tool-approval state machine
(#11685, `goose.md:1761`), thinking-block preservation (#11636, `:1653-1677`),
permission revocations across managers (#11383, the disabled-tool-on-fork twin at
`:915-949`), the execute-shell and command-scanner hardening (#11492 `:1768`,
#11440 `:1778`), and the two streaming tool-call fixes (#11750, #11637) dismissed
at `agentic-design-patterns.md:9001-9008`. Re-verified rather than assumed on the
last two: `OpenAIRequest.Stream` (`agent/openai_types.go:46`) is declared and
assigned nowhere in the tree, so inber has no OpenAI streaming accumulator to get
the tool-name delta wrong; and the SDK's `Accumulate`
(`anthropic-sdk-go@v1.35.0/messageutil.go:32-64`) ignores `event.Index` and
always writes `acc.Content[len-1]`, which is safe under Anthropic's sequential-
block guarantee and is not inber's code to key wrongly.

Two carry something.

### 1. #11429 — anchor the second breakpoint on the tail, measured in blocks

[`5f642327`](https://github.com/block/goose/commit/5f642327) — *fix(cache):
anchor chat-payload breakpoints on the agentic tail*.

goose anchored its two chat-payload breakpoints on the last two `user` messages.
On an OpenAI-shaped envelope a tool result is `role:"tool"` and a tool call rides
on `role:"assistant"`, so both breakpoints collapsed onto the last human turn and
the whole agentic tail was re-billed on every iteration of the tool loop. The fix
anchors the primary on the last cacheable message whatever its role, and places
the secondary a fixed `LOOKBACK_BLOCKS = 20` **content blocks** behind it — sized
to Anthropic's roughly 20-block cache lookback, so the next request's tail
breakpoint still lands inside the window even when one iteration appends many
blocks at once (parallel tool calls). It also adds a `has_cacheable_content`
guard: never stamp `cache_control` on an empty text block, on a thinking or
`reasoning` block, or on an assistant message whose payload is in `tool_calls`
with `content: null`.

inber is native Anthropic, so the role half does not transfer. The placement half
does, and inber has the same shape of bug for a different reason.

**Verified in inber.** `a.turnAnchorIdx` is set once, at turn start
(`agent/agent.go:318`), and `a.FrozenIdx` once per turn from the staged
conversation (`engine/build.go:44`). `buildRequest` then calls
`placeHistoryCacheBreakpoints(params.Messages, a.FrozenIdx, a.turnAnchorIdx)` on
**every** API call of the turn (`agent/agent_run.go:134`, and again at `:215`
after a prune-retry) with those two indices unchanged, and
`HistoryCacheBreakpointIndices` (`agent/agent.go:478-500`) returns exactly
`[frozenIdx-1, turnAnchorIdx]`. Nothing advances either index inside `Run`'s
loop, which is capped at 50 API calls (`agent/agent.go:336`); the one other write
to `turnAnchorIdx` (`agent/agent.go:535`, `-= dropped`) moves it *backwards* to
survive a prune. So on API call *k* of a turn, every message appended since call
1 sits past the last breakpoint and is sent at full price, and the turn's
uncached total grows with k².

Probed in the `agent` package with `frozenIdx=3`, `turnAnchorIdx=3` and one
assistant + tool-result pair per iteration:

```
iter= 0 messages=  4 breakpoints=[2 3] msgs_after_last_bp=0
iter=10 messages= 24 breakpoints=[2 3] msgs_after_last_bp=20
iter=20 messages= 44 breakpoints=[2 3] msgs_after_last_bp=40
```

Live instance in `~/.inber/server/server.db`, request `124`: `turns=26`,
`input_tokens=227,437`, `cache_read_tokens=335,453`, `cache_write_tokens=11,155`
— 40% of that turn's input billed at full price, 8,748 uncached tokens per call
against a 4,012 baseline for single-call requests. Across the `turns>=9` bucket,
2,777 uncached tokens per call over 118 calls.

- **What inber should consider:** the transferable idea is *offset-based*
  placement — measure the second anchor a fixed distance behind the tail, not a
  fixed distance behind the head. Filed this run. Three things a fix has to
  decide, none of them decided here: **(1)** which of the two message breakpoints
  gives way, since tools and system already hold two of Anthropic's four
  (`agent/agent_run.go:36`, `engine/turn_prompt.go:218,224`) — retiring the
  frozen-zone anchor gives up the thing that makes the frozen/staging split pay,
  retiring the turn anchor gives up the cheap first call; **(2)** how often the
  tail anchor moves, since every call maximizes the read but pays a 1.25× write
  each time, and goose's 20-block stride requires counting content **blocks**
  where `HistoryCacheBreakpointIndices` counts **messages**; **(3)** whether
  `markLastContentBlock` (`agent/agent.go:544-566`) needs goose's
  `has_cacheable_content` guard at all — it stamps `cache_control` on an empty
  text block and on an empty tool result, which `agent/agent_run.go:421` produces
  whenever a tool returns `""`, but no production path was found that puts such a
  message at a breakpoint index (`conversation.RepairEmptyContent`,
  `conversation/repair.go:131`, runs only on the three resume paths, not on the
  live turn), and it was not established that Anthropic rejects it. Held at 30%
  and deliberately not filed.

### 2. #11469 — upstream retired the pattern this doc offered inber as option (b)

[`bbf0ddf7`](https://github.com/block/goose/commit/bbf0ddf7) — *refactor: remove
fast model routing*. `GOOSE_FAST_MODEL` is gone from `model_config.rs` and from
every provider's `base.rs`, about 100 lines deleted. A pure refactor with no
defect behind it, and it would not be worth a line except that it closes an
option this file left open.

The 2026-08-20 entry (`goose.md:1327`) gave inber three answers for which model
compaction should run on, and **(b)** was *"give it a declared Anthropic model —
`SummarizeConfig.Model`, the field that already exists and has never been set."*
Re-verified this pass: `SummarizeConfig.Model` (`conversation/summarize_config.go:10`)
is still assigned by nobody, all three branches of `DefaultSummarizeConfig`
(`:34,:41,:48`) omit it, and `conversation/summarize.go:58-61` falls through to
the caller's model every time.

- **What inber should consider:** nothing yet — this is evidence, not a decision.
  Record against that entry that the upstream harness which had a separate
  cheap-model route for auxiliary work has now deleted it, so option (b) can no
  longer be argued from goose's precedent. Whether inber still wants it is a
  separate question about inber's own compaction costs, which
  `conversation/summary_generation.go` currently does not measure at all (see the
  2026-09-03 entry in `claude-code.md`).

## Harness-watch — 2026-09-04 (#11787, #11743, #11604, #11782, #11627): five repairs, one shape — the guard ran, and it was pointed at the wrong object

Five `fix(security)`/`fix(tracing)` commits from one author landed within hours on
2026-09-03, and they are worth reading together rather than separately, because
none of them adds a missing check. In every case the check existed and was applied
to the wrong noun. The cross-cutting write-up is at
`agentic-design-patterns.md` under 2026-09-04; this entry keeps the mechanisms and
the inber checks.

### 1. #11787 — content decided control flow

[`feecb14f`](https://github.com/block/goose/commit/feecb14f). Every CLI-wrapping
provider opened `stream()` with
`if super::cli_common::is_session_description_request(system)`, and that predicate
was literally `system.contains("four words or less") || system.contains("4 words or less")`.
So **any** request whose system prompt happened to contain that substring never
reached the CLI at all — goose fabricated a reply locally from the first user
message and returned it as if the model had answered. The repair binds dispatch to
the call path instead: a new `uses_local_session_naming()` on the `Provider` trait,
`generate_session_name` calling the local generator directly, and
`is_session_description_request` **deleted** along with the sniffing block in
`claude_code.rs`, `codex.rs`, `cursor_agent.rs` and `gemini_cli.rs`. A second fix
rides along: the local generator now filters `m.is_user_visible()` and
`filter_for_audience(Role::User)`, so agent-only content stops leaking into a
persisted session name.

**Checked in inber, and it carries nothing.** The only content-substring branches
on a live path are `conversation/stash.go:113-159` (`DetectContentType`) and
`conversation/manage_auto_save.go:78,96`. `stash.go` **is** live — `engine/engine.go:148`
builds the config, `engine/turn_prepare.go:23` and `engine/turn_stashing.go:38`
consume it — but the content type it derives is carried into the result struct and
the tags and **decides nothing**: importance is `cfg.DefaultImportance` at
`stash.go:195` on every branch. A label, not a gate. Nothing to file.

### 2. #11743 — the human approved a string the provider wrote

[`f90aa8ed`](https://github.com/block/goose/commit/f90aa8ed). `find_tool_confirmation`
destructured `ActionRequiredData::ToolConfirmation { id, prompt, .. }` — throwing
away `tool_name` and `arguments` through the `..` — and the prompt printed only
`prompt`. So a provider could render *"read package manifest"* over an
`arguments` of `{"command": "cat ~/.ssh/id_rsa"}`. The repair returns the whole
`ToolConfirmationRequest`, prints the provider's text under an explicit
`"Provider-provided approval notice"` header and the authoritative
`{"tool_name": …, "arguments": …}` beneath it, moves the output to **stderr**, and
escapes bidi controls (`U+061C, 200E, 200F, 202A–202E, 2066–2069`) on top of the
existing ANSI/OSC/DCS stripping. The tests assert the *effect*: one re-spawns the
test binary, feeds it an 80-line forged provider prompt, and asserts the
authoritative token is on stderr and not stdout.

**No inber surface today.** `guard.Config.ApprovalFunc` is assigned by nobody,
`engine/build_hooks.go:85-88` says so in a comment, and `NeedsApproval` is refused
rather than queued. Recorded as the constraint for whenever an approver exists: the
prompt must render the payload that will execute, from the trusted struct,
untruncated and control-escaped, with any model- or provider-supplied narrative
*alongside* it under an untrusted label — never instead of it.

### 3. #11604 — hiding a tool from a list is not an authorization control

[`35524fac`](https://github.com/block/goose/commit/35524fac). `manage_extensions_impl`
took no `session_id` at all and `list_tools` took `_session_id: &str`
(underscore-discarded), so a delegated subagent could enable arbitrary extensions;
`handle_start_agent` had no caller identity, and `handle_send_message`'s only guard
was a self-send check. The repair resolves the caller's `Session` and gates on
`session_type` in **both** layers — the tool is hidden *and* the handler refuses —
and fails closed when the session cannot be resolved
(`.is_ok_and(|session| session.session_type != SessionType::SubAgent)`). Messaging
becomes sibling-only: same `parent_session_id`, both `SubAgent`.

**inber's twin is exact and already open** as
`9e31d359-462e-4492-a1ca-317c08733564`. Re-measured this run rather than taken
from the todo: `server/spawn.go:224` and `server/session_forking.go:47` both pass
`RunRequest{}`, `applyRequestOverrides` copies only non-zero fields, and a probe
through `guard.ParseMode` + `LimitConfig.GuardConfig` prints

```
parent cfg: mode="assist" maxCost=5 maxTurns=10 maxInputTokens=100000 maxDuration=600
child  cfg: mode=""       maxCost=0 maxTurns=0  maxInputTokens=0      maxDuration=0
shell_commands   parent(assist)=NeedsApproval  child()=Allowed
write_files      parent(assist)=NeedsApproval  child()=Allowed
edit_files       parent(assist)=NeedsApproval  child()=Allowed
deploy           parent(assist)=NeedsApproval  child()=Allowed
spawn_agent      parent(assist)=Allowed        child()=Allowed
```

so an assist-mode parent — which cannot itself run `shell_commands` without an
approver that does not exist — spawns a child that can. That is the escape the
todo's 2026-07-31 note predicted, now measured end to end. The todo is parked on
the shared-pot-versus-per-child decision and stays parked; the only thing to carry
onto it is goose's sentence, **tool-list filtering is discoverability, not access
control**, which says the fix has to reach the handler and not just the advertised
set.

### 4. #11782 — the opt-out gated the standard attributes and not the legacy ones

[`0338189c`](https://github.com/block/goose/commit/0338189c).
`capture_message_content()` (default **false**) gated the standardized `gen_ai.*`
span attributes while goose's own legacy fields recorded content unconditionally —
`dispatch_tool_call` recorded full tool `arguments` with no gate at all, and
`user_message`/`trace_input`/`trace_output` sat *above* the `if`. Opting out still
exported the same secrets under different keys. Every content-bearing field moved
inside the gate; non-content attributes stayed out.

**inber's structural twin is open** as `d60ec4a3-4004-4f8e-b2ef-d4d0a70bb51e` — the
redactor on one door of four, with NATS, logstack and SSE getting the raw
arguments. Nothing new to file. The importable part is not the fix but the test:
`tool_arguments_are_not_traced_without_content_capture` plants an
`"api_key": "tool-input-super-secret-token"` and asserts it appears **nowhere in
the whole exported field map**, rather than asserting the guard was called. That
effect-level assertion is what catches the *next* bypass, and it is the style to
copy onto the redactor todo.

### 5. #11627 — upstream just shipped one of the two options inber is parked on

[`0f7d763b`](https://github.com/block/goose/commit/0f7d763b) is mostly a feature —
`ToolOperation<S>` for the new state-machine agent — but two duplicate-name guards
ride along, and one of them is a real fix to the **legacy** agent:
`reply_parts.rs` gained `ensure_unique_tool_names(&tools)` after `list_tools`,
erroring `"multiple tools registered '{name}'"`, where before two extensions
exporting one name produced a silently ambiguous tool list. `machine.rs` threads a
`HashSet<String>` across all operations for the same reason.

inber's twin is open as `e2d0b07b-5034-4f7d-b97f-2a534141dfc1` and is documented
in the code itself — `engine/extra_tools.go:8-15` says the fix *"needs a policy
decision (reject a duplicate at registration, or keep the first and warn), so this
function preserves the existing behaviour rather than guessing at it."* **goose
just shipped the first option**, and that is the whole contribution of this entry:
new evidence for a decision that was parked for want of any. Mechanism re-read
this run: `agent/agent_run.go:29-45` builds `toolParams` as a **list** over every
entry while `toolMap[t.Name] = a.tools[i]` is a **map**, so on a collision both
definitions go on the wire and dispatch resolves to whichever was registered last —
the model reads one schema and a different implementation runs.

Worth noting goose did not fully escape it either: its own dispatch is still a
name-based join (`.find(|tool| tool.definition.name == call.name)`), safe only
because uniqueness is now enforced at registration, and its README requires that
provider tool names and handlers *"remain stable between those boundaries"* — a
contract that is documented, not enforced.

- **What inber should consider:** nothing new is filed from this window. Two open
  todos gain evidence rather than scope — `e2d0b07b` gains an upstream precedent
  for rejecting duplicates at registration, and `9e31d359` gains a measurement of
  the escape and the sentence that says a fix must reach the handler. The one
  genuinely new item is a testing style: assert that a canary token is absent from
  the whole exported surface, not that the guard was invoked.
