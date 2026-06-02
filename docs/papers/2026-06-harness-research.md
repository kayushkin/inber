# Harness Research — Early June 2026

Notes from the harness-watch sweep on 2026-06-02. Five arXiv papers from the
last ~30 days that passed the not-already-covered check against
`2026-05-harness-research.md` (no overlap with the May IDs).

## Code as Agent Harness: Toward Executable, Verifiable, and Stateful Agent Systems

[arXiv:2605.18747](https://arxiv.org/abs/2605.18747) — submitted 2026-05-18.

Position/taxonomy paper that re-centers *code* as the operational substrate of
a harness rather than just an agent output. Three-layer view: a harness
interface where code connects reasoning to actions and to an explicit
environment model; harness mechanisms (planning, memory, tool integration)
where the load-bearing claim is that **planning should be a filesystem-backed,
Git-versionable control object that subagents review and consume**; and scaling
via multi-agent coordination over shared, reviewable code artifacts. It's the
cleanest articulation this cycle of the "engine owns control flow / planning is
durable state" thesis that SWE-Edit and LCM (in the May doc) attack piecemeal.

**What inber should consider:** Inber's plan/state lives implicitly in the
transcript + `memory-store`. Have `spawn_agent` pass a path to a durable plan
file (in the session workspace, Git-tracked via forge worktrees) instead of
re-serializing plan context into each child prompt — the child amends the file,
the parent reads the diff. Planning state then survives compaction (pairs with
LCM summary-DAG), subagents get a shared coherence channel without
agent-to-agent messaging, and the cached prefix stays stable since the plan is
referenced by path, not inlined. Candidate section in `docs/multi-agent-design.md`.

## Coordination as an Architectural Layer for LLM-Based Multi-Agent Systems

[arXiv:2605.03310](https://arxiv.org/abs/2605.03310) — submitted 2026-05-05.

Argues coordination should be a separate, independently-configurable layer —
distinct from agent logic and information access — so failure modes can be
reasoned about systematically. Leads with: production multi-agent LLM systems
fail at 41-87%, and most failures are *coordination* defects, not base-model
capability gaps. The experimental design holds everything else fixed (one LLM,
fixed tools, fixed output cap, fixed prompt) and varies only the coordination
configuration across five layouts; two dominate the cost-quality Pareto
frontier. The methodological contribution — isolating coordination as the only
variable — is the takeaway, not the prediction task.

**What inber should consider:** Inber's coordination logic is entangled with
agent identity in `agent-store` and the spawn path in `engine/`. Define 2-3
named coordination profiles (e.g. flat-fanout vs hierarchical-with-aggregator)
selectable per session, and tag bus events with the active profile so the
dashboard can attribute outcome differences to coordination rather than model.
Complements the orchestration-trace schema work from 2605.02801 (May doc).

## AgentTrust: Runtime Safety Evaluation and Interception for AI Agent Tool Use

[arXiv:2605.04785](https://arxiv.org/abs/2605.04785) — submitted 2026-05-06
(AGPL-3.0, ships an MCP server).

The closest published mirror of inber's permission prehook. Intercepts tool
calls (file/shell/HTTP/DB) *before* execution and returns one of four
structured verdicts — **allow / warn / block / review** — not a binary. Four
components: a shell-deobfuscation normalizer (decodes base64/concat/hex tricks
before judging), SafeFix (suggests a safer alternative rather than just
blocking), RiskChain (detects multi-step attack sequences where each step is
individually benign), and a cache-aware LLM-as-Judge that only fires on
genuinely ambiguous inputs so most calls are decided by cheap deterministic
rules. 95.0% verdict accuracy at low-ms latency on 300 scenarios; ~93% on
shell-obfuscated payloads in the 630-scenario adversarial set.

**What inber should consider:** Three direct upgrades for the bridge-server
prehook (see `project_permission_prompt_followups`): (1) widen the verdict enum
from allow/deny to allow/warn/block/review — already aligned with inber's
status-enum-granularity principle; "review" maps onto the existing
human-in-the-loop prompt, "warn" lets suspicious-but-nondestructive calls
proceed flagged; (2) add a shell-deobfuscation normalizer in front of the
command matcher — a base64/concatenated `rm -rf` likely bypasses string rules
today; (3) adopt the cache-aware LLM-as-Judge so the prehook only pays for an
LLM call on ambiguous commands. RiskChain (cross-call sequence detection) is a
larger bet but the bus already carries the per-session call sequence to compute
it. Pairs with the RHB verification-bypass thread (2605.02964, May doc).

## Slipstream: Trajectory-Grounded Compaction Validation for Long-Horizon Agents

[arXiv:2605.08580](https://arxiv.org/abs/2605.08580) — submitted 2026-05-09.

Two coupled compaction problems: (a) synchronous compaction inflates
end-to-end time by 26-44% because the agent stalls while the summarizer runs,
and (b) compaction decides what to keep without knowing what comes next, so
lossy summaries silently drop load-bearing constraints. Slipstream runs
compaction **asynchronously off the execution path**: candidate summary and the
agent's next steps are generated independently from the same pre-compaction
state, then a **judge validates** that the condensed summary preserves the
agent's intentions/constraints before it is committed. Up to +8.8pp task
accuracy and -39.7% end-to-end latency vs synchronous compaction on SWE-bench
Verified + BrowseComp.

**What inber should consider:** Inber's compaction is server-side and
synchronous in `engine/turn_summary.go` — the agent waits, and the summary is
committed with no check it preserved what later turns rely on. Two
independently-adoptable wins: (1) kick compaction off asynchronously (the
scheduler/background-job substrate makes this tractable) and keep the agent
running against pre-compaction context until the validated summary is ready;
(2) add a validation gate — before swapping the live context to the summary,
run a cheap judge that checks it still entails the open constraints, falling
back to raw turns if it fails. This is the safety net that makes
ContextWeaver dependency-edges and the proposed Rollback verb (both May doc)
safe to act on. Candidate section in `docs/smart-truncation.md`.

## Parallel Context Compaction for Long-Horizon LLM Agent Serving

[arXiv:2605.23296](https://arxiv.org/abs/2605.23296) — submitted 2026-05-22.

Serving-side counterpart to Slipstream: standard compaction is one big blocking
summarization call that stalls inference for tens of seconds. This splits
compaction into **parallel per-block summarization**, giving fine-grained
control over summary volume and enabling **per-block prompt engineering**
(different blocks get different compaction prompts). Across four backbones
(8B-120B, dense+MoE) on HotpotQA + LoCoMo, at matched decode volume it cuts
end-to-end wall time and improves compaction throughput over the sequential
baseline.

**What inber should consider:** Inber summarizes the whole truncated span in a
single call. Chunk the span into blocks and summarize them in parallel with a
block-type-specific prompt (tool-output blocks get aggressive compaction,
reasoning blocks conservative). Maps naturally onto an LCM-style hierarchical
summary DAG (May doc) — each parallel block summary becomes a leaf under a
topic node. Combined with Slipstream the full picture is: compact per-block, in
parallel, off the hot path, then validate before commit. Fold into the same
`docs/smart-truncation.md` section as Slipstream.

## Cross-cutting takeaway (2026-06-02 sweep)

This cycle clusters hard around **compaction as a first-class, asynchronous,
validated subsystem** (Slipstream + Parallel Compaction) and **durable,
inspectable control state** (Code-as-Harness plan files + Coordination-as-layer).
Both reinforce the engine-owns-control-flow direction inber is already on. The
single most actionable item is the compaction pair: inber's `turn_summary.go` is
synchronous and monolithic today, and the two papers give an orthogonal
async-off-hot-path × parallel-per-block × validate-before-commit design that is
implementable on inber's existing background-job substrate. AgentTrust is the
most directly portable to the permission prehook (verdict enum widening +
deobfuscation normalizer + cache-aware judge).
