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
