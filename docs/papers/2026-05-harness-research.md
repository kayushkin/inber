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
