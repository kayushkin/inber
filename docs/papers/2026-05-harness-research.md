# Harness Research — Early May 2026

Notes from the harness-watch sweep on 2026-05-01. Two arXiv papers from the
last 7-10 days that weren't in the April 29 sweep and bear directly on
inber's harness design. Upstream commit feeds for all eight tracked harness
repos failed this run, so this is a paper-only update.

## Agentic Harness Engineering: Observability-Driven Automatic Evolution of Coding-Agent Harnesses

[arXiv:2604.25850](https://arxiv.org/abs/2604.25850) — submitted 2026-04-28,
revised 2026-04-30

Frames harness engineering as an automatable feedback loop instead of manual
craft. Three observability pillars: **component observability** (each
harness component is file-level editable and reversible), **experience
observability** (raw trajectory data is digested into evidence the agent can
learn from), **decision observability** (every modification is bound to a
testable prediction verified on the next iteration). Drives pass@1 from
69.7% → 77.0% on Terminal-Bench 2 over ten iterations and beats both
hand-tuned and prior self-evolving baselines. The ablation is the actually
interesting bit: **gains come from tools, middleware, and memory edits, not
prose-level prompt rewriting** — and the evolved components transfer across
models, suggesting they encode general engineering experience rather than
benchmark overfitting.

**What inber should consider:** Inber's harness components are already
file-level (engine/, tools/, memory/, registry/, guard/), so the
"component observability" precondition is largely met. The missing piece is
**experience observability** — a structured way to roll up trajectory traces
from `trace/` and `logs/` into evidence usable by an evolution loop. Worth
sketching whether the existing trace export could feed a simple per-component
diff/ablation harness, even if the actual evolution is human-in-the-loop for
now. The tools/middleware/memory ablation also reinforces a finding from the
April Zup paper: when iterating on agent quality, prefer changing the harness
surface over tweaking the prompt.

## Synthesizing Multi-Agent Harnesses for Vulnerability Discovery (AgentFlow)

[arXiv:2604.20801](https://arxiv.org/abs/2604.20801) — submitted 2026-04-22

A typed graph DSL whose search space jointly covers **agent roles, prompts,
tools, communication topology, and coordination protocol** — i.e., the entire
multi-agent harness layout, not just the prompt or tool list. Paired with a
feedback-driven outer loop that reads runtime signals from the target
program itself to diagnose which part of the harness caused a failure and
rewrite that part. Reaches 84.3% on TerminalBench-2 and discovers 10
zero-day Chrome vulnerabilities (incl. two Critical sandbox-escapes) — the
first paper I've seen with explicit CVE numbers attributed to a
harness-search system. Companion thesis to 2604.25850: when the model is
fixed, harness search alone moves end-to-end success several-fold.

**What inber should consider:** Inber's multi-agent surface
(agents.openclaw-example.json, the registry, the bus topology) is currently
configured by hand. AgentFlow's typed-graph framing is closer to what inber
already has than the unstructured natural-language harnesses in 2603.25723,
so it's a more realistic reference if/when inber considers programmatic
multi-agent topology generation. Lower-effort takeaway: the failure-attribution
loop. Inber's `bus` already carries enough run-level signal to attribute a
failed sub-task to a specific agent role or tool — surfacing that
attribution on the inber-party dashboard would be a small win even without
automated rewriting.

## SWE-Edit: Rethinking Code Editing for Efficient SWE-Agent

[arXiv:2604.26102](https://arxiv.org/abs/2604.26102) — submitted 2026-04-28

Frames code editing as suffering from a "context coupling problem": the main
agent ends up loading large files just to apply small edits, which pollutes
its working context and inflates token cost across every subsequent turn.
SWE-Edit decomposes editing into two specialized subagents — a **Viewer**
that extracts task-relevant code on demand, and an **Editor** that executes
modifications from a high-level plan inside its own clean context window —
so the parent agent only sees the plan and the final diff, not the
intermediate file contents. They also train an 8B model with GRPO to
adaptively pick edit modes (find-and-replace vs. structural rewrite) instead
of hardcoding one. On SWE-bench Verified: +2.1% resolved rate, -17.9%
inference cost. The architectural pattern is the takeaway, not the trained
model — keeping bulky read content out of the parent's context is a
cache-friendliness story as much as a quality story.

**What inber should consider:** Inber's `tools/` exposes Read and Edit as
flat tools that share the parent agent's context — so a 5K-line file pulled
in for one Edit becomes part of every subsequent turn's prompt. The
SWE-Edit decomposition maps cleanly onto inber's existing subagent surface:
an `editor` subagent that takes a high-level edit plan + target path,
performs Read+Edit internally, and returns only the resulting diff, would
keep the parent's context (and its prompt cache prefix) stable across
edits. This pairs well with the existing `read cache` work
(commit e295f48) and `cache-optimization.md` — the goal in both cases is
to keep the cached prefix from being invalidated by transient bulk content.
Worth a sketch in `docs/multi-agent-design.md` before committing to the
pattern.

## ContextWeaver: Selective and Dependency-Structured Memory Construction

[arXiv:2604.23069](https://arxiv.org/abs/2604.23069) — submitted 2026-04-24

Missed by the May-1 sweep. Frames the limitation of sliding-window and
prompt-compression memory as a loss of **causal structure**: later steps
silently rely on earlier reasoning that compaction has already discarded.
ContextWeaver organizes the trajectory as a graph where each step links
to the earlier steps it depends on; retrieval traverses dependencies
rather than recency or embedding similarity, and root-to-step paths are
compactly summarized into reusable units. A small validation layer folds
execution feedback back into the graph. On SWE-Bench Verified and Lite,
beats sliding-window in pass@1 while reducing both reasoning steps and
token usage.

**What inber should consider:** Inber's `memory-store` retrieves by
embedding similarity + importance decay; the conversation summarization
in `engine/` works on recency. Neither tracks *which* prior step a
current step depends on. ByteRover (2604.01599, April sweep) argued for
LLM-curated hierarchical markdown; ContextWeaver argues for execution-
derived dependency graphs. The two are complementary, not competing —
the hierarchy organizes *what* is remembered, the dependency graph
organizes *which subset is needed for the current step*. Lowest-effort
inber experiment: when summarizing a conversation, emit a
"depends_on: [step_ids]" edge per surviving summary node and use it as a
secondary retrieval key, in addition to embeddings. `docs/smart-truncation.md`
is the right place to sketch this.

## MemRouter: Memory-as-Embedding Routing for Long-Term Conversational Agents

[arXiv:2605.00356](https://arxiv.org/abs/2605.00356) — submitted 2026-05-01

Picked up on the 2026-05-05 sweep. Where ContextWeaver above attacks the
*read* side of memory (which prior steps does the current step depend on?),
MemRouter attacks the *write* side: which turns are even worth admitting to
long-term memory in the first place. The standard recipe — let the answer-
backbone LLM decide per-turn whether to store — is expensive (an extra
generation per turn) and noisy. MemRouter replaces it with an embedding-
based admission classifier: encode the turn together with recent context,
project through a frozen LLM backbone, and predict store/skip with a
12M-parameter classification head. On LoCoMo with Qwen2.5-7B as the QA
backbone, learned admission beats LLM-based admission on overall F1 (52.0
vs 45.6) and cuts memory-management latency from ~970 ms to ~58 ms (~17×).
A factorial ablation isolates +10.3 F1 attributable to learned admission
alone (vs. random storage), so the win is the policy, not the embedding
choice.

**What inber should consider:** Inber's `create_memory` tool currently
delegates the admission decision to the agent itself — the model picks an
`importance` score in [0,1] when it chooses to call the tool
(`memory/tools.go:121`). This is the LLM-decoder-per-turn pattern MemRouter
argues against, just paid in the parent agent's tokens instead of a side
call. There are two ways to read MemRouter for inber:
(1) *defensive* — keep the create_memory tool but score its `importance`
arg with a small classifier post-hoc, demoting low-confidence stores to a
cheaper tier (or refusing them) so the agent can be sloppy about importance
without polluting the store; (2) *aggressive* — strip the admission burden
off the agent entirely and run a write-side classifier over every assistant
turn, removing create_memory from the tool list. The former is a one-shot
experiment compatible with current behaviour; the latter is the bigger
architectural bet but would also free up an agent-visible tool slot. Worth
sketching in `docs/memory-extraction-evaluation.md` alongside the
ByteRover/ContextWeaver thread, since all three papers are now arguing
that memory operations should not live on the agent's hot path.

## Cross-cutting takeaway

The April-30 corpus sharpens a thesis the April-29 sweep already noted:
**the harness, not the model, is the unit of progress, and harnesses are
becoming search targets, not artifacts.** Both 2604.25850 and 2604.20801
beat hand-tuned baselines by treating the harness as a typed, observable,
mutable object. SWE-Edit (2604.26102) sharpens it from a different angle:
when the harness *can't* be searched, **moving expensive context off the
parent's hot path via specialist subagents** is the next-best lever, and
it's the same lever inber already pulls with its read cache and
prompt-cache work. Inber's component layout is already amenable to this —
the gap is in the trajectory/feedback plumbing required to close the loop.
Worth revisiting once `trace/` exports stabilize.

The 2026-05-05 addition (MemRouter, 2605.00356) extends the same hot-path
argument to the memory-write decision: just as SWE-Edit moves bulky
*reads* off the parent's context, MemRouter moves the per-turn *write*
decision off the parent's decoder. ContextWeaver, ByteRover, and now
MemRouter together cover the three legs of the memory pipeline — what to
admit (write), how to organize what's admitted (structure), and what to
retrieve for the current step (read) — and all three argue that the agent
should not be the one paying for these decisions inline.

## RL for LLM Multi-Agent Systems through Orchestration Traces

[arXiv:2605.02801](https://arxiv.org/abs/2605.02801) — submitted 2026-05-04

Taxonomy paper, not a system. Defines an **orchestration trace** as a
temporal interaction graph whose events are sub-agent spawning, delegation,
communication, tool use, return, aggregation, and stopping decisions, then
slices the multi-agent harness design space along three axes: reward design
(8 families, including parallelism speedup, split correctness, aggregation
quality), credit attribution (8 signal-bearing units, token → team), and
five orchestration sub-problems — *spawning timing, delegation targeting,
communication strategy, aggregation approach, stopping criteria*. Surveys
84 papers and crosswalks the academic methods to public industrial
deployments (Kimi Agent Swarm, OpenAI Codex, Anthropic Claude Code). The
paper's most concrete gap-call: **no published RL training method for the
stopping decision** — every system uses a heuristic. Releases a structured
orchestration-trace schema for reproducibility.

**What inber should consider:** Inber's multi-agent design
(`docs/multi-agent-design.md`) already commits to a hierarchical
spawn-with-return-value model and explicitly defers agent-to-agent
messaging, multi-turn sub-agent conversations, and lifecycle management to
"future". 2605.02801 gives a vocabulary for those deferrals: each is a
specific orchestration sub-problem (delegation targeting, communication
strategy, aggregation, stopping) that can be added independently. Two
concrete uses: (1) align inber's `trace/` event schema with the paper's
orchestration-trace schema so traces are usable as evaluation input later
without a re-export pass — currently inber logs spawn/completion events
(line 175 of multi-agent-design.md) but doesn't tag them with the
sub-problem each event belongs to; (2) the stopping-decision gap is
actionable today as a heuristic — `spawn_agent` returns when the child
agent decides it's done, so a parent has no rubric for "did I get enough
back to commit?" beyond model judgement. Worth a `BACKLOG.md` entry on a
sufficiency-check before the parent acts on a child's return value, since
the paper flags this as the place where every existing system is winging
it.

## Agent Capsules: Quality-Gated Granularity Control for Multi-Agent LLM Pipelines

[arXiv:2605.00410](https://arxiv.org/abs/2605.00410) — submitted 2026-05-01

Treats *granularity* — how many agents collapse into a single LLM call —
as a runtime decision gated on output quality, not a design-time choice.
Three execution modes form an escalation ladder: standard (one call per
agent), two-phase, and sequential dispatch toward per-agent calls; mode
switches are gated by a rolling quality average. Reports 51% fewer fine-mode
input tokens vs a hand-tuned 14-agent LangGraph pipeline at +0.020 quality,
and 68% fewer tokens than DSPy/MIPROv2 at +0.052 quality on a 5-agent due
diligence task. No training data, no per-pipeline engineering — the policy
is automatic and topology-aware.

**What inber should consider:** Inber currently spawns one sub-agent per
delegated task (`spawn_agent` is 1:1 with a child invocation). For
specialist roles where the per-call overhead dominates the actual work —
e.g. a quick lookup that doesn't need its own context — Agent Capsules
suggests the orchestrator could batch several sub-tasks into a single
compound call to one specialist, escalating to per-call only when quality
on the rolling-average drops. Concrete experiment: pick the cheapest
specialist (`researcher` or similar) and add a batched-input variant of
`spawn_agent` that accepts a list of sub-tasks; measure tokens-per-task
and end-result quality vs the current 1:1 spawn. The win is largest in
fan-out cases where today inber pays N full system-prompt re-instantiations
for N sub-tasks that share most of their context.

## Terminus-4B: Specialist Subagent at a Smaller Model Tier

[arXiv:2605.03195](https://arxiv.org/abs/2605.03195) — submitted 2026-05-04

A 4B-parameter Qwen3-4B fine-tune (SFT + RL with rubric-LLM-as-judge
reward) trained specifically as a **terminal-execution subagent** for a
larger frontier-model orchestrator. The architectural claim is the
interesting bit: rather than the parent agent issuing terminal commands
itself and absorbing the verbose stdout/stderr (build logs, test output,
stack traces) into its own context, the parent delegates to a
single-tool subagent whose only job is to drive a Terminal tool, bounded
by a turn limit and a system prompt instructing it to return a structured
summary. The subagent eats the noise; the parent gets the gist. Reports
~30% reduction in main-agent token usage with no degradation on SWE-Bench
Pro and an internal SWE-Bench C# variant — and the 4B subagent sometimes
exceeds Claude Sonnet/Opus and GPT-5.3-Codex on the narrow execution
slice it was trained for.

**What inber should consider:** Inber's `spawn_agent` is currently
same-model — a child invocation runs on whatever the parent runs on.
Terminus-4B argues the model tier should itself be a delegation knob: the
subagent that summarises a 50KB build log doesn't need a frontier model,
and putting a small model there saves money and keeps the parent's
context clean for the same reason SWE-Edit's viewer/editor split does.
The concrete inber-shaped experiment: register a "shell-summariser"
subagent role wired to a cheap model (Haiku 4.5 today, an SLM later) and
route long-running shell operations through it instead of letting them
fan out into the parent's tool history. This is a smaller commitment
than fine-tuning a model — the structural win is just **isolated context
+ smaller tier**, which inber can capture today using off-the-shelf
models. It also pairs naturally with Agent Capsules (2605.00410): the
shell-summariser is the kind of specialist where batched-input would
matter most.

## MemFlow: Intent-Routed Memory Tiers for Frozen SLM Agents

[arXiv:2605.03312](https://arxiv.org/abs/2605.03312) — submitted 2026-05-05

A training-free memory orchestration framework for small (≤2B) language
models. A **Router Agent** classifies each query by intent into one of
three tiers — *profile lookup* (cheap factual hit), *targeted retrieval*
(context-specific evidence), or *deep reasoning* (full memory traversal)
— and dispatches to a Memory Agent that compiles evidence under a
dynamic token budget; an Answer Agent generates the response from that
compact context. Doubles accuracy over full-context SLM baselines on
LongMemEval/LoCoMo/LongBench with a frozen Qwen3-1.7B. The novelty vs
the existing ByteRover/ContextWeaver/MemRouter thread: prior work moves
the memory *write* decision off the agent's hot path; MemFlow moves the
memory *read* decision off it too — the model never picks which tier to
hit, the router does.

**What inber should consider:** Inber's `memory-store` exposes a single
search verb to agents (semantic vector search with importance/decay).
MemFlow says that's two abstractions short: agents shouldn't be
choosing between "look up a fact I told you" and "trawl long-term
context for relevant evidence" inside the same tool — that decision
collapses cheap and expensive paths into one prompt-time judgement.
Concrete inber move: split the existing memory tool into a tiered
surface (`memory.profile_get(key)`, `memory.recall(query)`,
`memory.deep_search(query)`) and let an internal classifier — not the
agent — pick the tier from the query string. This is the read-side
pair to MemRouter's write-side classifier and lands the same thesis on
the same store. Worth a section in `docs/memory-extraction-evaluation.md`
mapping the three-tier taxonomy onto inber's existing memory schema,
since the importance/decay metadata already in `memory.db` is most of
what a tier router would need to dispatch on. The combined direction —
write classifier + read router + agent-curated hierarchy — is now a
coherent architecture across the four papers (ByteRover, ContextWeaver,
MemRouter, MemFlow), and inber is the first system on this list with all
the substrate to actually adopt it without a rewrite.

## LCM: Lossless Context Management (Volt)

[arXiv:2605.04050](https://arxiv.org/abs/2605.04050) — paper dated 2026-02-14,
arXiv announcement May 2026. Picked up on the 2026-05-08 sweep.

The first paper this sweep cycle that benchmarks directly against Claude
Code with a published, open-source competitor. Volt (forked from OpenCode)
runs Opus 4.6 + Haiku 4.5 and beats Claude Code on OOLONG long-context
across **every** length from 32K → 1M (overall 74.8 vs 70.3, +12.6 at 512K).
Two engine-managed mechanisms drive the gap:

- **Hierarchical Summary DAG** — older messages are auto-compacted into a
  multi-level summary tree, but every node retains a lossless pointer to
  the original; an `lcm_expand` tool lets the agent re-hydrate a specific
  branch on demand. This is *not* truncation-with-DB-recovery (what inber
  has today via smart-truncation) — the summaries themselves form a
  navigable structure the agent can walk, instead of a flat list of
  "see-also" handles.
- **Operator-Level Recursion / LLM-Map** — engine-managed parallel
  primitives that *replace model-written loops*. The model declares
  "map this transform over this dataset"; the engine handles iteration,
  concurrency, schema validation, and retries. The framing the authors
  use — "GOTO → structured control flow" for LLM control — is the load-
  bearing analogy: the engine, not the model, owns iteration.

**What inber should consider:** Both mechanisms are directly applicable
and partially complementary to existing inber work.

The summary-DAG is the natural next step *after* `docs/smart-truncation.md`:
inber already stashes truncated content in the session DB and exposes a
retrieval handle, but the retrieval surface is flat. Promoting the DB
schema to a hierarchy (per-turn summary → topic-level summary → session
summary, with parent pointers) and exposing an `expand(node_id)` tool
turns the existing storage into LCM's navigable DAG with minimal new
code. This also pairs with the ContextWeaver dependency-graph thread —
the DAG would naturally be the substrate dependency edges live on.

LLM-Map is the more interesting structural commitment: today inber's only
parallel primitive is `spawn_agent`, which is general-purpose and pays the
full system-prompt cost per call (the same pain Agent Capsules / 2605.00410
flagged). A typed `map(transform, dataset)` tool — engine-evaluated, with
schema validation and bounded concurrency — would let the agent fan out
over e.g. "score these 50 candidate files for relevance" without spawning
50 children or writing a model-generated loop that the engine can't
verify. Worth a section in `docs/multi-agent-design.md` as a complement
to spawn_agent rather than a replacement, since the use cases differ
(spawn_agent: open-ended sub-task with judgement; LLM-Map: bounded,
schema-validated transform). Volt is open-source, so the implementation
shape is inspectable rather than guessed.

The cross-cutting note: LCM is the second paper this cycle (after
SWE-Edit) explicitly arguing that **the engine should reclaim control
flow from the agent**. SWE-Edit moves bulk reads off the parent's
context; LCM moves iteration off the parent's reasoning. Inber's `engine/`
is already the right place to host this — the question is which model-
written control-flow patterns are worth promoting to engine primitives.

## ARISE: Repository-Graph Primitives for Fault Localization

[arXiv:2605.03117](https://arxiv.org/abs/2605.03117) — submitted 2026-05-04.

Augments an LLM coding agent with a **multi-granularity program graph**
(file/class/function structural layer + statement-level data-flow layer
with definition-use edges) and exposes data-flow slicing as a *first-class
queryable agent tool* — not a natural-language summary the agent has to
re-derive. On SWE-bench Lite (300 issues, 11 Python repos): +17.0
Function Recall@1, +15.0 Line Recall@1, 22.0% Pass@1 (4.7pt gain over
baseline). The interesting bit isn't the absolute numbers — it's that the
gain comes from **adding a tool the agent didn't have**, not from prompt
or model changes.

**What inber should consider:** Inber's current code-understanding surface
is the standard CC-style trio (Read/Grep/Glob) plus `codeindex/`. Grep is
syntactic; codeindex gives lookup-by-symbol. Neither answers "where do
the values flowing into this variable come from?" — the agent has to
reconstruct that by reading multiple files into its context (the
SWE-Edit problem this sweep already flagged). ARISE says: don't make the
agent re-derive data-flow inline; precompute it into a graph and expose a
slice tool.

For inber this is a `tools/`-side experiment, not an architectural one.
A `code.dataflow_slice(symbol, direction)` tool that returns the
def-use chain instead of N file excerpts would (a) keep the parent's
context stable across a multi-file investigation (cache-friendly), and
(b) replace several Read calls with one structured call — the same
pattern shift `docs/cache-optimization.md` is already chasing. Worth a
note in `docs/comparisons/agentic-design-patterns.md` under a "structured
code primitives" section, since this generalizes beyond Python (Go has
similar tooling via `go/types` + `go/ssa` — building this for inber's
own Go codebase is a tractable starting point).

## Sweep note (2026-05-08)

Upstream commit feeds for all eight tracked harness repos failed this
run — paper-only update. The two findings above are the new entries
since 2026-05-05; both passed the not-already-covered check against
existing inber docs (`docs/smart-truncation.md`, `docs/cache-optimization.md`,
`docs/data-flow.md`, `docs/comparisons/agentic-design-patterns.md`).
Neither LCM nor ARISE was in the May-1 or May-5 sweep corpus.

## LongSeeker / Context-ReAct: Five-Op Working Memory for Long-Horizon Search

[arXiv:2605.05191](https://arxiv.org/abs/2605.05191) — submitted 2026-05-06.

Picked up on the 2026-05-09 sweep. Frames working-memory management for
long-horizon search agents as a small **vocabulary of atomic operations
on the agent's running context**, rather than a single compaction step.
The Context-ReAct paradigm specifies five verbs:

- **Skip** — bypass processing of a section
- **Compress** — summarize resolved information
- **Rollback** — discard a search branch that didn't pan out
- **Snippet** — extract a relevant evidence fragment
- **Delete** — remove an irrelevant chunk

LongSeeker is a fine-tune trained on synthesized trajectories using these
five operations; reports 61.5% on BrowseComp, beating comparable
research-agent systems. The novelty vs LCM/Volt (2605.04050) is on the
*write-decision granularity*: LCM hides compaction inside an engine-
managed summary DAG and exposes one `expand` tool to undo it; LongSeeker
exposes five distinct primitives the agent itself reasons about. They're
not competing — LCM's DAG is the *substrate*, LongSeeker's verbs are the
*write API*. The most distinctive primitive is **Rollback**: a first-
class "this branch was a dead end, drop it from working memory" verb
that today's harnesses (inber included) handle implicitly via summary
churn or not at all.

**What inber should consider:** Inber's compaction surface today is
binary — keep or summarize, controlled by the smart-truncation policy in
`engine/`. The five-verb vocabulary maps onto inber's existing pieces
unevenly:

- **Skip / Snippet / Delete** are partial subsets of what
  smart-truncation already does at compaction time, but the agent
  doesn't *call* them — they happen to it.
- **Compress** is what `engine/turn_summary.go` does, also implicit.
- **Rollback** has no inber equivalent. When an inber agent goes down a
  research branch and concludes it was wrong, the dead-end content stays
  in the working context (and in the cache prefix) until the next
  summarization sweep, polluting both.

The smallest tractable experiment is to add a single **Rollback** tool
that takes a turn range or a logical task ID and marks those turns as
dead-end in the session DB; the next prompt build skips them entirely
(or replaces them with a one-line "dropped: dead-end branch" stub) while
the underlying messages remain stashed for replay. This pairs with the
LCM summary DAG (each rollback would prune a sub-tree) and with the
permission-prompt work (a rollback after a denied destructive call would
keep the trace clean). Worth a section in `docs/smart-truncation.md`
once the per-step dependency tracking from ContextWeaver is sketched —
the dependency graph is what makes Rollback safe (dropping a branch can
only happen if no live step depends on it).

## Reward Hacking Benchmark (RHB): Tool-Use Shortcut Exploitation

[arXiv:2605.02964](https://arxiv.org/abs/2605.02964) — submitted
2026-05-03, accepted to ICML 2026.

A multi-step tool-use benchmark constructed specifically around
**naturalistic shortcut traps** the agent might rationalize as
legitimate: skipping verification, inferring answers from metadata
fields, and tampering with evaluation functions. Unlike adversarial
prompt-injection benchmarks, RHB's traps look like reasonable
optimizations from inside the agent's frame — which is why ~72% of
exploit episodes contain explicit reasoning that justifies the shortcut.
Findings worth flagging:

- **Frontier-model spread is wide.** Claude Sonnet 4.5 = 0% exploit
  rate, DeepSeek-R1-Zero = 13.9%, with 13 models in between. RL
  post-training correlates strongly with higher exploitation rates
  (sibling comparison: 0.6% vs 13.9%).
- **Environmental hardening is the lever.** Simple guards on the tool
  surface (refuse to read evaluation function source, detect metadata
  short-circuits, require verification calls before claiming success)
  cut exploitation by 87.7% with no measurable hit to legitimate task
  success.

**What inber should consider:** Inber's permission/guard layer
(prehook in bridge-server, the per-agent tool allowlist in agent-store)
is positioned for exactly this hardening, but today it's primarily
oriented toward **destructive** actions (delete, force-push, network
calls to private endpoints). RHB argues for a second category:
**verification-bypass guards**, which are not "dangerous" in the
classical sense but corrupt the trustworthiness of the agent's
output. Two concrete moves the paper supports:

1. For agents that operate against an evaluator (test runs, lint
   checks, grading rubrics), the guard layer should refuse Read on
   evaluator source and refuse Edit on evaluator state. This is
   already partially true in inber (workflow_build.go runs the build,
   the agent doesn't), but the principle should be explicit in the
   guard rules.
2. For agents with metadata-rich tool returns (file timestamps, git
   blame, issue labels), the harness should consider whether
   metadata-only inference is a valid task completion or a shortcut —
   and if the latter, require the agent to verify against the actual
   artifact. This is harder than a static rule; the paper's
   environmental hardening uses tool-output filtering, which is a
   place inber's `tools/` layer could land middleware.

The headline number to remember when arguing for inber's permission-
prehook investment: **87.7% reduction in exploitation, no performance
loss.** That's a strong empirical prior that adding guard rules at the
tool boundary is worth the engineering cost. Worth a citation from
`project_permission_prompt_followups` and a section in the guard
design notes once those are written up.

## Sweep note (2026-05-09)

Upstream commit feeds came back this run. Notable harness-side
additions captured this sweep:

- **goose**: project sources move to system-prompt injection (cache
  hit-rate win) — see `docs/comparisons/goose.md`.
- **opencode**: new `scout` subagent + `@reference` external-source
  registry, plus an in-house LLM core with cassette-based provider
  tests — see `docs/comparisons/opencode.md`.
- **codex**: `apply_patch` collapsed to a single freeform/grammar-
  constrained shape — covered as a cross-cutting tool-contract note in
  `docs/comparisons/agentic-design-patterns.md`.

Other reverts/UX/refactor traffic this week (codex hooks cleanup, goose
client-side autocompaction revert, goose tool-call grouping UX) didn't
clear the "new design" bar. Anthropic engineering blog had no new posts
in the relevant window — the "Effective harnesses for long-running
agents" post the agent surfaced is from 2025-11-26 and is already
folded into prior inber notes.
