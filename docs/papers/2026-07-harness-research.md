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
