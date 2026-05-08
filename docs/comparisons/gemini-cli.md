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
