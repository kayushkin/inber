# Harness Research — July 2026

Notes from the July harness-watch sweeps. Every paper here passed the
not-already-covered check against `2026-04/05/06-harness-research.md` and
`agentic-design-patterns.md`, and every `published` date was confirmed against the
arXiv Atom API rather than a search snippet (search results routinely surface
out-of-window work with in-window-looking IDs).

- **2026-07-13 sweep** — four papers; the two routing papers are best read as a
  **pair that disagrees**, and the disagreement is the point (cross-cutting note
  at the end of that section).
- **2026-07-14 sweep** — six papers in three groups (non-summarizable context;
  the approval-view fidelity gap; retrieval as a trajectory-time loop). Jump to
  `# 2026-07-14 sweep` below.
- **2026-07-18 sweep** — two papers that reframe this cycle's permission-enforcement
  thread as *isolation*: a five-boundary isolation taxonomy (a lens for inber's
  own harness-control-matrix) and a subagent-orchestration benchmark whose headline
  is that the leader's bottleneck is **least-privilege granting**, not perception.
  Jump to `# 2026-07-18 sweep` below.

# 2026-07-13 sweep

## ActPlane: Programmable OS-Level Policy Enforcement for Agent Harnesses

[arXiv:2606.25189](https://arxiv.org/abs/2606.25189) — submitted 2026-06-23 (v2).
Code: [eunomia-bpf/ActPlane](https://github.com/eunomia-bpf/ActPlane).

Argues that tool-call guardrails are structurally blind: they see the *tool
call*, not the *system actions it causes*. ActPlane gives the agent a policy DSL
close to natural language, compiles those rules into **eBPF programs enforced in
the kernel at the syscall layer**, and tracks cross-event state with an
**information-flow model** (labels marking which sources influenced each object,
propagated across process/file/network operations). It explicitly benchmarks
against prompt-filters and tool-regex "as used in Claude Code hooks" and beats
them on *indirect* execution paths that tool-call interception cannot observe:
2.0–3.2× better violation-trace outcomes, 74% of baseline-unsafe behaviors
prevented on 361 safety tasks, at 1.9–8.4% end-to-end overhead. It also repairs
the sandbox feedback failure mode — the agent gets a human-readable "why you
were stopped and what to do instead" instead of an opaque `EACCES`, so it
re-routes rather than flails.

**What inber should consider:** this names the exact blind spot in bridge-server's
PreToolUse prehook. A prehook that approves `Bash(./deploy.sh)` has approved
*everything that script then does* — every write, every socket, every `rm`. The
gate sees one string; the blast radius is a whole process tree. Three adoptable
pieces, in increasing cost: **(a)** make prehook denials *semantic* — return why
it was denied and what would be allowed, not a bare block (cheapest, and the
paper shows it's what keeps the agent from retrying variations of the same
forbidden call); **(b)** carry **taint/provenance labels** on prehook decisions so
content that entered via a web fetch or an ingested skill-store repo cannot reach
a write to `~/.ssh` or a push — this composes directly with the memory-poisoning
provenance work already noted for [2606.04329](https://arxiv.org/abs/2606.04329),
and with the "approval authority" entry in `agentic-design-patterns.md` (07-13);
**(c)** the full eBPF plane under the harness, which is the real fix but a
project, not a patch. Note (a) alone is a small change to the prehook response
shape and is worth doing regardless.

## Self-GC: Self-Governing Context for Long-Horizon LLM Agents

[arXiv:2607.00692](https://arxiv.org/abs/2607.00692) — submitted 2026-07-01.
Hao, Meng, Yin, Zhu, Cao.

Treats context as a **garbage-collection problem over typed objects**, not a text
suffix to truncate (the name deliberately echoes GC: it governs the *lifecycle* of
context objects rather than just reclaiming tokens). It turns user turns, tool
spans, and skill state into **indexed objects**, has a side-channel planner propose
`fold` / `mask` / `prune` per object, and has the **harness enforce** three things
the planner cannot: **recoverable sidecars** (pruned content stays retrievable by
ID), **safe commit boundaries**, and **cache-aware commit**. The framing critique is
precise and lands on inber: chronological pruning is "blind to future
dependencies," while self-summary "preserves narrative state but often hides exact
evidence, locators, and editable artifacts."

**What inber should consider:** this is the object-lifecycle layer inber's
compaction path lacks. `conversation/summarize.go` renders an old span to *prose*
via `messagesToText` and stores it lazily — which is exactly the "hides locators
and editable artifacts" failure mode: after compaction inber cannot recover the
exact tool output, only a narrative about it. Concretely: give compaction an
**object index** (turn / tool-span / skill-state) with per-object fold-mask-prune
verdicts, and put the pruned objects in **memory-store (:8160) as recoverable
sidecars addressable by ID** rather than dissolving them into summary prose. Then
align the prune boundary to a **cache-safe commit boundary** at `turn_prompt.go`'s
Anthropic cache breakpoints. That last part is the actionable extension of the
TokenPilot constraint already on file ([2606.17016](https://arxiv.org/abs/2606.17016)):
TokenPilot said *don't break the cache prefix*; Self-GC says *here is the object
granularity at which to decide, and where the safe cut points are*. This dovetails
with the cline sidecar/projection entry in `agentic-design-patterns.md` (07-13) —
the harness change and the paper independently arrive at "pruned content must
remain recoverable."

## Scaling Enterprise Agent Routing: Degradation, Diagnosis, and Recovery

[arXiv:2606.17519](https://arxiv.org/abs/2606.17519) — submitted ~2026-06-17.
Gillespie, Perry (Superhuman, Inc.).

A deployment study on a real **110-agent / 584-tool** catalog, sweeping 10→110
agents across three frontier models from two providers. Routing F1 on
under-specified requests **drops 16–23pp** as the catalog grows. An oracle
analysis decomposes this into a **retrieval gap** (the right tool never surfaced)
and a **confusion gap** (even with perfect retrieval the ceiling drops 10pp) — so
retrieval fixes only part of it. **Embedding-based shortlisting recovers +10–11pp**
at full scale, confirmed on real traffic (1,435 human-labeled utterances, three
annotators) at +10–17pp. The finding that matters most to inber:
**tool-level retrieval beats every pack-level approach by 2–4pp.**

**What inber should consider:** this lands directly on the **unbuilt Phase 1
resolver** in the session-bundling project (repo-store + bundle-store, :8306/:8307).
That design auto-selects **bundles** of skills/tools/MCPs per repo+task — i.e. it is
a *pack-level* approach, and this paper measures pack-level as strictly worse than
retrieving individual tools. Build the resolver as an **embedding shortlist over
individual tool/skill rows** (tool-store + skill-store), and demote bundles to a
*prior that boosts scores* rather than the unit of retrieval. Their retrieval-gap
vs confusion-gap oracle decomposition is also a cheap diagnostic worth running
against inber's own registries *before* committing to the resolver design. This is
the rare paper that arrives in time to change a design inber has scaffolded but
not yet built — but read it against the next entry before over-investing.

## Looking Is Not Picking: An Attention-Segment Account of Tool-Selection Failures in LLM Agents

[arXiv:2606.16364](https://arxiv.org/abs/2606.16364) — submitted 2026-06-15
(v2 2026-06-27). Shiyang Chen.

The honest counterweight to the previous entry. By instrumenting attention over
labeled tool-definition segments on real BFCL failures, it finds the model
**already attends most to the correct tool 80% of the time** (vs 21% chance), and
the gold tool is the under-attended segment in only **10%** of failures. This
**refutes the "crowded harness / lost-in-the-middle" story**: the model is *looking*
at the right tool and still *picking* the wrong one — the failure is at the
**decision readout**, not in prompt layout. Confirmed by intervention: repairing
the prompt (reordering or duplicating the gold tool) recovers **≤23%** of failures,
while readout-side interventions recover **59–91%**.

**What inber should consider:** mostly a *stop-doing* result, and it should cap
inber's enthusiasm for tool-surface trimming. It says the marginal return on
bundle-store, tool-schema compression
([2605.26165](https://arxiv.org/abs/2605.26165)), and skill-description budgeting
is **bounded near 23%** of tool-selection failures, because most failures survive a
perfect prompt. Before spending more on shrinking the tool surface, run the cheap
diagnostic: log which tool the model *picked* vs which was correct, and check
whether the correct tool was even in context. If it usually was, the problem is not
the registry and no amount of bundling will fix it. Be clear-eyed that the paper's
*effective* fix is readout-side (logit/attention steering), which inber
structurally **cannot do** — it drives claude-code/codex behind vendor APIs with no
logit access. So inber can adopt the **diagnostic and the negative result**, not the
cure.

## Cross-cutting takeaway (2026-07-13 sweep)

The two routing papers are the interesting part of this sweep because they
**disagree about inber's in-flight bundle-store design**, and the disagreement is
resolvable by measurement rather than argument. 2606.17519 says a bigger catalog
measurably degrades routing and an embedding shortlist recovers most of it (and
that inber's chosen *pack* granularity is the wrong one). 2606.16364 says the
crowded-catalog story is mostly false and prompt-side fixes are capped at ~23% of
failures. Both can be true — 17519 measures *end-to-end F1 on a 110-agent catalog*,
16364 measures *attention on BFCL failures* — but they imply very different
budgets for the same work. The cheap move, before building the Phase 1 resolver at
all, is 16364's diagnostic on inber's own traffic: was the correct tool in context
when the model picked wrong? That single log line tells inber whether it is buying
a +10pp retrieval win or a ≤23%-capped prompt-layout win, and it costs a day.

The other pairing is quieter but cleaner: **ActPlane and Self-GC are the same
insight applied to two different substrates** — an approval, and a pruned context
object, are both *claims about something the harness can no longer see*. ActPlane's
answer is to enforce below the level the agent can describe (syscalls, not tool
names); Self-GC's is to keep a recoverable sidecar so the elided thing can be
fetched back. Both reject the harness's habit of trusting its own summary of what
happened. That is the same thesis as this cycle's harness commits (see the two
07-13 sections in `agentic-design-patterns.md`): *a record the agent produced is
not evidence of the thing it describes.*

**Notably absent:** despite several query phrasings, no new prompt-caching /
KV-cache-aware prompt-assembly paper landed in the 7–30 day window (searches kept
returning pre-window work — KVFlow, KVCOMM, SparseX 2606.01751 — and the
already-known TokenPilot). Multi-agent orchestration / blackboard queries likewise
returned only pre-window results. The live areas this cycle were **permission
enforcement** and **context-as-typed-objects**; cache and agent-teams were quiet.

**Runner-up, not written up:**
[arXiv:2607.07405](https://arxiv.org/abs/2607.07405) *"Reason Less, Verify More:
Deterministic Gates Recover a Silent Policy-Violation Failure Mode in Tool-Using
LLM Agents"* (2026-07-07) — read-only pre-execution gates that inspect a proposed
tool call *against current state* before allowing a write; 29.6%→42.0% on
τ²-bench airline. Same permission-boundary slot as ActPlane but weaker for inber
(non-coding benchmark, domain-specific gates).

---

# 2026-07-14 sweep

Six further in-window papers (all `published` dates confirmed against the arXiv
Atom API, not search snippets). They fall into three groups, and each group lands
on a section written the same day in `agentic-design-patterns.md`.

## Group A — some context is not summarizable, and compaction is where safety dies

**Governance Decay: How Context Compaction Silently Erases Safety Constraints in
Long-Horizon LLM Agents** — [arXiv:2606.22528](https://arxiv.org/abs/2606.22528)
(2026-06-21). The finding is stark: in-context governance constraints that agents
reliably obey *while visible* get dropped by the summarizer, and the violation rate
goes **0% → 30%** (up to 59% on some models) purely as a function of compaction. The
conditional is the proof: when the constraint survives the summary, violations stay
at **0%**; when it is dropped, **38%**. They then demonstrate a **Compaction-Eviction
Attack** — adversarial context crafted to bias the summarizer into omitting the
policy — which defeats every model tested. The mitigation, *Constraint Pinning*
(quarantine constraints from the lossy path entirely), restores 0%.

**Plans Don't Persist: Why Context Management Is Load Bearing for LLM Agents** —
[arXiv:2606.22953](https://arxiv.org/abs/2606.22953) (2026-06-22). Using "replay
pairing" (same trajectory with and without the plan in history, measuring
hidden-state divergence), plan signal **collapses 4.1× within a single
action-observation step**: models do not internalize plans as persistent state, they
depend on the plan text remaining *literally* in context. Naive plan eviction costs
**34.7pp** on ALFWorld — and the negative result is the valuable half:
**probe-gated re-surfacing does not recover it.** Once the trajectory has drifted,
putting the plan back does not undo the drift. Prevention only; there is no repair.

**What inber should consider:** these two papers are the research statement of the
same structural claim as the 07-13/07-14 compaction entries in
`agentic-design-patterns.md`, and together they say something sharper than either
alone: **a compactor that is allowed to see and rewrite everything is a compactor
that will eventually delete the one thing that was load-bearing.** The harness needs
a **pinned region** — content re-rendered verbatim every turn, at a stable position,
which the summarizer is structurally incapable of touching. Two concrete inber
consequences: **(a)** inber's `conversation/summarize.go` renders *all* old messages
into `messagesToText(oldMessages)` and hands them to a model — so any behavioral
constraint that arrived as prompt text (`CLAUDE.md` directives, "surface ambiguity
before implementing", the auto-ship rules) is inside the blast radius and can be
summarized away mid-session, silently, with no error. inber is *partly* immune where
enforcement is a prehook rather than prompt text — which is a strong argument for
migrating *any* rule that actually matters out of the prompt and into the
permission-store, since a prehook rule cannot be summarized away. **(b)** The
Compaction-Eviction Attack is the nastier half and inber is exposed: a hostile file
that inber *reads into context* can bias its own summarizer into dropping a
constraint. That makes "what may the summarizer see" a security boundary, not just a
budget one. And per *Plans Don't Persist*, inber's plan/todo state must be
re-rendered verbatim each turn rather than left to survive as history — with the
explicit warning that a "detect decay, then re-inject" recovery scheme, which is the
obvious thing to build, **is measured not to work.**

## Group B — the approval view can lie, and the retry path is where privilege escalates

**Unicode TAG-Block Concealment of Tool-Metadata Payloads in MCP: An Approval-View
Fidelity Gap** — [arXiv:2607.05744](https://arxiv.org/abs/2607.05744) (2026-07-07).
Nothing in MCP requires that the bytes rendered in the one-time tool-approval dialog
match the bytes injected into the model's context on every later turn. Unicode's TAG
block (U+E0000–U+E007F) has **no glyph in any mainstream terminal or IDE renderer**,
so a payload is invisible to the human approver and survives byte-for-byte into the
tokenizer. 8/8 techniques across 5 MCP metadata surfaces defeated client-side
defenses against a real client/server pair.

**When Lower Privileges Suffice: Over-Privileged Tool Selection in LLM Agents** —
[arXiv:2606.20023](https://arxiv.org/abs/2606.20023) (2026-06-18). Agents routinely
select a higher-privilege tool when a sufficient lower-privilege one exists — and
**transient tool failures amplify escalation**. The retry path is where privilege
creep happens.

**What inber should consider:** 2607.05744 is a direct extension of the 07-13
authority entry ("consent must be attributable to a principal outside the model's
reach"). That entry assumed the consent *dialog* was trustworthy; this paper breaks
that assumption — **the human can approve a string that is not the string the model
receives.** inber's `tool-store` (:8302) ingests MCP servers and its prehook renders
approval prompts, so the fix belongs at both ends: **sanitize or reject
non-renderable codepoint ranges in tool `name`/`description`/schema at
tool-store ingestion time** (they have no legitimate use in tool metadata), and
**re-verify tool metadata on every turn, not only at first approval** — a server can
change its `tools/list` response *after* you approved it, which is the same TOFU
problem codex #32301 solved for hook code with a content-hash pin. The rule
generalizes: *whatever bytes you showed a human, hash them; if the bytes you are
about to send differ, the approval is void.* 2606.20023 adds a smaller but
actionable note that composes with ActPlane's "return semantic denials, not
`EPERM`": inber's prehook denials are an escalation surface, because an agent that
gets an opaque failure retries with a bigger hammer (`shell` instead of the scoped
tool). Denials should say *why*, and inber should log the low→high tool swap after a
failure as the escalation signal it is.

## Group C — retrieval is a trajectory-time loop, not a session-start step

**SING: Synthetic Intention Graph for Scalable Active Tool Discovery** —
[arXiv:2606.16591](https://arxiv.org/abs/2606.16591) (2026-06-15). Dumping all
schemas is expensive and imposes a closed-world inventory; one-shot embedding
retrieval misaligns isolated tool descriptions with the agent's *evolving* intention.
SING builds an intention → tool-capability → tool-collaboration graph and retrieves
**dynamically as task state changes**. On a 7,471-tool corpus: Global Recall@5 up to
**+59.8%**.

**SWE-MeM: Learning Adaptive Memory Management for Long-Horizon Coding Agents** —
[arXiv:2606.28434](https://arxiv.org/abs/2606.28434) (2026-06-26). Rather than a
fixed "compact at 80% full" rule, memory becomes a **tool the agent calls**, deciding
when/what/how to compress from trajectory state, task progress, and remaining budget
(trained with memory-aware GRPO doing step-level credit assignment across the
compaction boundary). 43.4% / 60.2% on SWE-bench Verified at 4B / 30B, beating
static-compression baselines on both quality *and* tokens.

**What inber should consider:** SING is the **third voice in the bundle-store
argument** the 07-13 cross-cutting note left open (2606.17519 "shortlist helps a lot"
vs. 2606.16364 "crowded catalogs are mostly a myth"), and it reframes the question
usefully: both of those measure *one-shot* selection, while SING's claim is that the
retrieval **timing** is the actual bug — the capability you need typically only
becomes apparent *after* decomposition or an observation, so a bundle resolved once
at session start is resolving against an intention the agent does not have yet. That
matters directly for inber's Phase 1 resolver: **re-run selection during the
trajectory, not only at session start**, and rank on **co-occurrence** edges (the
tool you need next is best predicted by the tool you just used) rather than on
description-embedding similarity alone. Combined with the codex shadow-selection
entry from the same day, this gives inber a full build order: *shadow first, measure
recall against the full-catalog baseline, re-run selection per turn, rank on
co-occurrence.* SWE-MeM is the analogous move one layer over — even without any RL
training, **exposing the remaining context budget as an observable and offering a
`compact(what)` tool** lets a frontier model make a keep/drop decision that a generic
summarizer, which has no idea which files are still live, cannot. Note the tension
with Group A, and resolve it explicitly: an agent-invoked compactor is *still* a
compactor, so the pinned region (constraints, plan) must be outside its reach too —
"the agent chose to drop it" is not a defense.

## Also in-window, logged but not written up

- **TraceLab: Characterizing Coding Agent Workloads for LLM Serving** —
  [arXiv:2606.30560](https://arxiv.org/abs/2606.30560) (2026-06-29). A real trace of
  ~4,300 Claude Code + Codex sessions (~350k LLM steps, ~430k tool calls), with
  analysis code. Two directly useful findings: the tool-call distribution is heavily
  tailed (a handful of tools dominate — load the rest lazily), and prefix-cache hit
  rates are high but imperfect, with **cache damage concentrated around human-paced
  idle gaps** (a session resumed after a pause pays full prefill). Free ground truth
  against which to check inber's own cache assumptions instead of guessing.
- **Don't Blame the LLM: How Scaffolding Evolution Shapes Coding Agent Quality** —
  [arXiv:2607.03691](https://arxiv.org/abs/2607.03691) (2026-07-04). Holds the model
  fixed and varies *only* the harness across 35 sequential Qwen Code CLI releases;
  regressions practitioners blamed on the model were the scaffold. Argues for a
  pinned harness-vs-harness regression eval — which is what inber's
  `repo-build-guard`/`repo-deploy-guard` do for buildability but not for behavior.
- **When Does Restricting a Coding Agent to `execute_code` Help?** —
  [arXiv:2607.10569](https://arxiv.org/abs/2607.10569) (2026-07-12). Three-arm
  ablation on Claude Code + Codex: a single `execute_code` tool is cheaper than or
  tied with tool-rich rivals in 3 of 4 (regime × agent) cells — but the cheapest tool
  surface **depends on the task regime**, so there is no universal answer. A caution
  against inber adopting code-mode as a blanket default.

**Cross-cutting takeaway (2026-07-14 sweep):** every paper in Groups A and B is an
instance of one claim — **the harness's own lossy record of a thing is not the
thing.** A summary is not the constraint (2606.22528). A summary is not the plan
(2606.22953). The approval dialog's rendering is not the tool metadata (2607.05744).
This is the same thesis the 07-13 sweep reached from ActPlane and Self-GC, now with a
much sharper edge: in every case, the *gap* between the record and the referent is
directly attackable, and in two of the three the attack has been demonstrated. The
harness-side answer, which the 07-14 `agentic-design-patterns.md` entries converge on
independently, is always the same — **keep the canonical artifact, make the derived
view rebuildable from it, and never let the derived view be the only copy.**

---

# 2026-07-18 sweep

Two in-window papers (`published` dates confirmed against the arXiv Atom API). Both
land on the **permission-enforcement** area the 07-13/07-14 sweeps flagged as the
live one — but they name it *isolation* and give it structure, which is exactly the
missing frame for inber's existing boundary-audit work. This partly revises the
07-14 "multi-agent orchestration was quiet" note: the orchestration paper below was
already on arXiv (06-30) inside the window and the earlier sweep missed it.

## A five-boundary isolation taxonomy for the harness-control-matrix

**Isolation as a First-Class Principle for LLM-Agent System Safety: Concepts,
Taxonomy, Challenges and Future Directions** —
[arXiv:2607.12406](https://arxiv.org/abs/2607.12406) (2026-07-14). A survey whose one
load-bearing move is to argue that prompt injection, tool misuse, and memory
poisoning are not distinct bug classes but the *same* structural failure — a lost
isolation boundary — and to organize the whole literature by **where** the boundary
sits: five interfaces — **user↔agent, agent↔tool, agent↔execution, agent↔agent,
system↔environment**. The value is diagnostic, not a mechanism: given a failure, it
locates *which* boundary leaked first and *how* the compromise propagates across the
others, and it argues for "isolation-by-construction" (the boundary is a structural
property of the harness, not a runtime check the agent can talk its way around) —
the same conclusion ActPlane reached (07-13) from the enforcement-below-tool-names
angle.
- **What inber should consider:** inber already has the raw material — the
  *harness-control-matrix* memory is a 39-boundary audit of llm-bridge (3/18/18) and
  inber (0/27/12). But it is scored per-component (guard/trace/checkpoint), not
  per-*interface*. Re-project that matrix onto these five boundaries and the gaps
  read differently: inber's `permission-store` covers **agent↔tool** and
  **agent↔execution**; **MCP descoping** and browser-MCP OOM were a
  **system↔environment** leak; **team-orchestration / task-completion-loop** subagents
  are an **agent↔agent** boundary with (per the next paper) almost no least-privilege
  enforcement today. The concrete move is one table in `HARNESS-LAYER` mapping each
  of the five boundaries to inber's enforcement point and marking the ones that are
  hot-path stubs (inber's guard/trace/checkpoint already are) — turning a component
  checklist into a leak-path map.

## The subagent-orchestration bottleneck is privilege-granting, not perception

**ClawArena-Team: Benchmarking Subagent Orchestration and Dynamic Workflows in
Language-Model Agents** — [arXiv:2606.31174](https://arxiv.org/abs/2606.31174)
(2026-06-30). Isolates the *management* ability of a single LLM acting as team leader:
the main agent is deliberately crippled (text-only perception, partial workspace
access) and commands a fixed local subagent pool, so score deltas reflect delegation
skill, not raw capability. Scoring is execution-based (no LLM judge) — the
**Subagent-Management Score** multiplies task correctness by a **least-privilege ×
modality-routing** factor. Three findings matter for inber: (1) the bottleneck is
**privilege granting** — *no* model exceeds 50% workspace-permission precision, i.e.
leaders systematically over- or under-grant subagent access; (2) cost and management
quality are **decoupled** (API cost spans >100×, score <4×; cheap open models sit on
the Pareto frontier); (3) leaderboard scores cluster within ~10 points while actual
orchestration behavior diverges >10×, so a single aggregate score hides the skill.
- **What inber should consider:** inber's *team-orchestration* / *task-completion-loop*
  currently spawns subagents with whatever the leader hands them; there is no
  least-privilege-granting step and no measurement of whether the grant was right.
  Two cheap actions: (a) make the dispatcher's subagent-spawn carry an explicit
  scoped grant (which repo paths / tools / MCPs) rather than inheriting the parent's
  full surface — this is the **agent↔agent** boundary the isolation paper names, and
  it dovetails with the already-scaffolded `repo-store`/`bundle-store` per-task
  skill/tool selection; (b) because cost ⟂ quality here, do **not** reach for a
  bigger leader model to fix bad delegation — log workspace-permission precision
  (granted-vs-needed) as the metric and treat over-granting as the defect. The paper
  is a benchmark, not a method, so the takeaway is the *diagnostic*, not code to port.

**Cross-cutting takeaway (2026-07-18 sweep):** the two papers are the same claim at
two altitudes — **least-privilege at every interface is the harness's job, and the
agent can neither be trusted to enforce it nor scored on whether it did by a single
number.** 2607.12406 says *where* the boundaries are; 2606.31174 measures the one
boundary (agent↔agent, via privilege grants) that current models fail hardest, and
shows a bigger model does not buy the fix. For inber this points at one artifact — a
boundary map with an enforcement point and a granted-vs-needed measurement per
interface — rather than more per-component guard stubs.

# 2026-07-24 sweep

Two compaction/context papers, both new to inber and both advancing the thread the
07-22 (`agentic-design-patterns.md`) SelfCompact entry opened: *the summarizer wording
barely matters; the hard parts are knowing when to compact and giving the model enough
self-knowledge to decide.* One supplies the missing mechanism, the other the theory
that says inber's current trigger is provably wrong.

## The model can manage its own context if you show it the context — a typed-block dashboard, training-free

**LLM Agents Are Latent Context Managers: Eliciting Self-Managed Context via State
Proprioception** ([arXiv:2606.30005](https://arxiv.org/abs/2606.30005) — v1 2026-06-29,
**v4 2026-07-23**, in-window on the revision). VISTA is a training-free, model-agnostic
layer that represents working memory as **typed, addressable blocks** and, each turn,
surfaces the model a **dashboard of per-block token usage, recency, and access history**;
the model then decides which blocks to keep or archive, and archived blocks are stored as
**recoverable full-fidelity payloads** (not lossy summaries) it can pull back on demand.
The thesis is that models already have latent context-management competence but no
*proprioception* of their own context state, so they compact blind; give them the meter
and they act well. Result: +four backbones on LOCA-Bench, Gemini-3-Flash 22.7 → 50.7%,
gains *growing with context pressure* and transferring across backbones (also GAIA,
BrowseComp-Plus).

**What inber should consider:** this is the concrete mechanism for the 07-22 rubric gap —
inber's compactor fires on `len(messages) > TriggerMessages` (`summarize.go:14`) and hands
the model an opaque prose summary; VISTA says the higher-leverage move is to let the model
*see* its own context and prune it. inber already has the substrate: WorldState-style typed
sections (memory-store, tool-store, permission context) are exactly VISTA's "typed
addressable blocks," and reversible noteboard delete + memory-store give the
**recoverable-archive** half for free. **(a)** Expose a per-block dashboard (token cost,
last-access turn, access count) as a WorldState section the model reads before deciding to
archive — cheap, no fine-tune. **(b)** Make compaction *archive-with-recovery*, not
lossy-summarize: move an evicted block to memory-store as a full-fidelity payload keyed for
retrieval, so the "which to keep" decision is reversible rather than a one-way summary. This
is the same rate-distortion point below, made operational.

## Recency and attention are the wrong keep-signals — a rate-distortion frame for why

**What to Keep, What to Forget: A Rate–Distortion View of Memory Compaction in LLMs and
Agents** ([arXiv:2607.08032](https://arxiv.org/abs/2607.08032), 2026-07-09). Reframes every
compaction level — KV-cache eviction up to agent-memory consolidation — as one
rate-distortion problem (which context-derived information to keep, at what fidelity, under
a budget) with a single objective and a layer-agnostic lower bound, and organizes the field
into a seven-axis taxonomy. The load-bearing finding: **attention-magnitude and recency
consistently signal what to keep, yet both fail the same way — they discard information
*before the query reveals what it needed*.** It also flags that iterative agent compaction
has no benchmark holding the budget constant across layers, so nobody is measuring the
thing that matters.

**What inber should consider:** this is the theory under inber's own 07-22 self-critique.
inber's trigger (message count) and any recency/oldest-first eviction are exactly the
"decide before the query arrives" failure this paper proves is lossy — you cannot know a
block's distortion cost until a later turn queries it, which is precisely why VISTA's
*deferred, model-in-the-loop, recoverable* archive beats an eager summary. Concrete: **(a)**
don't evict oldest-first on a count threshold; keep blocks recoverable (memory-store) so a
late query can re-admit them, turning an irreversible keep/forget call into a
retrieve-on-demand one. **(b)** If inber ever benchmarks its compactor, hold the token
budget constant across the message-history *and* memory-store layers together — the paper's
point that per-layer measurement hides the real cost. No code to port; it's the frame that
tells inber which of its compaction knobs are load-bearing (the trigger and the recovery
path) and which are noise (the summarizer prompt wording).

**Cross-cutting takeaway (2026-07-24 sweep):** the two papers close the loop the 07-22/07-23
compaction entries left open. 2607.08032 proves *why* inber's count/recency trigger is wrong
(keep-signals computed before the query are lossy by construction); 2606.30005 gives the
*fix that needs no training* (show the model a typed-block usage dashboard and let it
archive to a recoverable store). For inber both land on stores it already owns —
WorldState-typed sections as the blocks, memory-store + reversible noteboard as the
full-fidelity archive — so the work is wiring a context dashboard and making eviction
recoverable, not building new machinery.

# 2026-07-26 sweep

Two new-to-inber papers. One challenges *where* inber's memory store lives; the other is
meta — how an agent edits the harness it runs on. (The sweep's other hits — SWE-MeM
[2606.28434], VISTA [2606.30005], TokenPilot [2606.17016] — are already on file above.)

## Memory belongs *inside* the loop — an HTTP memory store is too slow to query per-step

**Memory in the Loop: In-Process Retrieval as Extended Working Memory for Language Agents**
([arXiv:2607.05690](https://arxiv.org/abs/2607.05690), v1 2026-07-06, v2 2026-07-19).
Agents observe-reason-act in a loop, but the memory they reason over sits *outside* it: a
networked store queried at most once per turn because it answers in tens-to-hundreds of ms,
and querying it per-step can inflate end-to-end latency up to 83×. The paper's thesis is that
latency is a property of **where the store lives, not the in-loop pattern**: an in-process
store answers in ~100µs (p50 80–165µs), three orders of magnitude below the network regime,
and at that speed the per-step tax collapses — memory becomes *extended working memory*, not
a tool consulted once. The causal result: holding a fixed per-turn memory-latency budget and
varying only store speed, redundant actions rise monotonically with latency — **0.0 of 12 at
in-process speed, 7.2 of 12 at a 110 ms cloud round-trip** — and recall improves 0/5 → 3.6–4.8/5.
Two honest caveats the paper raises itself: an instructed *restate-every-reply* baseline also
solves recall perfectly, at a token cost that grows with the working set; and the real
per-step bottleneck is **embedding** (~200–400 ms over the network), so an in-process store
fronted by a *networked* embedder buys nothing — pairing it with a small **local** embedder
returns the whole op to ~40 µs.

**What inber should consider:** inber's memory-store is exactly the HTTP-over-network shape
the paper measures as too slow for hot-path recall — it runs as a service (bridge-server
:8160) queried at turn boundaries, not per reasoning step. The takeaway is a **two-tier
split**: (a) an in-process/embedded index co-located with the engine, *with a local
embedder*, for per-step recall cheap enough to run every step; (b) the HTTP memory-store
reserved for durable cross-session state where a per-turn round-trip is fine. The
load-bearing detail inber would hit first is the caveat above — the bottleneck is the
embedder, not the store, so moving only the store in-process while embeddings stay networked
is a no-op. This interlocks with the 07-24 VISTA/rate-distortion pair: a *recoverable
archive* only pays off if re-admitting an evicted block is cheap enough to do mid-trajectory,
which requires the retrieval path itself to be in-loop-fast — so the in-process split is the
precondition for "make eviction recoverable" to actually help rather than just move the cost.

## A self-modifying harness needs a behavior→code map, not grep

**Harness Handbook: Making Evolving Agent Harnesses Readable, Navigable, and Editable**
([arXiv:2607.13285](https://arxiv.org/abs/2607.13285), 2026-07-14). As harnesses evolve,
developers (and agents) struggle to find *where* a behavior is implemented across a large,
tightly-coupled codebase. The paper builds an automated **behavior→source map** linking each
observable harness behavior to its implementation locations, and proposes **Behavior-Guided
Progressive Disclosure** to route an editing agent from a high-level behavior down to the
exact — often scattered — sites, verifying candidates against current source. Gains
concentrate exactly where grep fails: *scattered sites, rarely-executed paths, cross-module
interactions.*

**What inber should consider:** inber is itself a harness that agents modify (this
harness-watch job, the modularity work), and its behaviors are scattered precisely as the
paper describes — *compaction* alone spans `engine/lifecycle.go` + `conversation/summarize.go`
+ `conversation/summary_generation.go`; tool-schema projection lives in
`agent/openai_conversion.go` + the registry; permission gating sits in bridge-server's
prehook, in another repo entirely. A harness-editing agent that greps `compaction` never
finds `summary_generation.go`. Consider a behavior→code index so an editing agent starts from
behavior, not filename. Low-cost version: **this `docs/comparisons` set is already a partial
behavior→source index in prose** — the "what inber should consider" bullets routinely name
the exact file:line — so formalizing the source-location links (or a hand-maintained
`INBER.md` behavior map) is a small step from what already exists, not new machinery.

**Cross-cutting takeaway (2026-07-26 sweep):** both papers point at inber's *structure*, not
its prompts. Memory-in-the-loop says inber's memory boundary (HTTP :8160, once-per-turn) is in
the wrong place for the recoverable-archive strategy the 07-24 sweep recommended to pay off;
Harness Handbook says the map an agent needs to *edit* that boundary safely already half-exists
in these comparison docs and should be made into real source links.

---

# 2026-07-27 sweep

## AgenticSTS: memory as a *contract about what each decision may see* — bounded, typed-retrieval assembly beats an appended transcript

[arXiv 2607.02255](https://arxiv.org/abs/2607.02255) ("AgenticSTS: A Bounded-Memory Testbed
for Long-Horizon LLM Agents", July 2026). The paper's framing is the useful part: **memory is
a contract about what each future decision is allowed to see.** The default contract — append
every past observation, tool call, and reflection to the next prompt — makes prior context
trivially accessible but turns it into a jumbled mixture where no single memory component's
effect can be isolated (and, from the cache line this doc already tracks, wrecks the prefix).
The alternative it tests: a **bounded contract** where each decision runs from a *fresh user
message assembled by typed retrieval*, with **no raw cross-decision transcript appended**. The
testbed exists precisely to measure whether that bounded assembly holds up over long horizons.
*(Numbers unverified — WebFetch is denied in this job's sandbox; cited for the design thesis,
not as a measured result.)*

**What inber should consider:** this is the memory-store counterpart to the 07-24 sweep's
"recoverable archive beats an eager summary" and the 06-24 "fresh-window reset" pattern — and
it names the design choice inber has *not* made explicit. inber's conversation assembly
(`engine/turn_context.go`, `conversation/summarize.go`) is fundamentally the *append* contract:
history accumulates and is trimmed/summarized at a threshold, with memory-store entries layered
on top. AgenticSTS argues the boundary should sometimes be the other way round — assemble the
next turn's user message *from typed retrieval against memory-store*, and drop the raw
transcript rather than summarize it, when the task is at a clean subtask boundary. inber already
has the pieces: `MemStore` is the typed retrieval surface, and the 07-22 SelfCompact rubric says
*when* a boundary is clean enough to reset. The concrete step is to make "assemble-from-memory,
no transcript" a **selectable context contract** at subtask boundaries — not the only mode
(mid-derivation still needs the raw trace), but an available one — so a long-horizon session can
run in bounded-token steady state instead of paying summarization round-trips forever. This
composes with the 07-26 "memory belongs inside the loop" finding: bounded typed-retrieval
assembly is only affordable if the memory query is in-loop, not a once-per-turn HTTP hop.

---

# 2026-07-28 sweep

## Harness-evolution results are measured against unmatched baselines on the tasks they were tuned on

**Rethinking the Evaluation of Harness Evolution for Agents**
([arXiv:2607.12227](https://arxiv.org/abs/2607.12227), submitted 2026-07-14; AI2/UW authors
incl. Hajishirzi, Tsvetkov, Dasigi). A negative-result methodology paper aimed at the
automatic-harness-evolution literature. Two charges: **(a)** harness evolution is itself an
iterative search that spends inference compute, but it is compared against baselines that get
no matched budget — so a reported gain may be "more search," not a better architecture; and
**(b)** the search loop and the final evaluation run on the *same* benchmark, so the result
measures overfitting to that benchmark rather than a general improvement. Re-run with
budget-matched test-time-scaling baselines and held-out tasks (Terminal-Bench 2.1, GPT-5.4 and
Claude Opus 4.6), automatic harness evolution "does not consistently outperform simple
test-time scaling methods and exhibits limited generalization." *(Abstract-grade — read from
the arXiv abstract page, not the PDF; no per-config deltas seen.)*

The paired example landed the same week: **Recursive Harness Self-Improvement**
([arXiv:2607.15524](https://arxiv.org/abs/2607.15524), 2026-07-17, authors incl. Zaharia,
Tang) argues model and harness co-evolve — a harness is a *data-generating* component because
its traces become the next model's training data — and refines a prompt-level spec of the
agent loop via pairwise feedback over its own revision history, claiming low-reasoning-effort
agents beat max-effort baselines at up to 60% lower inference cost, with the gain attributed
to task-specific *context management* rather than longer reasoning. Suggestive, and exactly
the shape 2607.12227 says not to take at face value: 30 *synthetic* tasks, self-selected.

**What inber should consider:** this is a rule for *this job*, not a feature. inber is a
self-modifying harness and this weekly sweep is a harness-evolution loop — it reads upstream
diffs and papers, then proposes changes to inber's own scaffolding. The paper names both
failure modes the loop is exposed to. **(a)** When a proposed change gets evaluated, compare
it against a **compute-matched** baseline: "rubric-gated compaction beat threshold compaction"
means nothing if the rubric variant also spends an extra model call per turn — spend the same
budget on plain retries or a longer context and see if the gain survives. **(b)** Evaluate on
tasks that were **not** used to pick the change. The concrete risk here is that the docs are
tuned against the same handful of recurring inber scenarios (the compaction path, the skill
resolver, the permission gate) that motivated each entry. **(c)** The cheap discipline, given
inber has no benchmark harness: keep writing the entries, but stop treating an upstream
harness's *adoption* of a pattern as evidence it works — codex shipping a degradation ladder
is evidence that codex's team believed it, nothing more. Several entries in this doc set
already lean on abstract-grade numbers (flagged inline); this paper says the lean should be
toward the *design thesis*, which transfers, and away from the *deltas*, which do not.

## Self-state attacks: an agent corrupts its own memory and config through calls it is authorized to make

**Self-State Attacks on Self-Hosted AI Agents: How Far Can OS Defenses Go?**
([arXiv:2607.17986](https://arxiv.org/abs/2607.17986), submitted 2026-07-20, authors incl. Di
Pietro, Schmidhuber). Thesis: for a self-hosted agent the interesting attack surface is not
sandbox escape but the agent mutating **its own memory, instructions and configuration**
through entirely legitimate operations it already holds permission for — so gating at the
syscall/command boundary cannot see the attack, because nothing unauthorized happens. The
paper systematizes the space on four axes (Target, Mechanism, Granularity, Temporal) into a
23-cell matrix with 43 concrete operations on real self-state files, and tests a layered
defense: access control on the *instruction* and *config* layers, workload-conditioned
detection on the *memory* layer, periodic backup for recovery. Most cells are covered; a
residual surface stays "structurally indistinguishable at the OS level." *(Abstract-grade.)*

**What inber should consider:** inber is precisely the deployment this describes, and the
paper's axis — target *layer*, not target *syscall* — is the axis inber's gating is missing.
Permission-store keys on **command-string regex**, which sees shell commands and nothing else;
`memory_save` / `memory_forget` (`memory/tools.go:98,189`) are ordinary always-allowed tools
that never reach it. The exposure is concrete: `BuildContext` loads `AlwaysLoad` memories —
identity and standing instructions — into the **stable cached prefix** of every future session
(`engine/turn_prompt.go:83`, `memory-store/builder.go:26`), `memory_search` prints each hit's
`ID` (`memory/tools.go:81`), and `memory_forget` soft-deletes **any** ID with no notion of
which layer it belongs to. So a prompt-injected or merely confused agent can retire its own
identity/instruction memories for every subsequent session, using two authorized calls, and
leave no trace the command-regex gate could ever have matched. `memory_save`'s
model-settable `importance` (0–1) is the softer version: inflate it and crowd the context
budget. **(a)** Give memories a **layer** (identity/instruction vs. operational) and gate
writes and forgets on it — the agent freely manages operational memory, while instruction- and
identity-layer mutations require the same approval path as a destructive command. The field is
half-present already: `AlwaysLoad` is on the struct and, correctly, *not* exposed in
`memory_save`'s schema — the gap is that `memory_forget` ignores the distinction the save path
respects. **(b)** Recovery beats detection here and inber already has the mechanism — noteboard's
`deleted_at` + `item_revisions` made deletes reversible; memory-store's soft-delete should carry
the same revision history so an instruction-layer change is inspectable and revertible.
**(c)** Note the interaction with the open permission-store gate in MEMORY: that note worries
about `rm -rf /`, and the fix everyone reaches for is a better command regex. This paper says
the regex is the wrong instrument for the more likely failure — the agent does not need to run
`rm` to erase its own instructions.

## One object as tools, state and prompt

**NVIDIA-labs OO Agents: Native Python Object-Oriented Agents**
([arXiv:2607.20709](https://arxiv.org/abs/2607.20709), submitted 2026-07-22). Argues agent
code is fragmented across four artifacts kept in sync by hand — prompt templates, tool JSON
schemas, callbacks, workflow graphs — and collapses them into a single object: methods *are*
the actions, fields *are* the state, docstrings *are* the prompts. The claimed payoff is that
the developer's interface and the model's interface become the same object, so behavior is
testable with ordinary tooling instead of trace-staring. *(Abstract-grade, and unusually thin:
it names SWE-bench Verified, Terminal-Bench 2.0 and ARC-AGI-3 but publishes no figures at
abstract level. Cited for the thesis only.)*

**What inber should consider:** inber is already most of the way here and should notice the
one seam where it isn't. `agent.Tool` bundles `Name`, `Description`, `InputSchema` and `Run`
in one Go value, so a tool is a single object rather than a schema file plus a handler —
that is the paper's structure. The seam is that `InputSchema` is **hand-written and
independently parsed**: `agent/registry/spawn_tool.go` declares `type input struct {Agent,
Orchestrator, Task}` *and* a separate `Properties` map listing the same three fields, with
nothing tying them together. `memory/tools.go` repeats the pattern. Nothing fails loudly when
they drift — a field renamed in the struct silently stops binding, and the model keeps being
told the old name. The fix is small and mechanical: derive the schema from the input struct
via reflection over json tags (with a `desc:` tag for the descriptions), so the struct is the
single source of truth. That is worth doing on its own merits and does not depend on the
paper's unpublished numbers.

## Involuntary memory is a harness property — inber has the cue vocabulary and fires it in one place

**Delivery, Not Storage: Cue-Anchored Working Memory as a Harness Property for Coding
Agents** ([arXiv:2607.20972](https://arxiv.org/abs/2607.20972), submitted 2026-07-23,
Swapnanil Saha). Argues coding agents ship with only *one* kind of memory — documents the
agent must choose to write and choose to read back — while the load-bearing human tier is
situational facts retrieved involuntarily when the situation cues them. It contributes a
cue-anchored model where each memory carries first-class trigger conditions over a
composable vocabulary (**path, symbol, semantic, event, temporal**) evaluated
deterministically by the harness. The measurements are the reason to care: with a
pre-seeded store the agent performed **0 memory operations in 114 turns**; ten facts held
only in the conversation vanished at the first summary and stayed absent from **106 of 108
compactions**, with the deprived agent grepping the harness's own session files to rebuild
them, while the same facts injected from a harness-owned store survived all **138**
compact-resumes; **39%** of intra-session re-reads re-bought content already paid for
before a compaction boundary.

**What inber should consider:** inber is *not* the document-only harness the paper attacks,
and it should get credit for the two things it already has right. All four memory tools are
voluntary (`memory_search/save/expand/forget`, `memory/tools.go:52,99,151,190`) — the tier
the paper measures at zero use — but `BuildSystemPrompt` also runs an involuntary channel
every turn, and because it rebuilds the system prompt from the store rather than from the
transcript, harness-injected memories survive compaction by construction. That is the
paper's 138-of-138 result, already true.

The gap is *when the cue is evaluated*. `engine/turn_prompt.go:76` derives cues with
`memory.AutoTag(userMessage, "user")` — and that is the **only** `AutoTag` call site in
inber. `PatternTagger` already extracts **file paths** (`memory-store/tagger.go:38-42,117`),
so the path cue in the paper's vocabulary exists; it is simply never asked about anything
but the user's opening sentence. A memory tagged `engine/turn_prompt.go` fires only if the
user happens to type that path, not when the agent opens the file at step 7 of a
twenty-tool turn — and `buildTurnContext` is called once per user turn
(`engine/turn_prepare.go:69`), so the cue is never re-evaluated as the turn proceeds.
`TagWithToolName` (`tagger.go:107`) is the missing half, written and called by nobody.
**(a)** Re-evaluate cues against what the agent is *touching*, not only what the user
*said*: feed tool-call arguments — the path on a read/edit, the command on a shell call —
through the tagger and top up the volatile block. **(b)** That must land **after** the
`__CACHE_BOUNDARY__` sentinel (`turn_prompt.go:56,70`), in the volatile section, or every
file the agent opens invalidates the stable prefix — the paper's win is delivery, and
inber's cache ordering is what keeps delivery cheap. **(c)** Cheapest first step, no new
machinery: `AutoTag` currently sees the user message only, so give it the turn's tool
results too and the 39% re-read finding is directly testable against inber's own logs.

## Parallel agents need pre-write admission, not a note telling them to look before they leap

**Claim Plane: Enforceable Change Intents and Dynamic Scope for Parallel Coding Agents**
([arXiv:2607.21909](https://arxiv.org/abs/2607.21909), submitted 2026-07-24, Maxim
Nikolaev). Frames concurrent agent work as a **pre-write admission problem** rather than a
merge-time repair problem. Before implementing, each worker declares a versioned
`ChangeIntent` — exact base commit, typed resources, dependencies, and operations marked
*committed* or *contingent*. A deterministic control plane atomically admits compatible
intents, constrains same-file parallelism to declared regions, serializes unresolved
overlap, tracks dependency invalidation, and **fails closed on ambiguous authority**. A
contingent mutation reserves nothing up front; the first attempted write triggers atomic
scope promotion and re-admission against the current active set. The stated thesis is
separating *probabilistic planning* from *deterministic authority*. Evidence is explicitly
feasibility-only — six CooperBench pairs, 6/6 with full serialization, parallel admission
retained on half under dynamic scope, seven scope promotions, two undeclared mutations
caught by failing closed. *(Single-author preprint; cited for the architecture, not the
numbers.)*

**What inber should consider:** this is the paper for the standing "Parallel agent
collision" note — two nightly workers **will** pick the same only-unblocked todo. inber's
current answer is advisory (check the target repo's mtimes and untracked files first),
which is exactly the "continuous supervision" class the paper lists and rejects, and it is
enforced by nothing. The pieces to build it deterministically are already here: noteboard's
`hold` field is a fail-safe-by-default exclusion, auth-store already enforces leases,
kanban-store already links cards to entities, and `isolation: worktree` already gives a
worker its own tree. **(a)** The smallest real version is an admission step on the todo
itself — a worker claims with a lease naming the todo id *and the base commit it read*,
and a second worker is refused rather than warned. That alone converts the collision from
a duplicated-work incident into a refusal. **(b)** Keep the base-commit field; it is what
turns "someone else is on it" into "your premise is stale", which is the failure the
mtime check cannot see. **(c)** Copy *fail closed on ambiguous authority* verbatim — it is
the inverse of the disabled-deny-rules problem in the open permission-store gate, where
ambiguity currently resolves to allow. **(d)** Do **not** copy the typed-resource
declaration yet; asking a model to enumerate the files it will touch before it explores is
the part of the design least likely to survive contact, and the lease gets most of the
value without it.

## Also in-window, checked and logged

- **SkillGate** ([2607.25619](https://arxiv.org/abs/2607.25619), 07-28) — screens skill
  packages before install with a regex prefilter plus an LLM judge on matched snippet
  windows only (F1 0.817, FPR 1.13%, 77% fewer judge input tokens on SkillsBench, n=1650,
  9.1% malicious). Relevant because skill-store ingests third-party GitHub repos by
  `git clone --depth=1` and walks every `SKILL.md` beneath with **no screening step**
  (`skill-store/ingest.go:30,321`), and one seeded source is a community list. Not written
  up further because the fix is a decision (add a gate, choose a judge model, choose what
  happens on flag) rather than a defect — filed here so the exposure is on the record.
- **Agent Team Work Zone** ([2607.22917](https://arxiv.org/abs/2607.22917), 07-24) —
  filesystem "workstations" preserving teammate state across compaction and process exit.
  Confirmatory rather than new: this is what noteboard's `workspace` type plus
  `jobs.workspace_id` already do, and the paper reports no numbers.
- **Distributing Security Controls Through Harness Engineering**
  ([2607.25890](https://arxiv.org/abs/2607.25890), 07-28) — SHarD packages OS sandboxing,
  skill scanning and tool restriction into a distributable harness built on Pi, scored
  against a 23-test suite from the OWASP Top 10 for Agentic Applications. The transferable
  observation is that **model non-determinism produced inconsistent security outcomes**,
  which is an argument for gating in the harness rather than in the prompt. Logged for the
  OWASP suite as a possible checklist for the harness-control-matrix.
