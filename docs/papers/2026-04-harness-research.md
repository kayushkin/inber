# Harness Research — April 2026

Notes from the harness-watch sweep on 2026-04-29. Four arXiv papers from the
last 30 days that intersect with inber's design surface. One paragraph + one
concrete consideration each. No fluff.

> **Mined 2026-08-07 (nightly worker). Do not re-mine this file expecting
> defects.** All four of its "What inber should consider" entries are design
> proposals; none reports a defect in inber's code. This is the **last**
> unwalked doc on the harness-watch shelf — see todo `a88bca06`.
>
> Two factual claims about inber's code were checked:
>
> - 🔴 **"skill-store" is not part of inber.** The Externalization entry
>   (2604.08224) maps inber onto "memory-store, tool-store/skill-store,
>   llm-bridge protocol". `tool-store` is real — a `require` **and** a
>   `replace` in `go.mod`. **`skill-store` is neither**, and inber's Go has
>   **zero** case-insensitive hits for "skill". The passage then reasons from
>   the false half ("'Skills' as a first-class externalization is what
>   `skill-store` already provides — but inber agents don't yet have a clean
>   way to invoke skills"), so its conclusion is right by accident.
>
>   This is the **second recorded site** of the same wrong claim: the
>   `gemini-cli.md` pass (2026-08-07, earlier) found "inber's skill story sits
>   in skill-store" and refuted it the same way. Two independent docs asserting
>   the same non-existent dependency is a pattern, not a typo — **inber is
>   repeatedly described as owning services that merely live on the same box.**
>
> - ✅ **`docs/reference-based-prompt-architecture.md` exists**, as cited.
>
> Not a defect, though it reads like a broken link on a grep: `docs/safety-layers.md`
> is **proposed**, not cited — "Worth adding a `docs/safety-layers.md` if/when
> guardrails become a deliberate workstream." A path-existence sweep flags it;
> reading the sentence clears it. Check the verb before filing a missing file.

## Architectural Design Decisions in AI Agent Harnesses

[arXiv:2604.18071](https://arxiv.org/abs/2604.18071) — published 2026-04-20

Empirical study of 70 publicly-available agent-system projects. Distills the
design space into five recurring dimensions: **subagent architecture, context
management, tool systems, safety mechanisms, orchestration**. Key cross-pattern
finding: "deeper coordination pairs with more explicit context services,
stronger execution environments with more structured governance,
formalized tool-registration boundaries with broader ecosystem ambitions."
Tool-system trend: registry-oriented systems still dominant; MCP- and
plugin-oriented extensions are emerging. Context management corpus favors
file-persistent, hybrid, and hierarchical strategies. Safety: intermediate
isolation common, high-assurance audit rare. Projects cluster into five
archetypes (lightweight tools, balanced CLI frameworks, multi-agent
orchestrators, enterprise systems, scenario-verticalized).

**What inber should consider:** This is the cleanest external taxonomy yet
for organizing `docs/comparisons/`. Audit each existing comparison doc against
the five dimensions — most current docs focus on tool systems and miss
safety/orchestration cleanly. Worth adding a "five-dimension scorecard" section
to `agentic-design-patterns.md` and back-filling each comparison.

## Externalization in LLM Agents

[arXiv:2604.08224](https://arxiv.org/abs/2604.08224) — published 2026-04-09

Frames recent agent progress as a shift from "capabilities-in-weights" →
"capabilities-in-context" → "capabilities-in-harness." Proposes four
externalization mechanisms: **memory** (state across time), **skills**
(procedural expertise), **protocols** (interaction structure), **harness
engineering** (the layer that unifies the other three under governed
execution). Argues that infrastructure components are now reliability
multipliers rather than supporting cast, and predicts adaptive/shared
harness ecosystems as the next frontier.

**What inber should consider:** ⚠️ *Corrected 2026-08-07.* Inber's architecture
maps cleanly to two of the four (memory-store, tool-store — both `require`d and
`replace`d in `go.mod` — plus the llm-bridge protocol). **`skill-store` is not a
dependency of inber and inber's Go contains zero hits for "skill"**, so the
original claim that "'Skills' as a first-class externalization is what
`skill-store` already provides" was describing a service that runs on the same
host, not a part of inber. The conclusion still stands, and more strongly than
written: inber agents have no way to invoke skills as procedural artifacts,
because inber has no skill surface at all. Worth re-reading
`docs/reference-based-prompt-architecture.md` against this framing to see
whether skills should be promoted from documentation into a first-class
runtime invocation.

## ByteRover: Agent-Native Memory Through LLM-Curated Hierarchical Context

[arXiv:2604.01599](https://arxiv.org/abs/2604.01599) — published 2026-04-02

Inverts the typical agent-memory pipeline: instead of agents calling external
vector/graph/embedding stores, the same LLM that reasons about tasks also
curates, structures, and retrieves knowledge. Storage is plain markdown in a
hierarchical Context Tree (Domain → Topic → Subtopic → Entry). Zero external
infrastructure. Each entry carries importance, maturity tier, and recency
decay. A 5-tier progressive retrieval strategy resolves most queries under
100ms without an LLM call, escalating to reasoning only for novel questions.

**What inber should consider:** Inber's `memory-store` is closer to the
classic SQLite + vector + importance-decay model. ByteRover's claim is that
markdown-on-disk + LLM-curated hierarchy beats it on debuggability and
coordination across agents. Worth a side-by-side: would `memory.db`'s vector
search lose anything if memories were promoted to a hierarchical markdown
tree owned by the agent? `docs/memory-extraction-evaluation.md` is the right
place to evaluate this.

## Building an Internal Coding Agent at Zup

[arXiv:2604.09805](https://arxiv.org/abs/2604.09805) — published 2026-04-10

Lessons from rolling out an internal coding agent at scale. Three findings
the authors frame as more important than the model itself:
(1) **targeted tool design** (e.g., string-replacement edits over full-file
rewrites) moved reliability more than prompt optimization;
(2) **layered safety guardrails** outperformed prompt engineering for
preventing bad actions;
(3) **progressive human oversight modes** drove organic adoption without
mandating trust. Headline: "the engineering decisions surrounding the model
— not the model itself — determine whether a coding agent delivers real
value in practice."

**What inber should consider:** The string-replacement-over-rewrite finding
matches inber's existing tool surface (Edit vs Write), so this validates a
choice already made. The layered-guardrails point lines up with the
"safety mechanisms" gap from 2604.18071 above — high-assurance audit is rare
across the corpus and inber doesn't have it either. Worth adding a
`docs/safety-layers.md` if/when guardrails become a deliberate workstream.

## Cross-cutting takeaway

The April 2026 corpus is converging on the claim that the harness, not the
model, is the unit of progress. Three of the four papers ship a taxonomy
(5-dim, 4-mechanism, 5-tier retrieval). Inber's docs are heavy on per-harness
comparison and light on cross-cutting taxonomy — a single
`agentic-design-patterns.md` section that maps inber against the
2604.18071 five dimensions would compress a lot of these external frameworks
into one inber-anchored view.
