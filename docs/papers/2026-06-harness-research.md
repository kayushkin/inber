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

## 2026-06-04 sweep — skills as a context-budget problem

Two papers picked up alongside the harness-watch finding that four harnesses
shipped skills systems the same week (agentic-design-patterns.md, 2026-06-04).
Both are in-window (mid/late May) and converge on the same thesis: a large
skill/tool surface is a context-budget liability, so the harness must be
selective about what reaches the model.

### The Scaling Laws of Skills in LLM Agent Systems

[arXiv:2605.16508](https://arxiv.org/abs/2605.16508) — submitted 2026-05-15.

Empirical study across 15 frontier models and 1,141 skills: skill-routing
accuracy **decays logarithmically as the library grows**, and full skill text
(not just names/descriptions) carries the routing signal. Law-guided
organization of the library lifted routing from 71.3% → 91.7%.

**What inber should consider:** this is the quantitative case for codex's
per-turn catalog over a static global registry — putting the whole `SKILL.md`
library in front of the model measurably degrades selection as the library
grows. Inber should (a) cap what's resolved into any single turn (environment-
scoped per-turn catalog), and (b) keep enough skill *body* (not just a one-line
description) in the candidate set for the model to route correctly — a tension
with naive truncation. Pairs with goose's per-skill token accounting (goose.md
§3) as the budget meter.

### Tool-Schema Compression Enables Agentic RAG Under Constrained Context Budgets

[arXiv:2605.26165](https://arxiv.org/abs/2605.26165) — submitted 2026-05-24.

Shows tool/skill schemas themselves consume a large share of the context budget;
under an 8K-token cap, compressing the schemas lifted task accuracy from ~2.6%
to ~22%. The representation of the tool surface — not just which tools — is a
first-order cost.

**What inber should consider:** inber's tool inventory and SKILL.md descriptions
sit in the cacheable system-prompt prefix, so their *size* directly trades
against everything else in the window. Worth measuring the schema/description
footprint (goose-style token counts) and applying a compressed-schema rendering
for the long tail of rarely-used tools, expanding to full schema only when a tool
is actually selected for the turn — the read-side analogue of the per-turn skill
catalog above.

## 2026-06-05 sweep — harness as a measurable surface, spawn-boundary security, memory-as-workload, provider routing

Four new in-window arXiv papers (none overlapping the IDs above or in
`2026-05-harness-research.md`).

### Harness-Bench: Measuring Harness Effects across Models in Realistic Agent Workflows

[arXiv:2605.27922](https://arxiv.org/abs/2605.27922) — submitted 2026-05-27.

Empirical argument that "agent capability" is a property of a *model-harness
configuration*, not the base model, measured across 5,194 trajectories along
seven harness dimensions: context, tools, state, constraints, permissions,
tracing, recovery. Its sharpest finding is recurring **execution-alignment
failures** — plausible model reasoning decoupling from actual tool feedback,
workspace state, and verifiable output contracts. It names the exact design
surface inber's engine already owns and gives a vocabulary for ablating each
lever independently.

**What inber should consider:** report inber results as model-harness pairs (a
config, not a model), and add an explicit execution-alignment guard in the engine
loop that re-grounds the model on actual tool output / workspace state before
accepting a turn's conclusion — the loop-level complement to Slipstream's
compaction-validation check above.

### When Child Inherits: Modeling and Exploiting Subagent Spawn in Multi-Agent Networks

[arXiv:2605.08460](https://arxiv.org/abs/2605.08460) — submitted 2026-05-08.

Threat model for subagent spawning: a compromised parent propagates to children
via four vectors — insecure memory inheritance, weak resource control, stale
post-spawn state, and improper termination authority — with defenses based on
security invariants enforced at the spawn boundary. This is the security
counterpart to the Code-as-Harness "durable plan file passed to children" design
above: the moment spawn state becomes inheritable, its attack surface is inherited
too.

**What inber should consider:** treat the `spawn_agent` boundary as a privilege
boundary — sanitize/scope inherited memory + plan-file context, hand children
narrowed tool/resource leases (tie into auth-store leases), and keep termination
authority with the parent/engine, never the child (which echoes codex MAv2's
"workers may not close_agent themselves", agentic-design-patterns 2026-06-04).
Enforce it in the bridge-server permission prehook.

*Payload-confidentiality complement (codex [PR 26210](https://github.com/openai/codex/pull/26210), 2026-06-07):*
codex MAv2 now treats the inter-agent `message` arg of `spawn_agent`/`send_message`/`followup_task`
as opaque ciphertext that the orchestrator never sees in plaintext (encrypt/decrypt
happen server-side in the Responses API). A literal port isn't feasible for inber —
that mechanism only works because OpenAI controls both the encrypting model and the
decrypting server, and Anthropic exposes no equivalent encrypted-tool-arg primitive.
The transferable narrow lesson: inber's defenses above cover integrity/authority but
not *confidentiality of the payload itself*. When bridge-server persists subagent
spawn messages / plan-file context into rollouts and OTel telemetry, treat that
inter-agent task text as redact-at-rest rather than freely loggable (ties to
CONTEXT-MIGRATION + the spawn-boundary prehook).

### Is Agent Memory a Database? Rethinking Data Foundations for Long-Term AI Agent Memory (GEM)

[arXiv:2605.26252](https://arxiv.org/abs/2605.26252) — submitted 2026-05-25.

Argues record-level CRUD storage (what most agent memory, including a vector
store, effectively is) provably cannot satisfy four needs — bounded growth,
semantic revision, capacity-driven forgetting, and writeable (non-read-only)
retrieval — and proposes Governed Evolving Memory with four *state-level*
operators (ingestion, revision, forgetting, retrieval) on a property-graph
backend. Memory is a data-management workload, not a storage problem.

**What inber should consider:** inber's `memory-store` today is closer to
append + decay + read; add a **revision + governed-forgetting** path so stale or
superseded memories are semantically merged/retired rather than only
down-weighted — directly relevant to the memory-layer split work
(project_memory_layer_split) before it accretes contradictory facts.

### Latency-Quality Routing for Functionally Equivalent Tools in LLM Agents

[arXiv:2605.14241](https://arxiv.org/abs/2605.14241) (v2) — submitted 2026-05-14,
revised 2026-05-28.

Addresses routing *after* tool selection: when one tool interface is served by
multiple providers differing in latency/quality, rank by "quality per service
cycle" (latency as service capacity) via a contextual bandit with LLM-as-judge
feedback, rather than an additive speed-vs-quality reward. This is the layer below
inber's tool-selection logic.

**What inber should consider:** tool-store already abstracts one interface over
MCP/CLI/local implementations — route per-call by learned quality-per-latency
rather than a static preference order. Complements the tool-schema/skill-routing
papers (which decide *which interface*) by deciding *which provider* behind it.

## 2026-06-07 sweep — cacheable-prefix privacy, memory cost-profiling, plan-vs-execute diagnostics, structured retrieval

### CachePrune: Privacy-Aware and Fine-Grained KV Cache Sharing for Efficient LLM Inference

[arXiv:2605.23640](https://arxiv.org/abs/2605.23640) — submitted 2026-05-22.

Cross-user prefix-cache sharing leaks inputs through a reuse side channel, so most
systems disable sharing and lose the win. CachePrune does **token/span-level**
cacheability: a detector masks sensitive spans and only the privacy-irrelevant
segments (system instructions, public material) are made reusable — eliminating the
side channel while cutting TTFT 4.5× and raising cache hit rate 44% on vLLM.

**What inber should consider:** this is the formal version of inber's cacheable
system-prompt prefix strategy (CACHE-RULES / CC-VERIFIED). The shared prefix — tool
inventory + SKILL.md descriptions — is exactly the "privacy-irrelevant, safe to
share across sessions" segment, while per-session/user context is not. Making that
split explicit (a span-level "this is shareable prefix" boundary) lets inber
aggressively cache the common harness prefix across concurrent sessions without
leaking session context — pairs with the opencode session-scoped cache key
(comparisons/opencode 2026-06-06), which solves the *locality* side of the same
problem.

### Agent Memory: Characterization and System Implications of Stateful Long-Horizon Workloads

[arXiv:2606.06448](https://arxiv.org/abs/2606.06448) — submitted 2026-06-04.

First systems-level characterization of agent memory: a taxonomy along four axes
(flat retrieval, LLM-mediated extraction, consolidating fact stores, agentic control
flows) and a **phase-aware profiling harness** attributing cost to memory
*construction*, *retrieval*, and *generation*. Key finding: construction
(write/consolidate) — not just retrieval — is a first-order serving cost, and the
dominant cost center differs sharply by design.

**What inber should consider:** complements the already-documented GEM paper
(2605.26252) from the cost side — GEM says *what operators* memory needs, this says
*what they cost*. Add phase-aware (construction vs retrieval vs generation)
token/latency accounting to `memory-store` (bridge-server :8160), goose-style
per-phase meters, so the memory-layer-split work (`project_memory_layer_split`) can
choose a memory design by measured cost profile, not feature parity alone.

### Agent Planning Benchmark: A Diagnostic Framework for Planning Capabilities in LLM Agents

[arXiv:2606.04874](https://arxiv.org/abs/2606.04874) — submitted 2026-06-03.

A planning-specific diagnostic (4,209 cases, 22 domains, five settings) built because
end-to-end success can't separate *planning* failures from *execution* failures.
Notably includes robustness axes under **extraneous tools, broken tools, and
unsolvable tasks** (calibrated refusal), and shows APB-guided plan refinement lifts
downstream execution on ToolSandbox / τ²-bench.

**What inber should consider:** isolates the plan-vs-execute decoupling that
Harness-Bench (2605.27922) names but doesn't diagnose. Two concrete uses: (1) add
broken-tool / extraneous-tool / unsolvable-task cases to inber's own eval harness so
a tool-store regression surfaces as a planning-robustness drop, and (2) the
extraneous/broken-tool axis is the eval counterpart to the tool-schema-compression
and skill-routing threads — measure whether per-turn catalog trimming actually
improves robustness, not just token cost.

### Structure-Aware RAG: Structured Retrieval Augmented Generation from Noisy Data for Conversational Agents

[arXiv:2605.24366](https://arxiv.org/abs/2605.24366) — submitted 2026-05-23.

Argues text- and graph-RAG degrade on noisy/irrelevant retrieved context, and uses
**tables as an intermediate structured representation** that strips noise while
preserving load-bearing facts, plus a quality-aware table-metadata step. The
representation of retrieved context — not just retrieval recall — drives quality.

**What inber should consider:** a retrieval angle inber hasn't covered. For inber's
retrieval surfaces (noteboard search, skill-store SKILL.md lookup, memory-store
reads), consider rendering results as a normalized structured table before they
enter the cached context rather than raw concatenated text — same token budget, less
noise, and it dovetails with the tool-schema-compression principle that the
*rendering* of context is a first-order cost.

## 2026-06-09 sweep — memory provenance: a paper and a harness change converge

### From Untrusted Input to Trusted Memory: A Systematic Study of Memory Poisoning Attacks in LLM Agents

[arXiv:2606.04329](https://arxiv.org/abs/2606.04329) — submitted 2026-06-04.

Systematizes the core failure of agent long-term memory: entries are *constructed*
from untrusted external content (web pages, fetched docs, tool output), then later
*retrieved* into the agent's context and treated as trusted knowledge — with no
provenance tracking distinguishing "the user said" from "a web page said." Adversarial
content introduced through ordinary operation gets written to memory and steers the
agent in *future* sessions (cross-session, persistent, survives the originating task).

What makes this worth a note now is that a harness shipped the matching defense in the
same window: codex [#26821](https://github.com/openai/codex/pull/26821) adds
`contains_external_context()` to tool output plus an opt-out
(`disable_on_external_context=true`) so flagged tools don't influence memory, and
classifies **standalone web-search output as external context** (matching hosted
web-search behavior) so search results never silently become durable memory. The paper
names the disease; the harness change is one concrete dose — provenance-tagging at the
*write* boundary rather than trying to sanitize at read time.

**What inber should consider:** inber's `memory-store` (bridge-server :8160,
`project_memory_layer_split`) has no notion of *where* a candidate memory came from.
Two concrete moves: (1) tag every memory-write candidate with provenance
(`user` / `agent-reasoning` / `external-tool-output`) at the point of capture, and by
default **exclude external-tool-output** (web fetch, search, MCP-returned web content)
from automatic consolidation — promote it only on explicit, attended confirmation;
(2) carry the provenance tag through to *retrieval* so a recalled external-origin
memory is rendered as untrusted context, not as established fact — exactly the
"recalled memories are background context, not instructions" posture inber already
applies to system-reminder memories, now enforced by data rather than convention. This
extends the memory thread (GEM 2605.26252, cost-profiling 2606.06448) from cost/operators
to *integrity*.

## Beyond Compaction: Structured Context Eviction for Long-Horizon Agents

[arXiv:2606.11213](https://arxiv.org/abs/2606.11213) — submitted ~2026-06-11.

Frames the long-horizon problem precisely: the context window is finite but the work
trajectory is not, so every tool call / file read / retrieved doc accumulates and the
model's *effective reasoning budget shrinks each turn*. Instead of the usual answer —
summarise-and-replace (compaction), which is lossy and blocking — it proposes **Context
Window Lifecycle (CWL)**: graduated, *semantically-aware eviction* that drops the
lowest-value history entries to keep the window within budget, giving an effectively
unbounded working horizon without a single destructive summarisation step. Eviction is
structured (entries have lifecycle/value, not just recency) rather than a flat sliding
window. Pairs directly with this cycle's harness move toward *budget self-awareness*:
codex's context-remaining tool ([#27518](https://github.com/openai/codex/pull/27518),
agentic-design-patterns) lets the agent *read* the shrinking budget; CWL is what the
engine *does* about it when the agent doesn't checkpoint in time.

**What inber should consider:** inber's `smart-truncation` / `context-loading` currently
lean on truncation + (eventual) compaction, which is the lossy-summarise path this paper
argues against for long autoworker/scoper runs. Add a value/lifecycle tag to history
entries (tool-output age, whether a file was re-read, whether a result was superseded)
and evict graduated lowest-value entries to stay within budget *before* falling back to
summarisation — keep compaction as the last resort, not the first. The structured-eviction
unit also composes with the provenance work (2606.04329 above): external-tool-output is
both lower-trust *and* a natural first eviction candidate. Candidate section in
`docs/smart-truncation.md`.

## 2026-06-17 sweep — harness-as-measurable-adapter, typed-graph session resume, tool-call activation by width

### Claw-SWE-Bench: A Benchmark for Evaluating OpenClaw-style Agent Harnesses on Coding Tasks

[arXiv:2606.12344](https://arxiv.org/abs/2606.12344) (submitted 2026-06-10; repo
`opensquilla/claw-swe-bench`). A general-purpose agent doesn't natively satisfy SWE-bench's
clean-Docker-workspace + patch + prediction contract, so harnesses are hard to compare fairly.
Claw-SWE-Bench defines an **adapter protocol** (fixed prompt, runtime budget, workspace
contract, patch-extraction procedure, evaluator) over 350 multilingual instances (8 langs, 43
repos, from SWE-bench-Multilingual + Verified-Mini), plus an 80-instance cost/rank-aware Lite
subset. Headline: the same model+harness scores **19.1% Pass@1 with a bare adapter vs 73.4%
with the full adapter** — adapter/scaffold design, not the model, dominates. The patch is
always `git diff` taken by the runner *after the agent exits*, never agent-reported. This is
the eval counterpart to Harness-Bench (2605.27922, 06-05 sweep).

**What inber should consider:** this lands squarely on inber's `llm-bridge-*` harness/adapter
layer. Two concrete moves: (a) measure inber-via-llm-bridge as a *configuration* on a standard
adapter contract so harness changes are attributable, not confounded with model choice; (b)
adopt the "**extract the patch from `git diff` after exit, never trust agent-reported
diffs**" rule in the forge-worktree result path — it makes results tamper-resistant and aligns
with the verification-bypass / reward-hacking thread.

### TokenMizer: Graph-Structured Session Memory for Long-Horizon LLM Context Management

[arXiv:2606.06337](https://arxiv.org/abs/2606.06337) (Shweta Mishra, independent; 2026-06-04;
open-source `tokenmizer`). An open-source proxy that models session history as a **typed
knowledge graph** (14 node types, 7 edge types) instead of flat text, so resumable structure —
architectural decisions, task-status transitions, file-modification histories, resolved errors
— isn't silently lost when history exceeds the effective context window. An 8-layer compression
pipeline + semantic cache produce compact "resume blocks." This is the coding-session-flavoured
mechanism behind the memory-graph thread (GEM 2605.26252, 06-05 sweep) and complements CWL's
structured eviction (06-09).

**What inber should consider:** the session-resume use case maps onto inber's compaction/resume
path (`engine/turn_summary.go` + `memory-store`): emit a typed-graph "resume block"
(decision / file-history / task-transition nodes) as the durable session checkpoint rather than
a flat prose summary — recall of decisions survives a window overflow, and it composes with the
durable-plan-file (Code-as-Harness) idea and structured eviction (CWL).

### Pushing the Limits of LLM Tool Calling via Experiential Knowledge Integration and Activation (KATE)

[arXiv:2606.10875](https://arxiv.org/abs/2606.10875) (CAS / UCAS; 2026-06-09). Multi-step
tool-use failures stem from insufficient tool-related knowledge *and* poor activation of it.
Two findings are actionable without retraining: instance-level knowledge ("here's a worked
example of this tool") beats abstract intent-level guidance; and at inference, **expanding the
*width* of reasoning (parallel sampling + aggregation) activates latent experiential knowledge
better than expanding *depth* (longer single chains)**, which shows diminishing returns. KATE
also adds knowledge-aware post-training (RL > SFT), but the width-over-depth and
instance-over-abstract levers need no training.

**What inber should consider:** for inber's harder tool-routing/tool-call decisions, a small
parallel-sample-and-aggregate step over candidate calls may beat longer single-chain reasoning
— cheap to prototype in the engine's tool-selection path. And when inber injects tool guidance
(tool-store descriptions, skill bodies), prefer a concrete worked example over abstract intent
text. Pairs with the tool-routing-decay / schema-compression papers (2605.16508, 2606 sweeps):
those decide *which* tool surface reaches the model; KATE is about reliably *activating* the
right call once it's there.

## 2026-06-18 sweep — cache-continuity as a compaction constraint, harness recursion

### TokenPilot: Cache-Efficient Context Management for LLM Agents

[arXiv:2606.17016](https://arxiv.org/abs/2606.17016) (Zhejiang Univ. et al.; submitted
2026-06-16). Names the trade-off the whole compaction/eviction thread has been dancing around:
text pruning and dynamic memory eviction shrink the token footprint, but their *unconstrained
sequence mutations alter the prompt layout, causing prefix mismatches that invalidate the
prompt cache* — so you can spend more on cache misses than you save on tokens. TokenPilot keeps
both granularities cache-aware: (a) **Ingestion-Aware Compaction** stabilizes the prompt prefix
and strips open-world environmental noise *at the ingestion gate* (before it ever enters the
cached prefix), and (b) **Lifecycle-Aware Eviction** tracks each segment's residual utility and
offloads only on a *conservative batch-turn schedule* so evictions don't constantly break the
prefix. Reports 61% / 56% cost reductions on PinchBench and Claw-Eval.

**What inber should consider:** this is the missing constraint on inber's compaction/eviction
path (`engine/turn_summary.go`, structured eviction per CWL 2606.11213). inber leans hard on
prompt caching (the ScheduleWakeup cache-TTL economics are the same), so any mid-prefix edit —
dropping a stale tool result, rewriting an early summary — silently invalidates the cached
prefix from that point on, and the cache-miss cost can exceed the token saving. Two rules to
adopt: (1) do lossy compaction **at the ingestion gate** (when a tool result first arrives) so
the durable prefix is born clean and is never mutated afterward; (2) **batch evictions on a turn
schedule** rather than per-turn, and only ever evict from the *tail* — never rewrite the cached
head. Make "does this edit touch the cached prefix?" an explicit check before any compaction
write.

### Recursive Agent Harnesses

[arXiv:2606.13643](https://arxiv.org/abs/2606.13643) (submitted 2026-06-11). Generalizes the RLM
idea (model recursion — see [llm-gateway-rlm](../comparisons/llm-gateway-rlm.md)) to **harness
recursion**: the recursive unit is no longer a bare model call but a *full agent harness* with
filesystem, code execution, and planning. A parent agent **writes and runs an executable script
that spawns subagent harnesses in parallel** for fine-grained fan-out, and falls back to ordinary
structured function calls for small subtasks. The paper explicitly frames this as the pattern
behind Anthropic's "dynamic workflows."

**What inber should consider:** this is the formal name for the code-orchestrates-subagents shape
inber already touches via the Workflow tool (a script body fanning out `agent()` calls). The
paper's load-bearing claim is the *dispatch heuristic* — spawn a full recursive harness only for
fine-grained parallel workloads; use a plain function/tool call for small subtasks — because a
recursive harness carries real fixed overhead (its own context, tools, planning loop). inber's
orchestration layer (kanban task-completion-loop, autoworker) should make that the explicit
routing rule: decompose-and-fan-out to child harnesses when a subtask is independent and
parallelizable, inline tool calls otherwise, rather than spawning a session per sub-step by
default.

## 2026-06-19 sweep — ownership-typed token budgets as a defense against multi-agent overrun

### Token Budgets: An Empirical Catalog of 63 LLM-Agent Budget-Overrun Incidents, with an Affine-Typed Rust Mitigation

[arXiv:2606.04056](https://arxiv.org/abs/2606.04056)

Catalogs 63 confirmed production budget-overrun incidents across 21 orchestration frameworks
(2023–2026), each backed by a quoted GitHub issue, organized into an eight-cluster failure
taxonomy (plus 47 supplementary structural entries). The recurring root cause is that a token /
cost budget is treated as a plain mutable number passed around between threads — so it gets
**double-spent** (two subagents debit the same allowance concurrently), **cloned** (a budget
copied into a fan-out, each branch spending the full amount), or **used after delegation** (a
parent keeps spending a budget it already handed to a child). The mitigation is `token-budgets`,
a Rust crate that models a budget as an **affine resource**: cloning it, spending it twice, or
using it after it's been delegated are *compile errors*, not runtime overruns.

**Why it matters for inber / what to consider:** this is the failure-mode companion to the Codex
shared rollout-ledger finding (`docs/comparisons/agentic-design-patterns.md`, 06-19 entry) and to
inber's own Workflow shared token pool (`budget.spent()` is cross-agent across the main loop and
every sub-workflow). The pool is exactly the shape the paper says fails: a single allowance many
concurrent agents debit. inber doesn't need Rust affine types, but it should borrow the *invariant*
— a delegated budget slice must be **owned by exactly one agent at a time**: when a workflow fans
out, partition the remaining pool into explicit per-branch slices (or a single serialized ledger
with atomic debits) rather than letting N agents read-then-spend the same `remaining()` value, and
treat "parent spends after handing a slice to a child" as a bug to assert against. Cheapest concrete
step: make the cross-agent debit atomic and log any debit that would cross zero, so an overrun is an
observable event rather than a silent ceiling breach.

## 2026-06-22 sweep — keep less context on purpose; make the context manager a separate component

### Less Context, Better Agents: Efficient Context Engineering for Long-Horizon Tool-Using LLM Agents

[arXiv:2606.10209](https://arxiv.org/abs/2606.10209) (submitted 2026-06-08).

On a 50-task tool-using benchmark (GPT-5 over MCP tools), feeding the **full** history scored 71.0%
task completion; **pruning to the last 5 tool call/response pairs** raised it to 79.0%; adding
automated summarization of the dropped middle reached **91.6% at lower token cost**. The mechanism
is blunt and empirical: verbose tool responses both overflow the window and inject *stale state*
(an old directory listing, a superseded file read) that the model acts on — pruning removes the
stale evidence as a side effect, and summarization preserves the durable conclusions.

**What inber should consider:** this is direct evidence that inber's compaction should be a *default*
policy, not a last-resort overflow trigger. The "keep last N tool exchanges verbatim + summarize the
rest" recipe (N≈5) is cheap to A/B against inber's current retention and plausibly improves *both*
reliability and cost — the reliability win comes from evicting stale tool state, which a window that
never compacts keeps re-surfacing. Pairs with the 06-09 *Beyond Compaction* structured-eviction
finding (`agentic-design-patterns`): both say un-pruned tool output is an active correctness hazard,
not just a token-budget one. Concrete step: measure inber autoworker/scoper runs with and without an
always-on last-N+summarize pass before treating full-history as the safe default.

### Learning Agent-Compatible Context Management for Long-Horizon Tasks (AdaCoM)

[arXiv:2605.30785](https://arxiv.org/abs/2605.30785) (submitted 2026-05-29).

Trains a **separate external LLM** (end-to-end RL) to manage a *frozen* agent's context via flexible
modify/summarize/drop actions, rather than retraining the agent or hard-coding one fixed
summarization rule. The target is exactly the closed-model setting inber lives in — you can't
fine-tune the policy model (Claude via API / `claude -p`), and different models/tasks want different
context strategies — so the manager is a decoupled component you *can* tune.

**What inber should consider:** model compaction as a **pluggable context-manager component decoupled
from the harness and the agent**, so the strategy can be swapped or tuned per model/task without
touching the agent loop — a clean fit for inber's harness-layer separation
(`project_harness_layer_design`, the CONTEXT-MIGRATION doc). Even without RL, the architectural
takeaway holds: don't bake one summarization rule into the engine; make it a named strategy object
the harness selects, so the last-N+summarize recipe above and a structured-eviction recipe can
coexist and be A/B-tested behind one seam.

---

## Harness-watch addendum — 2026-07-01 sweep

### TokenPilot: Cache-Efficient Context Management for LLM Agents

[arXiv:2606.17016](https://arxiv.org/abs/2606.17016) (June 2026).

Argues that the usual context-shrinking tactics — text pruning, dynamic memory eviction — quietly
*hurt* because their "unconstrained sequence mutations" reorder the prompt, causing prefix
mismatches and cache invalidation: you save tokens on paper and lose the cache in practice.
TokenPilot is dual-granularity: (1) **Ingestion-Aware Compaction** at the harness's ingestion gate,
which stabilizes the prompt prefix and strips open-world environmental noise *before* it enters
context; and (2) **Lifecycle-Aware Eviction**, which tracks each segment's residual utility and
offloads it on a **conservative batch-turn schedule** rather than per-turn — so evictions don't
churn the prefix every step. Reported ~56–61% cost reduction with no task-quality loss.

**What inber should consider:** inber's `smart-truncation.md` / memory-eviction path evicts on a
per-request budget check, which is precisely the "mutate the sequence every turn" pattern TokenPilot
warns invalidates the cache. Adopt the **batch-turn eviction cadence**: only re-shape context every
K turns (or on a compaction boundary), so the cacheable prefix stays byte-stable between reshapes
and the eviction win isn't cancelled by cache re-creation. Directly compounds with the goose #10030
finding (`comparisons/goose.md`, 07-01) and the AdaCoM pluggable-context-manager takeaway above:
make "when to reshape" an explicit, cadence-controlled policy, not an every-turn side effect.
