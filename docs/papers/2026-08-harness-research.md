# Harness Research — August 2026

Notes from the August harness-watch sweeps. Every paper here passed the
not-already-covered check against `2026-04/05/06/07-harness-research.md` and
`agentic-design-patterns.md` (108 distinct arXiv ids extracted and machine-checked),
and every date was confirmed against the arXiv Atom API rather than a search
snippet — search results routinely surface out-of-window work with
in-window-looking IDs.

- **2026-08-01 sweep** — four papers, and **three of them are cost or negative
  results that point the same way**. The headline is that this doc set has spent
  six weeks optimizing a proxy (tokens), and the month it was finally measured
  against dollars on real Claude Code runs, the proxy decorrelated at r = 0.15
  and two of the three interventions tested *lost money*.

# 2026-08-01 sweep

## Token Reduction Is Not Cost Reduction

[arXiv:2607.12161](https://arxiv.org/abs/2607.12161) — v1 2026-07-13, v2 2026-07-15.
Weinberger, Hozez.

The most important paper in this window, and the one three previous sweeps
(07-13, 07-31) explicitly recorded as missing: *"no prompt-caching / prefix-cache-aware
prompt-assembly paper at the harness level — every cache result was serving-side KV
work inber cannot reach."* This is that paper, and it is measured on inber's own
substrate: a pre-specified, hash-frozen, **paired campaign of 2,908 provider-billed
Claude Code runs** (2,848 analyzed) over 103 tasks, 7 repositories and 3 models,
inside a broader ~5,500 billed executions. It compares a baseline against two
generations of hook-based output compression and an API-boundary proxy.

Three findings. **(a)** Prompt-cache traffic dominated cost composition: **~87% of
reconstructed four-component cost and ~80% of the actual bill**, with an 8.7%
dollar-weighted residual not attributable from retained telemetry. **(b)** Local
payload reduction did not predict end-to-end billed cost — an arm that removed
**38% of estimated raw tool-output tokens incurred 6.8% *higher* paired cost**
(95% CI +2.8% to +11.3%), and per-task reduction correlated with cost change at only
**Pearson r = 0.15**. The stated mechanism is cache-write amplification: a compressed
span differs from the previously-cached bytes, the prefix match fails, and repeated
cache *creation* costs more than the removed input tokens saved. **(c)** Aggressive
compression removes action-critical evidence — on SWE-bench-derived Go tasks,
compression cut successful patch application from **27/40 to 15/40** by corrupting
verbatim edit anchors. They propose evaluating context-reduction systems by
**success-adjusted billed cost**, not token reduction.

*Confidence note: the 87% / 38% / 6.8% / 27→15 figures are from the abstract, verified
via the Atom API. The cache-write-amplification explanation and the per-slice
four-component percentages were read at PDF-summary grade only (Figure 1 was not
extractable). Treat the headline deltas as solid and the mechanism as the paper's claim.*

**What inber should consider:**

- **The compaction trigger is measuring the wrong quantity.** `conversation/summarize.go:14`
  fires on `len(messages) > cfg.TriggerMessages`, and the entire compaction thread in
  these docs (Self-GC, VISTA, rate-distortion, CORVUS, addressable-recall) optimizes
  token count. 12161 says token count and billed cost decorrelate at r = 0.15 once
  caching is 87% of the bill — and that a compaction *event* is itself a
  cache-invalidation event, since rewriting history mid-session destroys the prefix
  `engine/turn_prompt.go:213,219` paid to create. **The cheapest response is not a new
  mechanism: log the four Anthropic usage counters (`cache_creation_input_tokens`,
  `cache_read_input_tokens`, `input_tokens`, `output_tokens`) per turn and compute
  dollars.** inber currently cannot tell whether its compactor saves money. Do this
  before any further compaction work, because every proposal in the queue is currently
  unevaluable.
- **The 27/40 → 15/40 result is the tool-result-bounding warning.**
  `conversation/dedup_files.go`'s path-keyed supersession and any output truncation sit
  exactly where compression corrupted "verbatim edit anchors" — the byte-exact
  `old_string` an edit needs. A bounding rule that elides the middle of a read is how you
  get a 30% drop in applied patches with no error anywhere in the logs. Any bounding must
  be **anchor-preserving or recoverable**, which is the same conclusion Self-GC and VISTA
  reached from the quality side and this reaches from the cost side.
- It also usefully **deflates** work: hook-based compressors and API-boundary proxies are
  two of the three interventions measured, and both lost money.

## Cost-Aware Stopping for Tool Acquisition (CAM-DF)

[arXiv:2607.27083](https://arxiv.org/abs/2607.27083) — 2026-07-29. Feng, Zhang, Cheng, Qi.

Names a gap the four routing papers already on file leave completely open. 2606.17519
(embedding shortlist), 2606.16364 (readout, not layout), 2606.16591 (SING, retrieval
timing) and 2607.17598 (one routing level is enough) all answer *which* tools to rank.
**None answers how many to admit** — "a ranking alone does not determine how many are
worth selecting." They formulate cost-aware marginal decision-focused stopping over
ranked tool prefixes, trained on the offline gap between stopping now and the best
continuation: the gap's **sign labels the decision, its magnitude weights each error by
the payoff at stake**. They prove the objective is Bayes-aligned with the stopping target
and that **score-only threshold rules are provably suboptimal under heterogeneous costs**.
Evaluated on 1,343 tasks over five tool-use domains; on τ-bench Retail it beats a
predict-then-threshold baseline across all five ranking sources and both cost regimes,
with **larger gains under weaker rankings**. In live execution it exposes the agent to
**37% fewer tools** at comparable task success. It is a lightweight pre-execution plugin
over an existing ranker — no fine-tuning.

**What inber should consider:** this lands on the **unbuilt Phase 1 resolver** in session
bundling (`repo-store` / `bundle-store`, :8306/:8307) — the same slot 2606.17519 landed on,
supplying the missing half. inber's tool candidates genuinely have heterogeneous cost: a
`tool-store` (:8302) MCP entry costs a subprocess and ~890MB RSS (the browser-MCP OOM that
forced MCP descoping), a `skill-store` (:8301) SKILL.md costs prompt tokens *in the cached
prefix*, and an in-process `agent.Tool` costs a schema. A top-k or score-threshold cutoff
prices all three identically, which the paper proves is the wrong rule. Concretely: make
the resolver's output **a prefix-length decision with a per-candidate cost term**, not a
similarity threshold. The "larger gains under weaker rankings" result is the operationally
relevant part — inber has no trained ranker, so it sits squarely in the regime where the
stopping rule buys the most, and the cheapest version (hand-set per-class costs, learn
nothing) is a config table. Composes with the "one routing level is enough" negative
already on file: keep one level, spend the effort on the cutoff.

## Twin Agent: Context Residual Compression for Privilege-Separated Agents

[arXiv:2607.19595](https://arxiv.org/abs/2607.19595) — 2026-07-21. Hu, Jacob, Huang, Chen,
Li, Wagner (Berkeley).

A privilege-separation *design pattern* rather than a policy engine. Two nearly symmetric
agents: an **Explore Agent** that inspects untrusted content and a **Safe Agent** that
holds tool privileges. The trick is residual coding — the Explore Agent is **conditioned on
the Safe Agent's current context** and returns only a compact hint about the next action,
so the hint carries only the *residual* information the Safe Agent lacks rather than a
re-narration of everything. That is why it beats prior privilege-separation baselines on
the security-utility frontier: those degrade utility because the sanitized channel has to
re-transmit context the privileged side already holds. They make the tradeoff an explicit
measurable dial by **sweeping hint length and plotting utility against attack success**.
Evaluated on **SWE-bench Lite** — long-horizon SWE, not just an injection testbed — plus
AgentDojo and DecodingTrust-Agent.

**What inber should consider, with an honest overlap disclosure:** the 07-31 sweep already
wrote up *"taint tracking is unusable because it's permanent — branch the context instead
of poisoning it"* and independently proposed exactly this shape (route an untrusted read
through `agent/registry/spawn_tool.go`, return a sanitized summary to the parent).
**Twin Agent is not a new idea for inber; it is the measured, named, SWE-benchmarked
version of the idea inber already wrote down**, from a group whose threat modeling is
credible. Its incremental contribution is two things that note does not have: **(a)** the
*residual* framing — the child must be conditioned on the parent's context so the returned
hint is minimal, a concrete correction to a naive `spawn_tool` call that starts the child
cold and gets back a verbose summary; and **(b)** hint length as an explicit tunable, so
"how much do we let back across the boundary" becomes a knob with a measured curve rather
than a judgment call. Note the compounding with 2607.12161: a bounded hint **appended** at
the end of the parent's context is cache-friendly, whereas re-summarizing the parent's
history to brief a cold child is a cache *write*. Both papers agree the cheap boundary is
*append a small residual*, never *rewrite the prefix*. Second in line — it refines a
pattern already scheduled — but adopt the "condition the child on the parent's state"
correction before anyone builds it.

## A Controlled Real-Cost Decomposition of Agent-Skill Optimisation

[arXiv:2607.03048](https://arxiv.org/abs/2607.03048) — 2026-07-03. Xu, Wu.

The methodologically tightest paper in the sweep and a clean negative. **Ten
skill-delivery conditions × 40 software-engineering tasks × 3 repetitions = 1,200
rollouts**, measuring quality (verifier pass rate) and **real monetary cost at standard
provider prices on the same runs**, with the task as the unit of inference, task-clustered
intervals and multiplicity-controlled contrasts. Conditions separate no-skill, raw skill,
deterministic shortening, linear vs structured rendering from a shared semantic ledger,
scoped loading, and both compiler and executor model tiers. Results: deterministic
shortening is close to raw but **fails to establish non-inferiority** within the preset
margin; **structured rendering and scoped loading lower pass rate on the compact executor
without lowering cost**; structured rendering is indistinguishable from linear text at
matched content; compiler tier shows no robust effect. **The only contrast surviving
multiplicity correction is executor capability: +27 percentage points at roughly 5× the
real cost.** Under real prices, no optimised representation reaches practical break-even.

**What inber should consider:** a *stop-doing* result aimed at three live threads at once —
skill-store (:8301) ingests SKILL.md repos, the bundle-store design does **scoped loading**,
and "skill-description budgeting" is named as an optimization target in the 07-13 sweep.
This paper measures scoped loading and structured rendering as **strictly worse than
serving the raw skill** on a compact executor. It is the skill-layer twin of 2607.12161's
context-layer finding, and together they should recalibrate this doc set: both the
"compress the context" and the "compress the skill" programs were measured this month and
both lost. Two concrete moves: **(a)** default to serving skill text **verbatim** and place
it *before* the `__CACHE_BOUNDARY__` sentinel (`engine/turn_prompt.go:56`) so its cost is a
cache read rather than a per-turn input charge — a real cost lever the authors did not
test, and cheaper than any rewriting scheme they did; **(b)** treat executor tier as the
dominant lever it measures as, which is the direct counterweight to the ClawArena finding
already on file that cost ⟂ *management* quality. For skill *execution* specifically, model
tier is the +27pp knob and representation is noise. Scope limit, honestly: 40 SWE tasks,
one skill family, and the pass-rate penalty was observed on the **compact** executor, so it
does not directly indict scoped loading for a frontier model.

## Checked and rejected

Recorded so the next sweep does not re-triage them.

- **2607.23586** *Are You Still the Agent I Authorized?* (07-26) — authorization continuity,
  immutable effect ceiling, proof that mutation cannot amplify protected effects. Formally
  elegant and it maps onto `auto_hold_at_usd` plus the open permission-store gate, but it is
  proof-only with **zero measurement**, and the "What Can Be Enforced?" stateless-gate proof
  already on file occupies the same slot.
- **2607.05743** *Balkanization of Execution-Security Research* (07-07) — systematizes 39
  papers into 17 categories; its gap (4), *"every enforcement mechanism assumes an honest
  policy author,"* is aimed squarely at the open permission-store gate. Systematization with
  no measurement of its own.
- **2607.06906** *The Harness Effect* (07-08) — 41% cost / 44% wall-clock / 38% token
  reduction from swapping only the orchestration layer across 6 models, harness leverage
  r=0.99. Rejected as a **vendor paper** (Writer, comparing their own harness to "a frozen
  conventional production loop"); 2607.12161 makes the same economic point with a
  pre-registered design and a result adversarial to its own authors.
- **2607.12340** *Skills That Don't Exist* (07-14) — 36–43% skill-name hallucination rate,
  5,669 distinct fake names, repeated deterministically across models; retrieval grounding
  cuts it 40.8%→3.2% but drops correct-recommendation to ~1 in 6. Real supply-chain finding,
  but the fix is registry-side and skill-store already ingests from named GitHub repos rather
  than model-suggested names. Logged, not written up.
- **2607.17937** *How Agent Skills Fail under Long Contexts* — n=10 runs, headline p=0.0698
  (trend-level, author-acknowledged); overlaps SIGIL's "44% of mandated steps don't happen".
- **2607.23809**, **2607.21503** (vendor whitepaper), **2607.14275**, **2607.27283**
  (position paper), **2607.25408** (formal-only), **2607.27877** (MSEval), **2607.25297**
  (MTGuard) — benchmark- or framing-only, or duplicating the SWE-MeM / VISTA /
  addressable-recall thread.
- **Notably absent, again:** nothing on session resumption, and no agent-memory paper this
  window that isn't a benchmark or a re-run of the memory-transaction thread
  (ChronoMem / MemTxn / MemTX already logged).

## Cross-cutting note

Three of the four picks are cost or negative results and they converge. 2607.12161 says
token reduction ≠ cost reduction because caching dominates the bill; 2607.03048 says no
skill-representation optimisation reaches break-even at real prices; 2607.27083 says the
rank-then-threshold rule everyone uses is provably wrong once candidates have unequal cost.
The shared lesson is that the cheapest next step for inber is **not a new mechanism** — it
is to log the four Anthropic cache usage counters per turn, so the next proposal in this
doc set can be argued in dollars instead of tokens.
