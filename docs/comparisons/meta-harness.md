# Meta-Harness Comparison

**Paper**: "Meta-Harness: End-to-End Optimization of Model Harnesses" (arXiv:2603.28052, Mar 2026)
**Authors**: Yoonho Lee, Roshen Nair, Qizheng Zhang, Kangwook Lee, Omar Khattab, Chelsea Finn (Stanford/MIT/KRAFTON)
**Language**: Python (harnesses), Claude Code Opus 4.6 (proposer agent)
**Focus**: Automated search over LLM harness code — the prompt construction, retrieval, memory, and orchestration logic that wraps a frozen model
**Key Insight**: The code surrounding a model matters as much as the model itself, and a coding agent with full filesystem access to prior attempts can optimize that code automatically

## Architecture Overview

Meta-Harness is an outer-loop system that optimizes **harness code** — the software layer that determines what context, prompts, and tools an LLM sees. The base model is frozen; only the harness changes.

The search loop has three steps, repeated ~20 iterations:

```
1. PROPOSE  — Coding agent reads filesystem of all prior harnesses + scores + traces
                → writes a new harness (single-file Python program)
2. EVALUATE — Run the harness on eval tasks with the frozen base LLM
                → collect scores + full execution traces
3. STORE    — Save harness code, scores, and traces to a new directory
                → filesystem grows, proposer sees more history each iteration
```

The proposer is Claude Code with Opus 4.6. It uses standard dev tools (`grep`, `cat`, file navigation) to selectively read from the filesystem. A typical proposer reads **82 files per iteration**, referencing 20+ prior candidates. Each evaluation can produce up to **10 million tokens** of diagnostic information — 3 orders of magnitude beyond prior text optimizers.

No fixed mutation operators, no crossover, no parent-selection heuristic. The agent decides what to inspect and what to change. The filesystem *is* the memory.

## What Meta-Harness Does Well

### 1. Filesystem-as-Memory for Search History

The central design decision. Every prior harness candidate gets its own directory with source code, evaluation scores, and raw execution traces. No compression, no summarization. The proposer navigates this with standard shell tools.

This is the opposite of most optimization approaches (OPRO, TextGrad, AlphaEvolve) which compress feedback into scalar scores or short summaries. Meta-Harness operates at ~10 MTok/iteration of available context vs 0.002-0.026 MTok for prior methods.

**Inber connection**: Inber's memory system takes a middle path — it stores references and auto-extracted facts, but stashes/summarizes large content. Meta-Harness argues that full traces are worth the cost, at least during optimization.

### 2. Coding Agent as Proposer (Not Raw LLM)

The proposer isn't a raw LLM doing next-token prediction on prompt templates. It's a coding agent that can:
- Selectively read files (doesn't need to ingest everything)
- Form causal hypotheses about what changes caused regressions
- Make targeted edits rather than full rewrites
- Navigate accumulated experience that exceeds any context window

The paper shows the agent isolating confounded edits across candidates, identifying shared prompt interventions that caused regressions, and pivoting to safer additive modifications. This is **agent-level reasoning about its own optimization history**.

**Inber connection**: This validates the agent-with-tools approach over pure prompt engineering. Inber's agents already have shell, file, and memory tools — the same capabilities Meta-Harness relies on for its proposer.

### 3. Unrestricted Search Space

Harnesses are arbitrary single-file Python programs. No fixed scaffold, no template with blanks to fill. The proposer can change prompt templates, retrieval logic, memory/state management, tool-use patterns, and orchestration code. This means it can discover fundamentally different approaches, not just tune parameters.

**Inber connection**: Inber's engine already organizes these exact concerns (prompt building in `turn_prompt.go`, context budget in `turn_context.go`, memory in the memory package, tools in `build_tools.go`). Meta-Harness essentially searches over the space of possible engine configurations.

### 4. Results That Transfer

Discovered harnesses generalize to:
- **OOD datasets**: Text classification harnesses beat ACE on 6/9 unseen datasets
- **Unseen models**: Math retrieval harness improves accuracy across 5 different models not used during search
- **Weaker models**: TerminalBench agent works even better relative to baselines on Haiku 4.5 vs Opus 4.6

This suggests the optimized harness code captures genuine strategies, not model-specific quirks.

### 5. Inspectable Optimization

Since optimization happens in code space (not weight space), you can read what changed. The paper shows discovered strategies like hierarchical class grouping, adaptive confidence thresholds, and experience replay — all visible in the harness source. Overfitting manifests as brittle if-chains or hard-coded mappings, which are obvious on inspection.

## What Inber Should Adopt

### 1. Execution Trace Logging for Self-Improvement (HIGH PRIORITY)

Meta-Harness's key advantage is that the proposer can read full execution traces. Inber already logs sessions as JSONL, but the traces aren't structured for automated analysis.

**What to adopt:**
- Log full API request/response pairs (already partially done via `session.Entry.Request`)
- Log tool call inputs AND outputs with timing (already done)
- Add structured outcome tagging: did the turn succeed? Was the tool output used? Did the agent backtrack?
- Make these logs queryable, not just appendable

This would enable an outer loop that reviews past session performance and suggests prompt/tool/context changes — a manual version of Meta-Harness's automated search.

### 2. Harness-Level Configuration as Code (MEDIUM PRIORITY)

Meta-Harness treats the harness as a single-file program. Inber's equivalent — the engine config + prompt building + tool selection + context budget — is spread across many files. Making this configurable as a coherent unit would enable:
- A/B testing different engine configurations per agent
- Saving and comparing "harness snapshots" with their outcomes
- Eventually, automated search over configurations

**Concrete step**: Extract the key knobs (context budget curve, stash thresholds, pruning intervals, tool allowlist, system prompt structure) into a serializable `HarnessConfig` that can be saved alongside session outcomes.

### 3. Pareto Frontier for Agent Configuration (LOW PRIORITY)

Meta-Harness maintains a Pareto frontier of harnesses (accuracy vs. context cost). Inber could track a similar frontier across agent configurations:
- Token efficiency vs. task success rate
- Cache hit rate vs. context completeness
- Response time vs. quality

This would help answer "is this agent config better?" when there are multiple competing objectives.

## What's Different

| Dimension | Meta-Harness | Inber |
|-----------|-------------|-------|
| **Purpose** | Automated harness optimization (outer loop) | Runtime agent orchestration (inner loop) |
| **Search** | Proposes and evaluates harness variants | Fixed harness, adapts context per turn |
| **Memory** | Filesystem of all prior attempts (no compression) | SQLite semantic memory (compressed, tagged) |
| **Model** | Frozen — never changes | Selected per turn (failover, model health) |
| **Agent role** | Proposer that writes harness code | Executor that uses tools to complete tasks |
| **Feedback** | Full execution traces (10 MTok/iter) | Token/cost stats, session logs |
| **Optimization target** | Prompt templates, retrieval logic, orchestration | Context budget, memory loading, tool selection |
| **Time scale** | 20 iterations over hours | Real-time, per-turn decisions |

## Key Relationship

Meta-Harness and inber operate at different levels of the stack:

- **Meta-Harness** = the search loop that discovers *which* harness configuration works best
- **Inber** = the harness itself — the runtime that builds prompts, manages context, selects tools, and runs the agent

They're complementary. Meta-Harness could theoretically optimize inber's engine configuration (context budgets, stash thresholds, prompt structure, tool selection) by running many variants and comparing outcomes. Inber provides the runtime infrastructure that a Meta-Harness-style search would optimize over.

## Key Takeaway

The paper validates that **harness engineering matters enormously** — a well-optimized harness can outperform state-of-the-art hand-tuned systems across classification (+7.7 pts over ACE), retrieval (+4.7 pts over no retrieval), and agentic coding (76.4% vs 58.0% base Claude Code on TerminalBench-2). The approach only works because coding agents with tool access can navigate massive trace histories. For inber, the immediate lesson is: **invest in structured execution logging** — it's the prerequisite for any kind of harness optimization, whether automated or manual.
