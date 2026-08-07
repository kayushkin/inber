# Gemini CLI Comparison

**Project**: [Gemini CLI](https://github.com/google-gemini/gemini-cli)
**Language**: TypeScript (Node, packaged as Single Executable Application)
**Focus**: Google's first-party agentic coding CLI for Gemini models
**Window observed**: v0.38 → v0.40 (April 2026)

## Why this doc exists

Gemini CLI is the closest analogue to Claude Code in the Google ecosystem and
ships fast — three minor releases in April landed primitives that map cleanly
onto open questions in inber's session, memory, and offline-tooling stories.
The standout is **Chapters** (v0.38), which is the first credible attempt by a
mainstream harness to give long agent sessions structure beyond a flat event
stream. The other two releases (v0.39 skill curation, v0.40 offline runtime)
are documented for completeness but the inber-impact analysis goes deepest on
Chapters.

inber's bridge to this harness is `~/repos/llm-bridge-gemini` (currently a
24-line scaffold — see `BRIDGE_PARITY.md`). Patterns documented here are
candidates for inber's *engine* / *bridge-ui* / *memory-store* layers, not
just for the bridge wrapper.

> ⛔ **WALKED 2026-08-07 (nightly worker). This doc is now SPENT — it was the last untouched
> comparison doc on the `a88bca06` shelf.** It is also the oldest (May 8), and it behaved
> the way the previous pass predicted: of its checkable premises about inber, **five are
> now false**. Every "What inber should consider" passage here is an *idea to import*, not
> a "inber is broken here" claim, so the shelf's own filing rule yields **zero** defect
> todos from the recommendations themselves. One latent duplication was found in the
> checking and is filed as `f086536d`.
>
> Two corrections to this paragraph:
> - **"24-line scaffold" is stale in the digits, right in the substance.** `main.go` is 34
>   lines (`037f072`, 2026-05-10, added a `-discover` handler two days after this doc). It
>   is still a scaffold: `main.go:32-33` is
>   `fmt.Fprintln(os.Stderr, "llm-bridge-gemini: not yet implemented")` / `os.Exit(2)`.
> - **"see `BRIDGE_PARITY.md`" points at a document that never mentions gemini.** The file
>   is `~/repos/inber/BRIDGE_PARITY.md` (repo root, not `docs/`); `grep -i gemini` on it
>   returns nothing. It is about `llm-bridge-inber`'s own capability parity.

---

## v0.38 — Chapters ⭐️

**Most relevant to inber.**

### What it is

Chapters group consecutive agent interactions in a session by **intent** and
**tool-usage pattern**. Instead of a flat scroll of turns, a session becomes a
sequence of named, scoped units: e.g. *"Investigate failing test"* (mostly
read/grep), *"Implement fix"* (mostly edit/write), *"Validate"* (mostly
exec/test). Boundaries are detected when the dominant intent shifts or when
the tool-use cluster meaningfully changes.

Chapters aren't a UI affordance bolted on at render time — they're a
first-class structural property of the session log, so every consumer
(replay, memory consolidation, evaluation, resumption) sees the same
boundaries.

### Why this matters architecturally

Two ideas worth pulling out separately:

1. **Sessions have natural seams, and the harness should name them.** Before
   Chapters, every consumer of a session log invented its own segmentation
   (or punted and treated the whole thing as one blob). Chapters move that
   segmentation upstream into the harness, so downstream consumers all agree.
2. **Intent + tool-cluster is a good cheap segmentation signal.** Not perfect,
   but it's derivable from the existing event stream without an extra model
   call per turn — a classifier-on-completion is enough.

### What inber should consider

inber's current state: `log-store` stores a flat event stream per session;
`memory-store` extracts memories from the whole session log; bridge-ui
renders turn-by-turn; the kanban classifier (see
`reference_kanban_classifier`) signs an entire session into a single card.
Every one of these would be sharper with chapter boundaries.

> ⛔ **WALKED 2026-08-07. Three of the four clauses in that sentence are wrong.** The
> Chapters idea may still be worth wanting, but the "current state" it argues from is not
> this system's, so do not quote this paragraph as a baseline.
>
> 1. **"`log-store` stores a flat event stream" — PARTLY.** Storage is flat
>    (`log-store/internal/store/store.go:92-100`, one `events` table, no group column),
>    but log-store is not grouping-blind: it derives a first-class **turn**
>    (`internal/server/turnmodel.go:491-494`, *"A turn is one bridge TurnID's worth of
>    events"*) and a second `GroupID` axis (`turnmodel.go:55,358-381`). "No segmentation
>    at all" is too strong.
> 2. **"`memory-store` extracts memories from the whole session log" — REFUTED, wrong
>    repo.** memory-store has no extractor and never reads a session log; it is a passive
>    CRUD/search store whose content always arrives from the caller
>    (`memory-store/server.go:14-21`, eight routes). `memory-store/sessions.go:6-10`
>    explicitly disclaims session metadata. The extractor is **inber's**
>    (`conversation/extract.go:20-21`, called per turn at `engine/turn_postprocess.go:35`)
>    and it is already **per-exchange**, capped to 500 tokens (`extract.go:63-69`). The
>    mixing problem proposal 2 below is built to solve therefore cannot arise from this
>    code path.
> 3. **"the kanban classifier signs an entire session into a single card" — REFUTED, and
>    inverted.** It creates **one card per task**, and says so in its own package doc:
>    `scheduler/cmd/kanban-classifier/main.go:19-20` *"new_tasks → create a new card per
>    task on the board"*; the loop is `main.go:462-493`. Nor is it once per session — it
>    is a 15-minute scheduler job that re-classifies a live session repeatedly
>    (`main.go:6-9,16-18`).
>
> **Confirmed:** inber has no notion of a named sub-session unit. `chapter|segment|phase|
> milestone|stage` across non-test Go yields only path components and code-comment
> "phases". The closest thing is the unnamed frozen/staging boundary
> (`agent/agent.go:448-450`), which has one position and no semantics.

Concrete proposals, smallest to largest:

#### 1. Add chapter metadata to the canonical event/message contract

A `chapter_id` (UUID) and `chapter_intent` (short string) on log-store events
or on `msg.Conversation` entries. Boundaries can be set by any of:

- **Agent self-declaration** — agent emits `<chapter intent="…">` markers in
  thinking blocks; cheapest, least reliable.
- **Heuristic on tool-call clusters** — boundary when the dominant tool kind
  (read/write/exec/web) changes for N consecutive turns; no extra model call.
- **Post-hoc classifier** — Haiku 4.5 pass over the session, similar shape to
  the existing kanban classifier (`reference_kanban_classifier`), at
  ~$3/mo budget tier.

Per CLAUDE.md "single source of truth" — the boundary should be set once at
write time and read by all consumers, not rederived per surface.

#### 2. Memory extraction runs per-chapter, not per-session

`memory-store`'s extractor today reads a whole session log, which means a
single dump can mix research findings, implementation decisions, and
debugging dead-ends — and the consolidation has to disentangle them. With
chapters, the extractor gets one bounded, intent-coherent unit at a time and
can write a memory with a known kind tag (`research`, `decision`, `recipe`).
This composes with the Codex memory-extensions pattern already noted in
`agentic-design-patterns.md` § *Self-describing memory extensions* — chapter
intent → memory extension → consolidation rule.

#### 3. Smart truncation drops chapters, not turns

`docs/smart-truncation.md` describes turn-level truncation today. Chapter
boundaries are the natural unit: a *completed* research chapter whose
findings have already informed a later implementation chapter is the
cheapest thing to evict from the context window. Turn-level truncation in
the middle of a tool call is structurally wrong; chapter-level truncation
isn't.

#### 4. Prompt-cache breakpoints align with chapter starts

`docs/cache-optimization.md` already cares about cache-hit rates. A chapter
start is a stable point — the prefix up to that boundary will not change for
the rest of the session — so it's a natural cache-control breakpoint. This
turns "did the cache hit?" from a per-turn lottery into a per-chapter
guarantee.

> ⛔ **WALKED 2026-08-07. §3's premise is REFUTED; §4's is CONFIRMED but the proposal has
> no budget left to spend.**
>
> - **§3: "`docs/smart-truncation.md` describes turn-level truncation"** — that document is
>   titled *"Smart Truncation of **Tool Results**"* (`smart-truncation.md:1`) and is entirely
>   tool-result-level; the word "turn" appears twice, incidentally. Turn-level truncation
>   does exist in inber, but somewhere else and undocumented here:
>   `conversation/manage.go:63-70` ages messages in turns-from-end and prunes by age
>   (`:106,121-122`), with per-role thresholds in `conversation/manage_config.go`.
> - **§4: cache-optimization.md does care about hit rate** — CONFIRMED
>   (`cache-optimization.md:26-30,212-213,298-300`). **But all four of Anthropic's
>   `cache_control` blocks are already spent**, so "add a breakpoint at each chapter start"
>   has nothing to allocate: BP1 tools (`agent/agent_run.go:34-37`), BP2 last stable system
>   block (`engine/turn_prompt.go`), BP3 frozen/staging boundary and BP4 this turn's user
>   anchor (both from `agent.HistoryCacheBreakpointIndices`, `agent/agent.go:478-503`,
>   which caps itself at two and dedupes). A chapter-aligned scheme would have to *replace*
>   one of the four, not join them. Note `cache-optimization.md`'s own "Current State"
>   header still says three breakpoints; it is annotated stale at `:23-25`.

#### 5. Kanban + task-completion-loop become chapter-aware

`reference_kanban_autoworker` and `reference_kanban_task_completion_loop`
both operate at session granularity today (one card per session run, one
goal decomposed into sub-cards). Chapters give a finer unit:

- Classifier creates one card *per chapter*, not per session — closer to the
  unit of work the user actually thinks in.
- Dispatcher's "resume vs fresh" decision can target a specific chapter
  rather than the session head — "rerun the validation chapter against the
  new fix" without spawning a whole new session.

#### 6. Session resumption forks at chapter boundaries

bridge-ui's session resume/fork affordances today resume the conversation
from its tail. A "resume from chapter N" affordance is structurally cleaner
— you keep the chapters that established context and discard the ones whose
work has been superseded.

#### 7. inber-party turns chapters into quests

Lower priority but on-theme: `inber-party`'s RPG view already renders
agent activity as quests. Chapters map almost literally onto quests —
named, bounded, intent-scoped narrative units. This is the kind of feature
that's nearly free once the metadata exists upstream.

### Order of operations

(1) is the prerequisite for everything else and is small — it's a schema
change in log-store + a heuristic boundary detector in llm-bridge-server.
(2)–(4) each become small once (1) lands. (5)–(7) are surface-level
consumers and can land independently after (1).

---

## v0.39 — Skill curation in `/memory inbox` and gated Plan Mode

### What it is

- `/memory inbox` — a review queue for skills the agent picked up during
  recent sessions. The user triages: keep, edit, drop. Closes the loop on
  "skills are written by sessions but vetted by humans."
- **Plan Mode requires confirmation for skill activation** — entering Plan
  Mode now prompts the user before letting an autonomously-discovered skill
  influence planning. Stops a junk skill from quietly shaping a plan.
- **Decoupled ContextManager** — internal refactor; ContextManager is no
  longer entangled with the prompt-builder. Useful as a code-quality
  reference, not a feature.

### What inber should consider

inber's skill story sits in `skill-store` (canonical SKILL.md registry,
ingested from `anthropics/skills` and `antigravity` — see
`reference_skill_store`). Today skills are pulled from upstream repos; there
is no analogue of "skills the local agent discovered during a session."

If/when inber starts auto-promoting in-session skills (e.g. the kanban
classifier today writes notes; a sibling loop could write skills), an
inbox-with-confirmation pattern is the right shape — agents propose,
humans approve, and Plan Mode treats unconfirmed skills as inert. This
parallels the permission-prompt pattern (`project_permission_prompt_followups`)
and could share the same UI surface.

The decoupled ContextManager is worth a glance during the next pass on
inber's `engine/`'s context loader (see `docs/ENGINE_REFACTOR_PLAN.md`) —
if there's any prompt-builder/ContextManager entanglement still left after
the recent refactors, this is precedent for splitting them.

> ⛔ **WALKED 2026-08-07. One premise stale, one framing imported, one real finding.**
>
> - **"skills are pulled from upstream repos" — now REFUTED.** skill-store gained local
>   on-host directory sources on 2026-08-04 (`d3ad34f`): `schema.sql:14-18` has
>   `kind TEXT ... 'git' | 'local'` and `local_path`. The two named upstream seeds are
>   still right (`cmd/seed/main.go:17-37`).
> - **"no analogue of skills the local agent discovered" — CONFIRMED**, and so is the
>   absence of a review gate: neither `sources` nor `skills` has a status/approved column
>   (only `enabled`, `schema.sql:24,50`). The doc's "agents propose, humans approve" shape
>   is still unbuilt and still the right shape.
> - ⚠️ **"inber's skill story sits in `skill-store`" is not true of inber the binary.**
>   `grep -i skill` over inber's Go returns **zero hits**, and `go.mod` has no skill-store
>   dependency. skill-store is a host service (`:8301`) that installs into `~/.claude/skills`;
>   inber does not read it. Any proposal premised on inber already touching skill-store is
>   greenfield, not incremental.
> - **"if there's any prompt-builder/ContextManager entanglement still left" — there is no
>   ContextManager** (`ContextManager|ContextLoader|LoadContext` = zero hits; the doc is
>   importing gemini-cli's vocabulary). But the entanglement it asks about is **real**:
>   `engine/turn_prompt.go` `BuildSystemPrompt` does the memory *fetch* (`:80-91`) and the
>   block *assembly* (`:100-124`) in one method on `*Engine`, which is the exact defect
>   `ENGINE_REFACTOR_PLAN.md:22` named ("`build_prompts.go` handles both context budget
>   calculation and system prompt assembly"). The file split happened; the unit split did
>   not. That plan (Apr 5) carries no done-markers and is partly executed — Phase 0 and 3
>   done, Phase 1 (`engine/workflow/`, `engine/openclaw/` subpackages) not started.

---

## v0.40 — Bundled ripgrep, local Gemma, MCP resource tools, prompt-driven memory

### What it is

- **Bundled ripgrep in the SEA** — Gemini CLI's Single Executable Application
  ships with `rg` baked in, so search works offline and without an external
  binary on PATH. Removes a long tail of "search isn't working because rg
  isn't installed" support cases.
- **`gemini gemma`** — runs against a local Gemma model with no cloud round
  trip. Same harness UX, different runtime.
- **MCP resource list/read tools** — first-class tooling for the MCP
  *resources* primitive (not just tools). Lets the agent enumerate and read
  resources exposed by MCP servers.
- **Prompt-driven memory editing across 4 context tiers** — the agent can
  edit memory at four scopes (session, project, user, global) by emitting
  edit prompts rather than calling typed memory APIs.

### What inber should consider

#### Bundled ripgrep / SEA-style packaging

Inber today assumes the host has the search tools the agents need. For OSS
distribution (`feedback_oss_outreach_value_fit`,
`reference_local_dir_convention`) the bundled-rg pattern is worth a look —
not necessarily SEA, but vendoring or auto-installing the small set of
binaries inber's tools depend on, so a cold install works on a fresh box.
Lower priority, but a real friction point for OSS adoption.

#### Local Gemma alongside cloud models

inber's provider story is multi-provider already through `model-store` and
the `llm-bridge-google`/`llm-bridge-anthropic`/`llm-bridge-openai` triple.
A local-model bridge (`llm-bridge-local` or extending `llm-bridge-google` to
target a local Gemma endpoint) would complete the picture and is in scope
for `BACKLOG.md`. Reference value here, not a new pattern.

#### MCP resources, not just tools

`tool-store` (`reference_tool_store`) and the existing MCP integration in
`llm-bridge-server` focus on the MCP **tools** primitive. MCP also defines
**resources** (read-only addressable content, e.g. `file://`, `db://`) and
**prompts** (templated user prompts the server supplies). Gemini CLI v0.40
shipping resource list/read tools confirms that the resources primitive is
becoming worth supporting. This composes with the *Goose MCP Apps* pattern
already documented in `agentic-design-patterns.md` § *Capability-negotiated
MCP* — `ui://` was a Goose-specific extension, but generic resources are
the same wire shape and would be the natural place to start. Worth a sketch
in `docs/mcp-apps.md` (or wherever the MCP capability work lands) before
prototyping.

> ⛔ **WALKED 2026-08-07. CONFIRMED that only the tools primitive is implemented — but this
> passage badly understates how far along the client is, and misplaces where the work is.**
>
> inber already has a **complete, tested MCP client** at `tools/mcp/` (`client.go` 480
> lines, `adapter.go` 115, three test files). It speaks exactly four methods — `initialize`
> (`client.go:228`), `notifications/initialized` (`:246`), `tools/list` (`:251`),
> `tools/call` (`:385`) — so "tools only" is right. **It has zero importers**:
> `grep -rn "tools/mcp" --include=*.go` across inber and across every repo returns nothing
> but its own tests. Already recorded at `docs/harness-control-matrix.md:104` and
> `docs/comparisons/opencode.md:429`; repeated here because this passage frames MCP
> resources as the next increment when the *tools* increment is built and unwired.
>
> `tool-store` and `llm-bridge-server` implement **no** MCP primitives at all — tool-store
> stores launch specs (`schema.sql:16,24-27`; `tool.go:48-49` *"MCPSpec describes how to
> launch or reach an MCP server"*) and llm-bridge-server passes the config blob through to
> the harness (`internal/server/tool_provision.go:18,24-25`). The protocol work is
> delegated downstream, so "focus on the tools primitive" is true of inber's client and
> vacuous of those two.
>
> `docs/mcp-apps.md` **does not exist** (the passage hedges, so this is a proposal, not a
> false claim).

#### Prompt-driven memory across tiered scopes

inber's `memory-store` is a uniform schema (importance, decay, embeddings)
without an explicit scope tier. The four-tier model
(session/project/user/global) is a useful frame: session-scope memories
decay fast, global-scope memories are essentially user preferences. A
`scope` column on memory-store is small. The "prompt-driven editing" half
— letting the agent edit memory by emitting edit prompts rather than typed
API calls — is harder to assess without seeing how Gemini CLI handles
conflict and idempotency; treat as a curiosity, not a recommendation,
until there's more public surface area on it.

> ⛔ **WALKED 2026-08-07. CONFIRMED, and already filed — do not re-file it.**
>
> The schema is as described: `importance` and `embedding` are columns
> (`memory-store/store.go:132-142`), decay is behaviour (`management.go:55-57`,
> `POST /memories/decay`), and **there is no `scope` column** — `grep -i scope` over
> non-test Go returns zero hits, and no migration adds one (`migrations.go:30-53`). The
> nearest thing is `orchestrator TEXT NOT NULL DEFAULT ''` (`migrations.go:45`, *"Orchestrator
> scoping"*), which is a different axis. So "without an explicit scope tier" survives.
>
> **The `scope` column is already a filed todo — `54465690`** (adds the column plus
> scope-aware Save/Search), blocked on a Q2 decision, not on anyone noticing the gap. This
> passage is a duplicate of work already queued.
>
> **Found in the checking, and it is the one live thing this doc produced.** The
> stable/volatile partition that protects inber's cached system prefix is implemented
> **twice, in two repos, and the two rules disagree**:
> - `memory-store/builder.go:305-320` `isVolatileMemory` matches the `fileref:` / `recent:`
>   / `file:` id prefixes **plus any memory carrying the `recent` tag**.
> - `inber/engine/turn_prompt.go` `isVolatileMemoryID` matches the three id prefixes and
>   **has no tag check**.
>
> A memory tagged `recent` with an ordinary id would be sorted last by memory-store and
> then classified *stable* by inber — landing it in a system block in front of BP2, so it
> would bust the cached prefix on every turn it changed. **This is LATENT, not live:** the
> only producer of that tag (`memory-store/prepare.go:190`) also gives the memory a
> `recent:` id (`:196`), which inber's prefix check catches. Filed as `f086536d` rather
> than fixed, because collapsing two repos onto one rule is a single-source-of-truth
> change with an API question in it, not a one-line patch.

---

## Cross-cutting takeaway

Of the three releases, **Chapters is the structural one**. v0.39 and v0.40
each ship a couple of useful primitives, but Chapters is the kind of change
that becomes the substrate for many others — once a session has named
boundaries, memory extraction, truncation, caching, kanban classification,
and resumption all get sharper at once. The order of inber work that
follows from this should be: (1) chapter metadata in log-store and the
canonical message contract, then (2)–(7) as independent follow-ons.

Worth revisiting in 1–2 release cycles to see whether chapter intent is
agent-emitted, heuristic, or classifier-derived in Gemini CLI's
implementation, and whether the chapter contract has stabilized enough to
pattern-match against.

> ⛔ **WALKED 2026-08-07 — this doc is SPENT. What it cost and what it returned.**
>
> This was the last untouched comparison doc on the `a88bca06` shelf, and it closes the
> comparison-doc seam. Yield: **one filed child** (`f086536d`, and it came from checking a
> premise, not from a recommendation), **one code fix** (the dead pre-VolatileContext
> residue in `engine/turn_prompt.go`, below), and **five falsified premises**.
>
> **The pattern worth carrying forward.** Every "What inber should consider" section in
> this doc is an *idea to import*; not one is a "inber is broken here" claim. The shelf's
> filing rule turns that into zero todos. But the ideas are argued from statements about
> inber's current state, and *those* are checkable — three of the four clauses in the
> Chapters baseline sentence were wrong, and the skill-store and llm-bridge-gemini
> premises had both gone stale. **On an idea-import doc, the findings are in the baseline,
> not in the proposal.** That is the sharpest version of the onboarding note's rule 10.
>
> **Why the code fix is not in any recommendation either.** Checking proposal §4 (align
> cache breakpoints with chapter starts) meant enumerating where inber's four
> `cache_control` blocks go. That enumeration turned up a `__CACHE_BOUNDARY__` sentinel
> constant that nothing has emitted since volatile content moved into the user message: two
> readers that could never fire, a zero-caller `buildDynamicBlocks`, a zero-caller
> `isVolatileBlock` whose comment named a caller in the wrong repo, and a `BuildSystemPrompt`
> doc comment still listing fleet status and recent files as system blocks 4-6. The residue
> was not harmless — **three of inber's own research papers were still reasoning about where
> to place new content relative to that sentinel** (`docs/papers/2026-07-harness-research.md:34,787`,
> `2026-08-harness-research.md:169`), i.e. a stale name was steering live design work.
> `goose.md:520` had already spotted it was "retained but skipped as a legacy marker" and
> left it there.
>
> **Shelf status after this pass:** every comparison doc is now walked. What remains
> unmined is the three paper files, which the onboarding note's §1 already prices as
> low-yield.
