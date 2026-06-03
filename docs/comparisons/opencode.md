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
