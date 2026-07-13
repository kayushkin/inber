# Harness Research — July 2026

Notes from the harness-watch sweep on 2026-07-13. Four arXiv papers from the
last ~30 days that passed the not-already-covered check against
`2026-04/05/06-harness-research.md` and `agentic-design-patterns.md` (highest
previously-cited ID: 2606.17016 TokenPilot).

Two of the four (routing) are best read as a **pair that disagrees**, and the
disagreement is the point — see the cross-cutting note at the bottom.

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
