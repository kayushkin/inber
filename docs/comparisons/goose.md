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

## Harness-watch — 2026-07-16: a fixed-window safety classifier is padding-bypassable — chunk with overlap, take the max score, fail closed on oversize

[goose #10416](https://github.com/block/goose/pull/10416) fixes a real evasion of goose's command-injection classifier (bashcat-distilbert), which has a hard 512-token input window: anything past the window is silently ignored, so a benign prefix can push the malicious command out of view and the model returns SAFE (measured on the live endpoint: SAFE 0.0006 → INJECTION 0.9997 once fixed). The fix (`command_chunker.rs`) splits the command into **overlapping** windows sized to a worst-case byte budget (bytes as a token proxy, so no window can exceed the limit), classifies them **concurrently**, and takes the **max** injection score. Two fail-closed details: a partial-window failure never discards a detection — a clean-but-incomplete result falls back to pattern scanning rather than being trusted — and oversized commands (beyond `MAX_WINDOWS`) are flagged *before* detections run, so the fail-closed signal survives truncation. Logs record counts only, never command content.

The general lesson: **any fixed-context safety check — a classifier, a regex over the first N bytes, or an LLM screening prompt with a token cap — is bypassable by padding the dangerous span past the window unless you scan the whole input.** Overlap matters because a naive split lets a payload straddle a boundary; max-over-windows is the safe aggregation; the truncation and failure paths must fail *closed*.

**What inber should consider:** inber's guard/permission layer (the PreToolUse prehook in bridge-server, the `permission_store` rules) matches commands against patterns — a regex like the `^curl\b` allow or any `rm -rf` deny (see `project_permission_store_live_rules_open`) is evaluated against the command string as given. Confirm those matches scan the **whole** argument, not a truncated head, and that oversized / unparse-able commands fail *closed* (deny or prompt) rather than falling through to allow. If inber ever adds an LLM- or classifier-based command screener, adopt goose's shape directly: overlapping worst-case-sized windows, max score, pattern-scan fallback on partial failure, flag-don't-drop on oversize. This is the concrete mechanism behind the `feedback_disarm_dont_document` and open permission-store concerns — a screen that only sees the first N bytes is a screen an attacker pads around.

## Harness-watch — 2026-07-20: a raw JSON-Schema tool definition is not portable — normalize `oneOf`→`anyOf` before it reaches an OpenAI-compatible backend

[goose #10571](https://github.com/block/goose/pull/10571) fixes a 400 that killed every request in a session: schemars (Rust's schema generator) emits `oneOf` of `const`s for enums with per-variant docs, and Moonshot's/Kimi's server-side schema validator only follows `anyOf` in its `$ref` termination check — so a `$ref → oneOf` is misreported as infinite recursion and rejected. The fix in `validate_tool_schemas` rewrites every `oneOf` to `anyOf`, recursing into nested `$defs`/subschemas. The rewrite is provably safe as a widening: anything valid under `oneOf` is valid under `anyOf`, so no legal argument is newly rejected. The load-bearing detail is that this is a *provider-portability* normalization, not a goose-specific patch — **OpenAI structured outputs also accepts `anyOf` and rejects `oneOf`**, so the same rewrite is the correct shape for any strict OpenAI-compatible validator, and it belongs in the one place tool schemas are projected toward that provider.

The general rule: a tool's JSON Schema is authored once (often by a codegen that emits `oneOf`, `$ref`, `format`, or other keywords a given backend won't traverse) but is consumed by N provider validators with different strictness. The bridge that projects a canonical tool definition into a provider's function-call shape must **normalize the schema to that provider's accepted dialect**, not forward it verbatim.

**What inber should consider:** inber has this exact bug latent today. `agent/openai_conversion.go:11 ConvertAnthropicToolsToOpenAI` marshals each tool's Anthropic `InputSchema` straight into the OpenAI `Parameters` map (`json.Marshal(t.InputSchema)` → `json.Unmarshal` → `Parameters: schemaMap`) with **zero normalization** — whatever `oneOf`/`$ref`/unsupported keyword an MCP tool declares is passed through unchanged to every OpenAI-compatible backend inber routes to (Kimi/Moonshot and OpenAI structured outputs among them). An MCP server whose tool schema uses `oneOf` (common: enums-with-docs, tagged unions) will 400 the whole turn against a strict validator, and inber currently has no path that would catch or rewrite it. Fix: add a schema-normalization pass in `ConvertAnthropicToolsToOpenAI` (and the equivalent Google projection) that recursively rewrites `oneOf`→`anyOf` and strips/downgrades keywords the destination provider is known to reject — a widening rewrite, applied at the bridge edge, keyed off the destination provider, never mutating the canonical `InputSchema`. This is the tool-schema analogue of the 07-18 modality-projection rule (`agentic-design-patterns.md`): *project the canonical artifact into what the destination will actually accept, at the edge, as data — don't forward it raw and don't assume every backend reads the same dialect.*

## Harness-watch — 2026-07-26: a compaction summary should be a typed, section-ordered, user-templatable artifact — not one opaque prose blob

[goose #10471](https://github.com/block/goose/pull/10471) replaces goose's freeform prose compaction summary with a structured contract. The compaction prompt now asks the model for an `<analysis>` scratchpad plus a single ```json block matching `StructuredSummary` (`context_mgmt/structured.rs`): **nine named sections — user intent, technical concepts, files + key code, errors and fixes, problem solving, user messages, pending tasks, current work, next step — each ordered most-important-first.** The parsed object is rendered to markdown by a **minijinja template the user can override** at `~/.config/goose/prompts/compaction_summary.md`, so changing *what survives compaction* is a config edit, not a code rebuild; unknown JSON fields are preserved so custom prompt+template pairs can carry extra sections. Parsing is deliberately forgiving — brace-balanced candidates are tried after each `</analysis>`/```json fence or at response start, wrong-shaped fields are stringified rather than rejected, and **any parse failure falls back to keeping the raw response verbatim, i.e. exactly the old behavior** (a strict widening, never worse). A blind-judge probe eval (30 replayed conversations, Haiku 4.5 + Sonnet 4.6) held summary fidelity at parity while improving decision-recall and retaining **24–48% fewer context tokens** post-compaction. A second fix in the same PR: goose's post-compaction context baseline had been using the summarizer call's *raw output token count*, overstating retained context ~2.3×; it now bills the raw output but sets the session baseline to the *estimated tokens of the actually-retained conversation*.

The general lesson: the summary a compaction step emits is a **data structure, not a paragraph.** Typing it into ordered sections lets you (a) control and prioritize what survives, (b) trim least-important sections under pressure instead of doing prompt surgery, and (c) render it through a swappable template. The forgiving-parse-with-verbatim-fallback discipline is what makes it safe to adopt: a malformed model response is never worse than the prose blob you have today.

**What inber should consider:** inber's summarizer is exactly the opaque-blob shape goose just left. `conversation/summary_generation.go:generateSummary` prompts for a freeform bulleted summary against five loose focus areas ("main topics / key decisions / important info / project status / next steps") and joins the model's text into one string (`summarize.go:88`); there is no typed contract, no per-section importance ordering, and no way to trim or template what is retained. Adopt goose's shape: (1) prompt for a typed `StructuredSummary` (inber can reuse goose's nine sections nearly verbatim — they're generic agentic-session fields), parse it forgivingly, and **on any parse failure fall back to the current prose join** — that fallback is already inber's error path (`generateSummary` err → `mechanicalSummary`), so the discipline fits. (2) Render the parsed summary through a template so *what survives compaction* becomes tunable per role without touching Go. (3) One thing inber already gets right and should keep: `result.SummaryTokens = memory.EstimateTokens(summary)` estimates the *retained* summary text, not the API's raw output-token count — inber does **not** have goose's 2.3× baseline overstatement bug, so don't "fix" it toward the raw usage number. This complements, from the summary-structure side, the compaction rules already in `agentic-design-patterns.md` (07-13 budget projection, 07-13 drop-thinking-when-summarizing).

## Harness-watch — 2026-07-30: freeze the whole turn-context, not just the clock — plus a *message* needs its own identity, and a delegate inherits *runtime* state rather than config

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
