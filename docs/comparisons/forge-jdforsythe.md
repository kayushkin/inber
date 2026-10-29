# Forge (jdforsythe) Comparison

**Last reviewed**: 2026-10-29 against jdforsythe/forge@b192c5c (2026-07-03). Quiet but not archived. Its one big change since this doc was written is the July 2026 commit "Re-derive research base and skills from verified 2025-2026 evidence", which withdrew several of the numbers this doc first quoted. The sections below now give the current figures and say what was withdrawn.

Not to be confused with the `forge` worktree library that inber imports (`server/workspace_roots.go`).

**Project**: [Forge](https://github.com/jdforsythe/forge)
**Language**: Markdown/JSON (zero code — Claude Code plugin)
**Focus**: Research-backed AI team assembly using a multi-agent scaling study (Kim et al. 2025, Google Research/MIT), persona research (PRISM, Hu et al. 2026), the MAST failure taxonomy, and vocabulary routing
**Key Strengths**: Research-grounded methodology, vocabulary routing, structured artifact handoffs, anti-pattern watchlists, progressive context disclosure

## Architecture Overview

Forge is NOT a framework or runtime. It is a Claude Code plugin consisting entirely of markdown files and JSON schemas. Claude Code is the runtime. Forge provides:

```
4 skills (slash commands):    mission-planner, agent-creator, skill-creator, librarian
3 infrastructure agents:      verifier, researcher, reviewer
11 library agents:            software, marketing, security domain specialists
3 team templates:             pre-built team topologies
Research docs:                8 documents (docs/research/), re-checked July 2026
```

Installation: `/plugin add https://github.com/jdforsythe/forge` inside Claude Code, or `npx add-skill jdforsythe/forge`. The entire system is prompt engineering artifacts structured as a plugin — no executable code.

## What Forge Does Well

### 1. Vocabulary Routing ⭐️

The central idea. Every agent definition carries 15-30 precise domain terms organized in 3-5 clusters (`METHODOLOGY.md`; Forge calls the count a house standard, not a measured value). Precise terms such as "circuit breaker pattern" steer the model's output toward expert register better than "handle errors gracefully".

Terms must pass the **"15-year practitioner test"** — would a senior practitioner use this exact term with a peer? Generic consultant-speak ("leverage," "best practices") is banned. Since July 2026 the method also warns against too much jargon: one cited study (Schreiter 2025) found a middle level of specificity works best. Forge has withdrawn the earlier explanation that terms "activate knowledge clusters in embedding space"; no published source supports it (`docs/research/persona-science.md`, "Retired claims").

**Inber connection**: Inber's agent prompts in agent-store use natural language descriptions (soul, principles, values). Incorporating vocabulary routing — precise domain terms rather than generic instructions — would improve prompt quality significantly.

### 2. Research-Backed Team Sizing

Based on Kim et al. (2025), "Towards a Science of Scaling Agent Systems" (Google Research/MIT, with DeepMind co-authors), as Forge reads it in `docs/research/scaling-laws.md`:
- **The ~45% rule**: when a single agent already passes about 45% of a task, adding agents gives *negative* returns, not just smaller ones.
- Across all benchmarks, multi-agent setups averaged −3.5% versus one agent, with wide spread by task.
- No study supports one universal saturation point. Forge's team size of 3-4, with a hard cap of 5, is its own rule informed by the study.

*(Updated 2026-10-29: this list first quoted "saturates at 4 agents", "3.5x tokens for 2.3x output" and "7x for 3.1x" and called the study DeepMind's. Forge's current research docs no longer carry those figures.)*

Forge enforces a **3-level decision flow**: Level 0 (single agent) → Level 1 (known library pattern) → Level 2 (novel decomposition). Always start at lowest level, never skip.

**Inber connection**: Inber runs a fixed roster of named agents (more than ten), loaded from agent-store, and lets an agent spawn subagents up to `MaxSpawnDepth` levels deep (default 2, `server/config.go`). This research suggests most tasks should be handled by one agent, and teams kept to 3-4. The 45% rule could inform when to spawn sub-agents vs handle in-agent.

### 3. Structured Artifact Handoffs

Teams communicate through typed artifacts (PRDs, ADRs, test reports) with explicit schemas, not free-form dialogue. The cited evidence is MetaGPT (Hong et al. 2023): structured handoffs scored 3.75/4 on executability against 2.25 for dialogue-based ChatDev, with about 3x fewer human revisions. *(Updated 2026-10-29: this said "~40% reduction in error propagation". Forge now says that figure appears in no version of the paper, `docs/research/team-design.md`.)*

**Inber connection**: Inber's agents communicate via NATS messages (free-form text). Adding typed artifact schemas for inter-agent communication would reduce misinterpretation.

### 4. Anti-Pattern Watchlists

Every agent definition includes named failure modes with detection signals and concrete resolution steps. The source is MAST (Cemri et al., NeurIPS 2025): 14 failure modes in 3 categories, from 1,600+ annotated traces. Specification issues are the largest category (41.77%); step repetition (17.14%) is the most common single mode. Forge's own watchlist, W-1 to W-7 (Rubber-Stamp Review, Role Overlap, Unbounded Review Loops, Confident Overreach, Error Cascade, Handoff Loss, Premature Done), is labeled as house rules mapped to their nearest MAST modes, not part of MAST. *(Updated 2026-10-29: this called rubber-stamp approval MAST mode FM-3.1 and the most common; Forge now maps it to FM-3.2/3.3.)*

### 5. Progressive Context Disclosure

Context is layered:
- **Layer 1** (always loaded, 200-500 tokens): role + vocabulary
- **Layer 2** (task-triggered, 500-2000 tokens): SOPs
- **Layer 3** (on-demand, 2000+ tokens): full documentation
- **Layer 4**: compressed summaries

Forge's token figures per layer are its own standards. Its research notes now say output gets worse steadily as input grows and that there is no evidence for an "optimal utilization percentage" (`docs/research/context-engineering.md`). *(Updated 2026-10-29: this said Forge targets 15-40% of the window; that target is gone.)*

**Inber connection**: Inber's memory budget (`engine/turn_context.go`, `contextBudget`) scales by turn state and message complexity. Forge's layered approach with explicit token budgets per layer is more structured.

## What Inber Should Adopt

### 1. Vocabulary Routing in Agent Prompts (HIGH PRIORITY)

Review all agent prompts in agent-store and replace generic instructions with precise domain vocabulary. For each agent, define 15-30 terms that a senior practitioner would use. This is a prompt engineering change — no code required, high impact.

### 2. Spawn Threshold Rule (MEDIUM PRIORITY)

Before spawning a sub-agent, estimate whether the current agent can achieve >45% of optimal on its own. If yes, handle in-agent with mode switching rather than spawning. This reduces token waste from multi-agent overhead.

### 3. Anti-Pattern Detection (MEDIUM PRIORITY)

Add failure mode detection to the agent loop:
- **Rubber-stamp**: if a review/verification agent approves everything without substantive feedback, flag it
- **Infinite delegation**: if agents keep spawning sub-agents without progress, halt
- **Echo chamber**: if agents repeat each other's outputs without adding value, consolidate

*(2026-10-29: of these, only delegation depth is bounded, by `MaxSpawnDepth` in the server. `guard/guard.go` says its repetition detector has no caller outside tests and nothing checks for rubber-stamping. MAST's data points at step repetition as the mode worth catching first.)*

### 4. Typed Artifact Handoffs (LOW PRIORITY)

Define schemas for inter-agent communication artifacts. Instead of free-form text via NATS, agents pass typed messages (TaskResult, CodeReview, BuildReport) with required fields. Reduces ambiguity in multi-agent workflows.

## What's Different

| Aspect | Forge | Inber |
|--------|-------|-------|
| **Type** | Design methodology (prompt artifacts) | Runtime orchestration framework |
| **Code** | Zero (all markdown/JSON) | ~6.4k non-test lines of Go in `engine/` alone |
| **Runtime** | Claude Code plugin system | Standalone Go server |
| **Agents** | Dynamic team assembly per task | Pre-configured named agents, plus spawned subagents |
| **Scaling** | 3-4 recommended, hard cap 5 | Fixed roster; spawn depth capped (default 2) |
| **Communication** | Typed artifacts (PRDs, ADRs) | Free-form NATS messages |
| **Persistence** | Stateless (plus a usage log in `library/`) | SQLite agent-store and memory-store |
| **Research** | Heavily cited (8 research docs) | Pragmatic/operational |

## Key Takeaway

Forge is the most research-minded project in this comparison set, and its July 2026 pass, which withdrew figures it could not trace to a source, is itself a model worth copying. It doesn't build infrastructure — it provides **research-backed principles for designing agents and teams**. The vocabulary routing concept alone could improve every agent prompt in inber. The team sizing research should inform inber's sub-agent spawning decisions. Forge answers "what agents to create and how to prompt them" — inber answers "how to run them." They're complementary, and Forge's principles should be applied to inber's agent definitions.
