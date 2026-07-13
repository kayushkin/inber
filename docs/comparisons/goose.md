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

## Harness-watch — 2026-06-05: bound sub-tasks by turn budget the agent can see, not a wallclock timeout it can't

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

## Harness-watch — 2026-07-13: *signed* thinking is an exactly-once replay contract — dedupe it, don't strip it (and never guess the cause from a generic error string)

[goose #10083](https://github.com/block/goose/pull/10083) fixes a recurring Anthropic 400 by adding a `dedupe_signed_thinking` fixer to the shared `fix_messages` pipeline in `goose-provider-types/src/conversation.rs`, positioned **immediately after `merge_consecutive_messages`** (load-bearing — the merge is what creates the duplicates). `is_signed_thinking` = a `Thinking` block with a non-empty signature, or any `RedactedThinking`; a conversation-wide `seen` set drops exact (text + signature) repeats, keeping the first. The contract, verbatim from the diff's doc comment: **"Signed blocks must be replayed exactly; unsigned reasoning summaries need not"** — and unsigned reasoning is deliberately *left alone*, since providers like Kimi/DeepSeek require it echoed on every tool-call message. Duplicates arise two ways: intra-message (a standalone thinking message merged into a tool-call message that re-embedded it) and **cross-message (one provider turn split into several tool-call messages interleaved with tool results, each carrying a copy of the turn's signed thinking)**. Because the fixer sits in the provider-neutral repair pipeline upstream of every formatter, one change covers direct Anthropic, Bedrock, Databricks and Vertex.

The design idea is the asymmetry most harnesses get wrong: *signature present ⇒ exactly-once verbatim replay; signature absent ⇒ echo freely.* The signature is also what makes exact-match dedupe **provably safe** — two blocks with an identical signature can only be the same turn's thinking copied onto split messages, never two genuinely distinct thoughts. And it establishes the **conversation-repair pipeline as the right home for provider-contract normalization**, rather than each provider's formatter.

**What inber should consider:** this is directly load-bearing, because inber runs the exact configuration that produces the bug and its current remedy is a sledgehammer. `agent/clients.go:103` sends `interleaved-thinking-2025-05-14` — thinking interleaved with `tool_use` across split assistant messages, i.e. goose's **cross-message** duplicate cause precisely. (inber's `RepairAlternation` merges consecutive *user* messages but inserts a `[continued]` placeholder between consecutive assistants, so the intra-merge cause is absent; the cross-message one is not.) inber's only thinking fixer, `conversation/repair.go:207 RepairThinkingSignatures`, **blanket-strips every `OfThinking` / `OfRedactedThinking` block** and substitutes a `"[thinking redacted]"` text block. It fires from two places, and both are wrong in a different way:

- `engine/turn_execute.go:51` strips-and-retries when `apiutil.IsThinkingSignatureError(err)` — and that predicate is literally **`msg == "Error"`** (`internal/apiutil/apiutil.go:12`), a string-equality guess against the word "Error". So *any* error whose message happens to be exactly `"Error"` destroys all thinking in the conversation and retries. inber is guessing at the cause and applying the maximally destructive remedy — when the actual common cause in this configuration is a *duplicate*, for which the correct fix is dedupe-keep-first.
- `server/session_creation.go:102` strips **unconditionally on every resume**, so a resumed session forfeits interleaved thinking's continuity across tool calls — which is the entire point of the beta header inber enables one file over.

The stated rationale (signatures are credential-bound) is real, but only for the *credential-rotation* path it was written for; it is being applied as a general-purpose repair. Recommended: **(a)** narrow `RepairThinkingSignatures` to the credential-rotation case it actually addresses, and stop invoking it on generic-error retry and on every resume; **(b)** add a `DedupeSignedThinking` fixer to inber's repair pipeline (which already *is* a fixer chain — `RepairEmptyContent` → `RepairDanglingToolUse` → `RepairAlternation` → … ), ordered after any merge step, keying on (text + signature) and keeping the first; **(c)** adopt the signed/unsigned split as an explicit contract — inber today has no notion of it. Note this interlocks with the cline budget-projection entry in `agentic-design-patterns.md` (07-13), which independently arrives at the same rule from the compaction side: drop thinking when feeding a *summarizer*, preserve it verbatim for the *provider request*.
