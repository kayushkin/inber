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
