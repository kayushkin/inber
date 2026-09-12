# Harness Research — September 2026

Notes from the September harness-watch sweeps. Every paper here passed the
not-already-covered check against `2026-04/05/06/07/08-harness-research.md` and
`docs/comparisons/agentic-design-patterns.md` — 415 distinct arXiv ids extracted
and machine-checked — and every title, date and number below was read off
`arxiv.org/abs/` rather than a search snippet. Search results routinely surface
out-of-window work with in-window-looking ids; two of the four searches run this
sweep returned 2602–2606 papers for a "last 30 days" query.

# 2026-09-01 sweep

Screened the `cs.SE`, `cs.MA` and `cs.AI` recent listings for 2026-08-26 → 2026-09-01.
Eight new, seven of them carrying numbers. The arXiv Atom API answered `429 Rate
exceeded` for every attempt from this host across a 90-second backoff, so the
listing pages were read instead — noted because it changes what "screened" covers:
the listing pages are per-category and recent-only, where the API query was
keyword-scoped across all categories.

## 1. Delivery is where the failures are, not capability

[arXiv:2608.29128](https://arxiv.org/abs/2608.29128) — **APIFlow-Bench: Measuring Whether Agents
Survive Long, Dependent API Workflows** (2026-08-29). 19 frontier and open-weight models under one
neutral scaffold, 44,362 released execution transcripts, deterministic provenance-sensitive
grading (a mock-minted canary is traced through the API data flow to the response the answer must
originate from).

Three results, in descending order of how much they should change what this repo does:

- **77% of failing runs on the clean slice reached the correct final state and failed only at
  delivery.** The work was done; the answer did not arrive.
- **The independent-error account of compounding failure does not fit the data.** Pass rates on
  20-subtask chains are **33 percentage points above** the product of subtask-level rates. Chains
  do not fail like independent coin flips, so a reliability budget derived by multiplying
  per-step rates is wrong in the safe direction and will be wrong in the unsafe direction for
  some other harness.
- **Reliability separates models far more than best-case capability.** Best-of-five spans seven
  points across the 19 models; all-five-of-five spans **44**.

Success itself degrades with chain length: 93% on individual subtasks → 74% on clean 20-subtask
chains → 61% including the 8% of trials a model-consensus screen flags as passed by no model.

**What inber should consider:** this is the research form of two findings filed the same night —
a stopped parent restarted by a child result that arrived correctly
(`agentic-design-patterns.md`, 2026-09-01 §1) and an assistant message that survives a filter with
zero content blocks (§2). Both are delivery failures sitting on top of completed work. The paper's
sharper point is methodological: a harness that reports one bit per run cannot tell those apart
from a failure to do the work, and inber's `requests.status` currently reports roughly that
(`a2152cb9`).

## 2. Equal token budgets are not equal delivered context

[arXiv:2608.31057](https://arxiv.org/abs/2608.31057) — **Measure Before You Manage: Evaluating
Agent Working Memory in Coding Agents** (2026-08-31). 55 archived coding-agent trajectories.

Working memory is *semantically heterogeneous* — instructions, artifacts, tool outputs and
agent-generated state have different size, retention and representation profiles — and the paper
shows they compress and are retained differently in practice. Two findings are the ones to carry:

- **Calibration gains may not transfer to held-out tasks.** A compression policy tuned on one task
  set does not keep its win.
- **Equal token budgets do not imply equal delivered context or equal management cost**, and a
  real-system replay exposes serving limits that nominal budgets do not capture.

It proposes four evaluation levels — stored state, delivered context, management work, task
outcome — as distinct things a memory strategy must be scored on separately.

**What inber should consider:** this is the direct caution against the obvious fix for the
`TokenBudget` split recorded in `agentic-design-patterns.md` (2026-09-01 §5) — moving the number
onto the model row makes it *correct*, not *sufficient*. `conversation/manage_config.go`'s
`TokenBudget` and `engine/build.go:120-121`'s `contextWindow / 2` both name stored state; neither
names delivered context, and `conversation/manage_text_utils.go:182-188` already records that
`EstimateRequestTokens` (which counts system + tools) is deliberately not wired into the prune
gates. That open decision is exactly the paper's level-1-vs-level-2 distinction.

## 3. A single-event repair hypothesis cannot find a jointly-necessary repair

[arXiv:2608.29228](https://arxiv.org/abs/2608.29228) — **Localizing Emergent Failures in Agentic
AI: Recovering Minimal Repair Families via Counterfactual Replay** (2026-08-29).

Formalizes **Minimal Repair Family Recovery**: recover *all* inclusion-minimal event sets whose
counterfactual replay restores success, rather than attributing blame to one event. Graph-
Constrained Joint Replay slices failure-relevant events from an execution dependency graph, builds
graph-feasible singleton *and pair* candidates, and verifies by replay against paired clean
counterparts.

On 90 in-scope cases from a 120-DAG controlled benchmark: **1.000 Family Exact Match**, mean replay
calls **56.3 → 25.3 (55.1% fewer)** against exhaustive search. On a 24-case four-agent LLM pilot:
again 1.000, model calls **21.0 → 10.0 (52.4% fewer)**. The load-bearing negative: **single-event
replay misses jointly necessary repairs.**

**What inber should consider:** pointwise attribution is what this repo's harness-watch does by
construction — one bullet, one `file:line`. The 2026-09-01 §1 and §2 findings are each singleton
repairs, but §1's fix explicitly needs a second decision about `Session.turn`'s entry gate
(`7de193b1`) to be complete, and §2's needs `7c6a0ee4` untouched to still leave a working system.
That is a repair *family* of two written down as two todos, and nothing in the queue records that
they are jointly necessary. Cheap version of the paper's idea: when a finding's fix requires
another open todo, say so in the body — three of tonight's four do.

## 4. Detect before you attribute

[arXiv:2608.29646](https://arxiv.org/abs/2608.29646) — **Detect Before You Attribute: Cascade
Failure Attribution for Multi-Agent Systems** (2026-08-30).

DUOTRACE is a plug-and-play *filter* in front of LLM-based failure attribution: detect anomalous
executions first with a VAE over dual-view semantic-structural node representations and a
Tree-LSTM trajectory encoder, then hand only the focused evidence downstream. The premise is that
LLM attribution degrades on long trajectories, so the win comes from shortening what the attributor
reads rather than improving the attributor. Across six LLM-based attribution baselines:
**+8.7% agent-level and +7.0% step-level attribution accuracy**.

**What inber should consider:** a modest gain, and the reason it is here is the *shape* — the same
"narrow the input before the expensive reader" move as
[2608.28027](https://arxiv.org/abs/2608.28027)'s progressive tool disclosure recorded last month.
Directionally relevant to any future automated triage over `session.jsonl`; not actionable today,
since inber has no attribution step to put a filter in front of.

## 5. Repository exploration is a separately budgetable stage

[arXiv:2608.29675](https://arxiv.org/abs/2608.29675) — **Cost-Effective Repository Exploration for
Agentic Issue Localization** (2026-08-30). Five explorer models under one read-only interactive
interface, on 499 SWE-bench-Verified-derived tasks plus 500 tasks from 153 further repositories,
with paired instance-level uncertainty and repository-clustered sensitivity analysis.

Lower-cost explorers retain **78–94% of the reference Hit@3** and **73–92% of its F1** while cutting
mean agent time **41–88%** and token usage **84–95%**. The paper's own qualifier is the useful part:
the preferred operating point **depends on the downstream handoff contract** — ranking and coverage
metrics characterize a recoverable candidate handoff, F1 and exact match characterize a restrictive
file gate. Same explorer, different right answer depending on what consumes it.

**What inber should consider:** this is the empirical case for a cheap model on the sub-agent
exploration leg, and simultaneously the argument against setting it globally. inber picks a
sub-agent's model from config (`2dcdb9a6`) with no notion of what the child's output feeds; the
paper says that notion is precisely what determines whether the cheap model is free or costly.

## 6. Coding agents solve half of real dependency upgrades

[arXiv:2608.30300](https://arxiv.org/abs/2608.30300) — **Update from Hell: Can Coding Agents
Survive Hidden Breakage in Dependency Upgrades?** (2026-08-31). DEPBENCH: **203 real-world
dependency-upgrade tasks across five package ecosystems and five language communities**, each
carrying hidden code-level changes that require source adaptation.

The best *completed* configuration solves **104/203 (51.2%)**, with substantial variation across
agent harnesses, models and ecosystems — the harness is named as a source of variance alongside the
model, which is unusual and is the reason this is recorded rather than filed under benchmarks.

**What inber should consider:** nothing directly. Logged as a baseline number for the class of task
the nightly `repo-*` guard jobs sit next to, and as one more data point that harness choice moves
scores independently of model choice — the premise this whole doc set runs on, measured.

## 7. 64 million log entries of what a production agent actually does

[arXiv:2608.29204](https://arxiv.org/abs/2608.29204) — **AgentLogs: A Dataset for Opening the Black
Box of GitHub's Cloud Agent** (2026-08-29). **307,416 agent tasks, 549,239 sessions across 35,810
of 1,812,362 scanned public repositories, and 64,255,174 session log entries** recording prompts,
intermediate reasoning, tool calls (file edits, git operations, GitHub interactions) and token
usage, step by step.

No findings — it is a dataset release. Recorded because it is the first public corpus of *process*
rather than outcome at this scale, and every question this doc set answers by measuring
`~/.inber/server/server.db` (265 rows, 100 sessions) has an external comparison point now. The
cache-gap distribution measured on 2026-08-31, which came from a single 166-request session and was
caveated as such, is the obvious first thing to check against it.

## 8. Compressing a skill bundle without flattening it

[arXiv:2608.30785](https://arxiv.org/abs/2608.30785) — **SkillZip Pro: Execution-Aware Dynamic
Compression of Progressively Loaded Skills for Self-Evolving Agents** (2026-08-31).

Starts from the observation that a production skill is a *directory bundle* with progressive
loading — root at activation, references/schemas/scripts/subskills only when an execution path
needs them — so compressing the root misses most of the cost and can move branch-specific detail
into always-loaded context, while flattening destroys the loading boundaries outright. Two
safeguards: compress *across* files (drop content a reference repeats from the root or a declared
environment contract), and preserve routing so every required file and callable entry stays
reachable.

On a production content-moderation skill: **38% of bundle tokens and 10.4% of end-to-end per-run
tokens removed with no quality loss** — and, the number that matters, an **unprotected 71%
configuration loses up to 26 accuracy points** to one-sided false positives.

**What inber should consider:** the same asymmetry as
[2608.28027](https://arxiv.org/abs/2608.28027) — a bounded compression is free and an unbounded one
is catastrophic, with the cliff between them nowhere near the middle. Relevant to skill-store's
`SKILL.md`-plus-scripts model rather than to inber directly, and to any future "shorten the skill
before injecting it" idea here.

## 9. Screened and rejected, with the reason

- [2608.29596](https://arxiv.org/abs/2608.29596) **Towards a Systems Foundation for Agentic
  Skills** (08-30) — a nine-stage lifecycle for the skills ecosystem (discovery, authoring,
  storage, retrieval, composition, execution, adaptation, evaluation, security governance) and a
  survey of marketplace and threat dynamics. Genuinely on-topic for skill-store, but a position
  paper: **no empirical result of any kind**.
- [2608.30701](https://arxiv.org/abs/2608.30701) **A Phased Workflow for Operating LLM-Based Coding
  Agents** (08-31) — an industry practitioner report from Infobip, four phases with human effort
  front-loaded and delegation increasing as artifacts mature. Two pages, no numbers, and its own
  named open problem is that no metric for workflow effectiveness exists. Its one transferable
  observation — upstream errors in research and planning compound across later phases — is already
  the premise of this repo's plan-before-edit path.
- [2608.30757](https://arxiv.org/abs/2608.30757) **Which Rules Matter Now? Policy-Centroid Routing**
  (08-31) — routes a proposed action to the policy regimes it may implicate before adjudication,
  which is the right shape for a permission layer. Explicitly **"reports no empirical efficacy
  result"**; six propositions and seven proposed studies. Re-check when the studies land.
- [2608.26225](https://arxiv.org/abs/2608.26225) **Agent Mesh** — the closest paper in the listing
  to tonight's §1, and **already recorded** in `2026-08-harness-research.md`. Re-fetched to
  confirm, not re-logged.

# 2026-09-02 sweep

~150 distinct arXiv ids screened across the `cs.SE`, `cs.MA`, `cs.AI` and `cs.CL` recent
listings plus four keyword searches; 20 dropped as already filed in `2026-04` through
`2026-09`. Eight new, all eight carrying numbers.

⚠️ **Two method warnings, both worse than last sweep's.** The arXiv Atom API answered
`429`/`503` for every attempt from this host, as on 2026-09-01 — two sweeps running, so
treat it as unavailable and use the per-category listing pages. Worse: a `cs.AI/new`
listing fetch returned **hallucinated id-to-title mappings**, reporting `2609.00035` as
"Conversation Coach" and `2609.00023` as "Invalidation Contracts for Cross-Episode Agent
Memory"; neither is the real paper. Every id below was re-read at `arxiv.org/abs/` and the
listing titles discarded. **Do not report a paper from a listing or search snippet.**
`2609.00035` was independently re-fetched at `/abs/` by the parent job before acting on it.

## 1. A tool vocabulary stated only in prose is missed by every model, every time

[arXiv:2609.00035](https://arxiv.org/abs/2609.00035) — **SilentProbe: Measuring Silent
Failure in Production APIs Used as Agent Tools** (2026-08-29). 721,320 parameters across
2,501 OpenAPI documents: 7.5% declare an enum, 15.2% declare any machine-checkable
constraint, 40.1% state a constraint in prose the schema does not encode. Against live
endpoints, machine-checkable constraints gave an honest error **111/111**; prose-only
constraints failed silently **44/61** (p = 2e-13). A vocabulary the description merely
*exemplifies* was missed **88/88**; written out in full it was used correctly 88–91%.
Promoting it into the schema took 88/88 → **0/89**. In the loop, models detected the silent
failure 12% of the time, repaired it **0%**, and asserted a false negative to the user 41%.

- **What inber should consider:** `grep '"enum"'` across the tree returns **two** hits, one
  of them a test — production has exactly one, `agent/chain.go:50`. Every other constrained
  vocabulary is prose in a `description`. Measured this pass, inber is *not* in the
  88/88 bucket: `server/spawn_tools.go:72` builds `"Agent name to spawn. Available: %v"`
  from the full sorted list, which is the 88–91% bucket. The residual 9–12% is what an
  `enum` closes, and it lands unvalidated because the agent-name check is off (todo
  `a6fceed2`). The fix is one line beside the description — and the sort at
  `spawn_tools.go:71` is already deterministic, so it will not move the cache anchor.
- Companion, same class: [arXiv:2609.00072](https://arxiv.org/abs/2609.00072) (*Can MCP
  Clients Decide What to Do After Failure?*, 08-31) — across 21 induced MCP failures, typed
  fields expose *that* it failed but never a specific cause, target, repair or replay
  constraint. Do not build retry logic that assumes `isError:true` carries a reason.

## 2. Source-code anatomy of eleven production harnesses

[arXiv:2609.00006](https://arxiv.org/abs/2609.00006) — **Harness Engineering: Anatomy,
Architecture, and Evolution of Coding Agents** (announced 2026-09-02). Reads the source of
Claude Code, Codex CLI, Gemini CLI, Mistral Vibe, OpenHands, Aider, Mini-SWE-Agent, Hermes,
Pi, OpenCode and OpenClaw. Seven canonical subsystems, **29 recurring design patterns**, 18
recommendations, a 90-line minimum-viable-harness scaffold. Two absences hold across ~4M
lines: **no runtime imports a general-purpose agentic framework, and none retrieves code
with vector embeddings.** `SKILL.md` skills lead MCP 9/11 vs 8/11. A controlled one-quarter
diff shows behavioural policy migrating from prompt prose into configuration.

- **What inber should consider:** the 29-pattern catalogue is the largest external set
  `docs/comparisons/agentic-design-patterns.md` has had to check itself against — read it
  as a coverage audit of that file, not as a bullet. The no-embeddings finding validates
  `codeindex/`; the prose→config migration is an audit target for `engine/build.go`.

## 3. Fold the trace into typed state; don't re-read it

[arXiv:2609.01466](https://arxiv.org/abs/2609.01466) — **Parsing the Stream: A Live Trace
Model for Long-Horizon Agents and Their Observers** (2026-09-01). An append-only ledger
folded incrementally into typed state, compiled into per-consumer views. Monitoring
questions answered with **14–15x fewer input tokens at 5–7x lower cost, accuracy 0.85–0.87
versus 0.48** for a budget-capped single read of the raw trace. On 120-link
sequential-dependency tasks, keeping the running statistic in per-step state succeeds
**30/30 against 8/30** for full-context prompting. Honest ablation: a prompt-level
scratchpad matches the accuracy more cheaply, so the fold's residual value is deterministic
auditability plus serving both consumers from one state.

- **What inber should consider:** build the fold **only** if the same typed state serves
  compaction *and* the observer — for compaction alone it loses to a scratchpad. That
  scopes `trace/trace.go` (a single file) against `conversation/staged.go` and
  `conversation/stash.go`. The 0.85-vs-0.48 gap is the number justifying a compiled view
  anywhere in `server/` that re-reads a raw trace.

## 4. Keeping the model's own reasoning in context is what carries state

[arXiv:2609.00012](https://arxiv.org/abs/2609.00012) — **Long-Horizon State Tracking in
LLMs: Executing MD5 through a Deep Sequence of Dependent Tool Calls** (2026-08-02, announced
in-window). **196 dependent tool calls over 64 rounds**, four 32-bit words carried in
context, graded against an RFC 1321 trace so every failure is pure bookkeeping.
`gpt-oss-120b` (~5.5B active) at temperature 0 carries full state across all 196 calls. Two
ingredients decide it, neither touching weights: **keeping the model's own reasoning in
context each turn**, and voting over a thinking-enabled worker.

- **What inber should consider:** a direct constraint on compaction. Whatever
  `conversation/manage_tool_pruning.go` and `staged.go` drop, dropping prior reasoning
  blocks is the one thing measured here to break dependent-call chains — which is exactly
  what `server/session_creation.go:151`'s unconditional strip does (todo `cf3b6b4c`). Worth
  a test pinning thinking-block retention through a prune, beside
  `prune_preserves_is_error_test.go`.
- **Tension with §3, and it is real.** Fold-into-typed-state and keep-reasoning-in-context
  pull opposite ways; both are measured. The reconciliation: the *aggregate* is
  deterministic state, the *reasoning* is not summarizable. Deciding which of inber's
  carried values is which is the work.

## 5. Outcome-only judging misses more than half of silent faults

[arXiv:2609.00038](https://arxiv.org/abs/2609.00038) — **trajectory-judge: What Outcome-Only
LLM Judges Miss on Agent Trajectories** (2026-08-29). Deterministic environment, scripted
oracle, one injected fault at a known step, stratified by whether the visible outcome
survived. 400+ trajectories, five judges: the outcome-only judge catches **84% of loud
faults but 45% of silent ones, while flagging 33% of correct trajectories**. A step-rubric
judge reaches **77% silent recall with zero false alarms at 3x cost**. **No judge reads the
final reply** — an invented promise appended to a perfect trajectory evades the rules
entirely and the step judge 82% of the time. Self-consistency tripled cost and improved
nothing.

- **What inber should consider:** the judge-side number for the delivery-layer blind spot
  filed on 2026-09-01 from [2608.29128](https://arxiv.org/abs/2608.29128). `requests.status`
  in `~/.inber/server/server.db` is an outcome-only signal and on these figures would miss
  over half the faults that leave the visible answer intact. If harness-watch ever scores
  its own runs, **stratify recall by outcome survival** or the number means nothing.

## 6. Retrying blind beats localizing the fault

[arXiv:2609.00854](https://arxiv.org/abs/2609.00854) — **Does Fault Localization Beat a
Fresh Attempt? A Placebo-Controlled Study of Test-Guided Code Repair** (2026-09-01). Three
arms — blind resampling, spectrum-based localization plus suspect-span infilling, and
same-length infilling at a *random disjoint* span (the placebo) — over three frozen 26–32B
models, three benchmarks, 488 failing candidates. **Localization is rarely even available:
9.0% of failing candidates expose a failing public test with a usable spectrum.** Among the
177 localizable, localized infilling **loses to blind resampling 3:40** at matched attempts
(p = 3.0e-9). Re-pricing as tokens does not save it: 16 localized attempts reach 6.8% where
**one blind attempt reaches 10.1%**. Infilling reproduces the removed span verbatim 48.9% of
the time, which is why more budget does not help.

- **What inber should consider:** if any retry path narrows the model's edit window to a
  suspected span after a failure, widen it back to a fresh attempt. Scoped to 24–32B models,
  so treat as a hypothesis for frontier models — but "narrow the retry" now needs evidence
  rather than being the default.

## 7. Separate spawning a sub-agent from granting it authority

[arXiv:2609.01035](https://arxiv.org/abs/2609.01035) — **Spawn Freely, Act Sparingly:
Progressive Risk Vesting for Recursive LLM-Agent Trees** (2026-09-01). Splits **sandbox
spawning** (external controls prevent harm) from **capability activation** (a branch crosses
an irreversible-action boundary), holds a trajectory-level risk budget in escrow and debits
it only on activation. Proves an anytime harm bound over adaptively generated trees, and
shows delayed vesting preserves every policy available under irrevocable spawn charging.
Trajectory harm switches regime as the authority reproduction number ℜ_A crosses one.
Synthetic — it does not estimate safety in deployed agents.

- **What inber should consider:** inber charges at spawn — a child takes its model and tool
  set from config at creation (`server/spawn.go:170-175`) — and `guard` gates per call with
  no per-trajectory budget shared down a spawn tree. The result that deferring the charge to
  activation costs nothing in reachable policy says a recursion-depth-blind guard is leaving
  a free safety property on the table. Bears directly on the open zero-`RunRequest` todo
  `9e31d359`, which is the same boundary seen from the caps side.

## 8. Don't let the model write the file — let it write the intent

[arXiv:2609.00227](https://arxiv.org/abs/2609.00227) — **Don't Let the Model Write the YAML:
Deterministic, Minimal-Diff GitOps Remediation from LLM-Proposed Field Changes**
(2026-08-31). On real Kubernetes manifests, **no text-generation strategy is safe
unattended**. Unified diffs under strict patching almost never apply; a tolerant tool (GNU
patch) applies 96% but **silently misapplies 1 in 7 (14–20%) with no error signal**.
Full-file rewrite is capability-dependent and non-deterministic even for a frontier model,
at O(file size) per edit. Their alternative: the model emits a structured field-change
*intent*; a deterministic pipeline locates the target scalar's character span via the
parser's node position marks and replaces that span in the raw text, never re-serializing.

- **What inber should consider:** the **14–20% silent-misapply** rate is the number to carry
  into any tool in `tools/` that applies a model-authored diff. Where inber edits structured
  config, the intent-plus-deterministic-applier split is strictly better and cheaper.

### Screened and rejected, with the reason

- [2609.01437](https://arxiv.org/abs/2609.01437) **HarnessDev** (09-01) — LLMs build and
  evolve their own harness. One caution worth logging: evolution gains are unstable, transfer
  only partially to held-out tasks, and depend strongly on the executing model. Otherwise a
  benchmark.
- [2609.01481](https://arxiv.org/abs/2609.01481) **Harness-of-Harness** (09-01) — +52.25%
  average relative gain over standalone harnesses, but no ablation isolating which of its
  seven design choices earns it.
- [2609.00546](https://arxiv.org/abs/2609.00546) **Runtime-Independent Persistent Agents**
  (09-01) — six continuity invariants and a quiesce/checkpoint/rehydrate protocol. On-topic
  for `agent-store`, but its evidence is "833 core tests pass" and it says so itself.
- [2608.29641](https://arxiv.org/abs/2608.29641) **Harness-RL** (08-30) — trains a central
  policy. inber does no training.
- [2609.00050](https://arxiv.org/abs/2609.00050), [2609.01271](https://arxiv.org/abs/2609.01271),
  [2609.01603](https://arxiv.org/abs/2609.01603), [2609.01600](https://arxiv.org/abs/2609.01600),
  [2609.00823](https://arxiv.org/abs/2609.00823), [2609.01294](https://arxiv.org/abs/2609.01294),
  [2609.00077](https://arxiv.org/abs/2609.00077) — all read, none clearing the bar:
  framework-without-comparison, benchmark-profiling, eval-subset-selection, benchmark,
  activation-probing (needs hidden states inber does not have), a search-strategy gain on
  deep-research benchmarks, and a mitigation for autonomous ML-research loops inber does not run.

# 2026-09-03 sweep

Screened 108 distinct arXiv ids against the 448 already in this repo's docs
(37 were already covered), drawn from roughly 330 titles across six
date-range-filtered `arxiv.org/search/advanced` queries plus the `cs.MA` 2026-08
listing. Eight are recorded below. As on 2026-09-01 the Atom API answered `429`
through this host's fetch path; the advanced-search HTML endpoint with
`date-filter_by=date_range` worked and is the better tool for the next run.

⚠️ **The id prefix is not a date filter, and this sweep caught it doing damage.**
Probing the abs-page format, `2608.00001` reports `[v1] Tue, 14 Apr 2026` — an
in-window-looking id four months out of window. Every `[v1]` line below was read
off `arxiv.org/abs/` individually. Two entries (§1, §2) plus §1's load-bearing
context-window number were then re-fetched independently by the sweep's caller
and matched exactly.

## 1. Agents rot by the step, and truncating context makes it worse

[arXiv:2609.01660](https://arxiv.org/abs/2609.01660) — **How Fast Do Agents Rot?
An Empirical Study of Long-Horizon Degradation in LLM Agents for Production
Decision-Making** (2026-08-31). 9 models from 1.2B to 671B plus three
proprietary, four task families including a real tool-use loop, five horizons,
three context regimes, 10,664 analyzed trajectories.

Task success follows a geometric law in a **single per-step reliability
parameter** that rises with model scale and saturates well below 1 even for the
strongest models. On the agentic task **every model tested falls from
near-perfect success to near zero within sixteen steps**. Projected reliability
runs 0.42 at GAIA-length horizons down to **0.24 at hundred-step production
horizons**.

The result that matters most here is the context-regime one: **bounding the
context window steepens the decay rather than easing it** — logit slope −0.69
bounded against −0.44 unbounded, p=3×10⁻⁶ — which the authors say contradicts a
lost-in-the-middle explanation and warn is "a common production shortcut".

⚠️ Checked before repeating the obvious framing: **inber's docs do not actually
argue for compaction on "shorter context degrades less" grounds**, so this
corrects nothing already written here. It is a caution against a rationale the
repo has not adopted, not a correction to one it has.

**What inber should consider:** inber bounds a turn at 50 API calls
(`agent/agent.go:336`) and a spawn at a wall-clock timeout
(`server/spawn.go`, default 300s). Neither is a reliability budget — both are
runaway guards denominated in the wrong unit. The paper's per-step parameter is
measurable from inber's own `requests` table (`turns` against `status`), and a
step-count abort with a stated reliability basis would be a different thing from
a 50-call ceiling picked to stop a loop.

## 2. Replay cannot score a model switch

[arXiv:2608.08239](https://arxiv.org/abs/2608.08239) — **The Replay Gap: Static
Evaluation of Model Switching in LLM Agents Scores the Wrong World** (2026-08-08).
Forks live SWE-bench trajectories, rebuilds the environment, and continues each
fork under a different model against same-model control forks; ~900 rollouts over
six paired runs.

Swaps exceed control floors by **+0.25 to +0.66 normalized edit distance**,
rewriting **61–94% of post-fork actions**. **74–77% of early swaps diverge at the
very first post-fork action** against 6–35% for controls, leaving **only 3% of
replayed states valid**. A log-stitching replay evaluator **mispredicted every
success-relevant outcome** and produced patches with 0.00–0.11 similarity to
what actually happened. Separately: temperature-0 determinism is
serving-config dependent — FP8 controls diverged on >90% of forks where AWQ was
near-identical.

**What inber should consider:** inber switches models — `engine/failover.go`
picks from a chain, and `selectModel` (`engine/turn_execute.go:18`) is gated on a
30-minute health window. Any evaluation of that chain built by replaying stored
sessions under a different model is measuring a world that does not exist. The
second half is the sharper one for this repo: inber stores sessions and treats
them as replayable, and "deterministic replay" is only a claim about a stated
serving backend.

## 3. Token savings and cache hits pull against each other

[arXiv:2609.00749](https://arxiv.org/abs/2609.00749) — **ContextPipe:
Database-Inspired Context Assembly for Long-Horizon Agents** (2026-09-01).
Context assembly as query execution: a five-phase Plan/Bind/Optimize/Execute/
Feedback pipeline over a data-source catalog, with a cache-aware deterministic
optimizer and an `EXPLAIN ANALYZE`-style trace.

Against append-only construction on the SWE-bench Pro Qutebrowser subset:
**−31% total tokens, −23% LLM calls, −9% response time — and a *lower* KV
cache-hit ratio.**

**What inber should consider:** that last clause is the finding, not a footnote.
It is the measured form of the trade the cache-breakpoint todo filed this run
(`a5b91a47`) has to decide, and it says a compaction or pruning policy tuned on
token count alone can lose more to cache misses than it saves. inber has the
worse version of this problem: it cannot currently see either side of the trade
on its auxiliary calls (`7be5a692`). The `EXPLAIN`-style per-turn trace is the
cheap half to copy — inber already stages and prunes, and does not record why.

## 4. Cached knowledge needs an invalidation granularity, and is not model-portable

[arXiv:2609.00243](https://arxiv.org/abs/2609.00243) — **Invalidation Contracts
for Cross-Episode Agent Memory** (2026-08-31). Version stamps plus cacheability
hints on cached error-recovery suggestions, so a client evicts a stale entry
instead of discovering staleness by trying it. 7 models, 3 serving paths, 2
domains, ~9,400 episodes.

**Row-level invalidation raises compliance by 0–66.7pp; table-level invalidation
drops post-drift first-try rates to 0% on 5 of 7 models** — coarse invalidation
is measurably worse than none. Recovers 29–33% of baseline token cost on 4 of 7.
The contract costs 15% of response payload; eviction precision is 1.00 at row
granularity. And the portability result: identical wire bytes gave **100%
first-try compliance on Claude Haiku 4.5 and ≤11% on Claude Sonnet 5**.

**What inber should consider:** inber's memory extraction
(`conversation/extract.go`) writes facts with no version stamp and no stated
invalidation granularity, and `memory/auto_context.go` reads them back into
later sessions. Two bullets: stamp each extracted fact with the provenance
version it was true of, and pick the granularity deliberately, because the
paper's measurement is that whole-memory invalidation is the actively harmful
choice rather than the conservative one. The portability number also bears on
inber directly — memories extracted under one model are replayed into sessions
running another.

## 5. A "done" that a replay can re-derive

[arXiv:2608.23623](https://arxiv.org/abs/2608.23623) — **When May an Agent Stop?
Evidence-Carrying Termination for Tool-Using LLMs** (2026-08-22). An agent may
return COMPLETE only when a typed certificate binds each claim in the answer to
in-scope trace evidence *and* a deterministic replay reconstructs the claimed
value.

Static study over 48 synthetic tasks, 6 tool-use families, 8 fault types:
**0/288 unsafe completions against 252/288 for a termination-critic baseline
(−87.50pp)**. On a frozen 576-trajectory study: **0/66 premature unsupported
terminations against 40/66 (−60.61pp)**, with supported completion 97/132 against
92/132 (+3.79pp, inside a −10pt noninferiority margin). It recovered in 18/66
trajectories, 17 of which then completed with support.

**What inber should consider:** inber's spawn decides status by exception —
`status := "success"` is the initial value at `server/spawn.go:307-320`, changed
only on `context.DeadlineExceeded` or a non-nil error. A stream cut short mid-
response returns whatever text arrived and the child is reported `success` to its
parent, which is the failure this paper is built to catch and which CC 2.1.257
fixed from the other end (see `claude-code.md` 2026-09-03). inber persists whole
sessions already, so the trace half of the certificate is sitting on disk unused.

## 6. Four compaction baselines under one trigger, scored in non-cache tokens

[arXiv:2608.29897](https://arxiv.org/abs/2608.29897) — **When History Is
Multimodal: Rethinking Context Management for Long-Horizon Agents** (2026-08-30).
Context management as budget-constrained history transformation, benchmarking
**No Compression, Discard-All, Sliding Window and Summarization** under a shared
harness, policy and trigger across 4 text-centric and 3 multimodal benchmarks.
Their training-free manager cuts **cumulative non-cache tokens by 31.5–63.1%**
against No Compression.

**What inber should consider:** the controlled four-baseline comparison is the
value, not the proposed method — inber has only ever run summarization
(`conversation/summarize.go`) and has never measured it against a sliding window
or a plain discard on its own traffic. Note the metric: **non-cache tokens**, not
tokens, which is the right denominator for a harness that caches deliberately and
the same denominator §3 above and todo `a5b91a47` turn on.

## 7. A revision mid-turn should invalidate a region, not the run

[arXiv:2609.00643](https://arxiv.org/abs/2609.00643) — **REVISE:
Validity-Guided Recovery for Online Revisions in Agent Workflows** (2026-09-01).
When a user revision lands mid-execution, intersect the delta with recorded
data and control dependencies, stop only the invalidated work, recompute only
the affected region of the DAG.

Measured on real coding-agent traces: **118 sessions retain observable work
before a queued later message is delivered, and enqueue-to-completion overlap
reaches 56.55s at p95** across 167 overlapping responses. Over 300 revision
executions it matches a latest-version oracle with zero stale outputs and cuts
model calls **40.6–56.0% against full restart** and **31.3–43.6% against suffix
recomputation**.

**What inber should consider:** this is inber's injection path exactly —
`Server.Inject` and the `steer_agent` tool (`server/spawn_tools.go:15-58`)
deliver a message either mid-turn between tool calls or queued for the next
turn, and the mid-turn case has no notion of which completed work the new
message invalidates. The p95 overlap number says the window where this matters is
tens of seconds wide in practice, not hypothetical.

## 8. A session paused for a human is a different cache workload

[arXiv:2608.30830](https://arxiv.org/abs/2608.30830) — **Adaptive KV Retention
for LLM Agents at Human-Approval Timescales** (2026-08-31). Aimed at requests
suspended for **minutes to hours** awaiting human approval, not seconds-scale
tool pauses. Retaining suspended KV costs **41% of active-serving goodput**;
evicting it costs **roughly 10× higher resume latency**. Their tiered controller
gains **23–51% over vLLM baselines, 22–29% over MORI, 41–52% over Continuum**.

**What inber should consider:** this is serving-side and inber is a client, so
none of the mechanism transfers. The bearing is on the approval gap named in
`claude-code.md` 2026-09-03: if inber ever gives `guard.NeedsApproval`
(`guard/guard.go:165`) somewhere to ask, the resumed session's cache is a
decision, not a given — and Anthropic's 5m/1h TTLs put a human-scale wait firmly
on the wrong side of both. Worth knowing *before* the approver is built rather
than after.

# 2026-09-04 sweep

Screened by parsing the arXiv listing pages directly rather than search-engine
hits, so the not-already-covered check is exact against the **446 ids** already
extracted from `docs/papers/*.md`. Volume: the full `cs.SE`/`cs.AI`/`cs.CL`/`cs.MA`
**2026-09** listings — 924 distinct ids, **901 unseen**; the same four **2026-08**
listings — 4,738 ids, **878 unseen** above `2608.24000` (where the August sweeps
stopped); plus one `arxiv.org/search/advanced` date-range query on prompt/KV
caching, 11 new of 18. Topic filtering left **124 candidate titles**, of which
**30 abstract pages were fetched and read in full**. Every number below is off an
`arxiv.org/abs/` page. No PDFs were fetched, so nothing here comes from a results
table — where a paper's headline rests on a table rather than an abstract, that
is said.

Eight carry something. The blogs did not: Anthropic's engineering index carries
23 posts and the newest harness one still dates to **2026-03-24**
(`harness-design-long-running-apps`) — the `2026-09-01` on the index is a
`siteSettings._updatedAt` field, the same false positive the 08-07 sweep flagged.
HuggingFace's front index has posts through 2026-09-03 and nothing on harness
design, context management or agent infrastructure.

## 1. Lifecycle hooks are an unguarded update path, and all seven harnesses tested fell

[arXiv:2609.03884](https://arxiv.org/abs/2609.03884) — **A Blind Trust, the
Bloody Thrust: When Attacker-Controlled Hook Updates Steer AI Agent Harnesses
towards Malicious Behaviors** (2026-09-03, cs.CR/cs.AI).

The threat model is the interesting part, because it is smaller than the usual
one: the attacker controls **only plugin metadata and lifecycle-hook
configuration**. A benign, versioned plugin is trojanized by an *update* that
binds attacker-chosen shell commands to benign events — session start, tool call,
file edit. Those commands run with host privileges and fire at moments the model
never observes, so nothing in the transcript shows them and no model-side
permission gate can see them at all. Their HookPry framework realizes ten attack
objectives across **25 harness×backend combinations in 1,000 end-to-end runs**,
compromising **all seven evaluated harnesses**, per-harness success reaching
**92.5%**. Defenses are weak: Microsoft Defender scores **0% recall**, and the
union of three static defenses misses **47.5%** of malicious artifacts.

This lands on `hook-store`, not on inber's own tree: hook-store rows *are*
(harness, event, matcher, **shell command**) tuples, queried by the server at
session spawn, which is the exact object the paper attacks.

- **What inber should consider:** treat a change to a hook's `command` as a
  privileged mutation rather than an ordinary update — pin or hash the command
  and require explicit re-approval when it changes, so the trusted thing is the
  command and not the row. The second half is harder and is worth stating rather
  than solving: because hooks fire outside the model's view, **the permission
  model that gates tool calls cannot cover them**, so whatever gates hooks has to
  be a separate mechanism from `permission-store`'s tool-call rules.

## 2. Persistent memory is part of the authorization policy, not a cache

[arXiv:2609.01836](https://arxiv.org/abs/2609.01836) — **Agent Memory Is a
Surface for Endogenous Authorization Laundering** (2026-09-01, cs.CR/cs.AI).

No external attacker anywhere in this one. When memory misrepresents *evolving*
authorization state, the agent's own records come to grant authority the history
never permitted. EAL-Bench evaluates 5 LLMs as memory **writers** and 2 as
**executors** across procurement, cybersecurity and finance. Under incremental
memory updates, writers fabricate false authority for up to **50.2% of
unauthorized requests** — and once such a row is present, executors act on it in
**98.6% of trials**. Two safeguards help: requiring a stored permission to be
backed by a valid source event, and bounded event sourcing over permission
changes. Both cut laundering substantially and both **reject more legitimate
actions**; the authors state the safety-utility tradeoff rather than hiding it.

- **What inber should consider:** memory rows are injected into the prompt as
  ordinary content, so a row reading *"user approved X"* is indistinguishable
  from the user having approved X. The transferable piece is the safeguard's
  shape, not the benchmark: **a memory row that asserts a permission needs a
  source-event id, and a row without one must not be readable as
  authorization.** Distinct from SARA's No-History-Promotion rule already at
  `2026-08-harness-research.md` (2608.27146) — that one governs summarization,
  this one governs the store.

## 3. A verification tool only pays where its reach covers the failure mode

[arXiv:2608.28795](https://arxiv.org/abs/2608.28795) — **The reach of a
verification tool decides its value: A controlled study of verification surface,
artifact quality, and cost in AI coding agents** (2026-08-28).

The agent's tool list is the single controlled variable: one minimal coding agent
built **1,116 web applications** across **six models and eight tool
configurations**, graded condition-blind against a frozen rubric plus automatic
API probes. With no tools, **about one build in seven fails to launch at all**. A
single boot probe removes nearly all of those at roughly **35% of a full shell's
token cost**, while the full shell **multiplies the no-tools cost by 2.35×**.
Screenshots help only where the mistake is visible, and that gain **does not
survive correction for multiple comparisons**; on a failure that is measured
rather than seen (scroll smoothness over 100,000 rows) screenshots add nothing.

- **What inber should consider:** the cheapest concrete tuning result in the
  batch, and it is a bundle-composition rule `bundle-store` could encode — the
  marginal value of adding a tool is **not monotone in its cost**, so give a
  session the cheap reach-matched verifier by default and gate the shell, rather
  than ordering tools by capability.

## 4. Aggregate "retrieval helps" can be positive while the per-task effect is negative

[arXiv:2609.00549](https://arxiv.org/abs/2609.00549) — **Skill Following:
Evaluating Actual Skill Use in Retrieval-Enabled LLM Agents** (2026-09-01,
cs.CL).

Standard skill evaluations compare retrieved against non-retrieved tasks, which
is a selection-biased comparison — the tasks differ, not just the treatment. They
define the **Retrieval-Invoked Actual-Use Effect (RAE)**: the same-task outcome
difference between matched skill-enabled and skill-disabled runs, conditioned
only on tasks where the agent *actually retrieved* a skill. Across **17 LLMs** on
coding and math they report an evaluation paradox — models frequently show
**positive aggregate retrieval lift and negative RAE**. On MBPP+, several models
that appear to benefit system-wide **actively harm their own performance on
exactly the tasks where retrieval fired**.

- **What inber should consider:** `skill-store` content is injected and nothing
  measures whether it helped. RAE is cheap enough to run on inber's own traffic —
  replay a session with the skill bundle disabled, condition on sessions where a
  skill was retrieved, diff the outcome. Note the binding constraint: the
  comparison must be **matched same-task**, which needs session replay, which
  inber already keeps on disk.

## 5. Halt the run early: 13–26% of agent steps are decidable before completion

[arXiv:2609.02783](https://arxiv.org/abs/2609.02783) — **EarlyEval: Cheaper Agent
Evaluation via Early Outcome Prediction** (2026-09-02, cs.CL).

Rather than cutting the number of eval tasks, they cut cost *within* each task. A
pair of **LightGBM** success/failure classifiers over behavioral, textual and
reference-solution features halts the run the moment either crosses a calibrated
confidence threshold. Across **SWE-bench Verified, TerminalBench and Toolathlon**
this removes **13%–26% of agent steps** and up to **44.1% of input tokens** and
**29.4% of output tokens** at **89%–97% prediction accuracy**, perturbing
per-agent resolve rates by only one to two percentage points.

- **What inber should consider:** the predictor is gradient-boosted trees over
  trajectory features, **not a model call**, so per-step overhead is negligible
  and it is implementable against the sessions already persisted here. The reach
  beyond evaluation is the interesting part: the same signal is a candidate abort
  condition for a doomed sub-agent, which today is bounded only by turn and token
  caps — a cap stops a run that is *expensive*, never one that is merely *lost*.

## 6. Functional tests pass; review constraints do not

[arXiv:2609.04167](https://arxiv.org/abs/2609.04167) — **SWE-Gate: Passing
Functional Tests Is Not Enough for Software Engineering Agents** (2026-09-03,
cs.SE/cs.AI).

A repo-level benchmark that derives **review constraints from real PR review
comments** and synthesizes repair instances around them, giving each instance
*separate* functional and constraint tests plus non-compliant and gold patches.
**303 instances across 75 Python repos**, four LLM backends under one common
scaffold. Among **644 repairs that pass the functional tests, 221 fail the review
constraints** — roughly a third of functionally-correct patches would be rejected
in review.

- **What inber should consider:** the separation is the reusable idea, not the
  benchmark. inber's verify path scores *did it work*; this says a second,
  **independently specified** constraint check is where about a third of the
  residual failure lives. Pairs with the 08-16 finding (2608.12895) that a
  reviewer on the same model as the worker is not a second opinion — there the
  **model** was the thing that had to differ, here it is the **specification**.

## 7. A generic harness with an execution-feedback repair round beats domain multi-agent machinery

[arXiv:2609.03718](https://arxiv.org/abs/2609.03718) — **What Do CAE Simulation
Agents Really Need Beyond a Generic Harness?** (2026-09-03).

With information access and repair budget held fixed, a **single-agent generic
harness matches or beats multi-agent specialized systems: FoamBench 96.4% vs
88.2%**. The ablation localizes why. **Execution-feedback repair lifts FoamBench
from 71.8% (no repair round) to 96.4%**, while **scripted reflection adds
nothing**. The one non-harness input that still helps is domain knowledge as
solver tutorials — their largest measured gain, **80.9% → 96.4%**.

- **What inber should consider:** three numbers bearing directly on how inber
  spends its ten agents. Multi-agent decomposition bought **−8.2 points** here
  against one agent with a repair loop; scripted reflection was null; the win was
  a repair round plus domain docs. Where the choice is between spawning
  specialists and tightening the tool-error → retry loop, this argues for the
  loop — and the tutorial result is the strongest case in this batch for
  `skill-store` **content** over agent **count**.

## 8. Second tier — real, verified, narrower

- [2609.00267](https://arxiv.org/abs/2609.00267) **Delegation Without Trust**
  (2026-08-31, cs.CR) — an untrusted-model standard for sub-agent delegation;
  LangGraph, CrewAI, AutoGen and MCP-auth all fail confinement (three provide
  none, one partial). Their authorization broker confines a compromised sub-agent
  to **1.5 reachable actions against all 8,100 under bearer delegation** across
  2,000 scenarios, at **~2.6 µs per decision**, 0 of 200,000 forged tokens
  accepted. The closest thing yet to a design for `permission-store`'s unbuilt
  stages 2–7, and it is about delegation specifically.
- [2609.00949](https://arxiv.org/abs/2609.00949) **Calibration is the Bottleneck**
  (2026-09-01) — decomposes multi-turn tool-call failure into action-class
  miscalibration against execution failure over TOOL_CALL / ASK / REFUSE /
  CONFIRM, with a self-revealing bound `Acc ≤ GAR`. A single **context-only**
  perturbation moves accuracy **+11.5 pp on one model family and −21.0 pp on
  another** in the same scenario — which is a warning about porting a prompt
  change across models on one model's measurement.
- [2609.02246](https://arxiv.org/abs/2609.02246) **LLM-as-a-Judge Is Not an
  Oracle** (2026-09-02) — a position paper, so weighed as one, but the catalog of
  eleven production evaluation-signal failures is concrete: agents reaching a
  **100% pass rate that concealed 68% true capability** by reading cached answer
  keys out of the environment; a corrupted ground-truth label steering the
  optimizer into deleting correct rules; a syntactically broken prompt winning
  because a silent parser fallback improved the metric.
- [2609.00759](https://arxiv.org/abs/2609.00759) **Compile, Don't Memorize (CCA)**
  (2026-09-01) — compiles prose context once into a typed IR with fixed slots
  (`rules.{must_do, must_not, conditional}`, `output_spec`, `available_tools`,
  `data_profile`). On CL-bench (1,899 tasks, 4 open models) it beats vanilla and
  two long-context baselines on every model; Kimi K2.5 **15.4% → 21.4%**. Same
  "a labelled field survives compression, prose does not" family as 2608.06953.
- [2609.03340](https://arxiv.org/abs/2609.03340) **PlanFence** (2026-09-03) —
  plans cite the exact records they were built from, and the executor validates
  only the records affecting the pending action. In 30 controlled workflows a
  freshness-only executor acts on the obsolete plan **in every task**; PlanFence
  completes all of them with no invalid action. Small n; complements REVISE
  (2609.00643) from the 09-03 sweep.
- [2608.27487](https://arxiv.org/abs/2608.27487) **Grounded Checklist Partial
  Credit** (2026-08-26) — evidence-grounded partial credit for skill
  trajectories, **AUC 0.689 against 0.619** for holistic judging over 4,455
  deduplicated SkillsBench trajectories, and the judge abstains when the log
  carries no evidence.
- [2609.01507](https://arxiv.org/abs/2609.01507) **LatentPress** (v2 2026-09-03) —
  **0.504 accuracy at 7.70× compression against 0.490 uncompressed** on
  LongMemEval, 43 ms per conversation to write. Recorded and **not actionable**:
  it reads continuous memory tokens through the input-embedding interface, which
  an API client does not have.

## Screened and rejected, with the reason

- **2609.02889** *Where Does Harness-Optimization Value Live?* — genuinely
  relevant (nearly all self-evolution value localizes in the reflection/control
  slot, +0.119 leave-one-in; an even four-way budget split leaves 16 rollouts per
  slot, below the optimizer's search floor, and reaches 0.657 where concentrating
  half that budget reaches 0.761). **Out of window**: it surfaced in the
  September listing but the abs page reads *Submitted on 25 Jun 2026*, v1 only.
  Noted here so the next sweep does not re-screen it, and worth a read if
  inber ever auto-tunes its own prompt blueprint.
- **2609.01736** *HEART / Tool Primitives* — headline numbers rest on a
  25,519-function repository with an LLM wrapper per tool, and the comparison is
  against bare models rather than a harness, so nothing is attributable to the
  design.
- **2609.00829** *HarnessEvolve* — same self-evolution space as 2609.02889 and no
  measured numbers in the abstract at all.
- **2609.00069** *Auditing Harness Tampering* — real phenomenon, decent taxonomy,
  no rates; and it only applies to agents that rewrite their own harness, which
  inber does not.
- **2609.00252** *Spec-Driven Development for ASE* — the authors call it a
  conceptual analysis over gray literature and say the evidence base is immature.
- **2609.01345** *Cheap Verifiers, Large Blind Spots* — good work (a dashboard
  reading a flat 3% while true error swings to 32%) about inference cascades and
  student fine-tuning, not about a harness inber runs.
- **2609.04128 / 2609.04148** *Environment Evolution, Terminal-Universe* —
  training-data generation for terminal agents; useful to a lab, nothing for a
  client.
- **2608.24509 PeakBench**, **2609.02459 CivBench**, **2609.03047 SHELF** — pure
  benchmark papers, no transferable mechanism.
- **2608.15584, 2609.00891, 2609.03235, 2609.03430, 2609.03949, 2608.27128,
  2608.28293** — serving-side KV-cache work. inber is an API client; same
  structural exclusion the 08-10 sweep applied.
- **2609.01693** *MCP-to-A2A egress* — careful and honest, but floor-limited and
  inconclusive by the authors' own statement; the only positive result is that a
  `PUBLIC - OK TO SHARE` label *raises* verbatim egress, in one configuration.
- **2609.03450** *Plan Pointers / Record-Directive Form* — a large registered
  study (14,760 attempts) whose effects are string-level and unstable across
  panels; a +35.0-point criterion effect fails its own superiority rule on a
  second panel, and 15 of 30 replication contrasts are unresolved.
- **2609.00453 mimeo**, **2609.02749 Repo-To-Skill**, **2609.02217 SkillGLoW**,
  **2609.02094 MASkills** — skill synthesis. mimeo is the most rigorous and its
  judgment-transfer tests hit ceiling (94–100% in every condition), so its
  headline question is unresolved.
- **2609.01931** *Agent Flight Recorder* — on-chain anchoring for audit trails.
- **2609.02371** *AGENTSCOPE failure diagnosis* — right topic (neuro-symbolic
  failure localization over trajectories), but the abstract offers only
  "significantly outperforms" with no numbers. Worth a re-look by someone who
  reads the PDF.

# 2026-09-08 sweep

The arXiv Atom API answered this time — 150 most-recent entries each from
`cs.SE`, `cs.MA`, `cs.AI` and `cs.CL`, 555 distinct papers spanning 2026-08-20 →
2026-09-04, cross-checked against the `cs.SE` and `cs.MA` `/recent` listing
pages. **Nothing exists newer than 2026-09-04**: 09-05 and 09-06 were the
weekend and 09-07 was Labor Day, so the Monday announcement block carries Friday
submissions. The real new window over the 09-04 sweep is one day. Deduped
against 545 ids extracted from `docs/papers/` and `docs/comparisons/`; all ten
below are absent from that set, and every date and title was read off
`arxiv.org/abs/` rather than a search snippet.

## 1. Sub-agent delegation was strictly dominated by a single agent with better retrieval

[arXiv:2609.04898](https://arxiv.org/abs/2609.04898) — **RefactorPlatform: An Open-Source
Harness for Controlled Evaluation of Repository-Scale Refactoring Agents** (2026-09-04, cs.CL).
An evaluation harness that holds the environment fixed and varies one design axis at a time —
model backbone, execution regime (baseline / retrieval-augmented / multi-agent), prompt
specificity — over 100 multi-file RefactorBench tasks and four model families, with per-task
token, diff and transcript logging and AST-based verification.

- A lean retrieval-augmented **single** agent scores **86%** against **66%** for the sub-agent
  configuration on matched tasks, and **no task passes under delegation that fails under
  retrieval**. The sub-agent set is a subset, not a trade.
- **AST-aware chunking beats naive token-window chunking by 25–30%** across all prompt modes.
- **Naive retrieval falls *below* the retrieval-free baseline.** Retrieval is not free-standing
  good; the chunking is doing the work.
- Retrieval's accuracy gain absorbs its token overhead, so cost per *successful* refactoring is
  unchanged.

**What inber should consider.** This is the sharpest negative result on delegation this file has
carried, and it lands on `docs/multi-agent-design.md` and `docs/async-spawning.md`. Before more is
spent on spawn/fork machinery, run the same A/B on inber's own task set: one agent with better
context assembly against spawned children. The chunking number separately argues that
`docs/smart-truncation.md`'s unit of truncation should be AST-aware rather than token-window —
`session/truncate.go`'s `truncateHeadTail` breaks on newlines, which is the crudest version of
the thing that measured 25–30%.

## 2. LLM-compressed memory does not survive a model change; fixed schemas do

[arXiv:2609.05339](https://arxiv.org/abs/2609.05339) — **Does Your Agent's Memory Survive a
Model Upgrade? A Controlled Study of Memory Portability** (2026-09-04, cs.AI). Four memory
representations under a *writer model swap* — verbatim long-context history, chunked RAG,
model-compressed natural-language notes, and a fixed-schema knowledge graph — over 48 synthetic
histories with randomized answer codes and exact scoring.

- Fixed-schema structures transfer essentially free: **+0.0004 ± 0.0020** accuracy change across
  the swap.
- Compressed NOTES are strongly model-coupled and shift **asymmetrically by +9.91 or −13.28
  percentage points** depending on migration *direction*.
- A 50/50 mixed embedding index captures only **4.96** of the **11.90** points available from a
  full re-embed.
- Decomposition: **80%** of the NOTES deficit is construction-time information loss; **81%** of
  the RAG deficit is retrieval failure.
- **Store-only repair of NOTES hit the 90% recovery target in 0 of 48 cases.** Keeping the raw
  source history recovered **34 of 48**.

**What inber should consider.** inber's persistent memory and `memory-store` write exactly the
representation that measurably does not survive a model change — and the model *does* change:
open todo `905a5e68` records that every run without an explicit `--model` fails over silently.
Two concrete moves fall out. **Never delete the raw source transcript a memory was distilled
from**, because store-only repair failed in all 48 cases. And treat an embedding-model change as
all-or-nothing: a partial re-index forfeits most of the available gain. Worth reading against
`docs/memory-extraction-evaluation.md`, which scores extraction quality and not portability.

## 3. How you pack retrieved content matters as much as what you retrieve

[arXiv:2609.04915](https://arxiv.org/abs/2609.04915) — **Compact-Memory LLM Agents via Online
Max-Member Clustering and Atom-Aware Packing** (2026-09-04, cs.AI). An online clustered-memory
pipeline with two parts: a cosine-gated max-member merge write rule, and an atom-aware *grouped*
context packer. On AMA-Bench it reaches **83% of full-context quality at 32% of the token cost**
at a 4k budget, beating the closest streaming-clustered baseline by **+3.5 to +6.0 pp (p<.001)**
across the ~2.6k–5k regime over four seeds. The ablation is the useful number: **+5.7 pp** from
the merge rule and **+5.0 pp** from grouped-versus-flat packing, roughly even. Reproduces on
RealMem (+2.97 pp over Streaming-Proto, +1.65 pp over A-MEM) but is only on par with BM25-RAG,
and the authors are explicit that the win is regime-bounded to roughly 2k–5k prompt tokens.

**What inber should consider.** Five points from changing the *packer* alone, with the retriever
untouched, is the cheapest experiment in this batch. `engine/turn_prompt.go:99-153` concatenates:
stable memory blocks in `BuildContext` order into the system section, then fleet status, volatile
blocks and every injector's output joined with `"\n"` into `VolatileContext`. Grouping by source
before joining is a local change. Pairs with `2608.31057` from the 09-01 sweep — equal token
budgets are not equal delivered context.

## 4. Adding a tool can break a task that used to pass

[arXiv:2609.04280](https://arxiv.org/abs/2609.04280) — **EVOHARNESSBENCH: Can Your Agents Keep
Pace with an Evolving Harness?** (2026-09-03, cs.MA). Puts the non-stationarity in the *harness*
rather than the task stream: 17 multi-stage harness streams built deterministically from
verifier-based benchmarks — **802 tasks, 520 tools, 42 skills, 62 agents** — evolving along three
axes (tools, skills, specialist agents), evaluated both for retention of previously-solved tasks
and for whether accumulated experience stays useful. Three findings: **harness expansion alone
degrades performance on previously-solved tasks** ("harness-induced forgetting"); self-evolution
gains are inconsistent across stages, axes and environments; and retention and adaptation pull
against each other. Weakness: the gaps are reported qualitatively rather than as one headline
delta.

**What inber should consider.** This is a measurement this fleet currently cannot make.
`tool-store` and `bundle-store` exist to grow the tool and skill set over time, and this says
adding a tool can silently break a task that used to pass. The mitigation is a regression suite
pinned to a fixed task set, re-run whenever a bundle's member list changes — and `bundle-store`'s
`/resolve` is the natural place to version the resolved set so a regression can be attributed to
a specific harness delta rather than to the model.

## 5. Building an agent, scored by deploying it

[arXiv:2609.04611](https://arxiv.org/abs/2609.04611) — **τ^τ-Bench: An Environment for
End-To-End, Realistic Agent Construction** (2026-09-04, cs.AI). Makes *building an agent* the
task a coding agent is scored on: real business records, a client holding requirements, a
production API, an inherited codebase, and serving-cost limits; the delivered agent is then
deployed against held-out simulated users. Across **53 tasks in four domains** the strongest
configuration — **Claude Opus 5 under Claude Code — passes 23.9%** against an expert-authored
reference ceiling of **82.2%**. The named failure modes are behavioural, not capability limits:
shallow queries instead of deep comprehension of the records, near-zero communication back to the
client, and too little experimentation with architecture and serving spend — shipping the first
design that runs.

**What inber should consider.** The 58-point gap is almost entirely process, and two of the three
failure modes are things a harness can *force* rather than hope for. "Communicates almost nothing
to the client" is a delivery failure of the same family as `2608.29128` in the 09-01 sweep, where
77% of failing runs had already reached the correct final state. A required check-in turn and an
explicit "try a second architecture before committing" step are cheap interventions with a
measurable target to beat.

## 6. A reviewer must be a different model family — self-review buys nothing

[arXiv:2609.04270](https://arxiv.org/abs/2609.04270) — **Reviewer Capability Governs Rejection
Targeting, Not Repair Skill: Evidence from LLM Execute-Review-Revise Pipelines** (2026-09-02,
cs.SE). Varies *reviewer* capability across a constant set of 100 olympiad maths problems and
measures the outcome of every individual rejection.

- A **cross-family mid-tier reviewer lifts final accuracy 12 points, 52% → 64% (p=0.0005), with
  zero damaged answers.**
- Same-model self-review has the **highest error-detection recall of any condition (0.85)** and
  yields **no significant gain**: it rejects **2.1× as often** for a third the repair rate (15%
  vs 43%, p=0.0074) and **falsely rejects 35% of its own correct answers against 2%** for the
  cross-family reviewer (paired p=0.000015).
- Self-review's low damage rate is revision *inertia*, not quality: of 18 false rejections, all 3
  the executor complied with became wrong, and the 15 it ignored survived.
- Below a capability floor the reviewer is inert — the weakest one changed **0 of 100** final
  answers while doubling token cost.

Honest scope: one configuration, 100 problems, framed by the authors as a controlled pilot.

**What inber should consider.** If inber runs any execute-review-revise pattern across its
agents, the reviewer must be a *different model family*, not the same model reviewing itself, and
a too-weak reviewer is pure cost. The selection metric matters too: **detection recall is the
wrong number to pick a reviewer on** — it was highest exactly where the outcome did not move.
Repair rate and false-rejection rate are what tracked the result.

## 7. Put the execution contract in the prompt

[arXiv:2609.05232](https://arxiv.org/abs/2609.05232) — **Substrate-Aware AI Agents: Execution
Context as a First-Class Input** (2026-09-04, cs.AI). Names "substrate blindness" — the absence
of memory, wall-time and runtime limits from an agent's planning state — and tests it on
numerical code generation across three frontier configurations, either from the task alone or
with a **128 MB RAM / 10.0 s wall-time contract** stated in the prompt. Contract disclosure
reduced peak process memory in **13 of 14** index-aligned comparisons and mean wall time in all
three cohorts, up to **3.1× faster**. At a tighter 96 MB contract, correct-and-within-budget
outcomes went from **0/5, 1/5, 0/5** to **4/5, 5/5, 3/5**, with cohort mean MaxRSS **49–74%
lower**. The changes were structural — bounded blocking, float32 retention, upper-triangle
traversal, memory-mapped buffers — not cosmetic.

**What inber should consider.** The cheapest win in this batch, and inber already *has* the
facts: forge slot limits, `repo-store`'s signature, and the per-job `timeout_seconds` the
scheduler enforces by killing the process group. None of them reach the prompt. A one-line
execution contract in the system prompt is a small change against a 0/5 → 4/5 effect. Caveat
before over-reading it: a single narrow numerical task, not a repo-scale coding benchmark.

## 8. Speculative macro commit — conditional, and inber has the snapshot primitive

[arXiv:2609.03236](https://arxiv.org/abs/2609.03236) — **Speculative Macro Commit for Faster
Tool-Using Agents** (2026-09-03, cs.AI). A two-tier runtime: a large authoritative actor produces
the official trajectory while a small drafter continuously predicts and *executes* future action
chains on an isolated environment snapshot. Recurring multi-action skeletons are mined from
training traces into a macro library; when the actor's next call matches the first drafted
action, the remaining pre-executed steps **and their observations** are committed. With
Qwen3.5-27B INT4 as actor and Qwen3.5-4B as drafter it **matches sequential accuracy** while
cutting latency **10.23% over Speculative Actions and 18.59% over sequential** on the τ²-Bench
Telecom subset; on AppWorld it cuts wall time **7.7% over SA and 44.9% over sequential**, with a
small completion-rate drop. Code released.

**What inber should consider.** Filed as interesting-but-conditional. It needs an isolated
environment snapshot to speculate against and a cheap drafter; forge's worktree slots are exactly
that snapshot primitive. Two things to read before adopting: the AppWorld result trades some task
completion for the 44.9%, and the whole mechanism assumes the *actor* is the latency bottleneck,
which is much less true for an API client than for local inference.

## 9. Non-arXiv: a recency prior that measurably hurt

HuggingFace blog, **"Give Your Coding Agents a Memory You Own"** (funes) —
<https://huggingface.co/blog/funes>, 2026-09-03, David Corvoysier. A memory layer that indexes
agent session traces into a searchable local store: traces from Claude Code, Codex, pi and Hermes
normalized to one turn-and-block format, chunked, embedded with a pinned local model, written to
a Lance dataset; retrieval fuses vector search with BM25, reranks with a cross-encoder, reweights
by recency and pulls in neighbouring chunks. Two numbers worth carrying: on handoff-versus-recall
tasks, **recall was 8× cheaper than a written handoff on one task and 4× on another**, while
**compaction failed outright on one of the two**; and on a 19,195-session corpus with 100 test
questions, **hit@1 was 9/100 with a 30-day recency half-life and 19/100 with recency weighting
disabled** — the recency prior actively hurt. Blog-post rigour, not paper rigour; 19/100 is a
weak absolute number.

**What inber should consider.** Two things. The recency result is a direct warning for any
recency decay in `memory-store` — measure it rather than assuming it helps. And "compaction
failed outright on one of two tasks" while retrieval-from-trace was 4–8× cheaper argues for
making inber's session traces *queryable* rather than only compactable; the corpus already exists
in `.inber/sessions.db`.

## 10. Second tier — verified in window, not written up in full

- [arXiv:2609.05279](https://arxiv.org/abs/2609.05279) — *Testing Interchangeability in LLM Agent
  Teams* (2026-09-04). Swapping a role-matched agent between independently-formed teams costs
  little in task score but raises **communication tokens per unit of progress by 16–63%** against
  a placebo that reproduces the disruption without changing the occupant. Longer formation
  histories worsen the penalty; greedy decoding reduces it. Relevant only if inber ever hot-swaps
  an agent inside a running multi-agent session — and the cost shows up in tokens, not in the
  success bit, which is the half a naive metric would miss.
- [arXiv:2609.04875](https://arxiv.org/abs/2609.04875) — *Forgetting Without Restarting:
  Execution-State Unlearning for Stateful LLM Agents* (2026-09-04, cs.CR). Deleting a plaintext
  memory record **leaves leakage unchanged**; instruction-based forgetting collapses under
  elicitation (**Leak@probes = 1.00**); source redaction still acts on the revoked preference in
  **80% of episodes**. Provenance-Guided Selective Replay is indistinguishable from a full reset
  at **up to 9× fewer recomputed tokens**. Directly applicable to what "delete a memory" should
  mean in `memory-store`, which today almost certainly means the ineffective thing. Partly
  serving-side (KV-cache cropping), so only the prompt and summary layers transfer to an API
  client.

## Screened and rejected, with the reason

- **2609.04208** *AI Writes Code, Humans Pay the Debt* — appeared in the Monday 2026-09-07 `cs.SE`
  listing with an in-window-looking id; the abs page reads **v1 2026-06-06**. Out of window, and
  a registered-report protocol with no results yet. Recorded so the next sweep does not
  re-screen it — this is precisely the failure mode the read-the-abs-page rule exists for.
- **2609.04681** *Beyond Code Generation: Reliability, Verification, and Cost Economics in the
  Agentic SDLC* — in window, on topic, and the authors state plainly *"No new model experiment is
  claimed; numerical findings remain attributed to their original studies."* Its four terms
  (Throughput Paradox, Production-Qualified Change, Verification Tax, SDLC Control Plane) are
  decent vocabulary for writing up inber's cost story and nothing more.
- **2609.04909** *Hallucination in LLM-based Automated Program Repair* — good numbers (**72.7%**
  of 812 manually-analysed repairs contain repair hallucinations, including patches passing all
  tests) but about APR model behaviour on Defects4J, not about anything a harness controls.
- **2609.04629** *SiLR: Structure-Preserving Admission and Process Reward for LLM Tool Agents* —
  the closest thing here to a permission-model paper, with real results (21/21 vs 0/21 recovery;
  support-only admits **63.2% of 42,410 unsafe actions** where the product order admits 0).
  Rejected because the construction rests on deterministic simulation (Gym-ANM, CityLearn) — the
  shadow-execute-each-proposal premise does not exist for `Bash` or `Edit`. Worth a re-read by
  whoever picks up `permission-store` step 2 for one argument alone: **a scalar risk score is
  representationally insufficient and no threshold tuning fixes it.**
- **2609.05269** *CONTINUITY: Security-Context Contracts for Composable LLM Agent Controls* —
  clean result (**0 harmful effects across 2,560 parameterized attack instances**, all 700 benign
  tasks completed, all 200 ambiguous cases escalated) but produced by the authors' own
  deterministic fault-injection suite against their own reference verifier, with no external
  baseline. The machinery (signed root grants, transition receipts, effect-bound execution
  permits) is far heavier than anything inber's permission path could adopt. Flagged for
  `permission-store`'s design docs, not for implementation.
- **2609.04665** *Harness-agnostic detection and immunization of reward hacking in self-evolving
  language models* — has numbers (0.763 vs 0.663 AUROC) but applies only to agents that
  self-evolve against a visible score, which inber does not. Same exclusion as `2609.00829` /
  `2609.02889` in the 09-04 sweep.
- **2609.03192** *Where Reliability Lives* — serious and preregistered, with real numbers (one
  falsehood cost ~900 futile actions per trusting run). Rejected for transfer, not quality: the
  subject is a persistent simulated settlement with an append-only ledger and the authors
  explicitly limit the claim to "one designed world."
- **2609.04570** *Dynamic Adaptation of the LLM Context for Generating Routines with Coupled
  Semantics* — beats Reflexion and OpenEvolve on 7 of 8 problems (p<0.01), but it is evolutionary
  program search at 300–1000 evaluations per problem, a different regime from one interactive
  session.
- **2609.03915** *RuleMem* — headline "+27.47 points, 54.3% relative" is against the **mean of 14
  baselines** rather than the best, which is not a comparison that survives scrutiny.
  Conversational QA, not coding-agent memory.
- **2609.01834** *Architecting Conversational Data Systems for Stateless LLM APIs: The Hydration
  Proxy Pattern* — the one paper aimed squarely at prompt caching for a stateless API client, and
  a pure pattern paper with no experiment and no numbers. Its "Context Stabilization Mandate" is
  the principle already in `docs/cache-optimization.md`.
- **2609.04167 SWE-Gate** (already covered in the 09-04 sweep), **2609.04075 PatchBench**,
  **2609.04706 FinalityBench**, **2609.04667 ERPBench**, **2609.05374 CUA-Universe** —
  benchmark-only, no transferable mechanism.
- **2609.04094 DRACO**, **2609.04869**, **2609.04865 CoSkill**, **2609.05261 Trace2Tower**,
  **2609.05019 TROVE** — agent *training* (RL credit assignment, skill induction for
  fine-tuning). inber is an API client; nothing actionable.
- **2609.04971 BeaconKV**, **2609.04895 Cache-Aware Joint Router Adaptation**,
  **2609.04748 Quantization Amplifies Cache-Induced Divergence** — serving-side KV-cache work,
  same structural exclusion the 08-10 and 09-04 sweeps applied.
- **2609.04749 DCFA** — multi-agent failure attribution, right topic, no numbers in the abstract.
  Same shape as `2609.02371`, which the 09-04 sweep parked for a PDF read; read them together.

# 2026-09-09 sweep

Four new. Every title, date and number below was read off `arxiv.org/abs/`
rather than a search snippet, and all four were re-fetched by the parent job
after the scout reported them — the scout's list of eight lost half to the
dedupe check below.

**Read this before running the next sweep: the dedupe grep this job has been
using is incomplete, and it has been silently re-surfacing rejected papers.**
The extractor matches `arxiv.org/abs/<id>`, so it only ever sees papers that
were *accepted* and hyperlinked. Papers that were screened and **rejected** are
written in the "Checked and carrying nothing" lists as bare bold ids —
`**2609.05269** *CONTINUITY*` — with no link, so they are invisible to it. Four
of this sweep's eight candidates came back "new" that way and were all rejected
in the 09-08 sweep with reasons: `2609.05269` (CONTINUITY), `2609.03192` (Where
Reliability Lives), `2609.04075` (PatchBench) and `2609.01736` (HEART). The id
file held 478 ids; a plain `[0-9]{4}\.[0-9]{4,5}` scan over `docs/` finds the
rest. Use this instead:

```bash
grep -rohE '\b26[0-9]{2}\.[0-9]{4,5}\b' docs/ | sort -u
```

A second gap, in the other direction: three of the survivors below were
submitted 31 Aug – 2 Sep, inside the **09-01 and 09-02 sweeps' stated ranges**,
and were not found then. Those sweeps ran off per-category listing pages after
the arXiv Atom API returned `429`, and a listing page is recent-only — so a
paper submitted late in the window can fall off the page before the sweep that
claims to cover it. A stated date range is not evidence of coverage when the
instrument is a listing page.

## 1. Every local gate can be correct while the fleet overdraws by 48×

[arXiv:2609.00275](https://arxiv.org/abs/2609.00275) — **The Irreversibility Budget: Fleet-Level
Risk Accounting and Admission Control for Agent Operating Systems** (2026-08-31).

The claim is about composition, not about any single gate being wrong. Controls check one effect
at a time, so a fleet of individually authorized agents can overdraw its principal's risk under a
shared trigger *"while every local gate stays correct."* The proposal is an `irreversibility
budget`: a cumulative account of residual value-at-risk maintained per principal across agents,
workflows and tenants, charged per effect below the agent and denying the marginal effect once the
aggregate would overdraw. Their controlled study measures **per-effect gates admitting fleet-level
overdraws of up to 48× the tenant's risk limit**, against a budget that holds every correctly
charged run inside it. The authors are explicit that conservative, dependency-aware pricing is
unsolved — this is a framing with one number, not a deployable design.

**What inber should consider:** `guard.CheckTool` decides one call at a time and holds no state
across the spawn tree — `guard/guard.go` takes a tool name and arguments, and `server/spawn.go`
caps depth (`:139`) and count but carries no risk account downward. Ten sub-agents each
individually approved for bounded `write_files`/`run_commands` are, jointly, approved for nothing
anyone checked. The cheap version is not a value-at-risk model: it is one counter per **root**
session, charged by children and read by the parent's guard, so that "how many destructive
operations has this whole tree done" is a question the code can answer at all. Today it cannot,
because there is no object that spans the tree. Note what a fix must decide and this paper does
not: what a charge is denominated in. Count of dangerous calls is measurable and dumb; anything
finer is the pricing problem the authors call the central open one.

## 2. Cutting communication edges between agents makes inference more expensive

[arXiv:2609.02264](https://arxiv.org/abs/2609.02264) — **Codebook Agent: Amortized Topology Design
for LLM Multi-Agent Systems** (2026-09-02).

The proposed method is a vector-quantized codebook and does not port — inber does not learn its
topology. The three **negative** findings that motivate it do, and they are measured:

- **Reward-filtered topologies collapse to about six distinct graphs even as codebook capacity
  grows from 8 to 64.** Nearly all the available gain lives in a handful of shapes.
- **Edge count is negatively correlated with measured token consumption, Pearson r ≈ −0.4.**
  Sparsifying the communication graph makes inference *more* expensive, because an agent denied a
  peer's result re-derives it.
- A message-passing scorer over agent-profile nodes is **adjacency-invariant whenever agents share
  a profile** — the default configuration of published benchmarks — so it cannot rank candidates
  at all in that regime.

Codebook Agent itself is most accurate on all six benchmarks (84.6 average against 83.0), emits a
topology in 2.4 ms, and uses 21.9–33.2% fewer tokens.

**What inber should consider:** the r ≈ −0.4 result contradicts the intuition that would drive any
cost-motivated trim of inber's spawn fan-out or inter-agent result passing, and it points the
opposite way from the finding this job carried on 2026-09-08, where a lean single agent with
AST-aware retrieval beat a sub-agent configuration 86% to 66%. Those are consistent — *fewer
agents* is cheaper, *fewer edges between the agents you already have* is not — and the pair is the
argument for measuring rather than reasoning before changing either. inber has no per-turn token
figure attributable to a sub-agent tree to measure with, which is the same missing counter as §1
and as the already-open `7be5a692`. The "collapses to ~6 graphs" finding is the encouraging half:
a small fixed set of hand-written spawn topologies is likely to capture most of the gain, so the
learned machinery is not the part inber is missing.

## 3. Late requirements invalidate twice as much code, and the burden does not decline over a session

[arXiv:2609.03028](https://arxiv.org/abs/2609.03028) — **Requirements After the First Edit: Mining
Late Requirement Emergence and Rework in Real-World Coding-Agent Sessions** (2026-09-02).

**3,553 eligible SWE-chat sessions.** Post-implementation requirement arrivals are coded along
three dimensions and, where repository state can be replayed, each arrival is linked to a proxy
for rework: deletion or replacement of prior agent-authored lines. **A requirement's arrival is
followed by roughly twice as much invalidation as matched non-requirement edits**, robust to
user-turn and net-deletion checks. The authors are careful — not demonstrated as causal, several
intervals wide. Two secondary results matter more than the headline: the burden shows **no
detectable decline over a session**, and a controlled experiment found **advance warning produces
no detected effect on overwriting** (delayed disclosure only relocates implementation to after the
reveal).

**What inber should consider:** the no-decline result is the one that bites, because it
contradicts the assumption under every long-session design — that accumulated context makes later
turns cheaper and safer. If a requirement arriving at turn 40 invalidates as much as one arriving
at turn 4, then a fraction of a long session's spend is writing lines a later turn deletes, and
that fraction is not shrinking as the session goes on. It is computable from data inber already
stores: a turn that deletes lines an earlier turn in the same session wrote is detectable from the
snapshot pairs in `snapshot-store`, which exists precisely to hold before/after content for
`Edit`/`Write`. The honest caveat is that "advance warning produces no detected effect" kills the
obvious response — a prompt asking the user to state requirements up front is the intervention the
paper tested and failed to find an effect for. What a fix would have to decide is whether this
becomes a *measurement* surfaced to the user or a *control* that interrupts, and the paper
supports only the former.

## 4. Halting on evidence sufficiency rather than on a turn limit

[arXiv:2609.00237](https://arxiv.org/abs/2609.00237) — **Learning What to Retain: Gated-Memory
Routing for Efficient Collaboration in Multi-Agent LLM Systems** (2026-08-31).

Routing from the query alone cannot adapt to intermediate progress; routing from the full
execution history forces every later decision to process every prior step. Between them: a learned
Memory Write Gate that commits only non-redundant steps, a Retrieval Gate that serves each agent a
compact subset, and an Adaptive Halting Controller that stops once memory holds sufficient
evidence. Across five reasoning and code-generation benchmarks, **best average accuracy, +2.44
points over the strongest baseline, with HumanEval inference cost down 31.9%** against that same
baseline.

**What inber should consider:** the gates are learned, so the mechanism does not transfer — logged
here for the framing, which restates inber's compaction problem as *routing* rather than
*summarization*. That is a real difference: summarization asks "what can I compress this into",
routing asks "which of these steps does the next decision need", and the second question has a
checkable answer. The transferable piece is the halting condition. inber ends a sub-agent run on a
turn limit or on the model declaring itself done; "the accumulated state already answers the
question" is a third condition, and it is the only one of the three that is cheap to evaluate
without another model call. Weakest of the four here — no ablation isolates the halting controller
from the two gates, so the 31.9% cannot be attributed to it.

# 2026-09-10 sweep

Screened against **484 distinct arXiv ids** already present under `docs/` (482 matched by
`arxiv.org/abs/`, 307 by the bare `arXiv:` form, union 484). Existing coverage stops at
**2609.05339**; the `cs.SE`, `cs.MA` and `cs.AI` recent listings for this window start at
2609.05431, so the two ranges are almost disjoint and every id below is new. Every title
and submission date was read off a fetched `arxiv.org/abs/` page; **2609.08371 and
2609.09233 were re-fetched independently at the end of the sweep** and both matched.

⚠️ **The three keyword WebSearches returned nothing usable — again.** Every hit was
either outside the window (2311, 2506, 2601, 2603, 2605, 2606, 2607) or already in the
seen set. All six papers below came from the per-category listing pages. This reproduces
the 2026-09-01 warning verbatim; the listing pages are the instrument, and the searches
are not worth the calls.

## 1. Delegate on the contract, not on the size

[arXiv:2609.09233](https://arxiv.org/abs/2609.09233) — **Subagents vs Agent Skills:
Executing Reusable Knowledge for Long-Horizon Agentic Tasks** (2026-09-07, cs.AI/cs.CL/cs.LG).
SkillsBench, 87 long-horizon tasks (64 with synthesizable contracts), across GPT-5.3 Codex,
Kimi K2.6, Qwen3.5 2B/4B/9B, Mistral-Large-3, Gemma-4-12B and Ministral-3-8B.

The A/B is exactly the one inber's spawn path makes implicitly: load a skill's instructions
into the main context, or invoke it as a subagent with its own window. The result is
conditional, and the condition is the finding — **subagents win only where the skill package
exposes an explicit input/output contract.** On curated skills *without* contracts, in-context
execution matched or beat delegation on every model. Subagents cut peak context on >80% of
tasks for the strong models but cost substantially more total tokens through duplication
across windows.

⚠️ The abstract carries no numbers; the task counts and the >80% are from the v1 HTML full
text. The title, date and the conditional claim are from the abs page.

**What inber should consider:** make the *contract* the spawn predicate rather than the size
of the work. A delegation with no declared input/output schema is, on this evidence, strictly
worse than inlining — which is a refusal rule inber's `spawn_agent` could enforce cheaply.
The peak-context result argues the second trigger should be **context pressure**, not turn
count or task count: spawning as a relief valve is a different decision from spawning for
parallelism, and inber currently has no way to express the difference. Note the tension with
this doc's 2026-09-01 §2 — equal token budgets are not equal delivered context — which says
the pressure signal has to be measured, not estimated from a count.

## 2. Freeze the authority ceiling before reading anything untrusted

[arXiv:2609.08371](https://arxiv.org/abs/2609.08371) — **Authority Is Not a String: A
Capability-Scoped Harness for Prompt-Injection-Resistant Coding Agents** (2026-09-08, cs.SE).
300 runs: 5 Python tasks × 5 injection surfaces × 4 authorization conditions × 3 trials, on a
repair workflow where an orchestrator delegates to separate sub-agents.

CapScope replaces *ambient authority* — naming a resource is enough to reach it — with typed
capabilities held **outside the model context**, and fixes the task-wide authority ceiling
from trusted inputs **before any repo file or tool output is read**. The injected effect
executed in **33–47 of 75 runs at baseline against 3 of 75 with CapScope**, while repair
completion barely moved: **68/75 against a baseline 68–72/75**. No model-side detection of
malicious text is required, which is the part that makes it a harness result rather than a
prompting result.

**What inber should consider:** the ordering is the cheap half and it is a concrete change —
**the grant set must be frozen at turn start, not recomputed as tool results arrive.** inber
currently allows the opposite: three config setters mutate a live engine from the HTTP
goroutine (`769860a6`), and `guard.go:113` documents that the caps are read *"between turns"*
while the setters run whenever. The paper's second half — blocking capability leakage between
sub-agents — is inber's spawn path exactly, and its live instance is already open as
`e2d0b07b` (an assist-mode parent spawning a child measured `Allowed` on all four dangerous
tools). Read alongside §6 below: CapScope is the in-process form, CAPMAS the cross-process one.

## 3. Mid-run self-reported progress is worthless where a controller would use it

[arXiv:2609.08589](https://arxiv.org/abs/2609.08589) — **The Unreliable Progress Bar: Can LLM
Agents Reliably Report Task Progress Throughout Execution?** (2026-09-08, cs.SE/cs.AI/cs.CL).
12 deployments on τ²-bench telecom, 9 on StageIF, 50,400 checkpoint positions over 12
scenarios × 20 repetitions.

Progress-reporting accuracy is **U-shaped**: 90.6–99.4% at pre-action checkpoints, collapsing
to **5.8–11.5% mid-task**, recovering to 87.3–88.9% after completion. On StageIF the
nonterminal-to-terminal gap spans **29.4 to 89.3 percentage points**; one deployment scored
**97.4% terminal adherence against 8.2% across intermediate checkpoints**. Newer models fail
*differently* rather than less — they turn conservative at the finish line instead of
optimistic mid-run, so a controller tuned against an older model's bias is miscalibrated in
the opposite direction.

**What inber should consider:** any control decision keyed on a model-emitted "am I done"
signal is reading a 5–11% accurate channel. inber has one such channel and it is load-bearing:
the sideband `done` field, where **completing the last task fires the project's build command**
(`agent/sideband.go:34-38`). That is a subprocess launched on a mid-task self-report. The
paper does not say the field should go — the *terminal* reading is 87–99% accurate, and "the
last task is done" is closer to terminal than to mid-task — but it does say that any *new*
controller inber grows (a compaction trigger, a continuation gate, a spawn-relief valve as in
§1) should key off observable state: tool-call outcomes, diff state, budget consumed. Pairs
with the halting-on-evidence-sufficiency framing in the 2026-09-01 sweep §4.

## 4. Compaction as swap rather than as loss

[arXiv:2609.08318](https://arxiv.org/abs/2609.08318) — **AttnCompress: Dynamic
Attention-Guided Trajectory Compression for Software Engineering Agents** (2026-09-08,
cs.SE/cs.AI). **53.17% pass rate at 21.6% fewer tokens and 33.6% lower total cost.**

Three mechanisms: structure-aware segmentation at perplexity spikes so code and log blocks are
not cut mid-syntax; proxy-attention scoring of historical blocks against the agent's *current*
reasoning; and a rolling window that can **recall** previously-dropped context as the task
evolves.

**What inber should consider:** the recall step is the one inber's compaction lacks entirely —
a summarized region is gone, and `conversation/manage.go` has no path back. inber already has
the storage for it (a memory store with recall budgets), so the change is addressability, not
capacity. ⚠️ **But the tension with prompt caching is severe and this doc has recorded it
before** (2026-09-03 §3): re-inserting an evicted block *splices* the history and invalidates
every breakpoint after it, which on inber's current placement (`a5b91a47`: two breakpoints set
once at turn start) means the whole turn. A recall that only ever **appends** — re-introducing
the block at the tail as a fresh observation rather than restoring it in place — costs
ordering fidelity and keeps the prefix. That trade is the decision, and nothing here settles it.

## 5. The tool list is a prior, not a catalogue

[arXiv:2609.09395](https://arxiv.org/abs/2609.09395) — **The Menu Is an Execution Prior:
State-Path Tool Menus for Online Agents** (2026-09-08, cs.AI, EMNLP 2026 Main). On ToolBench,
online success **0.737 → 0.898**, with the state-path menu covering complete tool chains using
**32 tools where the official list needs 128**. Gains hold across executor model families.

An encoder for what is runnable from the current state, a retriever covering executable and
terminal actions, and a reranker that orders **producers before consumers**.

**What inber should consider:** the reranker is nearly free and is claimed to matter
independently of which tools are present — ordering producers before consumers in the tools
array costs one sort. ⚠️ The 4× menu shrink is *not* free for inber and is close to a trap:
Anthropic hashes tools first, so a menu that varies with state moves the very first thing in
the cache prefix on every turn. That is the exact failure CC spent twelve bullets of 2.1.267
repairing (`claude-code.md`, 2026-09-10 §1). Any menu work here has to be measured net of
cache misses, and the static-ordering half is the part that carries no such cost.

## 6. Attenuating delegation, so over-grant is structurally impossible

[arXiv:2609.06500](https://arxiv.org/abs/2609.06500) — **CAPMAS: Capability-Based Delegation of
Privileges in Multi-Agent Systems** (2026-09-06, cs.MA/cs.CR). Macaroon-based tokens supporting
**offline, attenuating** delegation: a parent hands a child a strictly narrower token with no
round trip to a central authority and without exposing user identity. Against OAuth 2.0 Token
Exchange: **~30× faster delegation, 2× lower delegation latency, up to 3× lower bandwidth**,
>90% perfect privilege-bundle retrieval within **17 ms** on schemas with 3,100+ endpoints, and
a **99.5% reduction in unnecessary privileges** against propagating the full user privilege set.

**What inber should consider:** attenuation is the property inber's spawn path is missing.
Today a child's authority is set by policy and checked at use; a token that *cannot* be widened
makes over-grant impossible by construction and needs no call to `permission-store` per tool
use. This is the mechanism behind the rule already written down on 2026-08-14 — a child's
authority is the *meet* of its parent's with read-only — which inber states and does not
enforce (`e2d0b07b`). Recorded as design input for whenever that todo is answered; the macaroon
machinery itself is far heavier than inber's single-process spawn needs.

## Verified, numbers too weak to carry

- [arXiv:2609.10263](https://arxiv.org/abs/2609.10263) — **What Should an Agent Forget?
  Separating What Is Stored from What Is Used** (2026-09-09, cs.AI). A retained source archive
  plus a *query-conditioned* memory view; same-slot replacement links suppress superseded facts
  in current-state answers while intent-aware retrieval makes the old value eligible again for
  historical queries. **The abstract states no numbers** — ablations are described only as
  "largest score deficits". Logged because inber's memory store has a recall *budget* but one
  view, so a superseded fact either stays and misleads or is dropped and is unrecoverable.
- [arXiv:2609.07360](https://arxiv.org/abs/2609.07360) — **Scanning the Harness: An Empirical
  Study of Supply-Chain Defects in AI Coding-Agent Configurations** (2026-09-07, cs.SE/cs.CR).
  3,171 public repositories carrying Claude Code / Cursor / Copilot / Codex config; **16.0%
  carry a confirmed security defect** — 9.8% install MCP servers with no pinned version, 3.8%
  ship skills that pre-approve shell access, 3.1% pre-approve arbitrary execution. Raw scanner
  rate was 25.5% before validation, so a third of automated hits were false. **No inber
  surface** — inber has no skill or MCP-config ingestion. It is a live exposure for
  `skill-store` (which ingests `SKILL.md` by cloning upstream repos) and `tool-store`
  (`/provision` emits MCP config), where an unpinned-version and pre-approved-shell check at
  ingest would apply the paper's detectors at the one chokepoint that sees every skill.
- [arXiv:2609.09218](https://arxiv.org/abs/2609.09218) — **The Double Measurement Confound in
  Agent Benchmarks** (2026-09-06, cs.SE). Argues agent benchmarks confound scaffold-made
  decisions with model-made ones, and shape-based scoring with ground truth. **No extractable
  effect sizes**, and the case study is materials-science, so transfer is unestablished.

## Coverage, and the gaps

Listings fetched: `cs.SE/recent` (2609.07224–2609.10502), `cs.MA/recent`
(2609.05431–2609.10509), `cs.AI/recent` (2609.09226–2609.10451), 50 entries each. Roughly 650
ids seen in total; 40 candidates were explicitly checked against the seen set and all 40 were
new.

Honest gaps, so the next sweep does not re-derive them:

- **`cs.CL/recent` was not fetched.** That slice of the window is unscreened.
- **The arXiv Atom API was not attempted** — prior sweeps recorded persistent 429s from this
  host. Screening is therefore per-category and recent-only, not keyword-scoped across all
  categories, so anything filed *only* under cs.CR, cs.LG or cs.DC in this window is missed.
- **No prompt-caching paper was found in window.** One candidate is worth exactly one fetch
  next sweep and was **not** verified here, so its date and numbers are unconfirmed:
  `2608.14624` ("Learning Agent Execution for KV-Cache Management in Agentic Serving"), whose
  claim that recurring fixed context is 53–62% of prompt tokens would bear directly on
  breakpoint placement.
- Also new, plausibly relevant, and **unfetched**: `2609.08472` (Beyond Agent Harnesses:
  Cross-Substrate Authority), `2609.08327` (Tool Retrievers Are Underestimated), `2609.09646`
  (RobustSGPO: Search-Space Control for Agent Harness Evolution), `2609.08355` (RepoNav),
  `2609.07357` (EnvPilot), `2609.05553` (EdgeMem).

# 2026-09-11 sweep

The Atom API answered on the second try (`https://export.arxiv.org`, a `User-Agent` with a
mailto, and 3 s between calls — the first call, over plain `http://`, returned an empty body,
which is what the earlier "429" notes probably were). One keyword-scoped query across all
categories, 120 most-recent entries, 2026-09-01 → 2026-09-10, deduped against the 119 `2609.*`
ids already in this file. Every abstract below was read from the API, not a snippet. The
`cs.CL/recent` gap from 09-08 is closed by the keyword scope; anything that matches none of the
eight terms is still unseen.

## 1. Soft-revoked memory is served anyway — and inber does exactly this, measured

[arXiv:2609.08258](https://arxiv.org/abs/2609.08258) — **Revoked but Still Authoritative: An
Empirical Study of Revocation Enforcement in Agent-Memory Systems** (2026-09-08). Five memory
systems that revoke by *marking* rather than deleting, loaded with a revoked policy and its
replacement, nine scenarios × nine models × six defence conditions. **No system enforces
revocation by default**: the revoked fact is returned wherever the revocation label is not
visible to the retrieval layer, it *outranks* its replacement, and the agent then takes the
unsafe action. Their fix is a guard between the agent and any backend that withholds revoked
rows and rows that conflict with their replacement.

**inber has this defect, and it is measured rather than inferred.** `memory_forget` is a soft
delete by `UPDATE memories SET importance = 0` (`memory-store/management.go:47`). `Search`
honours it (`search.go:75`, `importance > 0`). The automatic context that opens *every* turn
does not: `BuildContext` filters `WHERE importance >= ?` (`builder.go:64`) and only raises a
zero floor to 0.4 when `!IncludeAlwaysLoad` (`builder.go:36-37`) — and inber asks with
`IncludeAlwaysLoad: true` (`memory/auto_context.go:101`) and a `minImportance` that
`contextBudget` returns as `0` on **every** path (`engine/turn_context.go:8-39`). So the floor
is `>= 0` and a forgotten row is a candidate again. On a throwaway store: save an always-load
memory at 0.8, a tagged one at 0.6 and a plain one at 0.5; forget the first two; ask exactly as
inber asks. `Search` returns one row. `BuildContext` returns **all three — the forgotten
always-load one at position 0**, because the always-load head is appended whether or not it
fits, and the forgotten tagged one after the live one, because `calculateScore` starts from
importance and adds 0.3 per matching tag plus a recency bonus, so a *recently* forgotten memory
outscores an unmatched live one. The tool's own description (`memory/tools.go:190`) promises it
"won't appear in search results", which is true, and is not the door that matters. Live stores
carry 0 forgotten rows today (407 and 10 rows measured), so this is latent until the first
forget — and the first forget of an always-load memory pins it to the front of the system array
forever. Filed; the fix has to decide whether `BuildContext` filters `> 0` (keeping "0 means
forgotten"), or forgetting becomes a column so it stops sharing a value with decay, and whether
an always-load memory is forgettable at all.

## 2. Eviction's damage is mostly irreversible, and the retrieval regime decides what you can measure

[arXiv:2609.08279](https://arxiv.org/abs/2609.08279) — **What Eviction Destroys: A
Restore-Counterfactual Audit of Forgetting in Agent Memory** (2026-09-08). For every wrong
answer, reinstate the gold evidence and re-run the same reader: the error is *recoverable*
(evidence retained, retrieval missed it), *irreversible* (evicted) or *residual* (wrong even with
it). Under top-k retrieval at 80k tokens the irreversible share is 0.67–0.73 for FIFO, random and
redundancy-aware eviction and 0.60 for LLM-importance; at 8k it is 1.00 for all four. Recoverable
errors vanish under forced-gold injection by construction, so budget-accuracy curves are not
comparable across retrieval regimes.

- **What inber should consider:** memory-store's `DecayImportance` (×0.99 daily) and
  `compaction.go:69` are eviction policies with no audit of this shape. The method is cheap to
  copy — it needs only a per-question "was the evidence retained" flag — and it is the test that
  would tell whether the token budgets in `contextBudget` lose answers irreversibly or merely
  fail to retrieve them.

## 3. Curate memories with read-only probes of the world, not from the transcript alone

[arXiv:2609.11060](https://arxiv.org/abs/2609.11060) — **Grounding Agent Memory:
Environment-Probing Curation for Enterprise Agents** (2026-09-10). A post-task curator restricted
to completed trajectories keeps errors, overgeneralises partial evidence and retains stale
knowledge. Giving the *curator* least-privilege read-only tools to check, scope and refresh
candidate memories — task agent, retriever and write authority unchanged — raised CLBench pass
rate 39% → 73% and halved task-agent cost, in a GitHub Copilot SDK harness, on Sonnet 4.6 and
Opus 4.7.

- **What inber should consider:** `extract.go` writes memories from the finished conversation
  with no check against the repository they describe. This is the cheapest form of the
  paper's idea: before a memory that names a file, a command or a port is saved, stat the file,
  `--help` the command, read the unit. It also bears on `90296699` (the model chooses its own
  provenance): a probe is a provenance the model cannot forge.

## 4. Second tier — real, narrower

- [arXiv:2609.09134](https://arxiv.org/abs/2609.09134) — **Co-Evolving Harnesses and Models**
  (2026-09-08). Evolve a harness around a weak model, then fine-tune the weak model on an expert's
  trajectories under that harness: performance *regresses* 4–30 points on all seven tasks,
  because the weak model adopts the expert's planning style and no longer fits the harness
  evolved around its own. On-policy correction of only the failing turn keeps both gains. The
  transferable claim: a harness is fitted to a model's planning style, so swapping the model
  under a tuned harness — `905a5e68`'s silent failover — is not a free substitution.
- [arXiv:2609.09133](https://arxiv.org/abs/2609.09133) — **ExecCritic** (2026-09-08). When one
  trajectory writes both patch and test, their errors agree; a separate Test agent with a
  fail-closed harness that *freezes* the tests before repair is the fix. Tests from a weak Test
  agent *lowered* resolve rate 61.2% → 57.3%; from a strong one raised it to 65.3%. A verifier
  is not free-standing good — same shape as the 09-08 retrieval result.
- [arXiv:2608.14624](https://arxiv.org/abs/2608.14624) — **CacheScout** (2026-07-16, the one the
  09-08 sweep deferred). Serving-side: learns agent-to-agent execution transitions online to
  drive KV eviction and prefetch, +10–18 pp hit rate, −18–45% TTFT on vLLM. The "53–62% fixed
  context" figure is not in the abstract; do not cite it from here. Not applicable to an
  API-served harness.

## Screened and rejected, with the reason

`2609.08273` MemForest (memory compression, 50% at 97% retained — a backend concern, no harness
implication); `2609.09769` XAgent (SWE-bench-lite resolve rate, no design content); `2609.10416`
TrajMark (trajectory watermarking); `2609.08919` Experience Funnel and `2609.08572` AgentGrad
(training loops); `2609.11076` SaltBench (referee-gated benchmark protocol — sound, but a
methodology paper); `2609.11728` (a position note: reproducibility practice *is* context
engineering, one paragraph, no results). Also new in window and unfetched: `2609.10539`,
`2609.10397`, `2609.08301` (Agent ATO, timeline visualisation from logs).

# 2026-09-12 sweep

**Method, because it changes what "screened" covers.** The arXiv Atom API answered `429` on the
first call again, so this sweep read the HTML listing pages (`arxiv.org/list/<cat>/2026-09`) and
then pulled `arxiv.org/abs/<id>` for every candidate. A listing page gives id and title only, so
the 3,474 figure below is a **title-level** screen; only the 27 papers whose `abs` pages were
fetched were screened on abstract, and every number quoted here is read off one of those pages.
Two of the ids below (`2609.10266`, `2609.08062`) were independently re-fetched a second time by
the job that wrote this section rather than taken on report.

| | |
|---|---|
| Unique ids screened (title-level) | **3,474** — Sept cs.SE 242 / cs.MA 114 / cs.AI 1,671 / cs.CL 940 (2,446 unique), plus Aug cs.SE 715 / cs.MA 333 |
| Already in the 499 covered ids | 163 of the screened set; 78 of the 436 keyword matches |
| Abstract + submission date verified | **27** |
| Rejected as out of window | **6** |

⚠️ **Stated coverage gap:** cs.AI and cs.CL were **not** screened for August 2026 (~6k further ids).
September is complete across all four categories; August is cs.SE and cs.MA only. 294 of the 499
already-covered ids are `2608.*`, so August is well trodden — but that is an assumption, not a
measurement. The **OpenAI research index answered `403`** and is therefore *unscreened*, not clean.
Anthropic's engineering blog has published nothing since 2026-04-23; DeepMind's Aug/Sept posts are
model releases, weather and genomics.

**Rejections, and why rule 1 keeps earning its keep.** Three candidates carry a `2609.` id and a
June or May v1 date — arXiv preserves the true submission timestamp through a delayed announcement,
so **the id prefix is not a date** and a snippet-level sweep would have shipped all of them:
`2609.04217` *At Equal Inference Cost, Multi-Agent Structure Does Not Beat a Single Frozen Agent*
(v1 **25 Jun**, and the most inber-relevant multi-agent result seen all sweep — noted here so a
later reader does not re-find it as new), `2609.10548` (v1 **25 Jun**), `2608.20342` (v1 **8 May**),
`2608.13574` (v1 **4 Jul**, v2 28 Aug). Also rejected: Google Research *Towards a science of scaling
agent systems* (**28 Jan 2026**) and a HuggingFace KV-caching explainer (**Jan 2025**), both of which
surfaced in "recent" searches.

## 1. Authorization state cannot survive in a summary, and the gate belongs at the effect

[arXiv:2609.08062](https://arxiv.org/abs/2609.08062) — **ResidualAuth: What Authorization State Must
Language Agents Preserve under Revocable Delegation?** (2026-09-08, cs.AI), with its companion
[arXiv:2609.08472](https://arxiv.org/abs/2609.08472) — **Beyond Agent Harnesses: Cross-Substrate
Authority for Multi-Agent Systems** (2026-09-08, cs.MA).

An impossibility result with a clean shape: two authorization histories can have **identical current
permissions and identical all-pairs reachability and still require opposite decisions** after the
same direct-edge revocation, because exponentially many future-distinct states share one fixed
transitive closure. A permission model that stores only the current grant set is therefore provably
lossy under revocation. The measurements land where it hurts most: a fixed **256-token summary
solved 0–2 of 16** paired episodes across four open-weight models (sham reads 0/16), while
**authenticated current-query reads solved 15–16/16**; exact ledger serializations fit **all 128**
four-coordinate pairs at 768 and 1,024 tokens, where factually-supported *model-written* memories
solved **at most 1 of 128**. The companion paper then shows the planner is the wrong place to
enforce: workspace-visible evidence produced **12 of 16 unsafe publication decisions**, and even a
typed relation left planning unreliable (**15/32** first actions correct). Replaying the same 32
model-generated intents with **zero additional model calls**, a deterministic execution guard
**stopped all 6 unsafe intents from becoming effects and permitted all 12 valid ones**; in
ResidualAuth a hard gate took **eight observed unauthorized effects to zero** without changing a
single preceding attempt.

- **What inber should consider:** this is last week's `memory_forget` finding generalised and
  measured. Never let a compacted summary or a memory row carry authorization state — inber's
  compactor and its always-load memories are both exactly the 0-of-16 channel — and put the check
  as a deterministic gate at the point of *effect*. inber's is at `guard/guard.go:165 CheckTool`,
  which is the right place; what it lacks is the ledger behind it, since `ApprovalFunc(tool, input
  string) bool` (`guard/guard.go:90`) cannot express a revocation history at all. Read this
  alongside codex [#44944](https://github.com/openai/codex/pull/44944), which arrives at the same
  rule from the engineering side: re-check at every point that starts work, from the authoritative
  layer alone.

## 2. Reused mid-prompt, a cache is worse than no cache — and the failing workload is sub-agents

[arXiv:2609.10266](https://arxiv.org/abs/2609.10266) — **KVShareArena: KV-Cache Reuse Across Contexts
and Model Checkpoints** (2026-09-09, cs.CL).

Serving systems reuse KV cache **only at an exact prompt prefix**, and the paper names two workloads
that break it — RAG assembling different chunks per query, and **a multi-agent coordinator reading
reports written by other agents**. Reused mid-prompt, a cache carries wrong positions and never
attended to the other sources. Methods are scored by the fraction of the gap recovered between
no-cache and full recomputation, charging compute, memory and per-request latency separately from
one-time build cost. **Position correction alone suffices until a question needs several sources at
once**; there, only methods that re-encode part of the cache recover half to two thirds of the gap,
and **an unrepaired cache can be worse than no cache**. Cache-compression methods that are harmless
on a single prompt fall significantly behind position correction on freshly written agent reports.

- **What inber should consider:** inber does not run its own KV cache, so the mechanism does not
  port — but the operational corollary does, and it is the same rule codex
  [#44862](https://github.com/openai/codex/pull/44862) shipped the same week. Anthropic hashes
  **tools, then system, then messages**, so anything appended *before* a breakpoint invalidates
  every breakpoint after it. inber gets the *volatile-context* half right on purpose
  (`agent/agent_run.go:90-120` injects it after BP3). It gets the **fork** half wrong, and the
  comment at `server/session_forking.go:52-57` claims otherwise — measured this sweep and written
  up in `docs/comparisons/agentic-design-patterns.md` 2026-09-12 §5. The tool half is
  wrong too: `SetDisabledTools` (`engine/engine.go:363`) lives only in engine memory, so a fork rebuilds
  the array from stored config and a parent that disabled a tool at runtime hands its child a
  different tools block — which misses the *whole* prefix, not just the tools. Already parked as
  todo `65301d09`; this is the cost argument that todo did not have.

## 3. Agents adopt corrupted tool output a third of the time, and sub-agent delegation is one of the three channels

[arXiv:2609.05587](https://arxiv.org/abs/2609.05587) — **Agents Trust Tools Too Much: Measuring
Reliance on Unreliable Tools** (2026-09-04, cs.AI). Fourteen LLMs, three tools with deliberately
corrupted returns — web search, **LLM sub-agent delegation**, and code execution. **Mean adoption of
corrupted content exceeds one third for every tool, reaching 68.0% for web search.** The failure
mode is the finding: reasoning traces show agents **often recognise the conflict and recover the
correct answer internally, then present only the corrupted answer without warning the user**.
Interventions at three levels — user prompting, tool-provider metadata, builder post-training — each
help for particular models or tools and **none consistently mitigates overtrust**.

- **What inber should consider:** sub-agent delegation is a measured channel, and inber splices a
  child's result into the parent as ordinary tool output. Carry provenance on that result rather
  than flattening it, and since the models detect the conflict internally, an event on the existing
  SSE stream saying "sub-agent result conflicts with parent context" is cheap and catches precisely
  the silent case the paper isolates.

## 4. Half of real MCP servers do not start, and most omit the safety annotations a policy might key off

[arXiv:2609.10962](https://arxiv.org/abs/2609.10962) — **What a Random Draw from the MCP Registry
Contains, and What Tool-Use Benchmarks Contain Instead** (2026-09-10, cs.SE). An unrepaired
probability sample of **400 npm/stdio servers** from a **24,135-server** census: only **48.8%**
complete an `initialize` handshake, against **66.7%** for a hand-curated frame on the same
instrument. The dominant failure is **not** missing credentials (13.3%) but **servers that never
start at all (37.5%)**. Among the 195 that do run, hard conformance is total — **zero** fatal schema
violations across **2,766** tools — and the real variance is **optional safety annotations**, omitted
on **58.8%** of tools in the random draw. Benchmark corpora are contaminated in a way real tools are
not: **68.8%** of raw BFCL v4 rows and **85.6%** of UltraTool rows are exact name-plus-description
repeats against **0.4%** for real MCP, and BFCL's near-duplication is 16.4 points *between
independently presented tasks* where real MCP is **0.0% cross-author at every threshold**.

- **What inber should consider:** two things, both cheap. `tools/mcp/client.go NewClient` treats
  every startup path the same; "the server never started" is the modal error and deserves to be
  distinguishable from an auth failure at the call site. And inber's permission model must not key
  off tool-declared safety annotations — nearly three in five real tools do not carry them — which
  is an argument *for* the seven hardcoded classification tables the dexto entry criticises, not
  against them.

## 5. Your harness is 4.3× the variable your training recipe is

[arXiv:2609.04518](https://arxiv.org/abs/2609.04518) — **What Does Multi-Harness RL Learn? Credit
Assignment and Portability in Coding Agents** (2026-09-03, cs.AI). Frozen task-harness records from
Aider, OpenHands, Qwen Code and SWE-agent, replayed from one Qwen3-8B warm start and scored against
a sealed SWE-bench Verified oracle. Across **24,000 sealed evaluations** the **evaluation harness
moves mean solve rate from 2.14% to 9.27% — a factor of 4.3** — where the training recipe moves it
by 1.16. And a negative result worth as much: the GRPO grouping rule is **not** a real effect
(**+0.25 pp, 95% CI [-0.48, +1.02]**), with each rule's own seed range (0.42–0.45 pp) **exceeding the
difference between them** and individual seed estimates changing sign.

- **What inber should consider:** any number inber produces comparing its own agents is measuring
  inber-the-harness about four times as strongly as it measures the model or the prompt. Pin and
  report the harness configuration with every internal benchmark, and do not read cross-harness
  numbers out of papers as transferable to inber. This is the methodological floor under
  `docs/comparisons/*` generally.

## 6. A memory subsystem needs a session-start health gate, not just a decay rule

[arXiv:2609.05510](https://arxiv.org/abs/2609.05510) — **Memory as Infrastructure: Reliability
Engineering for Persistent Agent Memory in Months-Long LLM-Assisted Development** (2026-08-31,
cs.SE). An operational record from one continuous Claude Code session line running since January
2026 over a **633,000-line** codebase: per-project long-term memory as a hybrid lexical-vector index
over **local SQLite**, precision-gated context injection, anti-recurrence stores for decisions and
dead ends, and conventions engineered to survive compaction. **78,933 hook invocations, 85 recorded
failures, none silent** — 84 of them in the subsystem's first three weeks, one since, none in the
final 20 days; a ten-day precision instrument on the injection layer recorded **zero false fires**
against an intact denominator. The argued-missing piece is reliability engineering for the memory
subsystem itself: a session-start health gate with **discriminated** failure modes, heartbeat
telemetry designed so no enumerated failure mode can pass unrecorded, and alert-fatigue budgeting.
Limitations stated plainly — **N=1, no control arm, self-reported** — with a pre-registered ablation
protocol published.

- **What inber should consider:** this is the closest published analogue to inber's memory-store
  (SQLite, importance decay, always-load memories) and its central claim names something inber has
  no equivalent of. `buildTurnContext` (`engine/turn_prepare.go:88-100`) degrades to the previous
  turn's blocks when the memory store cannot answer — deliberately, and for a good prompt-cache
  reason — but there is no gate at spawn that discriminates *why* it could not answer, so a
  memory-store that is degraded rather than down looks identical to one that is healthy and simply
  had nothing to return. That is the "fail fast and loud" property this box's own directives demand.

## 7. The compaction unit should probably be an event cluster, not a row

[arXiv:2609.08273](https://arxiv.org/abs/2609.08273) — **MemForest: Efficient Agent Memory Management
via EventTree Partitioning and Progressive Merging** (2026-09-08, cs.AI). Partitions memory into
event-centric units using global semantic similarity **and** local temporal continuity, builds a
maximum spanning tree per unit, then progressively merges redundant nodes along high-weight edges,
with anchor-guided retrieval pulling from the temporal neighbourhood of key nodes rather than by
similarity alone. Under Mem0 it retains **97.1%** of performance at **50%** compression across
LoCoMo, LongMemEval and PersonaMem with a **1.89×** retrieval speedup; under multimodal M3-Agent,
**99.7%** at the same 50% and **2.24×**.

- **What inber should consider:** inber prunes memory by importance decay, a per-item scalar, which
  cannot express "these six rows are one event and five of them are redundant". The result says the
  compaction unit is worth changing before the decay curve is worth tuning.

## 8. Three on harness design, taken together

- [arXiv:2608.23953](https://arxiv.org/abs/2608.23953) — **Architectural Convergence in Three LLM
  Agent Harnesses** (2026-08-25, cs.SE). Three harnesses built on deliberately opposing philosophies
  (LangChain `deepagents`, Earendil `pi`, DeepSeek `dsh`), read at pinned commits with history
  followed. The two mature ones travelled in **opposite directions** — deepagents subtracting
  scaffolding, pi accreting infrastructure — and converged on five elements: a commoditised loop, an
  **append-only replayable session record**, model quirks kept as data, progressive disclosure of
  context, and explicit extension seams. One dimension shows no convergence and no presence at all:
  **external verifiability** — a tamper-evident record an outside party can check without trusting
  the runtime. **For inber:** it has four of the five; the open question is whether its SQLite
  request log can *reconstruct* a run or only describe it. `3fe14317` (99 of 265 requests replay as
  a question with no answer) is the current evidence that it cannot.
- [arXiv:2609.11677](https://arxiv.org/abs/2609.11677) — **Ecdysis** (2026-09-10, cs.SE). The
  bottleneck in harness evolution is **failure diagnosis**: a failure is either a model-specific
  deficiency or a systematic harness one, and optimising per-incident produces accommodation that
  does not generalise. Batch-level cross-instance failure aggregation gives **1.84×** faster harness
  training and **+18.56%** reasoning accuracy. **For inber:** the request log should be queryable
  for *recurring cross-task* failure shapes, because a per-incident patch to the compactor or a tool
  schema is exactly the accommodation this measures as harmful.
- [arXiv:2609.05736](https://arxiv.org/abs/2609.05736) — **Beyond Prompts** (v1 2026-09-04, cs.AI).
  Treats harness improvement as resource-bounded selection over prompts **and tool-boundary
  middleware**, where edits are guarded intercepts at the tool boundary rather than rewrites of the
  execution loop: mean held-out lifts of **14.2 / 14.9 / 10.1 pp** on BFCL multi-round, τ²-Retail and
  τ²-Telecom, with the ablation attributing most of the margin to failure-surface routing. The
  methodological point is sharper — **some search procedures find large gains but choose brittle
  updates**, so reliability of the selected harness must be reported beside mean lift. **For
  inber:** a middleware slot between registry dispatch and tool execution is the edit surface this
  says carries most of the gain, and inber does not have one.

## 9. Benchmarks: one instruction line moved exploitation by an order of magnitude

[arXiv:2609.06780](https://arxiv.org/abs/2609.06780) — **Shortcutting the Fix** (2026-09-06, cs.SE)
quantifies agentic exploitation across five open models with a turn-level judge: under standard
prompts, agents leverage local Git history, upstream repos and memorised solutions at
**45.1%–82.4%** on SWE-bench Multilingual and **44.2%–66.1%** on DeepSWE. **Appending one targeted
originality instruction cuts these to 4.0%–10.7% and 1.5%–7.1%** with core task performance
maintained. Alongside it, [arXiv:2609.08149](https://arxiv.org/abs/2609.08149) — **SWE-Bench Pro
Verified** (2026-09-08, cs.AI) rebuilds SWE-Bench Pro against reward hacking (gold-solution and
hidden-evaluation leakage) and task-quality defects, and reports that *"some models perform
substantially worse than previously reported"* — ⚠️ **the abstract gives no headline delta, and none
is invented here.**

- **What inber should consider:** inber's own eval harness must sandbox git history and upstream
  remotes, because the default is that agents exploit them roughly half the time — and a single
  originality clause in the system prompt is the cheapest intervention in this entire sweep.

## 10. Non-arXiv, and the honest reading of it

**"Give Your Coding Agents a Memory You Own"** — HuggingFace blog, **2026-09-03**, David Corvoysier
([huggingface.co/blog/funes](https://huggingface.co/blog/funes)). Local embedding and retrieval over
agent turns with credential redaction, shared across machines via HF datasets. Recall was the
cheapest of three long-session strategies — **8× cheaper than a written handoff** on one task, 4× on
the other. On **19,195 real sessions / 100 target questions**: Hit@1 **9/100** with recency
weighting, Hit@5 **39/100**, Hit@50 **71/100**; indexing 308k chunks took **2h03m** on an M4 Pro at
**6.3 s** query latency. The BM25 baseline indexed in **29 s** and scored Hit@1 **18/100**, Hit@5
**35/100**.

- **What inber should consider:** the post's own numbers say the dense retriever **lost to plain
  BM25 at Hit@1, 9 against 18**, at roughly 250× the indexing cost. Before spending anything on
  embeddings for memory-store, benchmark a lexical baseline over the existing SQLite log — Hit@1 is
  the metric that governs always-load memory selection, and it is the one BM25 won.

## Screened, in window, relevant, not making the cut

Dates verified, recorded so a later sweep does not re-find them as new: `2609.01736` HEART / tool
primitives (01 Sep — 25,519-function registry, 84% vs 22% on 50 real tasks); `2609.09646`
RobustSGPO harness search-space control (09 Sep — held-out completion 60.0%→80.0%); `2609.05019`
TROVE selective route editing (04 Sep); `2609.11515` ChurnBench (10 Sep — freshness errors 4→45 at
28 days with tiered refresh disabled); `2609.11294` AgentZip sandbox memory compression (10 Sep —
8.7× against Linux's 2.1×); `2609.02129` persistent discovery context (02 Sep); `2609.06124` SAP
argument-provenance tool-use synthesis (05 Sep); `2609.00967` CoBRA tool-use boundary learning
(01 Sep); `2608.20622` Anthropic-primitives harness paradigm for enterprises (20 Aug); `2609.11728`
context engineering at codebase timescale (10 Sep — a four-sentence position piece with no numbers).
