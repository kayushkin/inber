# Forge (jdforsythe) Comparison

**Project**: [Forge](https://github.com/jdforsythe/forge)
**Language**: Markdown/JSON (zero code — Claude Code plugin)
**Focus**: Science-backed AI team assembly using DeepMind research, PRISM persona science, and vocabulary routing
**Key Strengths**: Research-grounded methodology, vocabulary routing, structured artifact handoffs, anti-pattern watchlists, progressive context disclosure

## Architecture Overview

Forge is NOT a framework or runtime. It is a Claude Code plugin consisting entirely of markdown files and JSON schemas. Claude Code is the runtime. Forge provides:

```
4 skills (slash commands):    mission-planner, agent-creator, skill-creator, librarian
3 infrastructure agents:      verifier, researcher, reviewer
11 library agents:            software, marketing, security domain specialists
3 team templates:             pre-built team topologies
Research docs:                8 documents citing DeepMind, PRISM, MetaGPT findings
```

Installation: `claude /plugin add https://github.com/jdforsythe/forge`. The entire system is prompt engineering artifacts structured as a plugin — no executable code.

## What Forge Does Well

### 1. Vocabulary Routing ⭐️

The central innovation. Every agent definition carries 15-30 precise domain terms organized in 3-5 clusters. The theory: terms like "circuit breaker pattern (Nygard)" activate specific knowledge clusters in the LLM's embedding space far more effectively than generic instructions like "handle errors gracefully."

Terms must pass the **"15-year practitioner test"** — would a senior practitioner use this exact term with a peer? Generic consultant-speak ("leverage," "best practices") is explicitly banned.

**Inber connection**: Inber's agent prompts in agent-store use natural language descriptions (soul, principles, values). Incorporating vocabulary routing — precise domain terms rather than generic instructions — would improve prompt quality significantly.

### 2. Research-Backed Team Sizing

Based on DeepMind's 2025 multi-agent scaling study:
- Performance saturates at 4 agents
- 3-agent teams cost 3.5x tokens for 2.3x output
- 5-agent teams cost 7x for 3.1x output
- **45% threshold rule**: if a single agent achieves >45% of optimal, adding agents yields diminishing returns

Forge enforces a **3-level decision flow**: Level 0 (single agent) → Level 1 (known library pattern) → Level 2 (novel decomposition). Always start at lowest level, never skip.

**Inber connection**: Inber has 10 named agents. This research suggests most tasks should be handled by 1-3 agents, not the full fleet. The 45% threshold rule could inform when to spawn sub-agents vs handle in-agent.

### 3. Structured Artifact Handoffs

Teams communicate through typed artifacts (PRDs, ADRs, test reports) with explicit schemas, not free-form dialogue. Based on MetaGPT research showing ~40% reduction in error propagation compared to open dialogue.

**Inber connection**: Inber's agents communicate via NATS messages (free-form text). Adding typed artifact schemas for inter-agent communication would reduce misinterpretation.

### 4. Anti-Pattern Watchlists

Every agent definition includes 5-10 named failure modes from the MAST taxonomy (14 failure modes across communication, coordination, and quality). Each has detection signals and concrete resolution steps. Most common: rubber-stamp approval (FM-3.1) where review agents approve everything due to LLM sycophancy.

### 5. Progressive Context Disclosure

Context is layered:
- **Layer 1** (always loaded, 200-500 tokens): role + vocabulary
- **Layer 2** (task-triggered, 500-2000 tokens): SOPs
- **Layer 3** (on-demand, 2000+ tokens): full documentation
- **Layer 4**: compressed summaries

Targets 15-40% context window utilization based on attention budget research.

**Inber connection**: Inber's context budget system (`turn_context.go`) scales by turn state and message complexity. Forge's layered approach with explicit token budgets per layer is more structured.

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

### 4. Typed Artifact Handoffs (LOW PRIORITY)

Define schemas for inter-agent communication artifacts. Instead of free-form text via NATS, agents pass typed messages (TaskResult, CodeReview, BuildReport) with required fields. Reduces ambiguity in multi-agent workflows.

## What's Different

| Aspect | Forge | Inber |
|--------|-------|-------|
| **Type** | Design methodology (prompt artifacts) | Runtime orchestration framework |
| **Code** | Zero (all markdown/JSON) | ~6.5k lines Go in engine alone |
| **Runtime** | Claude Code plugin system | Standalone Go server |
| **Agents** | Dynamic team assembly per task | Pre-configured named agents |
| **Scaling** | Caps at 3-5, research-backed | 10 agents, configurable |
| **Communication** | Typed artifacts (PRDs, ADRs) | Free-form NATS messages |
| **Persistence** | Stateless | SQLite agent-store |
| **Research** | Heavily cited (8 research docs) | Pragmatic/operational |

## Key Takeaway

Forge is the most intellectually rigorous project in this comparison set. It doesn't build infrastructure — it provides **research-backed principles for designing agents and teams**. The vocabulary routing concept alone could improve every agent prompt in inber. The team sizing research should inform inber's sub-agent spawning decisions. Forge answers "what agents to create and how to prompt them" — inber answers "how to run them." They're complementary, and Forge's principles should be applied to inber's agent definitions.
