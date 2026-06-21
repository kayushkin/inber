# OpenCode Comparison

**GitHub:** github.com/anomalyco/opencode
**Language:** TypeScript (Bun runtime)
**Focus:** Open-source, provider-agnostic AI coding agent with TUI-first design

## Architecture Overview

OpenCode is a large TypeScript monorepo (~19 packages) using Turbo for build orchestration and SST for deployment. It follows a client/server architecture with clean separation between the core engine and UI frontends.

```
┌─ UI Layer ──────────────────────────────────────────┐
│  TUI (primary) | Desktop (Electron) | Web | Slack   │
└─────────────────┼───────────────────────────────────┘
                  │
┌─ Core Engine ───┼───────────────────────────────────┐
│  Agent System | Session | Provider | Tool | MCP     │
│  Control Plane | Permission | Config | Bus          │
└─────────────────┼───────────────────────────────────┘
                  │
┌─ Dev Integration┼───────────────────────────────────┐
│  LSP | Git | Worktree | Shell/PTY | IDE | Snapshot  │
└─────────────────────────────────────────────────────┘
```

### Monorepo Packages

| Package | Purpose |
|---------|---------|
| `opencode` | Core engine — agents, tools, providers, sessions |
| `app` | Application shell |
| `sdk` | Public SDK for programmatic access |
| `plugin` | Plugin system |
| `desktop-electron` | Electron desktop client |
| `web` | Web interface |
| `ui` | Shared UI components |
| `slack` | Slack integration |
| `identity` | Auth/identity management |
| `enterprise` | Enterprise features |
| `console` | Console/admin interface |

## Agent System

**Dual Built-in Agents:**
- **build** — Full-access development agent with unrestricted file and command permissions
- **plan** — Read-only agent for exploration, requests permission before modifications

**Subagent Model:**
- A **general** subagent handles complex searches and multi-step reasoning
- Invoked via `@general` mentions within conversation
- Tab key switches between primary agents

This is simpler than inber's 10-agent system but follows a similar principle of role-based agent specialization.

## Provider System

**Provider Agnostic:**
- Supports Claude, OpenAI, Google, and local models
- Not locked to any single provider
- Commercial "OpenCode Zen" option provides hosted model access

**Comparison to inber:** Inber is currently Anthropic-coupled. OpenCode's multi-provider approach validates the direction of extracting a provider interface (similar to what Goose does).

## Tool & Extension Architecture

**MCP Integration:**
- Full Model Context Protocol support for tool extensibility
- `mcp/` module for MCP client implementations

**Plugin System:**
- Dedicated `plugin/` package for extending functionality
- `.opencode` project directories for agent specs and behavioral policies

**Skills:**
- `skill/` module — likely pre-built capability bundles (similar to Claude Code's slash commands)

**Comparison to inber:** OpenCode's plugin + MCP approach is more modular than inber's current hardcoded tool registration. The skill system is worth examining for inber's tool abstraction.

## Development Integration

OpenCode has deeper IDE/editor integration than most competitors:

- **LSP support** — Out-of-the-box Language Server Protocol for code intelligence
- **Worktree management** — Git worktree isolation for parallel work
- **Snapshot system** — State snapshots and recovery
- **PTY support** — Full pseudo-terminal for shell interaction
- **IDE module** — Direct editor integration
- **Zed extension** — First-party editor plugin

**Comparison to inber:** Inber has worktree support via forge but lacks LSP integration and snapshot recovery. The LSP integration is notable — it gives the agent richer code understanding without relying solely on the LLM.

## Session & State

- Session persistence and management
- Storage abstraction layer
- Sync mechanisms for state coordination
- Share functionality for collaboration

**Comparison to inber:** More developed session model with sharing capabilities. Inber's memory system is likely more sophisticated for long-term context, but OpenCode's session sharing is a feature inber lacks.

## Permission Model

- Dedicated `permission/` module
- Agent-level permission scoping (build = full access, plan = read-only)
- Command approval workflows

Similar in spirit to Claude Code's permission system.

## Infrastructure & Scale

- **Effect system** — Uses functional effect patterns (likely Effect-TS) for structured concurrency and error handling
- **Event bus** — Internal message bus for component communication
- **Control plane** — Central coordination layer

The Effect-TS usage is notable — it's a sophisticated choice for managing async complexity in TypeScript, similar to how Go uses goroutines/channels.

## What Inber Should Note

### 1. **LSP Integration**
OpenCode's LSP support gives agents real-time code intelligence (go-to-definition, references, diagnostics) without consuming LLM tokens. This is a meaningful architectural advantage.

**Inber action:** Evaluate LSP client integration for Go-aware code navigation within agent sessions.

### 2. **Snapshot & Recovery**
The snapshot system allows rolling back agent changes — a safety net that inber currently lacks beyond git-level undo.

**Inber action:** Consider checkpoint/snapshot support in forge worktree slots.

### 3. **Multi-Frontend Architecture**
OpenCode ships TUI, desktop, web, and Slack frontends from the same core. This validates inber's direction with si (communications layer) and the bus architecture, but OpenCode's execution is more polished.

### 4. **Provider Independence**
Multi-provider support from the start. Inber should continue pursuing provider abstraction via model-store.

### 5. **Plugin vs MCP Dual Extension**
Having both a plugin system and MCP support gives two levels of extensibility — lightweight plugins for common cases, MCP for standard tool integration.

## What Inber Does Better

### 1. **Memory Architecture**
Inber's memory system (conversation summarization, pruning, stashing) is more sophisticated than OpenCode's session-based approach. Long-term context retention across sessions is a genuine differentiator.

### 2. **Agent Depth**
Inber's 10-agent system with distinct personalities and specializations is richer than OpenCode's build/plan/general trio.

### 3. **Go Ecosystem**
Single binary, no runtime dependencies, lower resource footprint. OpenCode requires Bun and a Node.js ecosystem.

### 4. **Message Bus Architecture**
Inber's NATS-backed bus with si adapter layer is more robust for distributed agent communication than OpenCode's internal bus.

### 5. **Context Management**
Inber's smart truncation and context loading is more automatic and fine-tuned.

## Recommended Adoptions for Inber

**High Priority:**
1. **LSP integration** — Real code intelligence without LLM token cost
2. **Snapshot/checkpoint system** — Safety net for agent modifications

**Medium Priority:**
3. **Session sharing** — Collaborative features
4. **Skill/recipe abstraction** — Pre-built capability bundles

**Low Priority:**
5. **Desktop client** — Electron or similar GUI (inber's TUI is sufficient for now)
6. **Effect-style structured concurrency** — Go already handles this well with goroutines

---

## Harness-watch — 2026-05-09: `scout` agent + `@reference` external sources

[PR 24149](https://github.com/sst/opencode/pull/24149) (merged 2026-05-08) adds a built-in `scout` subagent dedicated to **research over external repos**, distinct from the existing `general` subagent that operates on the working tree. Two design pieces worth noting:

- **Named external references.** Config lets users register git repos or local dirs as first-class `@<name>` references. The scout agent receives those names as targets; opencode handles cloning into a managed cache, exposing them via two new managed tools `repo_clone` and `repo_overview`. The agent never sees a clone URL or a path — only the alias.
- **Research-only tool surface.** Scout's tool list is read/search/overview only; it cannot write to the working tree or shell out beyond the managed clone cache. The contract is "go look at this thing and report back" — the parent gets the report, not the side effects of the look-around.

**What inber should consider:** Two of inber's existing subagents (notably the documentation-researcher and the upstream-comparison agents implied by harness-watch itself) are already in scout's shape conceptually — research-only with bounded read surface — but they currently use the same Read/Grep/Bash trio as executor agents and manage their own cwd. The opencode pattern argues for a **named-reference layer** between the user and the subagent: instead of agents chasing paths, the harness owns a registry of "places to look" with managed clone/cache and a stable alias the model can reason about. For inber, the natural home is `forge` (which already manages worktree slots) extended with a read-only "external repo" slot type — and a `research` tool category that's allowlisted to those slots only. Worth a sketch in `docs/multi-agent-design.md`, paired with the SWE-Edit viewer/editor decomposition (2604.26102) which is the same "isolated read-only context" pattern from a different angle.

[PR 24712](https://github.com/sst/opencode/pull/24712) (merged 2026-05-08) is a separate but related move: opencode replaces its dependency on Vercel's AI SDK with an in-house Effect-based `packages/llm` covering 10+ providers, with a recorded-cassette test harness (`http-recorder`) that replays HTTP traffic against typed schemas. The cassette pattern is the borrowable bit — inber's provider bridges (`llm-bridge-anthropic`, `llm-bridge-openai`, `llm-bridge-google`) are integration-tested against live APIs today; a recorded-cassette layer would let the bridge tests run hermetically in CI without losing the wire-format coverage.

## Harness-watch — 2026-05-11: cache-policy auto-placement is the new default

A three-PR sequence in opencode's new in-house `packages/llm` lands a declarative cache policy and flips its default from `none` to `auto`:

- [PR 26779](https://github.com/sst/opencode/pull/26779) (merged 2026-05-11) adds a shared **breakpoint cap** across tools/system/messages — Anthropic and Bedrock-Claude both reject >4 cache markers, so the budget is allocated by priority `tools → system → messages` and the 5th+ marker is silently dropped (with a warning). Adds `CacheHint` support on `ToolDefinition` and `ToolResultPart`, and a `ttlBucket` helper that maps `ttlSeconds >= 3600` to the provider's "1h" bucket.
- [PR 26786](https://github.com/sst/opencode/pull/26786) (merged 2026-05-11) introduces the auto-placement algorithm: the policy runs once at `LLMClient.compile`, walks the request, and injects markers on **last tool definition**, **last system part**, and **latest user message** (explicitly *not* the latest assistant message). Manual hints are preserved — auto only fills gaps. The named insight: "a single user turn expands into many assistant/tool API calls all sharing that prefix" — caching at the user-message boundary makes every intra-turn API call hit the cached prefix.
- [PR 26798](https://github.com/sst/opencode/pull/26798) (merged 2026-05-11) flips the default from `cache: 'none'` to `cache: 'auto'`. Reasoning quoted in the PR: "Anthropic 5-minute cache write = 1.25× base, read = 0.1× — single reuse within 5 min already beats no-cache." On providers that cache server-side (OpenAI, Gemini), `auto` is a no-op, so the default is universally safe.

**What inber should consider:** Inber's `engine/build_prompts.go` puts BP3 on the *second-to-last message* (per `docs/cache-optimization.md`). For single-turn-then-stop conversations that's fine, but inber's tool-loop turns make multiple assistant↔tool round-trips per user turn — opencode's user-message anchor is strictly better in that case, since every intra-turn API call shares the prefix up to that user message. Worth a follow-up in `docs/cache-optimization.md` to retarget BP3 from "second-to-last message" to "latest user message" and measure the intra-turn hit rate explicitly. The shared breakpoint cap with priority allocation is also worth porting — inber currently emits 3 fixed BPs but as the system grows (e.g. when 4th-BP for history midpoint is added per the doc's "Future Considerations") a centralized allocator avoids over-marking. Independent of placement: flipping inber's default to *always* cache is the right move per the same cost arithmetic.

## Harness-watch — 2026-06-02: PermissionV2, CoW workspace clones, full-session-diff trap

### 1. PermissionV2 — structured (action, resource) rules with project-scoped remembered grants

[PR 30287](https://github.com/sst/opencode/pull/30287) replaces opencode's legacy
permission storage with a location-scoped (per-project) service whose unit is a
structured Rule of `{action, resource, effect: allow|deny|ask}`. User decisions
persist as normalized **remembered grants** in a SQL table keyed by a unique
`(project_id, action, resource)` index — an approval is scoped to a project, not a
global session, and is listable/addable/removable independently. Legacy wire
schemas are isolated in `PermissionLegacy`; additive `permission.v2.*` SDK events
are exposed. Meaningfully richer than the build/plan agent-level scoping already
in this doc.

**What inber should consider:** restructure the PreToolUse prehook's grant model
from per-session yes/no into a persisted `(project, action, resource) →
{allow|deny|ask}` ruleset table, so "always allow" decisions survive across
sessions and are project-scoped, not global.

### 2. Copy-on-write workspace cloning, distinct from git worktrees

[PR 30117](https://github.com/sst/opencode/pull/30117) adds a standalone Rust
`worktree` crate that creates managed workspace clones via real filesystem
**copy-on-write** (`clonefile` on APFS, `FICLONE`/reflink on Linux btrfs/XFS),
not git worktrees, with a `CowStrategy` that fails loud (`CowUnavailable`) when the
FS lacks reflinks. It tracks clone ancestry (a clone records its source, queryable
via `ancestors`). CoW gives near-instant, space-efficient whole-tree clones
*including untracked files and build artifacts and non-git dirs* — which git
worktrees cannot.

**What inber should consider:** add a CoW clone slot type to forge (reflink via
`FICLONE` on the host FS, fail loud if unsupported) for instant whole-tree agent
sandboxes that capture untracked files and build state, with source-ancestry
recorded per slot — complementing, not replacing, the git-worktree slots.

### 3. Automatic full-session snapshot diffs are an O(history) trap

[PR 30127](https://github.com/sst/opencode/pull/30127) removed automatic
full-session snapshot diffing after a snapshot-heavy session recomputed a
whole-session diff every turn (hundreds of snapshot parts, multi-MB diff). The fix
returns empty session-level diff data while keeping message-scoped turn diffs
(`session.diff({messageID})`) and explicit revert diffs intact. This doc already
flags snapshot/checkpoint adoption as high priority — this is the cautionary
footnote.

**What inber should consider:** if/when forge gets checkpoint/snapshot support,
scope diff computation to the current turn/message only — never recompute a
full-session diff per turn, or diff cost scales with total session history.
(The `run --replay` mode in [PR 30239](https://github.com/sst/opencode/pull/30239)
and editable queued prompts in [PR 30103](https://github.com/sst/opencode/pull/30103)
are minor UX layered on existing recording/queue plumbing inber's log-store
already enables — low priority.)

## Harness-watch — 2026-06-03: `scout` agent removed — walking back the dedicated-subagent + managed-tool design

[PR 30435](https://github.com/sst/opencode/pull/30435) (merged 2026-06-02) **deletes
the `scout` subagent** documented in the 2026-05-09 entry above — along with its
prompt and its managed `repo_clone` / `repo_overview` tools — and replaces it with
"simpler reference guidance using `Read`, `Glob`, and `Grep`." The whole external-
reference feature is now gated behind `OPENCODE_EXPERIMENTAL_REFERENCES`. So within
~three weeks opencode shipped a dedicated research subagent with a bespoke managed-
clone tool surface, then reverted to letting a general agent do reference lookups
with the ordinary read/search trio.

**What inber should consider:** this directly tempers the 2026-05-09 recommendation
to build a scout-shaped named-reference layer in `forge`. The signal: a *dedicated
research subagent + bespoke managed tools* was more machinery than the job needed —
general-purpose read tools pointed at a cloned path did the same work. Before inber
invests in a `research` agent type and a managed external-repo slot, prefer the
cheaper version first: keep one general agent, give it read/grep/glob over a path the
harness clones, and only graduate to a dedicated subagent + alias registry if that
demonstrably falls short. Ship the feature behind an experimental flag, as opencode
did, so walking it back costs nothing.

## Harness-watch — 2026-06-05: versioned "context epochs" + event-sourced admitted→promoted prompt lifecycle; the background-subagent anti-poll prompt

### 1. Context epochs: persist the system context as an immutable baseline + chronological deltas

[PR 30789](https://github.com/sst/opencode/pull/30789) replaces "rebuild the
privileged system context from scratch every turn/restart" with a **Context
Epoch** model: one immutable baseline plus a structured snapshot of context
*sources* (env/date, ambient + upward-project `AGENTS.md`) per epoch, in a new
`session_context_epoch` table. Changed context is admitted as durable
chronological `Message.system(...)` history **only at safe provider-turn
boundaries** (so an update can't split a tool call from its result), baselines
are replaced after model switches or completed compactions, and optimistic-
concurrency + location-fencing stop concurrent session moves from recreating
stale context. It rides on [PR 30785](https://github.com/sst/opencode/pull/30785),
which event-sources prompt admission as two durable facts: `prompt.admitted`
(accepted intent, pending input) and `prompt.promoted` (became model-visible
transcript at a safe runner boundary), separating `evt_*` event identity from
`msg_*` transcript identity.

**What inber should consider:** inber rebuilds its system prefix from
`engine/turn_prompt.go` per turn; this is the opposite discipline — persist the
*exact* context the model saw as an immutable baseline + chronological system-
message deltas keyed by durable message id, mutated only at provider-turn
boundaries. It makes context reproducible across restarts/revivals (kanban
task-completion-loop especially) and removes silent turn-to-turn drift, while
preserving cache locality on the unchanged baseline. The admitted→promoted split
also generalizes the dexto `interaction:blocked` lesson (dexto.md): model
"prompt submitted" and "prompt entered context" as separate durable events so
queued/steered input survives a restart and you can reconstruct exactly when each
prompt became visible.

### 2. The background-subagent completion-notify prompt only works if it bans polling AND orders the agent to end its turn

[PR 30790](https://github.com/sst/opencode/pull/30790) (building on the prompt
consolidation in [PR 30687](https://github.com/sst/opencode/pull/30687)) is a
7-line wording tweak whose lesson is concrete: opencode's background tasks use a
push-notification model ("you will be notified automatically when it finishes"),
but the parent kept **polling background tasks and re-investigating the same
files**. The fix reinforces the synthetic tool output to explicitly *(a)* forbid
polling/asking for status, *(b)* forbid duplicating work "with the same files or
topics it is using", and *(c)* "end your response" / "work on non-overlapping
tasks." "You will be notified automatically" alone was not enough.

**What inber should consider:** this maps onto inber's async-spawning model and
the harness's own push/wake mechanics (you are re-invoked when tracked work
finishes — polling is wasted). When `spawn_agent` runs detached, the synthetic
completion-pending tool result inber returns to the parent should carry the same
three clauses verbatim — *don't poll, don't touch its files/topics, end your
turn* — not just "you'll be notified." Cheap, high-leverage prompt hygiene worth
folding into `docs/async-spawning.md`.

## Harness-watch — 2026-06-06: v2 context-management subsystem — session-scoped prompt-cache key, provider-neutral overflow recovery, centralized tool-output bounding

A coherent batch landed on the v2 runner after the epoch/prompt-lifecycle work
(06-05 entry), turning context handling into a deliberate subsystem:

- **Session-scoped prompt cache key** ([PR 31036](https://github.com/sst/opencode/pull/31036)).
  The v2 runner now sets the provider `promptCacheKey` to the durable Session ID
  on every turn. Previously unrelated sessions sharing the same system prefix
  routed through the same cache combination; past ~15 req/min they overflowed to
  additional backend machines and lost cache locality. Keying by session keeps
  follow-up/tool-loop reuse while distributing unrelated sessions.
- **Provider-neutral context-overflow recovery** ([PR 31005](https://github.com/sst/opencode/pull/31005)).
  A normalized "context-overflow" classification across OpenAI/Anthropic/Bedrock,
  then: on overflow *before* durable assistant output, force a compaction that
  bypasses the local pressure estimate, rebuild, and retry **exactly once** (a
  second overflow is terminal); assistant persistence is deferred until real text
  or tool activity, so recovery only fires for invisible pre-output. Invariant:
  one logical provider turn gets at most one overflow recovery.
- **Centralized tool-output bounding** ([PR 30999](https://github.com/sst/opencode/pull/30999)).
  A single aggregate cap (2000 lines / 50 KiB UTF-8) applied in `ToolRegistry.settle`
  after tool-specific processing; oversized output spills to uniquely-named files
  in a managed dir (7-day cleanup) with bounded head/tail previews kept inline,
  and oversized structured results are converted to bounded JSON without mutating
  the validated value.

**What inber should consider:** all three are directly applicable.
(1) inber's cache strategy (`docs/cache-optimization.md`) should key the
provider cache by `BridgeSessionID`, not a shared prefix hash — this is the
operational complement to the "Don't Break the Cache" paper (papers/2026-05)
and matters as soon as concurrent autoworker/kanban sessions share a system
prefix. (2) inber treats context overflow as a turn error; a normalized
provider-neutral overflow classifier + *force-compact-and-retry-once* path
(distinct from the periodic pressure-based compaction) would recover the common
"one oversized turn" case without losing the session, with a hard one-retry cap
to avoid loops. (3) tool-output bounding belongs at the registry settle point
(one cap for every tool) with spill-to-file + head/tail preview, rather than
per-tool truncation — this pairs with inber's `smart-truncation.md`.

## Harness-watch — 2026-06-07: a stateful, frecency-ranked search backend behind the *same* grep/glob tool contract

[PR 27802](https://github.com/sst/opencode/pull/27802) swaps the ripgrep backend
of opencode's existing grep/glob tools for `@ff-labs/fff` — a native stateful
file/fuzzy finder with a background scan thread, fs watcher, mmap content index,
and **frecency + query-history DBs**. A ~550-LOC Search service wraps it
(`fileSearch`/`glob`/`directorySearch`/`mixedSearch` + grep with plain|regex|fuzzy
modes, time budget, cursor pagination) with a **full ripgrep fallback** on
unavailable/timeout/error. Crucially it adds **no new agent tools**: the grep/glob
descriptions change by one line each (dropping the "sorted by modification time"
claim), the old per-file `fs.stat` mtime-sort is deleted in favor of fff's
relevance/frecency scores, and `read.ts` now calls `search.open()/trackQuery()` so
**file Reads feed back into future search ranking**.

**What inber should consider:** inber's Grep/Glob are stateless ripgrep shell-outs
ranked only by mtime. Consider a stateful search service behind the *same* tool
contract that (a) ranks by frecency/relevance instead of mtime, (b) closes a
read→rank feedback loop (a file the agent just read should rank higher in the next
search), and (c) keeps a ripgrep fallback when the native index isn't warm — a
backend swap, so the agent-facing tool schema (and its cached prefix) stays stable.

## Harness-watch — 2026-06-09: cache identity has *two* layers — the provider cache key AND the gateway routing-affinity header

[PR 31511](https://github.com/sst/opencode/pull/31511) adds a one-line `X-Session-Id`
header (= the session ID) to every non-opencode provider request, alongside the
existing `x-session-affinity` header. The rationale is the part inber hasn't
documented: enterprise/Anthropic-compatible **proxies that front multiple upstream
accounts** use `X-Session-Id` to pin all requests from a session to the *same
upstream account*, because the cached prompt prefix physically lives on that one
account's KV cache. Without it the gateway load-balances a multi-turn conversation
across accounts and every turn is a cold prefix → cache miss → higher latency+cost.
This is **distinct from the `promptCacheKey` work** (06-06 entry, PR 31036): the
provider cache key decides *which cache entry* a request matches; the routing header
decides *which backend machine/account physically holds that cache*. Both must agree
on the same session identity or you still miss — opencode now sends both.

**What inber should consider:** inber routes provider traffic through its own gateway
layer (llm-gateway / vibeproxy) that can front multiple model-store credentials per
provider. Setting `promptCacheKey = BridgeSessionID` (already recommended on 06-06)
only wins if the gateway *also* pins that session to the same upstream account. Add a
session-affinity header (e.g. `X-Session-Id: <BridgeSessionID>`) on outbound provider
requests **and** have the gateway hash on it for upstream account selection — otherwise
concurrent autoworker/kanban sessions get scattered and the provider-side cache key is
moot. The two changes are a pair, not alternatives; document them together in
`docs/cache-optimization.md`.

## Harness-watch — 2026-06-15: advertise MCP client `roots` so servers scope to the workspace

[PR 32230](https://github.com/sst/opencode/pull/32230) has opencode advertise the MCP
client **`roots` capability** and answer `roots/list` with the instance's working
directory as a `file://` URI, registered before connection on both the plain and OAuth
client paths. `listChanged` is deliberately omitted because roots are fixed for an
instance-scoped client. MCP `roots` is the standard channel by which a client tells a
server which filesystem boundaries it's allowed to operate within — without it, a
filesystem/git MCP server has no authoritative notion of "the workspace" and either
guesses or operates unbounded.

**What inber should consider:** inber's tool-store (`reference_tool_store`) wires MCP
servers but, like most harnesses, likely connects without advertising roots. Have the
MCP client layer advertise `roots` and serve `roots/list` with the active session's
workspace dir (one fixed root, no `listChanged`) so filesystem-touching MCP servers
auto-scope to the right project instead of the server's launch cwd — relevant once
autoworker/kanban sessions run concurrent MCP-backed tools in different repos. It's a
small handshake addition with a real blast-radius payoff (servers can't wander outside
the declared root) and composes with the existing per-session permission gating.

## Harness-watch — 2026-06-19: a mid-run prompt must enter history as a *plain* user message — the steering wrapper busts the cache

[PR 33039](https://github.com/sst/opencode/pull/33039) removes the "steering-only system
reminder wrapper" that opencode used to wrap a prompt submitted **while a turn was already
running** (a steer/interjection). The wrapper rewrote the user message into a tagged
system-reminder shape before it entered history; the fix sends the mid-run prompt "as a
normal user message" and adds a test asserting "the next LLM input preserves the exact
user message shape." The unstated cache mechanic is the point: a steer arrives at the
*tail* of an already-cached conversation, so wrapping it perturbs the bytes appended after
the last cache breakpoint and forces the next request to re-process from an earlier point
than it should — the interjection itself becomes a cache-busting prefix mutation. Sending
it verbatim keeps the appended suffix identical to what a normal turn would have produced,
so the cached prefix still matches.

**What inber should consider:** inber injects messages mid-turn (llm-bridge-claudecode's
message injection, herald/`ask` relays, autoworker steers). Any such interjection should
land in history as a **plain user message with the exact shape a normal turn produces** —
do not wrap it in a system-reminder/steering envelope or retag it, because that mutates the
suffix appended after the live cache breakpoint and turns a cheap append into a cold
re-prefill. Presentation/role decoration belongs at the render edge, not baked into the
stored message — and add a test that pins "injected prompt == normal-turn user-message
shape" so a future wrapper can't silently regress cache hit-rate.
