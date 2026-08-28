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

# 2026-08-02 sweep

Dedupe base widened: **135 distinct arXiv ids** machine-extracted from the 04/05/06/07/08 research
docs plus `agentic-design-patterns.md`. The 08-01 sweep's own count of 108 was short because its
extraction pattern (`arXiv:` / `arxiv.org/abs/`) misses that sweep's "Checked and rejected" list,
which writes bare bolded ids (`**2607.23586**`). Use `\b(25|26)[0-9]{2}\.[0-9]{4,5}\b`.

**Window reality check, stated up front:** arXiv has **zero** cs.AI submissions indexed after
2026-07-30 (`submittedDate:[202607310000 TO 202608030000]` → `totalResults 0`; weekend plus
announcement lag), and Anthropic's engineering index has published nothing in July or August 2026.
So nothing here is newer than yesterday's sweep — all four keepers are mid-to-late-July work that
the 07-31 and 08-01 passes did not triage.

## ⚠️ Correction to the 2026-08-01 sweep: inber already logs the cache counters

The 08-01 entry closed with *"the cheapest next step for inber is not a new mechanism — it is to log
the four Anthropic cache usage counters per turn"*, and its 2607.12161 write-up asserted **"inber
currently cannot tell whether its compactor saves money."** That is wrong, and it was wrong when
written. Verified against the tree on 2026-08-02: `agent/agent_run.go:249-253` accumulates
`CacheCreationInputTokens` and `CacheReadInputTokens` off every response;
`session/session_logging.go:30-31,44-45` folds them into per-turn and cumulative session totals;
and both `session/session.go:285` and `session/timeline_jsonl.go:144` price them through
`CalcCostWithCache`. The counters are there and the dollars are computed.

What is *actually* missing is narrower and is already owned by open todo `71963446`: the **`trace`**
package declares `CacheRead`/`CacheWrite`/`Cost` at `trace/trace.go:49-50` and the single site that
builds a `trace.Turn` sets none of them. So the session path knows the numbers and the trace path
does not. Do not re-file the general claim; it is retired.

## Where Is the Cost of Third-Party API Routers in Agentic Software Development?

[arXiv:2607.23624](https://arxiv.org/abs/2607.23624) — v1 2026-07-26, v2 2026-07-29 (Atom-confirmed).
Fu, Li, Jiang, Dong. Code at `github.com/Riyasushin/SIDEL`.

Attacks the position between agent and provider — a third-party API router on the trusted path that
can rewrite responses. They build SIDEL (trace record / replay / inject / defend), curate **400
samples**, and run **four escalating intervention levels** (Response Substitution, Response Append,
LLM-Polished, LLM-Polished with Distribution Alignment) against **four representative coding
agents**. Headline: **all four agents achieved a 0% defense success rate at every injection level**
with no extra mitigations. Whitelist-based execution control and LLM review both improve resistance
but "do not fully restore end-to-end control." The framing claim is that client-side permission
mechanisms stop working once the router shapes what the model asks for.

**What inber should consider:** `guard/guard.go` gates on `CheckTool(name, input)` — a client-side
check on the call the model emitted — which is precisely the mitigation class the paper measures as
non-zero but insufficient. The concrete implication is that a whitelist surviving router injection
has to key on **repository effect, not tool name plus arguments**. Cross-file this against the open
permission-store gate item in memory: that note already records the store blocking ~nothing for
autoworkers, and 23624 says even a *working* client-side gate is bypassable from upstream. It is
also a direct warning about llm-bridge-as-gateway — every inber session routes through a bridge
sitting exactly where the paper attacks.

*Scope: the 0% is pre-mitigation and Atom-confirmed from the abstract; the post-mitigation numbers
are not in the abstract and the PDF was not read, so "whitelist + LLM review help but don't close
it" is the paper's claim, not a verified figure. The threat model needs a malicious or compromised
router — hygiene rather than active exposure if you only ever hit `api.anthropic.com` directly.*

## Retain or Consolidate? Budget-Dependent Operator Selection for Language Agent Memory

[arXiv:2607.17545](https://arxiv.org/abs/2607.17545) — v1 2026-07-20, v2 2026-07-21 (Atom-confirmed).
Kang, Liu, Kai, Liang, Tang, Cui, Zhong, Yuan.

**This is the agent-memory paper three consecutive sweeps recorded as missing** — not a benchmark
release, not another entry in the memory-transaction thread. It decomposes each memory operator's
utility into a **coverage effect** (on evidence retention would have omitted) and a **signed
replacement effect** (on raw evidence that already fits), then picks among Merge, Abstract and
Rewrite with a lightweight learner (OAS) using pre-generation features and held-out harm
calibration. On LongMemEval and LoCoMo: **consolidation improves absolute accuracy by up to 48%
under tight budgets, while retention is preferable under loose budgets**, with LoCoMo replicating
the crossover at a smaller budget. Secondary: **cross-note abstraction and merging generally beat
local rewriting** when compression is required.

**What inber should consider:** the finding maps onto a defect inber has already measured on itself.
The comment block atop `memory/auto_context.go` records that **12 of 13 assembled memories and 99%
of assembled tokens were compaction-archive fragments the reader never asked for**, drawn from ten
other sessions — that is the replacement effect going negative under a loose budget, observed in
production. Two moves. **(a)** `conversation/summarize.go:14` fires on
`len(messages) > cfg.TriggerMessages` (80/60/40 by profile in `summarize_config.go`) — an
unconditional *consolidate*. The paper says the operator choice is budget-dependent and that
consolidating under a loose budget loses accuracy, so make the trigger two-sided: consolidate only
when residual budget is genuinely tight, otherwise retain. **(b)** inber's compaction archive plus
the `memory_expand(id=…)` pointer is closer to Merge than to Rewrite, which is the side the paper
favors — worth keeping, and worth *not* replacing with a local-rewrite scheme.

*Scope: LongMemEval and LoCoMo are conversational-QA benchmarks, **not coding sessions**. A SWE
trajectory's evidence (verbatim edit anchors, file contents) is structurally different, and
2607.12161 already showed compression corrupts exactly those anchors. Take the shape — operator
choice is budget-dependent, abstraction beats local rewrite — and treat the magnitudes as
out-of-domain. OAS needs a trained utility estimator; the config-table version is the cheap
adoption.*

## Inference Economics of Enterprise Coding Agents: Cloud vs On-Premise

[arXiv:2607.13080](https://arxiv.org/abs/2607.13080) — v1 2026-07-13 (Atom-confirmed). Peng, Lin, Lee.

Two contiguous 28-day periods on a production monorepo, comparing API Claude Opus 4.7/4.8 on Claude
Code against on-premise GLM-5.1/5.2 on Opencode (NVFP4 on Blackwell), from LLM telemetry plus Git
history. **Prompt-cache hit rate 99.3%, cutting realized API cost by 88.6% to an effective $0.57 per
million tokens** — below the $2.83 amortized unit cost of the shared on-premise slice. On quality,
at comparable gross churn the local config had a **Fix Commit Ratio of 74.9% vs 45.9%**, with
commit-is-a-repair odds 2.6–4.9× higher within every difficulty tier (Mantel-Haenszel OR = 3.61).
TCO flips with allocation: on-premise saves 40.1% on a shared GPU, costs 43.8% more on a dedicated
reservation.

**What inber should consider:** this is the positive companion to yesterday's headline. 2607.12161
said token reduction decorrelates from cost because caching is ~87% of the bill; **13080 supplies
the other side — a well-placed cache boundary is an 88.6% cost cut, and a 99.3% hit rate is
achievable in a real workflow.** Given the correction above (inber already has the counters), the
next step is not to add logging but to **report realized cache hit rate per session and alarm when
it drops**. A hit rate sliding from ~99% to ~60% is the single observable that catches a
prefix-invalidating regression — including the `engine/turn_prompt.go` BP2 reshaping filed as a
defect this pass, which no existing test would catch. Second, the FCR result is a counterweight to
the cheap-local-model thread: the penalty appeared as repair burden, not as a lower pass rate.

*Scope, bluntly: **n = 1 developer, non-randomized, and the arms differ in model AND harness
simultaneously** (Opus/Claude Code vs GLM/Opencode), so the 74.9%/45.9% gap cannot be attributed to
model, quantization or harness separately. The 99.3%/88.6% caching figures are the robust part —
direct telemetry from one arm, not a between-arm contrast — and are what to carry forward. TCO is
Taiwan-market-parameterized.*

## SPORE: Persistence-Based Memory Extraction Against Per-User-Isolated Agents

[arXiv:2607.23444](https://arxiv.org/abs/2607.23444) — v1 2026-07-26 (Atom-confirmed). Gao, Chen,
Meng, Wang, Zang, Wang, Li, Guo.

Extracts private long-term memory through the **tool interface** rather than shared storage, so
user-level isolation is never violated. The observation: agents routinely embed LTM-retrieved data
into tool-invocation *parameters*, so a malicious tool exfiltrates memory as a side effect of being
called. SPORE beats two obstacles — adversarial-command semantics degrade retrieval precision (fixed
by persisting the command in short-term memory and emitting semantically *pure* anchors in tool
responses), and platform tool-call limits cap the per-trigger budget (fixed by persisting
**reactivation payloads** that resume the attack within and across sessions with no further user
trigger). **80.0% record extraction with unlimited triggers, 47.0% with only 20**, plus cross-user
identity linkage in multi-user deployments.

**What inber should consider:** `memory/auto_context.go` injects memory into the prompt without
anybody asking, and `memory/tools.go`'s `memory_expand(id=…)` adds a pull path; `tools/mcp` will
mount third-party servers resolved through tool-store (:8302), which is exactly the untrusted-tool
position SPORE assumes. The ask: **treat memory-derived text as tainted when it flows into a tool
argument**, enforced at `tools/adapter.go` / `guard.CheckTool` rather than at retrieval time — the
paper's point is that retrieval-side isolation stays intact and the leak is downstream. The
cross-session **reactivation payload** is the sharper half: a memory row written in session A that
re-arms in session B is a persistence primitive inber's store has no notion of, and `memory.db` is a
single shared store. Composes with Twin Agent (2607.19595, on file): the privilege-separated child
is the right shape, but SPORE shows the leak channel is the *parameters crossing outward*, not the
content coming back.

*Scope: numbers from the abstract, Atom-confirmed; PDF unread, so what "record extraction rate" is
measured against is unverified. Requires the attacker to control a tool the agent may call —
realistic for MCP-from-a-registry, not for inber's in-process `agent.Tool` set. Attack-side rates
are upper bounds under favorable conditions.*

## Below the measurement bar, but too actionable to lose

**[arXiv:2607.13071](https://arxiv.org/abs/2607.13071)** — *Compaction as Epistemic Failure* (v1
2026-07-11, Tamba). Documents a specific Claude Code failure: **partial stdout from timed-out
commands (exit 143) is recorded in compaction summaries as a confirmed result**, propagating false
positives across sessions and model versions with no re-verification. Root cause named as conflating
*observation* with *persistence*. Not a keeper — single-author case report, no n, no controls, no
ablation. Kept because it is the most directly actionable item in the window:
`conversation/manage_tool_pruning.go:63` already says "the field that matters is is_error," yet
nothing in `conversation/summarize.go` or `summary_generation.go` mentions exit codes or error
status. There is a `prune_preserves_is_error_test.go`; the equivalent assertion for the *summarize*
path does not exist. One session's work to check.

**✅ Checked and FIXED, inber `efafc69` (2026-08-02, nightly worker mining `a88bca06`). The claim
held, and the code was worse than the paragraph.** The gap is not in `summarize.go` or
`summary_generation.go` — it is one level down, in `messagesToText`
(`conversation/message_utils.go:154`), which builds the only text the summarizing model ever sees.
It rendered every result as `[tool_result: <first 200 chars>]`, `is_error` and all. Because the
truncation keeps the *head*, a command that prints progress and then dies loses its error message to
the ellipsis: the fixture `40 ok lines + "make: *** [all] Error 2"` reached the summarizer as forty
ok lines and nothing else. Exit codes are a red herring — inber never sees one; `is_error` is the
signal, and it was on the block the whole time. A failed result now renders `[tool_result failed:
...]` and the system prompt says such a call must not be written up as a confirmed result. Pinned by
`conversation/summarize_preserves_is_error_test.go` (three cases, sabotage-verified both ways). **Do
not re-file.**

## Checked and rejected

- **2607.15593** *Scalable LLM Agent Tool Access in the Cloud* (07-17) — 98% Top-15 recall over
  3,000+ tools, 8.9× faster selection, 23.8% less token usage. Genuinely relevant to `tools/mcp` and
  the MCP-descoping OOM, rejected only as a **vendor systems paper** whose numbers are self-reported
  ratios against its own prior deployment. Best candidate if a future sweep wants a fifth.
- **2607.26117** *Try Again, Don't Look Back* (07-28) — cleanest negative in the window
  (placebo-controlled; blind resampling beats self-repair below 7B at 2.5–5.5× fewer tokens;
  anchoring reproduces a near-identical program 33–68% vs 2–14%, r=0.96). Rejected on **scope**:
  1.5B/3B/7B on MBPP+, and statistically tied at 7B — plausibly gone at the only scale inber runs.
- **2607.13083** *Phantom Guardrails* (07-13) — self-improving harnesses fabricate failures (15/60 vs
  0/60). Synthetic micro-lab, and inber does no automated harness self-optimization.
- **2607.14004** *Do Agent Optimizers Compound?* (07-15) — GEPA transfers *below* baseline; but the
  winner is the authors' own product. Vendor paper.
- **2607.17044** *Where Does Agent Reliability Come From?* (07-19) — verification loop contributes
  only +1.5 of +11 points, adversarial to its own headline; single-author, compares Leni to its own
  base model.
- **2607.21672** *Pixels for Programs?* (07-23) — 75–86% input-token reduction rendering code as
  images, but explicitly disclaims measuring cost, accuracy or agent efficiency. Pure token
  reduction is the unit 2607.12161 just invalidated.
- **2607.22807** *The Best Programming Language for Tokenmaxxing* (07-24) — real trajectory analysis,
  but Python/Java/Rust/OCaml and **no Go**, so the one number inber would want doesn't exist.
- **2607.05378** *CompactionRL*, **2607.22690** *LazyMem* — training-side; inber cannot train a model.
- **2607.18161** *TRIM* (07-20) — post-hoc patch minimizer, not a hot-path component.
- **2607.19338** *CodeRescue* (07-21) — overlaps the CAM-DF cost-aware-stopping slot from 08-01.
- **2607.09175** *GRACE* (07-10) — optimizes a persistent system-instruction graph inber does not have.
- **2607.20764**, **2607.16961**, **2607.11423**, **2607.14159**, **2607.26520**, **2607.17751**,
  **2607.11138**, **2607.25032**, **2607.08740**, **2607.14336**, **2607.17641**, **2607.24604**,
  **2607.26587**, **2607.10441** — benchmark-, framing- or training-only, or duplicating threads on
  file.

**Still absent, third sweep running: session resumption.** `all:"session state"`, `all:"agent
session"`, `all:"long-running agent"` and `all:"resume"` over the window return physics resummation
papers or already-triaged memory work. Nearest miss is 2607.08740, which describes resumable
workflow objects but leaves transition semantics as future work. **The agent-memory gap is now
closed** by 2607.17545.

# 2026-08-03 sweep

One paper survived the not-already-covered check. Eight candidates were pulled;
2607.15516 (CAPC, cache-aware compression), 2607.00692 (Self-GC), 2607.10569
(`execute_code` ablation), 2607.05378 (CompactionRL), 2607.25656 (OrchBench) and
2607.05690 (memory latency) are all already in `2026-07-harness-research.md`,
`2026-08-harness-research.md` or `cache-optimization.md`, and the HuggingFace
July-intrusion timeline is at `agentic-design-patterns.md:1994`.

## CoACT: action-preserving observation compression

[arXiv:2607.02911](https://arxiv.org/abs/2607.02911) — Chen, Zhu, Zhang, Li, 2026-07-03.

Trains a small compressor for **tool observations** — file reads, test output, shell
results — under a *next-action preservation* reward rather than a summary-fidelity or
length reward. A teacher proposes candidate compressions of one observation; any
candidate that changes the agent's next tool call is discarded; among the survivors the
shortest wins. Length is the secondary objective, not the first. Reported: 33.0% total
token reduction on SWE-bench Verified across three agentic models, with task success
close to uncompressed.

This is the direct answer to the failure the 2026-08-01 sweep recorded above.
arXiv:2607.12161 measured an arm that removed 38% of raw tool-output tokens and
**cost 6.8% more**, and cut successful patch application from 27/40 to 15/40 by
corrupting the verbatim edit anchors the model was going to quote back. CoACT's
criterion is exactly the test that arm did not have: an anchor the next action depends on
is, by construction, an anchor whose removal changes the next action, so the candidate
carrying it is rejected. And the *slot* matters as much as the criterion — an observation
is append-only and sits after every cache breakpoint, so compressing it at the moment it
is produced writes no byte that was already cached. That is the half 12161's
cache-write-amplification mechanism punishes, and CoACT does not touch it.

**What inber should consider:**

- **Adopt the criterion, not the model.** Behavioural equivalence of the *next tool call*
  is testable offline against recorded trajectories without training anything: replay a
  stored session, apply the truncation, assert the next tool call is byte-identical.
  inber already truncates tool results at `session/truncate.go:58`
  (`TruncateToolResult`, head/tail with a per-strategy split at `:98`,`:134`) and has
  never had a test that asserts anything about the *agent* after truncation — the
  existing tests assert on the truncated string. A replay harness over
  `~/.inber/server` sessions is the cheap version.
- **Compress at production, not in history.** inber's live pruning path
  (`conversation/manage_tool_pruning.go:30` `pruneToolResult`, reached from
  `engine/build.go:123-125` → `conversation.PruneConversation` → `conversation/manage.go:102`,
  and from the staging path at `conversation/staged.go:131`) rewrites results that are
  *already in the message array*, i.e. already inside a cached prefix. That is the arm
  12161 measured losing money. Truncating harder at `session/truncate.go:58`, before the
  result is ever appended, costs the same tokens and invalidates nothing.
  Whether inber should therefore *stop* pruning history is a real trade — pruning is what
  keeps a long session under the context limit — and is not decided here.
- Dead code found while checking this, recorded so nobody re-derives it:
  `conversation/manage_tool_pruning.go:132` `truncateOldToolResults` is marked "(legacy)"
  and has zero callers, test or otherwise.

# 2026-08-04 sweep

Two papers survived the dedupe. Both dates were read off the arXiv `/abs`
submission-history block, not off a search snippet. The window's other strong
candidates were already on file: **2607.15516** (CAPC) is at
`cache-optimization.md:297` and `2026-07-harness-research.md:927`, **2607.25066**
(ARC) at `2026-07-harness-research.md:877`, and **2607.19595** (Twin Agent),
**2607.23809** (ACM) and **2607.13071** (Compaction as Epistemic Failure) are all
in this file already. Blogs were empty this window — Anthropic engineering's most
recent post is 2026-04-23, and DeepMind/HuggingFace/Meta AI published nothing on
harness design, context, caching or memory.

## Remember When It Matters: proactive memory beats retrieval, and beats always-on injection

[arXiv:2607.08716](https://arxiv.org/abs/2607.08716) — v1 2026-07-09, no revisions.
Wu, Zhang, Zhou, Wang, Peng, Li, Fan, Zhao.

Names the failure mode **"behavioral state decay"**: decision-relevant state —
task requirements, environment facts, prior attempts, diagnoses, open subgoals —
scatters across a growing trajectory and stops influencing decisions, either buried
in the window or pushed past it. The intervention is a **separate memory agent
running alongside an unmodified action agent**, maintaining a structured memory bank
from the recent trajectory and deciding, per step, whether to inject a
memory-grounded reminder **or stay silent**. Plug-and-play against frontier agents
and existing harnesses. Reported: **+8.3 pp pass@1 on Terminal-Bench 2.0** and
**+6.8 pp on τ²-Bench**, for both weaker and stronger action agents.

The ablation grid is the reason this is worth filing, because it is a **negative
result about the two designs inber actually has**. Selective intervention beat, in
order: passive bank exposure, **always-on injection**, advisor-only guidance, and
**general retrieval**. The claim is that *when* to inject is worth more than *what*
is stored — the store is not the lever.

**What inber should consider:**

- **inber is the "always-on injection" arm, and its retrieval is the "general
  retrieval" arm — the two baselines this paper beats.** Every turn,
  `engine/turn_prompt.go:81` derives `messageTags` from the current user message via
  `memory.AutoTag`, `:82` derives a budget from the turn counter, and `:86` hands both
  to `MemStore.BuildContext`, which scores by tag overlap plus a recency bonus and
  cuts on budget. There is no gate anywhere on that path that can answer "inject
  nothing this turn". The paper's result says that gate, not a better scorer, is where
  the points are.
- **The gate is cheap to host here and expensive to get wrong.** inber already spawns
  sub-agents, so the memory agent has a natural home, but note the trade this creates:
  a per-turn LLM gating call is a second request on every turn, and 2607.12161
  (2026-08-01 sweep) put cache traffic at ~87% of the bill. A shadow-mode rollout —
  rank, decide, inject nothing, log what it *would* have suppressed — is the version
  that costs nothing to learn from, and matches the discipline already recorded at
  `agentic-design-patterns.md:1504`.
- Deciding whether inber wants a *silent* option at all is a real call and is **not
  made here**: today's unconditional injection is also what makes the BP2 prefix
  predictable, and a gate that varies membership per turn is exactly the cache-churn
  shape the 2026-08-02 entry filed.

## PRO-LONG: keep the whole log and let the coding agent grep it

[arXiv:2607.20064](https://arxiv.org/abs/2607.20064) — v1 2026-07-22, v2 2026-07-23.
Fox, Wang, Rosu, Dhingra (Duke). Code and logs released.

Rejects lossy compaction outright. The argument: every harness commits to a strategy
for what to save and how to load it, and existing strategies face a tradeoff where
"preserving more information makes retrieving relevant details less tractable."
PRO-LONG's answer is to keep a **complete, structured interaction log** and lean on
the fact that the agent is *already a coding agent* — it searches its own history
with the file tools it has, rather than carrying that history in context. Reported on
the full ARC-AGI-3 public game set: **+18.0 pp average over a base coding agent**
across frontier models, matching or exceeding specialized harnesses at up to **76.1%
pass@1**, while using **4.2–5.8× fewer tokens**; 97.4% best@2 with Fable 5 at $1,750.

Honest caveat, since this doc set has been burned by transfer claims: the benchmark
is ARC-AGI-3, i.e. games, not code editing. The token-efficiency mechanism transfers
cleanly; the accuracy delta is argued, not shown, for SWE-style work.

**What inber should consider:**

- **inber already has the substrate and does not point the agent at it.** Every session
  writes a `session.jsonl` (`session/timeline_jsonl.go:178` `findSessionJSONL`,
  reconstructed at `:21`), and the agent has `read_files`, `list_files` and
  `shell_commands`. Nothing tells the model that file exists or where. Exposing the
  current session's log path as a readable artifact is close to free and is strictly
  additive to the prefix — it adds one path string, not a history.
- **It also lands directly on a deliberate inber decision, which is why this is a
  consideration and not a defect.** `tools/tools.go:39-41` removed the dedicated
  ripgrep tool on the documented rationale that grep-then-read "encourages a
  two-turn pattern when reading the file directly is one turn" — with `rg` still
  reachable through `shell_commands`. That rationale is correct for reading a *known
  file* and is exactly what PRO-LONG contests for *unknown history*, where reading
  the file directly is not an option because the file is 40 MB of JSONL. Whether the
  two-turn cost is worth paying for history search specifically — and whether that
  argues for re-registering a search tool scoped to the session log rather than the
  repo — is the open question, and it is not settled here.
- Composes with ARC (2607.25066, on file): ARC supplies the addressing scheme (stable
  ids, truncate the rendering and keep the row), PRO-LONG supplies the argument that
  the retrieval mechanism should be the agent's existing file tools rather than new
  machinery. Both keep the prefix append-only, which is the property
  2607.12161 says the bill actually turns on.

# 2026-08-05 sweep

Coverage check first, because it changes how this sweep should be read: `docs/`
carries **120 distinct arXiv ids and not one of them is `2608.*`** — the newest is
`2607.28147`. The 08-01 through 08-04 sweeps all logged late-July work. The entire
August listing was unswept, which is where all four of these come from, and it is
also why the crop is unusually good: this is a backlog, not a week's yield. Every
id below was confirmed against the arXiv Atom API (`export.arxiv.org/api/query`,
following the 301 to https — the bare http call returns empty and silently looks
like "not found"), title and `published` date both.

The theme, unplanned: **three of the four are about deciding something before the
model is asked, using state the model never sees.** Ledger governs a command before
it runs; the Wix gate drops a tool before it is described; CapLease admits an action
before it is authorized. Only the compression paper is about the prompt itself.

## Ledger: interaction history as deterministic execution state

[arXiv:2608.00808](https://arxiv.org/abs/2608.00808) — Wang, Xu, Li, Peng, Adams,
Hassan, Chen (Concordia / Queen's / ByteDance), published 2026-08-01.

Ledger keeps a deterministic online record of what the agent has observed, modified
and attempted, and applies it at two boundaries per step. **Inform** appends a compact
runtime-state view to the prompt before the model acts. **Govern** intercepts a
proposed command before execution and either returns a still-valid cached earlier
result instead of re-running it, or flags it as likely-redundant repetition. Zero
extra LLM calls; it wraps an unmodified agent. All 500 SWE-bench Verified instances:
Pass@1 56.2% → 64.2% (GPT-5 mini) and 75.8% → 81.0% (MiniMax M2.5), with **total cost
down 28.9% / 31.8%**; +3.4pp at −24.4% cost on OpenAI Codex. Ablations put most of the
*resolution* gain on govern and most of the *efficiency* gain on inform.

**Why it matters here.** This is the successor to CORVUS (2607.22711, on file at
`2026-07-harness-research.md`), and the difference is the part worth taking. CORVUS
fixes stale *file* snapshots with a synced registry — which is the half inber has
partially built, in `agent/read_cache.go`. Ledger's govern path is a *command*-level
result cache with a redundancy check, which CORVUS does not have and inber does not
have. Both paths are pure Go with no model dependency: the ledger is the shape of a
SQLite session-store table, and govern is a hook in the tool-dispatch layer that
inber already owns (`agent.ExecuteToolCallWithChainAndSideband`).

Two cautions before anyone reaches for this. First, it is the **first paper in this
corpus reporting a large billed-cost reduction that is not achieved by shrinking the
prompt** — which is exactly what 2607.12161 (the 08-01 sweep's headline, r = 0.15
between tokens and dollars) said to go looking for. That makes it interesting and
also means the mechanism, not the number, is the transferable part. Second, the
inform view is *appended* per step rather than rewriting history, so it does not
invalidate the cached prefix the way a compaction rewrite does. If inber implements
inform as anything other than an append at the tail, it inherits the cache-write
amplification 2607.12161 blames for the decorrelation.

**What inber should consider:** govern first, inform second. Govern is a dispatch-layer
hook with a measurable local win and no prompt-shape risk. Inform touches the cached
prefix and should be built only at the tail, where the volatile context already lives.

## Control Under Compression: the tool/policy block has a cliff, and it is per-context

[arXiv:2608.01056](https://arxiv.org/abs/2608.01056) — Hou, Yang, published 2026-08-02.

CompressAgent: 15,525 environment-verified runs over nine independently built "agent
control contexts" — the persistent system-side text specifying tools, arguments,
policies, execution protocols and recovery — across three task families and six
retained-context budgets. The reliability curve is **nonlinear and method-dependent**.
At 75% retained, generic rewriting and section-based compression hold 92.7% / 92.4%
against a 93.8% full-context baseline: essentially free. Between 50% and 35% the
methods diverge violently — at 35%, section-based 47.0%, obligation-aware 39.0%,
generic rewriting 19.9%. Below 25% executable protocols become fragile. Reliability
varies enough *across* control contexts that universal compressor rankings are
invalid; each context needs its own qualification. Failures surface as tool-execution
and action-parsing errors, not reasoning errors.

**Why it matters here.** Every prior compaction paper in this corpus is about the
*transcript*. This one is about the part of the prompt inber never compacts and always
caches: the tool-definition and policy block. It gives a defensible budget (trim to
~75%, stop), a warning that the safe ratio must be measured per tool-set rather than
assumed, and a diagnostic signature — a spike in tool-execution and parse errors, not
worse answers — that maps onto counters inber can already emit.

It also lands on a live inber finding. The uncapped `then`/sideband enum
(`agent/chain.go:115-128`, recorded on 2026-08-04) injects ~1.4–1.6 KB of identical
schema into *every* tool, measured at 6,566 B → 18,338 B for nine tools. That is the
obvious compression target, and this paper says the obvious move is the wrong one:
the tool block is the **stable cached prefix**, so shrinking it buys 0.10× reads while
risking the failure mode that shows up as broken tool calls. Deduplicate the repeated
schema — which is lossless — rather than summarizing the block, which is not.

**What inber should consider:** never auto-summarize the tool block as a cost measure.
If it is ever trimmed, qualify the ratio against inber's own tool set and watch tool-
execution error rate, not answer quality.

## CapLease: retries, delegation and crash-resume defeat single-use authorization

[arXiv:2608.01710](https://arxiv.org/abs/2608.01710) — Xu, Fan, Wang, Li, Liu,
published 2026-08-03.

Names **semantic replay**: an agent replans, retries, delegates to a subagent, or
resumes after a crash, and one user approval executes several times under *freshly
issued, individually valid* single-use token identifiers. The argument is an
impossibility one — identifier-local consumption provably cannot prevent reissuance
unless the issuer keeps **monotonic durable state** over the triple (authorized
action, confirmation event, remaining execution budget). CapLease binds an
authenticated confirmation to a *canonical* action and enforces transactional
Issue-Prepare-Commit. Across replanning, retry, delegation, concurrency,
confirmation-replay and crash-recovery scenarios, only stateful designs prevented
duplicate admission, and duplicate *external effects* additionally required an
idempotent sink. No headline accuracy numbers; it is a systems argument with scenario
coverage.

**Why it matters here.** This sits on the intersection of three inber properties at
once — subagent spawning (delegation), session resumption from the SQLite store (crash
recovery), and an approval prompt. inber's exposure is unusual and worth stating
precisely: it currently has **no approval routing at all** — `NeedsApproval` is
rendered as a flat refusal because no session sets `ApprovalFunc`
(`engine/build_hooks.go:85-88`) — so there is no lease to replay yet. That makes this
a *design input for work not yet done* rather than a defect, and it is the cheapest
possible moment to get it right: the approval record should be a durable row keyed on
a canonical action form with a decrementing budget and Issue/Prepare/Commit states,
written in the same transaction as the tool call, not a per-call boolean.

The canonical-action requirement links straight to 2607.27834 (canonicalize a command
before judging it, already on file) — same lesson, one layer up. And note the second
half that a store cannot fix: duplicate *effects* need an idempotent sink. An approval
lease stops inber admitting the same `git push` twice; it does not stop the push.

**What inber should consider:** when `ApprovalFunc` is finally wired, model approval as
a lease row, not a callback return value. Retrofitting durability onto an approval
mechanism after subagents already inherit it is the expensive order.

## Executability gating: drop a tool before you describe it

[arXiv:2608.01050](https://arxiv.org/abs/2608.01050) — Ashkenazi, Kloz, Ulianchenko
(Wix), published 2026-08-02.

A deployed three-stage pipeline. A recall-oriented semantic matcher narrows a large
skill library *without* consulting state; a **deterministic executability gate** then
drops every candidate whose own hard-stop preconditions currently hold; only the
survivors are described to the model, which makes the final call. The correctness
argument is **predicate parity** — the gate evaluates the *same* exit predicates the
skill itself would, against fresh authoritative state, so a blocked candidate provably
could not have completed. On 756.6K production messages: semantics retained 23.1%, the
gate removed 59.4% of candidate skill-message pairs and 59.1% of skill-description
tokens, **90.5% total context reduction**. Counterfactual replay: without the gate the
model picked a skill that would have failed in **7.8%** of cases.

**Why it matters here.** The deployment is customer care; the mechanism is domain-free
and lands on how inber decides what tools to put in the prompt. Today that decision
consults configuration and nothing else (`engine/build_tools.go`), so a tool whose
preconditions cannot hold is still described in full: `deploy` with no credential in
auth-store, a workspace tool with no worktree, an MCP tool whose server is unreachable.
The 7.8% is the price — a confidently selected tool that cannot run, burning a turn and
adding a cache-invalidating error observation.

The load-bearing constraint is predicate parity, and it is the single-source-of-truth
rule restated: **the gate must read the tool's own preconditions, never a copied
allowlist.** inber has already been bitten by the copied-table version of this twice —
`guard.isDangerous` and the read-cache invalidation switch are both closed name-keyed
tables that fail open on an unrecognized name. A gate built the same way would be a
third.

**What inber should consider:** this is the honest counter-argument to the
"deferred/lazy tool schemas" idea recorded against codex #36998, which inber does not
need at 13–17 tools. Gating on *executability* is cheaper than deferral, needs no
search tool, and removes tools that would have failed rather than tools that are merely
unlikely to be used. Note the tension with prompt caching, which this paper does not
address: a gate that changes the tool list between turns rewrites BP1, the first hashed
section, and cascades to everything after it. Gate at session start, or accept the
write. That trade is unresolved and should not be decided by whoever implements it
first.

## Checked and rejected

- **[arXiv:2608.03222](https://arxiv.org/abs/2608.03222) FailFast-RestartSmart** — a
  0.6B monitor predicts failure from observable prefixes; on alarm, restart a fresh
  rollout offering the interrupted repo diff as an *optional* overlay. 14.6–20.4% token
  savings at 5% FPR; 66.6% → 71.8% resolution at 25% FPR. Rejected only because it needs
  a separate monitor model trained and served, which inber has no path to. **The
  restart-with-optional-diff-overlay half is harness-side and free**, and is the part to
  remember if the trigger ever comes from somewhere cheaper than a trained monitor.
- **2608.00902** *Practical Online KV Cache Compaction for LLM Agents* — serving-side KV
  work. inber is API-side and cannot reach it. Same rejection this corpus already applies
  to the class.
- **2608.02639** *Instruction Stacking Collapse* — real result (follow rate 96% → 20% as
  constraints stack; "output JSON" jointly unsatisfiable with nine other constraints),
  but the remedy is an LLM prompt-rewrite whose benefit the paper states is nil for
  frontier models, which is what inber runs.
- **2608.02645** *Verified Tool Calls Under Non-Atomic Failures* — right idea
  (postcondition verification, verify-before-retry, idempotency keys) but simulated
  environment and no numbers. 2608.01710 above covers the same ground with a stronger
  argument.
- **2608.00122** *Shared Organizational Memory for Enterprise Coding Agents* — deployment
  snapshot, effects "remain under evaluation". No result.
- **2608.02680 TraceCompiler**, **2608.02679 AI Sandbox**, **2608.00267 LoopsBench**,
  **2608.01558**, **2608.00017** — benchmark-only or thin mechanism.
- **2607.05378 CompactionRL** and **2607.10582 MemDecay** surfaced repeatedly in search.
  CompactionRL is already rejected as training-side at line 421 of this file; MemDecay is
  serving-side KV eviction.

## Blogs: nothing in window

Anthropic engineering has published nothing since 2026-04-23. DeepMind's July output is
Gemini and robotics model launches. HuggingFace's only in-window agent post is Allen AI's
Shippy retrospective (~07-26), a maritime-domain agent whose lesson is per-session
Kubernetes sandboxing — no transferable mechanism. Meta AI had nothing in window.

# 2026-08-06 sweep

Coverage note: the 08-05 sweep's ceiling was **2608.03222**; the listing now runs to
**2608.05xxx**, so 2608.033xx+ was unswept, plus two late-July items earlier sweeps
missed. Dates below read off the arXiv `/abs` submission-history block, not search
snippets. Deduped against the 175 arXiv ids across all five paper docs and
`comparisons/`.

## ECLoop: gate the *action* on evidence the trajectory has already observed

[arXiv:2607.28815](https://arxiv.org/abs/2607.28815) — v1 2026-07-30, no revisions. Xu,
Li, Wang, Yang, Chen (cs.SE).

Names the failure **premature commitment**: a coding agent edits files or submits a patch
before it has looked at enough repository evidence to justify the change. ECLoop sits
between the agent and the repo. It compiles per-task **evidence conditions** from the
issue text and repo structure — what must have been observed before each *class* of
mutation — tracks which the running trajectory has satisfied, and **postpones** any
proposed action whose conditions are unmet. All 500 SWE-bench Verified instances, two
models × two scaffolds: **Pass@1 +4.8 to +11.8 pp**, no retraining and no scaffold
change, and **token consumption down up to 12.1%**, because the agent stops chasing
actions it cannot support. The ablation matters: structured conditions beat an equivalent
natural-language summary, so the structure is load-bearing rather than the reminder.

**What inber should consider:** this is the third dispatch-layer gate in this doc — after
Ledger's *govern* (2607/2608 entry, `2608.00808`) and the Wix executability gate
(`2608.01050`) — but it is keyed on **trajectory state** rather than a result cache or
static preconditions, and it is the one with a headline number on the benchmark inber
cares about. It slots into `agent.ExecuteToolCallWithChainAndSideband` as a predicate over
"what has this session read so far", which `agent/read_cache.go` already tracks part of.
The property that makes it cheap is the one the 08-01 entry (`2607.12161`) says actually
governs the bill: **it costs nothing in the prompt** — no injected text, no BP1/BP2 churn.
Pair the reading with CapLease: a postponement is a deferral, not a denial, so it needs the
same durable-state care the moment a subagent inherits it.

## Skill-Use: skill invocation is a property of the harness, not of the model

[arXiv:2608.04828](https://arxiv.org/abs/2608.04828) — v1 2026-08-05. Han, Xu, Liao, Wang,
Jiang, Di, Lu, Hu, Xiao (cs.CL).

A benchmark for skill use **under progressive disclosure** — the agent sees only a skill's
name and short description and must retrieve the full procedure before following it, which
is exactly inber's skill-store contract. The outcome is decomposed into three
independently-scored facets: **Trigger** (did it invoke the right skill at all),
**Compliance** (did it follow the prescribed procedure), and **Boundary** (did it avoid
forbidden operations), with execution credited only after trigger. 79 real skills, 177
executable tasks, nine domains, isolated Docker sandbox, trajectory-rubric scored. Eight
LLMs across two harnesses: the best configuration reaches an SU of only **0.613**, trigger
and compliance fail as **independent bottlenecks**, and both absolute scores *and model
rankings* shift with the harness.

**What inber should consider:** the harness-conditioning result means a skill-store quality
signal measured under one harness does not transfer, so inber cannot read skill efficacy
off an external eval — it has to measure trigger rate against its own prompt assembly. The
immediately useful piece is the **three-way split**. inber today can only observe "the
skill didn't help", which conflates a `description` that never fired with a procedure that
fired and was ignored — two defects with opposite fixes (rewrite the description field vs.
restructure the SKILL.md body). Logging trigger separately from compliance is a counter,
not a feature. Note also that **Boundary** is the skill-side twin of the permission guard:
a skill that says "never do X" and a guard that blocks X are two enforcement points for one
rule, and per the single-source-of-truth rule they should not each carry their own copy of
the list.

## Scrouting / SuperScout: the value is the *verified handoff*, and the paper's own ablation says the routing is not

[arXiv:2608.04804](https://arxiv.org/abs/2608.04804) — v1 2026-08-05. Bhola, Krishnan, NS
(cs.SE).

A 7B searcher explores the repository first and emits a **structured handoff whose
reproduction claims are sandbox-verified, with false claims stripped before delivery**; its
hidden states plus the task text then route to one of four frontier fixers (adding a fixer
needs no retraining). On the full Python slice of SWE-bench Pro (266 tasks, official capped
budget tier): **159/266 solved vs 158 for the best single model, at about one fifth the
cost per solve**, searcher compute under half a cent of GPU per task. The reason to file it
is the authors' own honest ablation — **a no-router baseline that always picks the cheapest
fixer, given the handoff, ties the full routed system.** A paired calibration study suggests
the handoff *redistributes* rather than adds solving ability, lifting the three cheaper
fixers while slightly hurting the strongest (directional only, N=99).

**What inber should consider:** inber already has subagent spawning, so the actionable half
is nearly free and the expensive half — a trained 7B searcher and a hidden-state router —
is the half the paper's own ablation says did not matter. The mechanism worth taking is a
**read-only scout subagent that returns a structured handoff whose claims it could not
reproduce in the sandbox are deleted rather than hedged.** Two cautions land directly: the
"slightly hurting the strongest fixer" direction argues for scoping a handoff to cheaper
models rather than applying it uniformly, and this is the third independent appearance of
the scout-subagent shape in this corpus — including one where **opencode built it and then
removed it** ([PR 30435](https://github.com/sst/opencode/pull/30435),
`docs/comparisons/opencode.md`, 2026-06-03 entry). That removal is evidence about the
*dedicated-subagent packaging*; this paper is evidence about the *verified handoff
artifact*. They are separable and should not be conflated.

## Provenact: authorization goes stale between approval and effect

[arXiv:2608.02764](https://arxiv.org/abs/2608.02764) — v1 2026-08-03. Peng, Wu (cs.MA).

Identifies **stale authorization** as the core failure of concurrent agent governance:
safeguards decide whether an action is allowed from the state visible *when the action is
requested*, but budgets, approval status and risk signals can change before the effect
actually occurs. It defines **policy-state serializability** — every committed effect must
be explainable as authorized against the policy state immediately *before it occurred*, not
at request time — and implements Provenact, a runtime that keeps policies as reviewable
programs while coordinating the state and effects needed to preserve their decisions. A
PostgreSQL-backed prototype prevents stale authorizations that baselines passing policy
state as ordinary request context miss, preserves delayed approvals while unrelated work
proceeds, and keeps policy evolution in policy text rather than in trusted provider code.

Measurement caveat, stated plainly: the procurement workflow is **scripted and LLM-free**.
This is a systems argument with scenario coverage, the same class this doc already accepted
CapLease (`2608.01710`) on — not an accuracy result.

**What inber should consider:** this is the *orthogonal* half of CapLease, not a duplicate.
CapLease is about one approval being **replayed** across retry, delegation and crash-resume;
Provenact is about one approval being **honored too late**, after the world it was granted
against has changed. inber's exposure is structural: subagents run concurrently against a
shared worktree and shared session state, so a verdict the guard reaches in Assist mode is
evaluated against state a sibling subagent may already have mutated. The design consequence
is one line and it is the same one CapLease implies from the other direction —
**re-evaluate the policy at commit, not at request**, which means a guard verdict cannot be
a value the caller carries around. Decide this *before* `ApprovalFunc` is wired
(`engine/build_hooks.go:85-88` records that nothing sets it yet), because both papers now
say the retrofit order is the expensive one.

## Checked and rejected — 2026-08-06

- **[arXiv:2608.01918](https://arxiv.org/abs/2608.01918) HarnessCompass** (08-03) reports
  SWE-bench Verified Pass@1 **54% → 66%** over five automatic-harness-evolution iterations,
  with claimed transfer to held-out tasks and other models. Withheld because it sits in
  direct tension with *Rethinking the Evaluation of Harness Evolution for Agents*
  ([arXiv:2607.12227](https://arxiv.org/abs/2607.12227), `2026-07-harness-research.md`),
  which argues AHE does not consistently beat matched baselines. That contradiction is
  worth resolving deliberately rather than filing as a finding, and the method needs an
  evolution loop inber has no path to. The one cheap transferable idea: **solicit
  first-person feedback from the agent about harness usage** as an evolution signal,
  separate from trajectory-derived signals.
- **[arXiv:2608.03463](https://arxiv.org/abs/2608.03463) LeanMem** (08-04) types memories by
  compressibility (profile / event / record), updates only the evolving type, and allocates
  retrieval budget per query — up to **+15.1 points** at lowest cost on LoCoMo and
  LongMemEval-S. Withheld on transfer: conversational-memory benchmarks with GPT-4.1-mini
  and Qwen3-8B, and it is a **store-side** improvement, which the 08-04 entry
  (`2607.08716`) argues explicitly is not the lever — the gate on *when* to inject is.

## Blogs: still nothing, 2026-07-23 → 2026-08-06

Anthropic engineering's index surfaces no post newer than the 2026-04-23 item already
recorded. DeepMind, Meta AI, Microsoft Research and HuggingFace published nothing in window
on harness design, context, caching, memory or permissions.

# 2026-08-07 sweep

Prior sweeps had swept arXiv up to **2608.04828**; the listing now runs to **2608.063xx**,
so the 2608.049xx–063xx band was unswept and three of the five below sit inside it.

## Resume Means Resume: a machine-checked conformance contract for checkpoint / interrupt / resume

[arXiv:2608.03836](https://arxiv.org/abs/2608.03836) — v1 2026-08-04, v2 2026-08-06. Sajjad Khan.

Defines a **resume contract** of six properties over a workflow-persistence API — prefix
continuation, effect exactly-once, fork determinism, checkpoint validity, consume-once, recovery
determinism — checks a reference semantics exhaustively in TLA+ (7.4M states), and then measures
five deployed agent-workflow frameworks at pinned releases with a deterministic LLM-free harness.
The measurements are specific rather than rhetorical: LangGraph 1.2.9 durably records a second
resume value and never consults it, persists schema-invalid state without complaint, and
re-executes durably-recorded work after a real `SIGKILL`; CrewAI 1.15.2 re-executes completed
effect-bearing methods against its own documented claim; **no two frameworks share a conformance
profile**. The sharpest result is a concurrency one: **consume-once holds sequentially and fails
under concurrency** — *k* processes resuming one parked interrupt fire the gated effect *k* times,
saturation 1.0 in 36 of 40 cells, and the failure crosses hosts.

This is the session-resumption paper the 08-02 and 08-05 sweeps recorded as *absent* for three
consecutive windows. It lands where inber's SQLite session store, `server/session_forking.go` and
concurrent `spawn_agent` intersect: the k-processes-one-parked-interrupt cell is inber's shape the
moment two subagents can resume the same session, and inber's resume path is exactly where a
`CapLease` lease ([2608.01710](https://arxiv.org/abs/2608.01710)) and a `Provenact` policy verdict
([2608.02764](https://arxiv.org/abs/2608.02764)) are both re-read.

- **What inber should consider:** take the six properties as a **checklist to test against, not a
  framework to adopt**. `fork determinism` and `prefix continuation` are already implicitly claimed
  by `server/session_forking.go:57` (the child inherits messages *and* turn counter so its BP3 lands
  on the parent's cached boundary) and nothing asserts either. `effect exactly-once` is the one with
  a live blast radius, because a resumed inber session replays tool calls and inber's tools are not
  idempotent.

## Towards a Risk Assessment of Malicious Skill Files in Coding Agents

[arXiv:2608.05223](https://arxiv.org/abs/2608.05223) — 2026-08-05. Yang, Fu, Tantithamthavorn,
Arora, Chua (Monash). Code/data: `github.com/awsm-research/AgentJailbreak`.

Transforms 471 real shell commands into benign-appearing SKILL files using six LLMs across four
families, releasing a **2,826-skill benchmark mapped to 11 MITRE ATT&CK tactics**, then
characterizes two enterprise coding agents over **5,629 completed runs** with a three-judge panel
validated against a blind human gold standard (κ = 0.85). **Gemini CLI is exploited in 95.5–96.1%
of runs and Qwen Code in 71.6–74.0%**, near-invariant to which model wrote the skill, and the agent
**explicitly recognizes the safety problem in 1.99% of runs**.

skill-store (`:8301`) ingests SKILL.md folders from GitHub repos, and the only thing standing
between an ingested skill body and a shell on this host is `guard.CheckTool(name, input)` on the
emitted command. This paper measures that exact configuration on a comparable agent at ~95%. It is
a **different** finding from [2607.12340](https://arxiv.org/abs/2607.12340) (hallucinated skill
*names*), already on file: that one is about the registry, this one is about the body.

- **What inber should consider:** the finding compounds with a defect already filed — the Assist
  denylist `isDangerous` (`guard/guard.go:328-334`) names four tools, so everything unclassified is
  allowed without approval. A malicious skill body does not need to name a dangerous tool; it needs
  to name one of the unclassified ones. The paper's 1.99% recognition rate is the argument against
  any mitigation that relies on the model noticing.

## EA-Graph: artifact-anchored verification memory under upstream drift

[arXiv:2608.04278](https://arxiv.org/abs/2608.04278) — 2026-08-04. Hsu, Chi, Everett.

Names a failure that applies to every memory store built out of prose: **a note preserves the
conclusion without the program state that supported it**, so after an upstream change the repo still
builds while the earlier verification claim is silently invalid. EA-Graph represents artifacts at
sub-path granularity, resolves aliases to leaf definitions, anchors each claim to the content used
to establish it, keeps **evidence strength separate from freshness** as two fields rather than one
score, and — the load-bearing choice — marks a claim **unprovable rather than guessed** when the
replacement content is unavailable. Preregistered over 42 sessions, seven worlds, three memory
conditions and two model tiers: artifact-anchored memory beat prose notes and no-memory in all seven
Haiku worlds (paired Wilcoxon p = 0.0156 each) and no session fabricated withheld content. The
authors state plainly that the Sonnet round hit control ceilings and its preregistered contrasts
were non-significant.

memory-store's rows are the prose-note condition this is measured against, and the
unprovable-not-guessed rule is the same rule inber already shipped one instance of when it stopped
summarization from laundering a failed tool call into a clean one
(`conversation/summarize_preserves_is_error_test.go`, `efafc69`).

- **What inber should consider:** splitting importance into **strength** and **freshness** is a
  schema change to the memory row, not new machinery — and it is directly relevant to a live inber
  problem rather than a speculative one, because the single blended `importance` score with its
  `+0.2` recency bonus and `importance*1.01` read-bump (`memory-store/access.go:8-13`) is precisely
  what makes the BP2 system prefix reorder itself between turns (goose.md, "held back", 2026-08-06).
  Separating the two fields is one of the candidate fixes for that open todo, arriving here with an
  independent argument for it.

## Comparative Approaches to Agent Retrieval over Large Skill Libraries

[arXiv:2608.06196](https://arxiv.org/abs/2608.06196) — 2026-08-06. Kolluru, Sportsman.

A clean negative over 690 skills and 117 realistic non-echoing queries: a hybrid lexical+dense
ranker puts the correct skill in the top five **73.5% ± 8.0** of the time, while a typed knowledge
graph encoding prerequisites, data flow and ordering is **significantly worse at matched token
budget (−11.2 points, p = 0.0007)**. The mechanism is a *pre-filter topology bound* — the graph's
candidate edges are drawn from the same embedding neighbourhood the ranker already searched, so
**98.6% of typed edges connect skills the ranker had already surfaced together**, and 73% of the
queries the ranker misses are unreachable through the graph at all. It also drops a methodology
result worth more than the headline: evaluating on **author-written** queries overstates hit@5 by up
to **44 points**, which would have hidden the entire finding.

- **What inber should consider:** this argues against the obvious shape of the unbuilt Phase 1
  resolver — added structure over a strong ranker buys nothing when the structure is *derived from
  the same embeddings*, which is the cheap way anyone would build it. It also corroborates codex's
  bet from 2026-07-17 (lexical-before-embeddings for skill routing). The methodology half binds
  immediately and regardless: **any skill-store recall number inber measures against queries inber
  wrote itself is worth up to 44 points less than it reads.**

## DCAS: decoupling CLI agent scaffolding, and the two senses of "planning"

[arXiv:2608.06113](https://arxiv.org/abs/2608.06113) — 2026-08-06. Thangarajah, Chen, Hassan
(Queen's / Concordia).

Shows the open coding-agent ecosystem has converged on a single training environment — trajectories
are collected almost exclusively under OpenHands — and that models fine-tuned on that data
**degrade substantially under any non-training scaffold, while untrained base models show no such
divergence**. The gap is therefore fine-tuning-induced and tied to scaffold convention, not to model
capability. The paper argues the load-bearing scaffold-specific behaviour is *planning structure*
and splits it into **explicit planning** (a pre-execution plan as a first-class artifact) and
**implicit planning** (structural conventions shaping the whole agent loop), showing the two are
empirically separable in training data. DCAS itself is a backend-substitution interception layer
routing API traffic between any CLI scaffold and any backend model without modifying the scaffold.

- **What inber should consider:** llm-bridge **is** DCAS's interception layer — it already routes
  between many CLI harnesses and many backend models — so the cross-scaffold evaluation the paper
  needs is something this host can run without building anything. The explicit/implicit split is a
  direct question for the engine turn loop: inber's `task_plan` tool makes a plan an artifact, while
  the sideband `then`-chain is implicit convention, and the paper says which of the two a fine-tuned
  model is sensitive to is measurable rather than a matter of taste.

## Checked and rejected — 2026-08-07

- **Prompt caching / KV reuse: nothing adoptable in window.** The in-window hits
  ([2608.01657](https://arxiv.org/abs/2608.01657) multi-tenant prefix-cache admission,
  [2608.01655](https://arxiv.org/abs/2608.01655) PrefixPlace,
  [2608.01126](https://arxiv.org/abs/2608.01126) spatial prefix caching for wireless edge) are all
  **serving-side**, which this corpus has repeatedly ruled unreachable from an API-side harness.
- **[arXiv:2608.06057](https://arxiv.org/abs/2608.06057) When History Lies** — structurally-valid
  but stale history flips **32.1%** of correct tool decisions. Strong effect, but measured on
  Qwen3-1.7B and the proposed fix is distillation: the same scope objection that sank 2607.26117.
- **[arXiv:2608.05778](https://arxiv.org/abs/2608.05778) When Do Prompt-Side Agent Playbooks
  Transfer?** — frozen-playbook transfer is a conditional cold-start rather than reuse-by-default;
  a global Holm correction retains **1 of 135** route-level effects. Honest, and too thin to act on.
- **[arXiv:2608.05604](https://arxiv.org/abs/2608.05604) SkillZip** — contract-preserving graph
  compression, 3.46× at 99.2% dependency preservation. Held because it sits in direct tension with
  [2607.03048](https://arxiv.org/abs/2607.03048)'s finding that no skill-representation optimisation
  breaks even at real prices. Worth resolving deliberately rather than filing.
- **[arXiv:2608.03169](https://arxiv.org/abs/2608.03169)** — prespecified equivalence study finding
  reasoning effort does not change unauthorized tool use. **Zero violations in 840 trajectories** is
  a floor effect; the study cannot discriminate.
- Also verified real and passed over: 2608.05810 *When Self-Evolution Backfires*, 2608.05791
  *TIPEX*, 2608.06301 *HarnessOpt-Bench*, 2608.05013 *OneDayAgent*, 2608.05446 *EvoHarness-RL*.

## Blogs: still nothing — and the Anthropic index timestamp is a false positive

Anthropic engineering's index surfaces a **2026-07-21** date that is a `siteSettings._updatedAt`
CMS field, not a post date; the newest actual article remains the 2026-04-23 item prior sweeps
recorded. Written down so the next sweep does not chase it. HuggingFace daily papers surfaced no
agent-harness item not already in the arXiv sweep — it did independently feature 2608.03836,
corroborating the first pick. DeepMind, Meta AI and Microsoft Research published nothing in window.

## 2026-08-08 sweep — three papers, and the middle one is a measurement inber can run on itself

### [arXiv:2608.00101](https://arxiv.org/abs/2608.00101) — Agentic Coding in the Wild (2026-07-30, Microsoft)

Production characterization of GitHub Copilot: 3.2M users, 13M sessions, 761M LLM calls, 95T tokens
sampled over June 2026. The number that matters here: **KV cache hit rate averages 90% *within* a
turn but falls to 55% *across* turn boundaries**, and is "drastically invalidated" by a model switch
or a context compaction. User idle at turn boundaries runs to minutes while agent turnaround is
fast, so the gap that kills the prefix is human think-time, not agent work.

**What inber should consider:** inber already records the ground truth and has never compared it to
its own prediction — `agent/agent_run.go:269-270` captures real `CacheCreationTokens` /
`CacheReadTokens` per call, and `engine/prompt_blueprint.go` predicts BP1/BP2/BP3 behaviour. Report
observed cache-read ratio split by within-turn vs first-call-of-turn and diff it against the
blueprint. This is the measurement the whole 2026-08-02/06/07 cache-stability thread has been
arguing about without data. Adjacent and cheap: no breakpoint in `engine/` or `agent/` requests a
1-hour cache TTL, so every one rides the 5-minute default that the paper's minutes-long boundary
idle will routinely outlive.

### [arXiv:2608.01507](https://arxiv.org/abs/2608.01507) — Deep Agentic Search for Repository-Level Code QA (2026-08-02)

Semantic search over a prebuilt repo index beat deep agentic search (planner delegates to a
context-isolated subagent that returns a condensed result — the Claude Code / Codex pattern) on
SWE-QA: **65.2% vs 46.2% correct, at less than half the cost per correct answer**. The hand-coded
failure taxonomy is the useful part: delegation did not remove failures, it *added a class* —
**41.8% of the agentic arm's failures occurred at the planner↔subagent hand-off**, and they were
typically silent, ending in a fluent, confident, wrong answer.

**What inber should consider:** `agent/registry/spawn_tool.go:92` sells spawn as "Delegate a task to
another agent. Always async — returns immediately." Async makes the measured failure mode worse, not
better: the parent has already moved on when the condensed result lands, so an empty-handed hand-off
is indistinguishable from a thorough one. Instrument before redesigning — have the spawn result
carry enough provenance (what the subagent actually read, whether it found nothing) that the parent
can detect a hand-off that returned nothing rather than confabulating over it.

### [arXiv:2608.06370](https://arxiv.org/abs/2608.06370) — The Bitter Lesson of Tool Calling (2026-08-06)

Programmatic tool calling — tools exposed as typed Python stubs the model invokes by writing code,
executed inside one agent turn — against native JSON tool calling on BFCL v4 across 14 models. PTC
matches or beats JSON in 11/14 models, 13/14 under parallel fan-out, and **stays stable under
context-rot conditions where the JSON baseline degrades 2.3%**. The authors argue the gain tracks
raw model code capability, so it compounds with model upgrades rather than being a prompt trick.

**What inber should consider:** inber's tool surface is JSON-only by construction —
`tools/interface.go:24` has every tool return `anthropic.ToolInputSchemaParam` — so an N-call fan-out
costs N schema blocks plus N `tool_result` blocks in permanent history, which is exactly the
material the paper's context-rot arm degrades on. Worth a prototype that renders the existing
`tools.Registry` as typed stubs behind one code-exec tool and measures the collapse against
`engine/prompt_blueprint.go`'s token accounting. Note the standing tension: this is a *second*
execution path, and inber's recurring finding is that its second path is where invariants go missing.

### Leads recorded, not read past the abstract

2608.01326 *Context Compaction Theory*, 2608.01347 *Prompt-Induced Waste in Coding Agents*,
2608.02670 *Permission Denied: Policy-Graded Evaluation in Hardened Environments*, 2607.29658 STAIR,
2608.05886 CodeGrep, 2608.02650 HyperAgent, 2608.04843 MemoryCPT. Out of window and passed over:
2607.06906 *The Harness Effect* (2026-07-08, one day early). The Anthropic engineering blog
published nothing in window, consistent with the 2026-08-07 note above.

# 2026-08-09 sweep

**The paper channel is saturated, and that is the headline.** A fresh multi-band arXiv sweep
(caching · coding-agent harnesses · compaction · memory/orchestration · tool schemas) over
2026-07-10 → 2026-08-09 returned fourteen candidates that cleared the date and mechanism bar.
**Thirteen were already documented here, in `2026-07-harness-research.md`, or in
`docs/cache-optimization.md`** — Ledger 2608.00808, CAPC 2607.15516, Copilot traces 2608.00101,
ARC 2607.25066, PTC 2608.06370, When History Lies 2608.06057, coordination-mode 2607.27877,
ACM 2607.23809, filesystem memory 2607.26637, Skill-Use 2608.04828, online KV compaction
2608.00902, and the two leads below. One was new, and it is not inber's. Recording the ratio so
the next sweep can decide whether this channel still earns a full pass, or should drop to
fortnightly and spend the budget on upstream code.

## Two leads from 2026-08-07 promoted out of "not read past the abstract"

The 2026-08-07 entry parked both as ids with no summary. Both are now read; neither changes a
decision, and both are recorded so they are not parked a third time.

**[arXiv:2608.01326](https://arxiv.org/abs/2608.01326) — Context Compaction Theory (2026-08-02).**
Tirmazi, Markelon, Bishop, Mitzenmacher. First formalization: a Context *Selection* Game (retain a
subset) and a Context *Generation* Game (emit a bounded summary), with a proof that the minimum
compaction budget to answer a query set within a target error equals the **one-way communication
complexity** of the induced problem. The useful part is the separation theorem — generation can
require strictly less budget than selection for some query sets, so trimming and summarizing are
provably *not* interchangeable. Includes a case study benchmarking Anthropic's context-compaction
endpoint against the theoretical optimum.

**What inber should consider:** this is the theory under a choice inber has already made by
default. `conversation.SummarizeConversation` (`engine/lifecycle.go:95`) is generation-only; there
is no selection policy to pick between. The paper does not say generation is wrong — it says the
right primitive is query-class-dependent, which means the honest inber question is whether it knows
its query classes well enough for a second policy to pay for itself. Nothing to change today; worth
citing the moment someone proposes "just drop old messages" as a cheaper compaction, because that
proposal is exactly the selection game and the separation theorem is the reason it can be worse.

**[arXiv:2608.01347](https://arxiv.org/abs/2608.01347) — Same Task, Different Work: Prompt-Induced
Waste in Coding Agents (2026-08-02).** Preregistered, 4,644 valid runs, 24 deterministic tasks, 7
reasoning models, 2 real harnesses. "Consider multiple approaches" phrasing costs 2.4–7.4× more
reasoning tokens and ~3 discarded branches per run with **zero** success improvement; "maximum
certainty" phrasing costs 18× the clean-run median, 2.5× tool calls and 3× wall-clock, again with
no success gradient. Harness design amplifies the effect 5×–30×.

**What inber should consider — checked, and inber is clean.** A case-insensitive scan of
`session/`, `engine/`, `agent/` and `prompts/` for the paper's intensifier families ("consider
multiple/several/different approaches", "maximum certainty", "be absolutely certain/thorough",
"exhaustively", "explore all/every", "think hard/deeply") returns **zero** matches outside tests.
Recorded as a negative result so the next sweep does not re-run it. The standing rule this implies
is cheap and worth keeping: an intensifier added to a system prompt is a cost change, and should be
justified by a measured success delta rather than by reading well.

## [arXiv:2608.00997](https://arxiv.org/abs/2608.00997) — Registry descriptions go stale unevenly (2026-08-02)

The one genuinely new find. 120 observations over 88.6 days across 19,099 MCP servers. Only 8.6% of
servers ever rewrote a description; the top 5% produce 61% of all change events; 11.9% of
descriptors change within 30 days against 35.8% under a naive uniform model. The negative result is
the sharp one: **re-auditing by prior drift catches only ~20% of changers at a 5% budget**, i.e. a
"re-check the churny ones" heuristic performs about as well as picking at random, because churn
does not predict itself at the server level. The authors' recommendation is content-binding
verification — store a hash of the description and revalidate when the hash changes — plus a
periodic full-catalog audit, rather than a prioritization heuristic.

**What inber should consider: nothing — and that is the finding.** inber's own tool descriptions are
compiled in (`tools/root.go`), so they cannot drift between reads, and its MCP client still has no
caller outside `tools/mcp/` (re-verified 2026-08-09), so inber consumes no third-party descriptor it
would need to re-audit. The actionable half belongs to **tool-store** (`:8302`), which is the
canonical registry that does hold third-party MCP descriptors, and the concrete advice there is to
key revalidation on a stored description hash and to *not* build the drift-prioritized re-audit
heuristic the paper measured as ineffective. Filed here rather than as an inber todo because it is
another repo's defect surface and this job files against inber; whoever next touches tool-store's
ingest should read it. No inber change.

## 2026-08-10 sweep — nothing new in window, and two KV-cache papers ruled out for a structural reason

No paper dated inside the 7-day window turned up on any of the searched channels, which is the
second consecutive empty week and supports the 2026-08-09 note's suggestion that this channel drop
to fortnightly. Two older-but-uncovered papers surfaced and are recorded here **with the reason they
cannot apply**, because both look directly on-topic for inber's standing cache-prefix problem and
the next sweep will otherwise find them again and read them again.

- **[arXiv:2606.01065](https://arxiv.org/abs/2606.01065) — Leyline: KV Cache Directives for Agentic
  Inference (2026-05-31).** Ma, Eitzinger, Koestler. Argues that agentic conversations evolve by
  *editing* — retried tool calls, dropped stale outputs, pivoted trajectories — which breaks the
  append-only assumption prefix caching is built on, and that harnesses therefore re-prefill on
  every edit. Introduces a serving-side directive that splices a span out of the cache in place,
  restoring attention math with a closed-form RoPE rotation: +11.2 pp replay cache-hit, up to 241 ms
  saved, and +14.3 pp solve rate on debug-gym from a ten-line truncation rule routed through it.
- **[arXiv:2607.21604](https://arxiv.org/abs/2607.21604) — AgentKVShift: Efficient KV Cache Reuse
  for Agentic Memory Systems.** Pandey et al. Per-memory-unit KV residual correction: the reuse
  residual decomposes into a shared memory-level offset plus token-wise noise, so a probe set
  estimates the offset and corrects every reused token rather than only the recomputed ones. Near
  full-recompute quality refreshing 10–30% of the cache; 2–3.5× prefill speedup.

**What inber should consider: nothing, and the reason generalizes.** Both papers act *below* the
prompt — they are serving-stack mechanisms that require owning the KV cache. inber reaches its
models through the Anthropic Messages API and the OpenAI-compatible path, where the only cache
control it has is `cache_control` breakpoint placement on a prefix it must keep byte-identical.
There is no interface through which inber could issue a splice directive or a residual correction,
so neither result is actionable at any effort level. Recorded as a **standing filter**: a
KV-cache-mechanism paper is only actionable here if inber ever serves a model itself. Leyline's
*framing* is still the one worth keeping — that a conversation which edits its own history is a
different cache workload from a chat — and that framing is already what the 2026-08-07 prefix work
and the open BP2-prefix-instability todo are about, arrived at independently.

## 2026-08-11 sweep — four papers from the Aug 7–10 band, and one of them names a field inber's memory schema is one key short of

Dedupe base for this sweep: 221 distinct arXiv ids already cited across `docs/papers/*.md` and
`docs/comparisons/*.md`. The highest `2608.*` on file was **2608.06370**, so the unswept band was
**2608.065xx → 2608.099xx**. All four keepers come from it. Everything below was read from its
`arxiv.org/abs/` page — title, authors, v1 date, category, abstract — and **no PDFs were read, so
every number here is abstract-grade.**

### 1. [arXiv:2608.08654](https://arxiv.org/abs/2608.08654) — the scaffolding matters more than the interface (2026-08-09)

Alier Forment, Casañ Guerrero, García-Peñalvo, Pereira. cs.AI. Holds one task family (git repository
operations) fixed and crosses **7 agent scaffoldings × 5 models × {MCP, CLI}**, measuring dollar
cost. The interface turns out to be the wrong variable: **scaffolding drove up to 139× cost
variation** on smaller models, while the MCP:CLI ratio ran **0.43× to 29× with no stable
direction**. Two scaffoldings finished the task with no MCP support at all, 5–28× cheaper. MCP runs
burned **12.9% of spend on failed runs against CLI's 2.2%**. The methodological result is the
sharpest: **agents frequently ignored the interface they were assigned**, so any MCP-vs-CLI
comparison that does not verify which path was actually taken is measuring noise. Harness and
measurement code released GPL-3.0.

**What inber should consider:** a design input for `tool-store`'s `POST /provision`, not a defect —
re-verified this pass that `tools/mcp` still has no importer outside `tools/mcp/`, the same absence
`agentic-design-patterns.md` recorded on 2026-08-10. It argues that a third-party capability
reachable as a CLI through `shell_commands` should be preferred over the same capability mounted as
an MCP server unless MCP is measured to win *on inber's scaffolding*, and it gives a second reason
to have stayed off MCP beyond the browser-MCP OOM. The transferable discipline is negative: **if
inber ever A/Bs MCP against CLI, log which interface the model actually used per call**, because
defection from the assigned interface is the paper's own headline. Scope: one task family, and the
outcome measured is cost, not success.

### 2. [arXiv:2608.06953](https://arxiv.org/abs/2608.06953) — explicit, not longer: stance survives compression when it is a labelled field (2026-08-07)

Alex Kwon. cs.CL. Asks what makes a claim's epistemic standing — "unconfirmed", "assumed",
"reported by X" — survive being written into agent memory, given that compressors are built to drop
qualifiers. Matched pairs: identical claim, identical stance, differing only in **where the stance
sits** — a labelled field versus a bracketed aside — compressed by one model under one budget among
identical filler notes, scored by a blind reader. Writing stance as a **labelled field** raises
retention ~15 points on two models (37→2 and 30→8 claims lost, permutation p=0.00005), with a
**pre-registered replication** on Haiku giving +15.6 (38→1). The ablation is the load-bearing part:
**labels help on both models (+9.7, +12.8); length helps on neither.** The paper prints nine
withdrawn claims, three of them former title claims.

**What inber should consider:** this is the general form of a fix inber already shipped narrowly in
`efafc69`, which rendered `is_error` into the summarizer's view so a failed tool call stops reading
as a result. The same argument applies to memory. ⚠️ **Correcting the scan that surfaced this
paper:** it reported `memory_save` as having "`content`, `tags` and `importance`" at
`memory/tools.go:100-103`. Measured — the schema is `:100-105` and carries a **fourth** key the scan
stopped one line short of: **`source`** (`:104`, "'user', 'agent', 'system'"). That changes the
recommendation from "add the first label" to something cheaper and better founded: **the labelled-
field mechanism already exists in this schema and is already rendered as a label** — `memory_search`
prints `source: %s` per hit at `:80` — so adding a `stance`/`confidence` key beside it is one more
entry in an existing `props(...)` map plus one more field in the same `Sprintf`. What `source` does
*not* carry is the thing the paper measures: it says **who** said it, never **how confident** they
were, so "the build passes (unverified)" still reaches the store as prose with the qualifier inside
the sentence, which is exactly the position the paper shows gets dropped. Two further notes: the
paper explicitly finds that **padding the text does not work**, which kills the obvious alternative
of instructing the model to write longer memories; and a labelled field composes with 2607.12161 on
file, being a few appended bytes rather than a prefix rewrite. Scope: 60 claims, single author,
small-n, and the authors concede the mechanism behind the label effect is model-dependent.

### 3. [arXiv:2608.07952](https://arxiv.org/abs/2608.07952) — persistent semantic entities, and contamination compounding along a spawn chain (2026-08-08)

Zhaohui Wang. cs.LG. Formalizes implicit state that survives sessions and crosses agent boundaries
via three ingredients — **name binding, event triggering, cross-boundary propagation** — measured
across **24 models from 11 families (1.5B–1T)**. Three results. **Name binding is necessary and
dominant: without it contamination is 0%**, with it 20–100% susceptibility across a 20-model panel.
**Persistence depends on contamination type, not scale** — preference contamination persists
undecayed (100% at t=10), persona-style injection decays 90%→10%. And **contamination compounds
1.9× along a four-stage agent pipeline, 40%→75%**, while context-isolated self-verification recovers
a median 36.5% with no oracle and **keyword detection produces systematic false positives**.

**What inber should consider:** the compounding result is the one aimed here, because
`agent/registry/spawn_tool.go` builds exactly such a chain and the Twin Agent entry on file
(2607.19595) proposes routing untrusted reads *through* a spawned agent — this paper says the chain
**amplifies** rather than dilutes, which is an argument against that shape, not for it. The
name-binding result is the CLAUDE.md rule "join on ids, never on names" arriving from an unexpected
direction: contamination needs a **name** to bind to. Worth saying plainly that inber is on the
right side of this one already — `memory_search` prints the row **id** first (`memory/tools.go:80`,
`"%d. [%s] ..."`) and carries the source alongside, which is the id-primary shape the rule asks for.
The adoptable item is the stop-doing: **do not build keyword-based contamination detection**;
context-isolated self-verification is the mechanism that measured. Scope: single author, controlled
injection rather than a live harness. It overlaps the memory-poisoning thread already on file
(SPORE 2607.23444, self-state 2607.17986, MemSecBench 2607.27080) — the genuinely new increments are
the name-binding necessity result and the 1.9× four-stage factor.

### 4. [arXiv:2608.06984](https://arxiv.org/abs/2608.06984) — HarnessSafe: score where the chain stopped, not whether it succeeded (2026-08-07)

Zhang, Wang, Fei, Li, Liang, Xiang, Gu, He. cs.CR. **328 executable cases across seven
persistent-carrier families** — memory, skills, tools, shared artifacts — against most mainstream
harnesses. The part that is not benchmark boilerplate is the **Persistent-Risk Lifecycle**: each
case traces attacker influence from entry, through persistence and boundary crossing, to a **later
benign trigger** and an observable violation, and scoring is **stage-resolved** — how far the chain
got and where it stopped — rather than a scalar attack-success rate. Findings: containment is
carrier-specific and depends jointly on harness **and** model backend, and **ASR cannot distinguish
distinct lifecycle progression patterns**.

**What inber should consider:** the measurement shape, not the benchmark. inber runs all four
carrier families in production — `memory.db`, skill-store SKILL.md text, tool-store tool
definitions, session artifacts — with no per-carrier containment measurement. Scoring inber's
boundary per carrier **and per lifecycle stage** (entry / persisted / crossed a boundary /
triggered / violated) is a natural extension of the 39-boundary `harness-control-matrix.md`, and it
replaces "did the attack succeed" with "where did it stop", which is the same upgrade that matrix
already made over a pass/fail table. The harness-model coupling finding also warns that a
containment result measured on Opus does not transfer to the compact executor tier that 2607.03048
calls the dominant quality lever. **Included with a caveat:** this is a benchmark paper, the class
this doc set usually rejects, and it shares a slot with the isolation taxonomy (2607.12406) on file.
It earns its place on the carrier decomposition and the stage-resolved scoring, not on any number.

### Checked and rejected — 2026-08-11

- **2608.02113 MemArbiter** (decision-time memory salience, five Memory Banks, +20.9/+25.4 pp on
  ALFWorld at 500/750-token budgets) and **2608.06745 MemPrism** (task-conditioned relational memory
  views) — both **embodied, not coding**, and both occupy the slot already held by 2607.08716 ("when
  to inject beats what is stored") and 2607.17545 (budget-dependent operator choice).
- **2608.06663 The Horizon Gap** — survey of 1,547 papers separating long-horizon (task) from
  long-context (model) from long-term memory (system). Useful vocabulary, no measurement.
- **2608.08282 Stateful CARS** — exact constrained decoding with cross-history invalidity
  certificates. Needs logit-level control inber structurally cannot reach, **and the paper's own
  matched comparison is negative** (0.942 against plain CARS; the Qwen comparison is null).
- **2608.07855 CommitKV** — serving-side KV compression. Falls under the standing filter recorded in
  the 2026-08-10 entry: a KV-cache-mechanism paper is only actionable here if inber ever serves a
  model itself.
- **2608.08793, 2608.09253 SkillSentry, 2608.09096 Evo-Bench, 2608.02276 Harness-R1, 2608.07545
  DarwinX, 2608.09885 SHE** — harness-evolution and skill-observability. The evolution cluster stays
  governed by the 2607.12227 critique on file (unmatched compute budgets, evaluation on the tuning
  tasks).

### Blogs: empty for the fourth consecutive sweep

Anthropic engineering's index shows nothing since **2026-04-23**; the featured *How we contain
Claude across products* is **2026-05-25**, outside window. HuggingFace surfaced only undated
evergreen posts. DeepMind and OpenAI: nothing on harness design, context, caching or memory in
window. Fourth empty sweep running — the 2026-08-09 suggestion to drop this channel to fortnightly
now has four data points behind it and should just be done.

---

## Harness-watch — 2026-08-12

Swept the 2608.09xxx–2608.11xxx band (everything past the 2026-08-11 sweep's high-water mark of
2608.09885) plus targeted searches on caching, compaction, tool contracts and memory. ~20
candidates, 2 clear. Both were verified against inber's live state, and the weaker of the two turned
up a hard defect independent of the paper — which is the argument for keeping the read-against-code
step even when a candidate looks thin.

### Why Does CLAUDE.md Keep Growing? Catastrophic Remembering in Agentic Coding

[arXiv:2608.11095](https://arxiv.org/abs/2608.11095) — Kushal Chakrabarti, 2026-08-11.

Names the inverse of catastrophic forgetting: agentic instruction files accumulate because
*deletion risks a correctness regression*, so nobody deletes. Measures 247,694 instruction lifetimes
across 1,867 repos — prompts more than triple over their lifetime (+226%), gaining +4.9 net
instructions per commit, and the deletion hazard *falls* with instruction age (log-hazard
−0.032/commit), i.e. old instructions become permanent. The useful half is the mitigation, not the
diagnosis: on an inverted-IFEval setup with known-optimal prompts, comments encoding latent
reasoning removed **99.3% of excess instructions** (+211.3% excess → +1.4%), and carried +23.1% on
WildIFEval.

**What inber should consider:** inber's CLAUDE.md analogue is the `identity` always-load memory,
assembled at `agent/registry/config.go:100-117` by concatenating four free-prose fields (`Soul`,
`Principles`, `Values`, `UserContext`) with no size accounting at all. That string lands in a slot
with two exemptions — `memory-store/builder.go:202` skips truncation when `m.AlwaysLoad`, and
`:207-210` appends an always-load row **even when it blows the token budget** — and it sits at the
front of the cached system prefix `engine/turn_prompt.go:130` builds. So there is no cap, no
truncation and no measurement on the one block that can only grow. The bypass *mechanism* is already
on file at `agentic-design-patterns.md:2513-2518`; what is new here is the growth law and the
mitigation.

**Counter-measurement, reported because it changes the priority — do not file this as urgent.** The
growth is **latent, not realized**: in the live agent-store every agent's `principle` (4,185 chars),
`value` (1,029) and `user` (597) block is byte-identical across all 11 agents and untouched since
**2026-04-05**. Totals run 6,639–9,376 chars/agent (~1,659–2,344 tokens), of which 5,811 chars is
that shared frozen boilerplate — 88% of dagda's identity, 62% of claxon's. The ask is therefore a
cap plus a growth monitor *before* someone starts editing those fields, not a rewrite; the paper's
own hazard curve is the argument for putting it in early. The 4-month freeze is itself weak evidence
for the thesis — nothing has been removed either.

### From Faulty Memories to Corrected Actions: Dependency-Guided Rollback Repair for Memory-Augmented Agents

[arXiv:2608.10502](https://arxiv.org/abs/2608.10502) — Yu, Wang, Zhang, Duan, Zheng, Wu, Shi, Cai,
2026-08-11.

Persistent memory makes errors durable — a poisoned, stale or misattributed row alters reasoning,
tool use, answers, *and subsequent memory writes*. The method builds a typed memory→action graph and
selectively replays only the affected computation: 85.3% recovery vs 77.3% for the best competing
method (150 cases, 3 tool-use domains, 4 failure types).

**The paper is weak** — 150 synthetic cases, and the same author cluster shipped a second memory
paper (MAP-Graph, 2608.10509) the same day. On its own merits it belongs beside the memory-poisoning
thread already on file (SPORE 2607.23444, MemSecBench 2607.27080, 2608.07952). It earns a slot only
because reading it against the code exposed a verified dead seam.

**What inber should consider:** the memory→action edge the paper's graph is built from is **already
designed into inber and never written.** `TrackMemoryUsage(memoryID, sessionID, turnNumber,
usageType)` is on the `MemoryStore` interface inber re-exports (`memory/memory.go:11`; backing decl
`memory-store/memory.go:92`), and the `memory_usage` table exists with two indexes
(`memory-store/sessions.go:27-37`) — but the whole inber tree has **zero production call sites**,
only three test mocks (`agent/registry/registry_test_mocks.go:80`,
`conversation/summarize_failure_test.go:55`, `engine/volatile_context_test.go:44`). Confirmed live:
`select count(*) from memory_usage` returns **0** in both the 404-row inber store and the 35,764-row
openclaw store. Consequently `memory_forget` (`memory/tools.go:189-206`) only zeroes importance via
`Store.Forget` — it cannot know which turns consumed the memory, so a retracted memory leaves every
conclusion derived from it standing. The cheap, non-speculative action is to fill the existing seam
(call `TrackMemoryUsage` from the `BuildContext` consumer at `engine/turn_prompt.go:90-124`), not to
build the paper's replay engine. Filed as a todo: an interface method with an indexed table and no
caller is a lie in the schema regardless of what this paper says.

### Channel note — blogs, fifth consecutive empty sweep

Anthropic engineering's index still shows nothing after **2026-04-23**; the featured *How we contain
Claude across products* remains 2026-05-25, outside window either way. HuggingFace, DeepMind and
OpenAI: nothing on harness design, context, caching or memory in window. **The 2026-08-09 suggestion
to drop this channel to fortnightly now has five data points behind it and should just be done.**

### Rejected this sweep (all grep-clean, rejected on merit)

2608.10934 Ark (descriptive taxonomy, inber is past it) · 2608.11166 Agentic Configuration
Management (the store family already implements typed versioned config with id joins; adds
vocabulary, no measurement) · 2608.10319 Do Personalized Skills Help Coding Agents? (genuine null
result, but per-*developer* personalization; inber's memory is per-repo-workspace siloed) ·
2608.10509 MAP-Graph (slot already held by 2608.07952, CapLease, Provenact) · 2608.10450 EvoX
Genesis (whole-architecture replacement offered as one demo run, no controlled comparison) ·
2608.10743 Mitigating Context Interference (refiner trained into the RL loop; inber cannot reach
logits) · 2608.10906 GitSkills (dataset, no method) · 2608.08311 Ouroboros and 2608.10299
Co-Evolution (governed by the standing 2607.12227 critique) · 2608.10669 REDAgentBench (superseded
by HarnessSafe) · 2608.09799, 2608.09072, 2608.08950, 2608.08413, 2608.09068 (benchmark papers, the
class this doc set routinely rejects) · 2608.09278, 2608.08968, 2608.09290, 2608.09181, 2608.11110
(out of domain). Every caching hit surfaced — Leyline 2606.01065, IntentKV 2606.09916, KV Packet
2604.13226, Don't Break the Cache 2601.06007, CommitKV 2608.07855 — falls to the standing 2026-08-10
filter (KV-mechanism papers are non-actionable unless inber serves a model) or is out of window.

## 2026-08-13 sweep — five papers, and the strongest is a price list for a bullet this repo has carried unquantified since May

Two of the five sit in id-ranges earlier sweeps walked past (`2607.19214`, `2608.06503`) rather than
outside the window, so the id-range walk is leaving gaps and deserves a second pass. Every id below
was fetched and its numbers checked against the abstract; every one was greped against all five
`docs/papers/*.md` files and is absent.

### 1. [arXiv:2607.19214](https://arxiv.org/abs/2607.19214) — Keeping the Cache Warm Pays: Keepalive Economics for Agentic Workloads (Khailo, 2026-07-21, rev 07-24)

Agentic workloads destroy prompt-cache benefit in a way chat does not: the agent sends a request,
then runs a tool or blocks on approval for minutes, and by the follow-up the cached prefix has been
evicted, so it pays full prefill again. A client-side keepalive that replays the prefix on a timer
during the pause cuts post-pause request cost **by up to 12.5x** across Anthropic, OpenAI, Google and
DeepSeek. The strategy result is the useful part, because keepalive cost falls monotonically in the
ping interval: the optimum is the **largest interval safely under the TTL — about 4 minutes against
Anthropic's 5-minute TTL, not the 30-second convention** — and keepalive stops beating a plain
re-prefill past an idle of roughly **46 min for Anthropic, 36 min for OpenAI and DeepSeek**.

**Be precise about what is new here.** `docs/cache-optimization.md:280` has carried
"**Cache keepalive pings** (Aider-style): prevent 5-min expiration during idle" as a Future
Considerations bullet since May. The idea is not new to this repo; the **economics** are, and they
are what turn a bullet into something implementable — an unquantified "ping during idle" invites the
30-second convention the paper measures as strictly worse, and has no stopping rule at all.

What *is* confirmed absent from the code: `grep -rn -iE "keepalive|cache.?warm|refresh.*cache"
--include=*.go .` returns nothing on the cache path. And inber's exposure is maximal, because all
four breakpoints are 5-minute ephemeral with no TTL override —
`engine/turn_prompt.go:218` and `:224` (system/stable prefix), `agent/agent_run.go:36` (last tool
definition), `agent/agent.go:549` (history), each via `NewCacheControlEphemeralParam()`, which sets
`Type: "ephemeral"` and no `ttl` (`anthropic-sdk-go@v1.35.0/message.go:469-473`), so the provider
default of 5 minutes applies to all of them.

**What inber should consider:** implement the bullet at the paper's numbers, not the convention —
replay at ~4 min, and stop at the ~46-min break-even rather than pinging a dead session forever. Two
inber-specific notes before anyone builds it. (a) The server-mode idle gap *between* user turns is
the expensive case and it is unbounded, so the stopping rule is load-bearing, not a refinement;
`server/session_reaper.go` already knows which sessions are live and is the natural place to hang
the timer. (b) A keepalive is a real API request that inber will bill and, more importantly, will
route through `recordModelHealth` if it errors — see the 2026-08-13 entry in
`comparisons/agentic-design-patterns.md`, where an unclassified error marks a model unhealthy
host-wide. A keepalive ping must not be able to demote a model. Not filed as a defect: an absent
optimization is not a defect, and the existing bullet already owns the idea.

### 2. [arXiv:2608.06503](https://arxiv.org/abs/2608.06503) — Toward Reliable Context Compression for Long-Horizon Agents: An Empirical Study of Execution Instability (Min et al., 2026-08-06)

Recurrent context compression **weakens the influence of recent interactions**, showing up as more
blocked actions, repeated exploration, and run-to-run instability — a *behavioral* cost distinct
from the information-loss framing these docs already carry. TRACE evaluates individual compaction
events by running paired closed-loop continuations from the same environment state, then uses the
resulting summary preferences to optimize the natural-language compression prompt **with all models
frozen**. Reports gains on AppWorld in task performance, multi-run reliability and efficiency.

**The diagnosis does not transfer to inber, and the method does.** inber does not do uniform
recurrent compression: `conversation/summarize.go:18` states it "keeps the most recent turns in full
and replaces older ones with a summary", `:40` computes `keepFrom` from `cfg.KeepRecentTurns`, `:47`
slices `recentMessages` off that boundary, and the default is `KeepRecentTurns: 15`
(`conversation/summarize_config.go:36`). So inber already has, structurally, the recency protection
whose absence the paper measures. Any writeup claiming inber needs to "give recent turns weight"
would be wrong about this repo.

**What inber should consider:** take the evaluation harness, not the fix. The paired-continuation
experiment is buildable here because both halves already exist — the compaction prompt is
configurable (`conversation/summarize_config.go`) and the session fork path is real
(`server/session_forking.go`). At a genuine compaction boundary, fork twice from identical state,
one compacted and one not, and diff the next N actions. That gives inber a number for a question it
currently answers by assertion: whether `KeepRecentTurns: 15` is protecting enough, and whether
frozen-model prompt edits move it. Worth pairing with the standing thesis in
`comparisons/agentic-design-patterns.md:1580` that summarizer *wording* barely matters and *timing*
carries the saving — this paper is evidence that wording moves *stability* even if it does not move
tokens, which is a different axis and does not contradict it.

### 3. [arXiv:2608.10037](https://arxiv.org/abs/2608.10037) — DOCSCHISEL: Adaptive Tool Documentation Optimization for LLM Agents (Lu, Zhang, Chen, Peng — Fudan, 2026-08-10)

Which *information fields* a tool's documentation should carry is highly dependent on task domain,
LLM backbone and agent paradigm, so no fixed tool doc generalizes. DocsChisel analyzes a target
agent's **failed execution traces** to find documentation-related causes, then iteratively adds,
removes and refines fields per tool. Task success rate improves **95.89% over the original tool
documentation** and **75.15% on average over EasyTool and DRAFT**, at limited token and time cost.

**What inber should consider:** every inber tool description is a hardcoded `Description() string`
(`tools/interface.go:20-21`) — the fixed input the paper says leaves roughly 2x success on the
table — and the same holds for tool-store's canonical rows on `:8302`. inber has the missing
ingredient the method needs: failed traces, in `trace/` and `logs/`. A first step that decides
nothing architectural is a batch job that mines failed calls per **tool-store id** (never name — two
tools can share one) and proposes description edits against the owning row. One hard constraint to
carry into any such job: tool definitions hold inber's cache breakpoint
(`agent/agent_run.go:34-36`), so every description rewrite invalidates the entire tools prefix.
Edits must be **batched and applied between sessions**, never dripped — the same batching rule
`comparisons/agentic-design-patterns.md:1418-1420` already derives for stale-read rewrites.

### 4. [arXiv:2608.11888](https://arxiv.org/abs/2608.11888) — Agent Skills Can Be Harmful: An Empirical Study of Skill-Induced Failures (Dong et al., 2026-08-12)

Differential analysis of skill-guided vs. baseline runs over **307 skill-induced failures** (125
functional, 182 efficiency) across two benchmarks. The counterintuitive headline: functional
failures come mostly from **seemingly relevant skills** making the agent implement task elements
incorrectly or omit them — not from obviously incompatible ones. Efficiency regressions are **not**
explained by prompt length; within "Excessive Procedure" the two largest sources are excessive
verification (67 cases) and heavy implementation pipelines (30), i.e. a skill turning optional
validation into mandatory procedure.

**What inber should consider:** the nearest thing on file is `2608.09253` SkillSentry, and it was
*named and rejected* rather than covered — it sits in the 2026-08-11 rejection list at `:1386` under
the harness-evolution cluster. So skill *retrieval* aside, nothing here covers benign, well-matched
skills making things worse, and this is the first paper in the corpus that measures it. This
lands directly on bundle-store's `/resolve` (`:8307`), which selects skill-store ids by repo tags
plus task tags — selection by **relevance**, which is precisely the filter this paper shows is
insufficient. Two concrete moves: A/B a resolved bundle against a no-skill baseline per repo-tag set
and demote skills with a negative delta; and measure "excessive verification" as turn count and
tool-call count inflation rather than trusting token count, since the paper's own finding is that
token length does not explain the regression. This is also a caution for the standing instruction in
`~/CLAUDE.md` to query skill-store before any non-trivial task — the paper's result is that a
relevant-looking hit is not free.

### 5. [arXiv:2608.11977](https://arxiv.org/abs/2608.11977) — Retry, Switch, or Abstain? Strategy-Aware Tool-Use Policies via Controlled Error Injection (Chen et al., 2026-08-12)

BENCH2ROBUST converts failure-free tool-use benchmarks into stochastic environments with
scenario-controlled solvability, so an episode explicitly requires retrying, switching to an
alternative, or stopping once paths are exhausted. Across 7 models from 4 families, injected tool
failures produce a near-universal robustness gap. **Bayesian Tool Memory** — a structured runtime
recovery context, **no retraining** — buys up to **+16.8 points** on held-out retail tasks; BTM plus
RL reaches 40.8–45.5% under injection while preserving failure-free performance.

**What inber should consider:** BTM is the retraining-free half and it maps onto the memory store as
a durable per-tool prior keyed by tool-store id — "this tool has failed transiently N times recently,
so retry is warranted" versus "switch" versus "stop asking". inber already preserves the raw signal
through compaction (`conversation/prune_preserves_is_error_test.go`,
`summarize_preserves_is_error_test.go` pin `is_error` surviving both paths), so what is missing is
only the aggregation step, not the data. The paper's actual point is that **undifferentiated retry
is itself the failure mode**, so *abstain* has to be a first-class outcome rather than what happens
after retries run out — which is the same question `comparisons/agentic-design-patterns.md:3834-3837`
already parks: inber "cannot decide 'is this evidence about the model' until it decides where
retryability lives — client, turn loop, or a shared classifier both loops consult." Read them
together; do not build the prior before that fork is settled.

### Channel note — blogs, sixth consecutive empty sweep

anthropic.com/engineering surfaces nothing newer than the 2025 context-engineering post.
huggingface.co/blog in window has a model release and two security disclosures, none of it harness
research. DeepMind and Meta AI: nothing in domain. **The 2026-08-09 suggestion to drop this channel
to fortnightly now has six data points and should just be done** — it was already overdue at five.

### Rejected on merit this sweep (all grep-clean)

`2608.12282` VAKRA (8,000-API enterprise benchmark; the finding that failures concentrate at
language-mediated reasoning rather than invocation mechanics is a model-capability claim a harness
cannot act on) · `2608.11552` Trajectory-Adapted Uncertainty Quantification (requires resampling
whole trajectories; cost prohibitive at inber's usage) · `2608.07169` Agent Memory Distillation
(+27.2pp on AppWorld, but scoped to 4B–8B students with a larger teacher, and inber runs frontier
models — the three-way Workflow/Subtask/Function typing is mildly interesting for the memory-store
schema and nothing else) · `2608.10504` MEGA/Wisdom Graph (three-layer self-evolving architecture,
abstract carries no numbers) · `2608.10039` FlowScout, `2608.09316` MemeMind, `2608.10366`
DSAgentBench, `2608.10042` UserToolBench (workflow synthesis, out of domain, benchmark-only).

**Coverage caveat, stated rather than hidden:** export.arxiv.org rate-limited (429/503) on several
queries this run and the gap was backfilled through the cs.SE listing and targeted search, so
**cs.CL coverage is thinner this sweep than cs.SE**. The next sweep should re-walk cs.CL for the
Aug 6–13 band rather than assume it was covered.

# 2026-08-14 sweep

Four papers. Every id and date re-verified by hand against the arXiv Atom API over **https with
redirects followed** — `http://export.arxiv.org` answers `301` with an empty body, which reads
exactly like "no such paper" if the redirect is not followed. That is worth writing down: it is a
way to conclude a real paper is fabricated.

**Two candidates were dropped as already-covered**, both of which this sweep's search surfaced as
new: `2608.10319` (Do Personalized Skills Help Coding Agents?) is already reviewed and rejected at
`:1482-1483` — genuine null result, but per-*developer* personalization, and inber's memory is
per-repo-workspace siloed — and `2608.00267` (LoopsBench) is already rejected at `:779-780` as
benchmark-only. Neither should come back without a new argument. Evidence grade for all four below
is **abstract only**; the numbers are the authors' own claims.

## Tool architecture is a cost lever, and scratchpad tools are not

[arXiv:2608.11386](https://arxiv.org/abs/2608.11386) — "The Devil Is in the Interface: Evaluating
How Tool Architecture Shapes Coding Agent Behavior", published 2026-08-11. Xu, Saghir, Wu, Côté,
Wang, Lakkaraju, Pei, Zhang.

Holds the capability set fixed and varies only how tools are organized and exposed: six
architectures, three actor models, 11,700 trajectories on repository-level issue fixing. Structured
low-level interfaces improve run-to-run consistency over a bash-only baseline by up to **4.7×**;
natural-language search raises access to relevant files by **>11%**; Python CodeAct-style interfaces
reach similar task success with **41.6% fewer steps and 56.3% lower token usage**. The embedded
negative result is the one to act on first: lightweight text-based *cognitive scaffolding* tools —
scratchpads, record-your-reasoning tools — had **limited effect** on behaviour.

**What inber should consider:** the tool surface is a tuned artifact, not plumbing, and a 56.3%
token reduction is a direct bill reduction rather than a proxy — which matters given
`2607.12161`'s finding that token cuts and billed cost decorrelate at r = 0.15. Note the caveat
that follows from the same paper: a CodeAct-style single execution tool routes work *through* the
shell, so it moves load onto the one path `guard.CheckTool` classifies by name
(`guard/guard.go:319-334`) and would make the unclassified-name gap sharper, not milder. The
scratchpad result argues against funding a notes/thinking tool for inber on current evidence.

## ⚠️ Matched benchmark scores can hide the harness eating the command

[arXiv:2608.13547](https://arxiv.org/abs/2608.13547) — "QuoteBench: How Matched Scores Can Hide
Command-Path Failures", published 2026-08-13. Li, Zhang, Tresp, Yang.

About the layer inber *is*. Coding agents emit Bash through interfaces that serialize, wrap and
reparse model output, and the paper shows a matched benchmark score cannot separate a
command-*generation* error from damage injected after generation by the transport. Replaying an
identical model reply through **one added parser** drops success by **55.4–73.2 percentage points**
across eight configurations. Disclosing the boundary to the model recovers 30.4–60.7 points for six
configurations and zero-or-slightly-negative for two. The headline case: a matched gap of −3.6
points concealing −64.3 points of transport damage masked by +60.7 points of model compensation.
Deployment configuration even reorders model rankings. Evidence is 56 one-shot tasks from 14
incident-derived families with exact final-state validation — small-n, large effects.

**Checked against inber this sweep, and it is on the safe side of the line.** The shell tool passes
the model's string to `bash -c` unsplit (`tool-store/tools/shell.go:76`, reached through
`tools/tools.go:49`), which is the raw path the paper says to preserve, and nothing in `agent/`,
`engine/`, `server/` or `tools/` re-parses a model-authored command — every `strings.Fields` /
`strings.Split` there is over git output, config, session keys or display text.

**What inber should consider:** keep it that way deliberately rather than by luck, because the
failure is invisible to scores. The one added parser in the tool surface inber consumes is
`strings.Fields(in.Flags)` in tool-store's ripgrep (`tool-store/tools/grep.go:45,60`), which
shreds a quoted flag value containing spaces — that is a **tool-store** defect, not an inber one,
and is recorded here rather than filed against inber's queue. The second implication is for the
stored conformance matrix: comparing harnesses on scores without pinning the execution path can
reorder the ranking, so the matrix should record the command path it measured through.

## ⚠️ Write-conflict safety for parallel agents is currently bought by giving up the parallelism

[arXiv:2608.00947](https://arxiv.org/abs/2608.00947) — "Claim Plane: Reliability Gains and the
Limits of Selective Concurrency for Parallel Coding Agents", published 2026-08-02. Nikolaev.

Confirmatory study over 30 frozen CooperBench feature pairs (15 conflicting, 15 clean), three coder
seeds, four coordination arms, 360 executions, using deterministic pre-write admission over
versioned change intents. Static admission raised pair-pass from 23.3% to **50.0%** (+26.7pp, 95%
bootstrap CI 9.6–60.0) and integration success from 65.6% to 96.7% — but did it by **serializing
96.7% of executions, including 93.3% of the clean, non-conflicting ones**. The more selective
dynamic variant failed closed on undeclared scope in 46 of 90 executions and fell to 22.2%
pair-pass. The author states plainly that the results **do not establish useful wall-clock parallel
speedup**.

**What inber should consider:** this is directly on inber's spawn/forge path, where children can
share a checkout. The honest read is that the value of isolating parallel agents is *reliability*,
not throughput — so if forge worktree isolation is extended, do not promise or design around a
speedup. Note the failure mode of the selective variant: 45 of its 46 blocks hit files already
declared, i.e. scope-declaration undercoverage is the hard part, which is the same problem as
asking an agent to predeclare what it will touch.

## ⚠️ Telling an agent its budget barely changes what it spends

[arXiv:2608.05519](https://arxiv.org/abs/2608.05519) — "EcoAgent-Bench: Evaluating Economic
Decision-Making in Budget-Constrained LLM Agents", published 2026-08-06. Wu, Gong, Cheng, Zhao.

Every task prices its actions and states a budget, so choosing between local lookup, broad search, a
composite research tool, a stronger model tier and human escalation *is* the task. 304 tasks over
five families, seven agents in tool-API and workspace-CLI settings, four scripted oracle controls.
Tool-API agents reach **3.9–24.0% micro strict success and at most 7.3% "economic consistency"** —
a metric defined as the worse of upgrade-oriented and save-oriented accuracy, introduced precisely
because micro-averaging rewards a degenerate always-escalate policy. The finding that matters here:
a threshold-crossing budget sweep moved one frontier model's escalation rate from **0% to only 3%**.

**What inber should consider:** do not implement model-tier selection or escalate-to-premium routing
by stating a budget in the prompt and trusting the agent — measured near-zero responsiveness.
Enforce it in the harness, where inber already has the machinery: `guard.CheckLimits`
(`engine/engine.go:255-258`) and the cost recording at `engine/turn_postprocess.go:86-88`. This also
bears on codex's skill-declares-a-model-tier design recorded in `agentic-design-patterns.md` under
2026-08-14 §5 — codex made that delegation *advisory* prose aimed at the model, and this paper is
the reason to expect an advisory budget signal to be ignored. It is evidence for the enforcement
side of that trade, not against the feature.

**Coverage note:** the sweep re-walked cs.SE and the arXiv listing API; the previous sweep's caveat
that cs.CL was thin for the Aug 6–13 band was **not** cleared this run and still stands.

## ⚠️ Attenuating a policy down the delegation chain is the *worse* of the two defences — re-anchor to the original request instead

[arXiv:2608.07556](https://arxiv.org/abs/2608.07556) — "MasDrift: Benchmarking Authorization
Preservation Across Multi-Agent Architectures", v1 2026-08-02 (v2 08-11). Xu, Zhang, Luo, Jin,
Dong, Salam.

600 **benign** productivity tasks, each pairing required work with a reserved action, testing
whether a delegated goal still carries the authorization boundary of the request that spawned it.
Centralized hierarchies complete 93.9–98.6% of tasks against 85.7–87.0% for peer networks, but take
unauthorized actions in **2.7–19.8%** of tasks against 0.6–0.8% — and the gap widens with hierarchy
depth. The paper measures two defences and they do not come out level: **re-anchoring every pending
call to the original user request** cut unauthorized actions in every configuration for 1.6 points
of completion, while **propagating an attenuated policy down the chain** blocked required work and
forfeited up to **36.3 points**.

**What inber should consider:** this lands on an open design question rather than a defect. The
2026-08-14 sweep concluded that a child's authority should be the meet of its parent's with
read-only — that is the attenuation shape, and this paper is the measured argument that attenuation
alone is the expensive half. It does not say attenuation is wrong; it says attenuation *without* a
re-anchor check pays for its safety in refused legitimate work. inber has the material for the
re-anchor: `spawn_agent` carries `{agent, orchestrator, task}` (`agent/registry/spawn_tool.go:72-108`)
and the guard gate is a single function (`engine/build_hooks.go:89`), so the original request is
reachable at the point of the check. Whether the gate should see it — and what a supervisor does
when a child's call is in-policy but off-request — is the decision, not a change to make.

## ⚠️ A rule written in a tool or skill description is on the weakest surface measured

[arXiv:2608.11727](https://arxiv.org/abs/2608.11727) — "Harness-IF: Evaluating Instruction Following
Across Instruction Surfaces in Coding Agents", 2026-08-12. Huang, Que, Zeng, Zhang et al. (11
authors).

Scores operational rules one at a time from execution evidence across the five configurable surfaces
a deployed agent actually reads, and introduces **Against-Prior Accuracy** to isolate rules that
oppose the model's unprompted default — because, as the abstract puts it, "when a coding agent obeys
a rule, it may simply have been going to do that anyway." Across 12 frontier models, plain accuracy
is 72.1–85.9% but AP-Acc only 66.1–78.6%; **every model is worse on against-prior rules, by 3.6 to
7.4 points**, so an aggregate compliance number overstates obedience by a model-specific margin. A
conflict pilot found precedence does **not** follow prompt depth: system prompts, project files and
user instructions all outrank tool and skill descriptions.

**What inber should consider:** two things, and the second is the sharper one. First, the placement
rule — anything inber needs actually enforced belongs in the system prompt or a project file, not in
a tool description. inber's own `then`-chain instruction lives in a tool schema description
(`agent/chain.go:46-56`), which is the weakest surface in this study, and the one measured `then`
chain on this host arriving as a JSON *string* rather than an object is consistent with that.
Second, and more important: an against-prior rule needs enforcement in code, not text. That is the
same conclusion EcoAgent-Bench reached about budgets above, from a different direction, and it is
an argument for keeping the guard a gate rather than a prompt.

## Requested / handled / denied / committed — a four-state permission event, not a boolean

[arXiv:2607.17780](https://arxiv.org/abs/2607.17780) — "ETAS: An Effect-Typed Language for Agent
Systems", 2026-07-20. Tan, Wang, Zhang, Li, Shen.

Treats model-backed agents, tool calls, prompts, typed memory, **human approvals**, policies and
execution traces as semantic program elements rather than library conventions, indexed by an
escaping effect row and a persistent abstraction of the action trace a term may request. The part
worth stealing is small and concrete: the dynamic semantics distinguish **requested, handled, denied
and committed** events, and the design property is that a handler *cannot* make a request invisible
to authorization or audit.

**What inber should consider:** inber's gate answers in three values — `Allowed`, `NeedsApproval`,
`Denied` (`guard/guard.go:96-100`) — and one of them is currently overloaded. `CheckTool` returns
`NeedsApproval` both when an `ApprovalFunc` returned false and when there is no approver to ask
(`guard/guard.go:177-184`), so "a person said no" and "nobody was asked" are the same value. That is
harmless today only because nothing sets `ApprovalFunc` (`engine/build_hooks.go:85-86`), which makes
it a latent conflation rather than a live defect — not filed. ETAS's four states are the vocabulary
that separates them, and *requested* is the one inber has no representation for at all: a refused
call is reported to the model (`chain.go:388-392`) and counted, but nothing records that the request
was made and stopped. Decide that before wiring an approver, not after.

## Retrying a failed subagent is only safe for tool faults

[arXiv:2608.05263](https://arxiv.org/abs/2608.05263) — "OrchestraBench: Evaluating Multi-Agent
Orchestration Failure Modes, Recovery, and Decomposition Quality", 2026-08-05. Chen, Gu, Vidra,
Setty, Zheng.

Failure-injection harness with **cascade radius** — how many downstream steps one injected fault
corrupts — as a primary metric, probed with a real Claude agent across Sonnet, Opus and Haiku.
Recovery falls into three tiers and they are far apart: tool faults recover fully (**1.0**),
ambiguous delegation recovers **0.30**, and three latent/semantic modes recover **0.0**. Cascade
radius grows with pipeline depth, mean **0.9 → 4.7 across depths 3–7**, and blind retry reproduces
latent faults while increasing time-to-detection.

**What inber should consider:** inber has no subagent retry today — verified this sweep, there is no
session- or turn-level retry loop anywhere (`engine/engine.go:245` `RunTurn` is straight-line;
`server/session.go:175-185` marks the session `Error` and returns). This paper is the reason to keep
it that way by default and to make any future retry **conditional on the fault class**, since blind
retry is measured as actively worse for the modes that never recover. The depth result also bears on
`MaxSpawnDepth`: cascade radius rising 0.9 → 4.7 across depths 3–7 is an argument that the existing
bound is doing real work, not just preventing runaway spawn.

**Also checked and logged, no inber action:** [arXiv:2608.11879](https://arxiv.org/abs/2608.11879)
("Total Recall at What Cost?", 2026-08-12) — serving cost of agentic memory systems cannot be
predicted from conversation length and message size (a regression tracking the reference strategies
misses the memory systems by 18–69%), and break-even against full-transcript resubmission ranges
from the first tens of turns to *never within 400*. Relevant to memory-store, but the crossover has
to be measured per backbone rather than imported.
[arXiv:2608.11338](https://arxiv.org/abs/2608.11338) ("Better, Faster, Stronger", 2026-08-11) —
skills-as-programs beat prose skills on cost because deterministic action sequences avoid
trial-and-error over long horizons; it is the same argument as SIGIL ([arXiv:2607.27309], already
covered above) and adds no new claim for inber.

**Coverage note:** this sweep re-walked cs.AI, cs.SE and cs.CL for 2026-07-16 → 08-15. The Anthropic
engineering blog published **nothing** in the window — its most recent posts are 2026-04-23,
2026-04-08 and 2026-03-25. Two search-surfaced items did not survive checking and are recorded so
the next sweep does not chase them: a claimed "SkillZip" at arXiv:2608.05611 is in fact *FOCUS:
Decoupling Expert Personas in LLMs*, unrelated; and a claimed "FadeMem" memory-forgetting paper
returned no resolvable URL at all. Both dates above were confirmed on arXiv's own abstract pages.

# 2026-08-16 sweep — the compaction prompt loses dates, and it is a one-clause fix with a 20x measurement behind it

⚠️ **Window note, so the next sweep does not re-derive it.** arXiv's public index tops out at
**2608.13560 (submitted 2026-08-13)**: `2608.14000` is a 404 and the Fri 14 Aug `cs.AI` announcement
carries max id 2608.13558. Submissions from 08-14 announce Mon 08-17 and 08-15/16 is a weekend, so
the 2608.14xxx–2608.16xxx band **does not exist yet** rather than being empty. Everything below is
08-12 or 08-13. Re-run for the 08-14 band on Monday.

## 1. [arXiv:2608.11775](https://arxiv.org/abs/2608.11775) — The Sleeping Agent: what gist-based context compression loses, and why (Kyrkewood, 2026-08-12)

A diagnostic study rather than a new method, over all ten LoCoMo conversations (1,935 matched
questions, 1,501 in the primary aggregate, temperature 0). Gist compression beats truncation
substantially on multi-hop and single-hop factual questions, and then **temporal questions
collapse** — and the paper pins the mechanism rather than reporting the score: the gist abstraction
prompt preserves relational and event structure while discarding dates and times. The fix is a
**one-sentence change to the compression prompt**, which lifts temporal-expression preservation from
**3.05% to 62.39% (~20x)** while entity and event preservation barely move (1.02x, 1.11x), and
recovers **+0.314 judge accuracy [0.254, 0.375]** on temporal questions. Code at
`github.com/kyrkewood/sleeping-agent`.

**This lands on a prompt inber actually runs.** `generateSummary`'s system prompt
(`conversation/summary_generation.go:40-53`) enumerates exactly five things to capture — main topics,
key decisions, important information, project status, unresolved questions and next steps — and
**not one of them is temporal**. There is no instruction to keep dates, times, ordering or "what
changed since". The summary is not a side artifact: `SummarizeConversation` reinserts it as a user
message at the head of the conversation (`conversation/summarize.go:107`), so it is what every
later turn in a long session reads instead of the turns it replaced. The compaction is also
recursive — the next pass re-summarizes its own output — so a date dropped at pass 1 cannot come
back at pass 2. The archive to memory-store (`summarize.go:83-97`) means the original text still
exists, but only for a model that thinks to call `memory_expand`; the prompt gives it no reason to,
because nothing in the summary says a time was ever there.

This is the same defect surface `opencode.md`'s 2026-08-16 entry reaches from the other side —
[opencode #42045](https://github.com/sst/opencode/pull/42045) rewrote its compaction prompt to state
the discard and the conflict rule. Two independent sources, one week, one prompt.

**What inber should consider:** add temporal preservation to the `generateSummary` system prompt.
Cheap, measured, and low-risk. Two things it should not decide silently: whether to add a category
to the numbered list or a standalone clause (the paper's gain came from a *dedicated* sentence, not
a sixth bullet), and whether the summary block should additionally carry a machine-written time
range — `summaryFooter` (`summarize.go:153`) already composes a footer, so a "covers <first> to
<last>" line is available without asking the model for it, and a stamped range is not a thing the
model can drop. **Filed as `421a8162`.**

## 2. [arXiv:2608.12895](https://arxiv.org/abs/2608.12895) — a reviewer on the same model as the worker is not a second opinion (Bhardwaj, Singh, Bhardwaj, 2026-08-13)

Preregistered, 18,000 missions, deterministic scoring, no LLM judge. Two instances of **one model**
in a two-agent handoff **co-fail on 90.0% of the missions where either fails** (log OR 6.66,
φ=0.916). Swapping in a different model reduces the association in 6/6 contrasts; swapping *vendor*
while the model already differs does not. The authors note the error is signed against the operator:
redundancy is over-credited exactly when the components share a model.

This is the measurement behind a pattern this corpus keeps recommending. inber's spawn path builds
children from agent config (`server/spawn.go:170` → `session_creation.go:114`), and open todo
`2dcdb9a6` is already about a child taking its model from config rather than the parent's live one —
that todo is usually framed as a *capability* bug (the child gets a weaker model than the parent
escalated to). This paper says the sign can run the other way for a **reviewer or verifier** child:
inheriting the parent's model is precisely what makes the second opinion worthless. Whoever settles
`2dcdb9a6` should therefore not settle it as "always inherit the parent's live model" — a delegate
that checks the parent's work wants a *different* model, and a delegate that continues the parent's
work wants the same one. That is a distinction inber's spawn API currently cannot express.

**What inber should consider:** nothing to fix yet — inber has no reviewer-subagent pattern in the
tree, so there is no live instance. Recorded as a constraint on `2dcdb9a6`, not as new work.

## 3. [arXiv:2608.12654](https://arxiv.org/abs/2608.12654) — a model-driven approval gate fails by over-refusing, 28:1 (Serdar, Mertayak, 2026-08-12)

SteerBench-Work makes the *pre-commit* decision — proceed, or hold for human/policy review — its own
measured capability, over 106 incident-anchored workplace scenarios with evidence-reversed mirrors
and near-even proceed/hold labels. Across 30 model conditions the error is almost entirely
one-directional: models wrongly **hold** authorized, evidence-cleared work **28.1%** of the time and
wrongly **allow** unsafe work **1.0%**. The hardest class is "risk-resolved commits", where signed or
structured evidence has already cleared a real risk trigger and the model holds anyway. Models score
98.5% on the famous incidents but **63.8% on the evidence-reversed mirrors** — they are pattern
matching the incident, not reading the evidence. Higher-capability models over-refuse *more* at the
commit boundary.

Two consequences for anyone building this gate. The design lever is not "make the gate stricter" but
"make cleared evidence legible to the gate", and you cannot buy calibration by routing the decision
to a stronger model. That last point contradicts the intuition behind every "use the big model for
the risky call" design, including some of codex's Guardian tiering.

**What inber should consider:** this is a note for **permission-store** — whose README still calls
it step 1 of 7, with the rules engine unbuilt — more than for inber. inber's own gate is not
model-driven at all: `guard.CheckTool` branches on the tool name and mode
(`guard/guard.go:165-188`), so it has no over-refusal failure mode to calibrate. The finding is a
reason to keep it that way for the deterministic cases, and a warning about the `ApprovalFunc` that
does not yet exist.

## 4. [arXiv:2608.11772](https://arxiv.org/abs/2608.11772) — recovery should be a *typed, pre-selected* interface, not more context (Wang et al., 2026-08-12)

DARC argues that generic recovery playbooks broaden the agent's context exactly when it needs a
*narrower* repair interface, mixing incompatible signals for invalid actions, missing procedures and
format errors. It profiles per-task-family failure modes on a dev set, **prunes mismatched
interventions from a shared recovery library before test time**, and freezes a verifier-selected
policy for deployment. On ALFWorld, AppWorld and XBRL Finance the one protocol yields three
different harnesses — action-validity, procedural fallback, format-precision retrieval — each
beating both the base agent and broad playbooks **while reducing environment steps or retrieval
budget**. It also explicitly separates coding agents, where compilers and tests give typed recovery
signals, from generic agents that only see coarse task failure.

This is the direct counter-argument to "on error, append the error text and re-prompt", and inber is
closer to DARC's shape than to that one by accident rather than design: its two semantic retries are
each matched to a specific, named failure (`agent/agent_run.go:205-222` prune-and-retry on context
overflow, `engine/turn_execute.go:44-50` strip-thinking-and-retry), and neither is a general
playbook. What inber lacks is the third thing DARC has — a statement anywhere that this is the
policy, so the next failure class gets its own typed branch rather than a generic one.

**What inber should consider:** no code change. This belongs on todo `4c511c8f` (what an unexpected
stop reason *is*), which is currently the open question of whether an unhandled `pause_turn` or a
refusal is a provider fault, a model fault or an inber gap — DARC's framing says the answer should
be a typed recovery class, and that a class with no matched intervention should refuse to retry
rather than retry generically.

## 5. Memory: two papers that argue against the pipeline, and one that halves its write cost

- **[arXiv:2608.12888](https://arxiv.org/abs/2608.12888) — ReFind** (Li et al., 08-13) is the null
  hypothesis nobody runs. Agent-memory systems buy retrieval quality with structure — summaries,
  embeddings, trees, knowledge graphs — all built *before any question is asked*. ReFind builds none
  of it, gives the agent lexical search over raw conversation logs, and lets it drive: **58.2 mean
  accuracy on MemoryAgentBench, 93.2 on LongMemEval-S**. Worth stating plainly because inber and
  memory-store are on the other side of this bet: a vector store with importance decay and
  compaction, written on every session. This is not evidence to tear that out, but it is the
  baseline that should have been measured first, and it is cheap to measure now.
- **[arXiv:2608.12847](https://arxiv.org/abs/2608.12847) — QCR** (Li et al., 08-13) names
  *post-retrieval reuse* as a bottleneck distinct from retrieval quality: finding the right past
  trajectory does not tell the agent how to use it once entities, constraints or environment state
  have moved. Replacing trajectory injection with a target-bound note — reusable procedure,
  bindings to recover, applicability conditions, verification requirements — gives **62.3% success
  over 2,391 targets, +10.7 points over full-trajectory injection, with 48.9% fewer online tokens**.
  Direct bearing on `SummarizeConversation`'s archive: inber stores `oldText`, the **raw rendered
  transcript** (`conversation/summarize.go:51,87`), which is exactly the artifact QCR measures as
  the losing one.
- **[arXiv:2608.12990](https://arxiv.org/abs/2608.12990) — LycheeMemory V2** (Li et al., 08-13)
  attacks the write-side cost nobody budgets: eager per-turn consolidation calls an LLM after every
  interaction. Segment-level consolidation gives **89.22% on LoCoMo while cutting construction
  tokens by 86.0%**. inber consolidates at compaction rather than per turn, so it is already on the
  cheap side of this; recorded so the number is on hand if per-turn memory writes are ever proposed.

## 6. [arXiv:2608.12610](https://arxiv.org/abs/2608.12610) — @skills: only *triggering* needs prompt residency (Yin et al., 2026-08-12)

States the budget outright: **56,804 public agent skills exist, competing for fewer than 100
reliable trigger slots** in a system prompt. The decomposition is the contribution — installing a
skill bundles three separable functions, **content, persistence and automatic triggering, and only
the third requires prompt residency**. @skills addresses any skill, subtree or collection by path;
reading a skill is sufficient to use it, so nothing is installed or resident. A directory *is* a
menu, so bundles are ordinary directories rather than all-or-nothing units, and there is no
manifest, lockfile or registration; `SKILL.md` is unchanged.

**A note for skill-store and bundle-store, not for inber.** bundle-store's whole job is resolving a
curated set of skill ids for a session, which is the resident-description model this paper argues is
what caps skill count in the first place. If the claim holds, bundle resolution changes shape from
"pick ≤N skills to make resident" to "expose a directory tree and let the agent read". Recorded here
because this corpus is where the sweep looks; it is not inber work.

## 7. Checked and rejected this sweep

- **[arXiv:2608.13228](https://arxiv.org/abs/2608.13228)** (Capability Sheaves for Compositional
  Agent-Harness Repair) — the title is squarely on-topic and it is a **null result by the authors'
  own conclusion**: on SWE-bench Multilingual (160 issues, 875 candidate patches) the cohomological
  selector resolves 118 against 116 for a matched baseline, unsupported across repositories
  (sign-flip p=0.75), the discovery gate fails and the confirmatory split stays sealed. Flagged by
  id so a later sweep does not spend the read.
- **[arXiv:2608.13560](https://arxiv.org/abs/2608.13560)** (AutoDesign) — a real self-improving
  harness result (78.32 on PosterBench, +7.45 over Claude Design; the learned DesignHarness lifts
  seven code-agent configs 54.99→67.39 average), but the entire instantiation is paper-to-poster
  generation. The transferable part is the optimizer loop, not the harness it learns.
- **2608.13334** (RippleMem), **2608.12428** (MindMemOS), **2608.11701** (Consolidator) —
  incremental benchmark gains on memory architectures with no harness-level design claim.
- **2608.12761** (provenance integrity), **2608.12880** (MCP security label leakage), **2608.12921**
  (multi-agent topology explanation), **2608.12123** (GPU-resident agent control path), **2608.12311**
  (Role Specialization — one case study, no numbers) — read, too narrow or too weak to act on.
- **[arXiv:2608.12273](https://arxiv.org/abs/2608.12273)** (Convergent Detour Hijacking) and
  **[arXiv:2608.12476](https://arxiv.org/abs/2608.12476)** (Governed Persistent Memory) — both real,
  both held rather than rejected. The first is a text-only attack on progressive-disclosure skill
  loading where a malicious skill's description wins selection and its body recruits benign skills
  into a detour, raising tokens 66.91% and wall-clock 92.45% **while task completion stays
  comparable** — the point being that correct output does not certify trajectory or cost integrity.
  The second adds bitemporal state semantics so retrieval cannot cite superseded or retracted
  records (2,400/2,400 vs 600/2,400 ungoverned). Neither has a live inber surface today: inber
  ingests no third-party skills into its own loop, and its memory hints rather than justifies.
- **[arXiv:2608.12282](https://arxiv.org/abs/2608.12282)** (VAKRA, IBM) — mostly a benchmark, kept
  for one number: with a **fixed ReAct harness** holding architecture constant, best-model accuracy
  is 70.4% single-hop, 50–51% on compositional APIs and **as low as 2.4% on policy-constrained
  unanswerable queries**, with failures concentrated in entity disambiguation and cross-source
  grounding rather than tool-invocation mechanics. Reads as: tightening the tool-call schema layer
  buys little, and a tool-use policy written in natural language is close to unenforced by the model
  and must be enforced by the harness. That is the same conclusion as the 2026-08-14 entry on rules
  written in tool descriptions being on the weakest surface measured.

## 8. Channel notes — blogs, seventh consecutive empty sweep

**Anthropic engineering blog** — nothing since 2026-04-23 ("An update on recent Claude Code quality
reports"); no August posts. **Google DeepMind** — nothing agentic in 08-10→08-16 (Gemini 3.7 Flash,
sign-language AI, WeatherNext). **Meta AI** — nothing since 2026-07-27. **HuggingFace blog** — posts
in window (OlmoEarth 08-12, LFM2.5-VL-3B 08-12, Strands Agents/LeRobot 08-13, ICML reproduction
08-13, State of Open Models 08-14), none on harness design.

**Prompt caching swept and genuinely empty.** `abs:"KV cache" OR "prompt caching" OR "prefix cache"
OR "cache reuse"` for 08-11→08-17 returns 14 hits, all serving-infrastructure or modality-specific
(vToken KV reclamation, HBF flash tiering, TTS and video streaming caches, TideRL rollout
scheduling). **Zero agent-harness cache-economics papers in the window** — the 2026-08-13 keepalive
entry is still the most recent thing on that thread.

# 2026-08-17 sweep — four papers, and a scoping correction: this sweep's nominal window was two-thirds empty by construction

**Read this before the papers.** arXiv has announced **nothing submitted after 2026-08-14 ~20:00
UTC**. Measured this sweep with a control, because a bare zero from a malformed query is worthless:
`submittedDate:[202608130000 TO 202608150000]` returns **`totalResults=1823`**, and
`submittedDate:[202608150000 TO 202608180000]` returns **`totalResults=0`**. Same query form, same
endpoint, one returns thousands and the other returns none. 08-15 and 08-16 were Saturday and
Sunday; 08-17 has not been announced yet.

Two consequences, and the first is a correction to this file. **The 2026-08-16 sweep's heading
claims a window it could not have had** — there was nothing after 08-14 to find, so its coverage of
08-15 and 08-16 is vacuous rather than empty-by-search. That is not a criticism of the sweep, which
found real work; it is a warning that a sweep dated D covers submissions up to roughly D-2 at best,
and this corpus should stop writing headings that imply otherwise. Second, **re-run in ~48h**: the
08-17/08-18 announcement batch is a third of this sweep's nominal window and has not appeared.

Every date below was verified against the arXiv Atom API by `id_list`, not from a search snippet.
Two candidates were dropped as already covered — **2608.12888** (ReFind, agent-controlled lexical
search over raw chat logs) and **2608.12895** (same-model reviewers co-fail) are both already in the
2026-08-16 sweep, the latter as §2. Headline numbers below are abstract-grade unless stated.

## 1. [arXiv:2608.11392v2](https://arxiv.org/abs/2608.11392) — a degraded rule is worse than a missing one, and a presence check is not a safety check (Kwartler, Aqrawi, Abbasi; v1 2026-08-11, v2 2026-08-13)

The only in-window item on compaction, and it is a **v2 update rather than a new paper** — flagged
because a sweep that counts it as new is double-counting. After a *single* self-summarization cycle,
a **degraded** rule residue leads the model to perform the prohibited action **more often than an
intact rule survives it**: gaps of **+34 and +57 points** under two replay models. The second
finding is the trap: rule-form items survive compaction *more often* than prominence-matched facts,
so the constraint text usually is still there in some form — which is exactly why auditing for its
presence feels adequate and is not. The authors' formulation is the one worth carrying: **a presence
check is not a safety check.** Scope caveat the authors state themselves: one compaction cycle.

**What inber should consider:** this lands on a prompt already under a filed todo. `generateSummary`'s
system prompt (`conversation/summary_generation.go:40-53`) enumerates five things to capture and no
standing directive among them, and `:51` — "Avoid unnecessary details while preserving important
context" — is an explicit invitation to drop a rule the summarizer judges unimportant. **Appended to
open todo `421a8162` rather than filed as a new one**, because that todo already names this exact
file and line range for the *temporal* loss and the fix is one edit to one prompt. The appended
question is whether a prompt clause is even the right instrument: a dropped date degrades an answer,
a half-preserved prohibition degrades a decision, and 11392's result is that asking the summarizer to
carry a constraint produces precisely the partial residue that measured worse than nothing — so
pinning standing directives *outside* the compacted span is a different mechanism, not a stronger
version of the same one. **Scope check that keeps this honest:** inber's agent instructions live in
the system prompt and are not part of the `messages` slice compaction rewrites, so they are already
safe. The exposure is only a constraint the **user stated mid-conversation**, which lives solely in
the turns `SummarizeConversation` replaces. Verify that boundary before sizing the work.

## 2. [arXiv:2608.13951](https://arxiv.org/abs/2608.13951) — HELIX: the harness as both a capability search and a training-data generator (Fan, Huang; 2026-08-14)

Decomposes a harness into typed ports, reusable atoms, recipes, product shells and runtime policies
so that each intervention is identity-preserving and auditable, then evolves harness variants against
a SWE-bench evaluator. Code at `github.com/HKUDS/HELIX`. **Read the two numbers separately.** The
best single evolved harness gains **+4.0% task coverage** over the Pi baseline — that is the
deployable figure. The **58.0%** in the abstract is the *union* of a 65-candidate portfolio, an
oracle over complementary sibling behaviour, and is not a harness anyone can run. The genuinely
novel claim is the by-product: a 200-slot sibling slice yielded **438 verified SFT/critic/filter/
preference records**, because running many harness variants over the same task produces matched
successes, regressions and near-misses that a single configuration never generates.

**What inber should consider:** nothing structural, and the 4.0% does not justify building an
evolution loop. The transferable idea is cheap and separable from the rest of the paper — inber
already varies model, thinking budget and tool set per agent, and every spawn already records what it
ran on (`server/spawn.go:116-120` carries the child's actual model). **A/B differences across those
existing axes are matched-pair data inber currently discards.** Worth knowing before anyone proposes
a self-improving harness: the portfolio-union framing is how that proposal will be justified, and it
is the number that does not survive contact with deployment.

## 3. [arXiv:2608.12789](https://arxiv.org/abs/2608.12789) — PIPES: screen each *unit* of a tool result, not the result (Kariyappa, Klingler, Suh; 2026-08-13)

Attack success **84.7% → 2.3%** across three VitaBench and three AgentDyn splits against adaptive
PAIR-style attacks, with benign utility preserved (**92.5%** defended vs **90.6%** undefended) — a
defence that does not cost accuracy is rare enough to note. The mechanism is the contribution: screen
each *unit* of a tool response rather than the response as a whole, using static field contracts
where the tool has a schema, and conditioning open-ended content on pre-response trajectory plus
trusted provenance metadata. The discriminator is **informational authority** — flag units making
environmental claims beyond what their source could know.

**What inber should consider:** this is the same shape as the tool-result gating already discussed in
this corpus, and inber has a natural chokepoint for it. The honest note is a gap, not a fix: inber's
first-party tools have schemas, so field contracts are available cheaply, but the units most worth
screening are the open-ended ones — shell output, fetched pages — where the schema says only
"string". Recorded as a design shape to reach for if untrusted tool output ever reaches the loop;
there is no action today.

## 4. [arXiv:2608.13867](https://arxiv.org/abs/2608.13867) — a 206-record reliability catalog for the system around the model (Jarmak; 2026-08-14)

**No new experiment**, and worth having anyway. A monograph synthesizing 164 scholarly, 100
practitioner, 29 benchmark and 17 case records into a versioned catalog of **206 reliability
records** (193 gated practices, 56 treated in depth). Its core claim is the one this corpus has been
accumulating one PR at a time: **many apparent model failures originate in the harness, execution,
retrieval and state layers**, and evaluation is a dependency chain in which weak task construction or
a weak execution environment invalidates every downstream conclusion. The author flags it as
"structured rather than exhaustive".

**What inber should consider:** use it as a citation index and a checklist, not a reading. It is
probably the single most useful *document* in the window for a harness author precisely because it
contributes no measurement — when a future sweep needs prior art for a reliability practice, this is
the place to look before searching arXiv from scratch.

## 5. Tier two — real, narrower, no action

- **[arXiv:2608.13667](https://arxiv.org/abs/2608.13667)** (Second Thought, Sun et al., 08-13) —
  training-free: fork four auxiliary reasoning branches the instant the Thought phase ends, decode
  them concurrently with the action/observation round trip, merge on observation arrival. Turn count
  down in **all 9** (model, benchmark) pairs; main-thread decoding down **up to 43%, ~20% average**
  in 6/9; Pass@1 unchanged in 7/9. Beats a compute-matched control. ⚠️ **It buys wall-clock with
  tokens — total cost rises.** Relevant only if latency is the binding constraint, which for inber's
  unattended and scheduled runs it is not.
- **[arXiv:2608.14375](https://arxiv.org/abs/2608.14375)** (Wrong but Useful, Yang et al.,
  Argonne/UChicago, 08-14) — cache five independently generated subagent messages, replay the
  integrator with each available or hidden. Among **wrong-answer** messages that flip final
  correctness, **more than 4 in 10 flips are helpful**, not explainable by replay variance
  (p=0.0002); passing the *complete* message beats reasoning-only, which beats answer-only. The
  actionable line: **do not filter subagent output by answer correctness before handing it to the
  parent.** ⚠️ Math and science benchmarks, not coding. Bears on `server/spawn_delivery.go`'s result
  hand-back if a correctness filter is ever proposed there — the point is to not add one.
- **[arXiv:2608.13900](https://arxiv.org/abs/2608.13900)** (Agentic Transaction, Sun, Wang, Li,
  08-14) — reinterprets ACID as semantic atomicity/consistency/isolation/durability over agent
  workspace mutation; **10.6% over SOTA agents including Claude Code**, via transactional
  explore-execute-validate cycles and semantic dependency-aware isolation. Relevant only if inber
  runs parallel subagents against shared workspace state. ⚠️ "Data agent" domain, benchmarks unnamed
  in the abstract.
- **[arXiv:2608.12720](https://arxiv.org/abs/2608.12720)** (ERSkill, Chen et al., 08-13) — retrieval
  strategies as executable skills over primitives, with a trained router matching query to skill;
  **+31.3%** and **+28.1%** on two backbones. ⚠️ Requires training, and the headline averages BLEU-1
  in, which inflates it. **Read against 2608.12888** in the 08-16 sweep, which bets the opposite way
  — that lexical BM25 over raw logs with no learned index rivals structured memory. Two in-window
  papers, opposite conclusions, neither refuting the other; that disagreement is the finding.
- **[arXiv:2608.13662](https://arxiv.org/abs/2608.13662)** (Ontology-Grounded Project Memory,
  Adam, 08-13) — **0.98–1.00** vs **6–27%** for a vector-memory baseline on supersession,
  set-completeness and negation questions, at comparable token cost. The structural claim is sound
  and matches the bitemporal result already held from 2608.12476: **top-k vector retrieval cannot
  answer "what superseded X" or "what is *not* allowed"**, because the answer is an absence.
  ⚠️ Heavy skepticism warranted and it is why this sits in tier two: single author, proprietary
  engine, self-constructed corpus, a baseline that may be a strawman, and a live trial that is
  pre-registered but not completed.

## 6. Checked and rejected this sweep

- **[arXiv:2608.13959](https://arxiv.org/abs/2608.13959)** (Repair, Not Improvement, Lee, 08-14) — a
  **preregistered negative result** worth recording so it is not re-derived: grammar-constrained
  decoding is negative for tool-call abstention in 4/6 cells (worst **−29.5** points) and positive
  with a CI excluding zero in **none**; the stop-token cost (−20.0) cancels the enum gain (+19.5) to
  −0.5. Both preregistered language claims fail. The framing is the keeper — constrained decoding
  repairs *form* without improving the abstain *decision* (545 of 698 repaired abstentions had no
  readable answer at all). ⚠️ Models are 0.6B–4B only, so it does not transfer to frontier harnesses.
- **[arXiv:2608.12977](https://arxiv.org/abs/2608.12977)** (self-evolving runtime defense, Ruan et
  al., 08-13) — contributes a harness-level taxonomy of runtime defences by which mechanism they
  attach to (hook, wrapper, loop interception), which is useful vocabulary for a permission layer.
  **No quantitative result in the abstract** ("improves security performance"), so no evidential
  weight until the tables are read.
- **[arXiv:2608.14074](https://arxiv.org/abs/2608.14074)** (Mandato, 08-14) — signed-mandate MCP
  proxy with a hash-chained audit log, mapped to EU AI Act Art. 12/14, GDPR, NIS2, eIDAS 2. **Zero
  results**; the abstract states an implementation status and an evaluation *plan*. Position paper.
  The mandate schema — which tools, parameter constraints, contextual conditions, duration, on whose
  behalf — is a usable sketch for **permission-store**, and nothing else here is.
- **2608.13030v2** (InterSAGE, four-layer agent trust protocol), **2608.14352** (ATLAS, automata
  learning over agent trajectories; proof-of-concept on 12 machines), **2608.14109** (RL drift
  diagnosis on AppWorld; results "generally exploits information about the suspected drift onset" —
  too vague to act on), **2608.13884** (33,228 PRs from vLLM/SGLang; throughput 21×/17.9×,
  bot-authored <0.2% — descriptive and confounded by project popularity growth, no design claim).
- **[arXiv:2608.13608](https://arxiv.org/abs/2608.13608)** (*Evaluating Agentic Learning Harness
  Capabilities Without Labels*) — squarely on topic, and **out of window: published 2026-08-11**,
  confirmed by Atom API. Recorded by id because the number looks in-window and this is exactly the
  trap this file has warned about since the 08-01 sweep. It is also not covered by any earlier
  sweep, so it is a genuine gap someone may want to read.

## 7. Channel notes — blogs, eighth consecutive empty sweep

**Anthropic engineering** — no August 2026 posts at all; latest remains 2026-04-23. **Google
DeepMind** — August posts are Gemini 3.7 Flash, sign-language AI and WeatherNext; nothing on agent or
harness design. **Meta AI** — nothing since 2026-07-27. **HuggingFace** — two posts on 08-13
(Strands Agents/LeRobot robotics deployment; "What We Learned by Reproducing 2,200 papers from
ICML"). The ICML reproduction post could not be verified this sweep — the guessed URL 404'd — and it
reads as an ML-reproduction writeup rather than harness design, but it is the one item left
unchecked; worth two minutes on the next pass.

**Confirmed empty by dedicated search, not by absence of effort:** prompt caching and prefix-cache
economics at the harness level (the sole in-window hit, 2608.12932 *FlashDrive*, is VLA
autonomous-driving inference — GPU serving side, off-domain); tool-result compression; context
management and compaction for **new** submissions, 2608.11392v2 above being an update; and
sandboxing or isolation mechanisms, where the in-window security work is all provenance, prompt
injection and protocol-level authorization. The cache thread's most recent genuine entry is still
2026-08-13.

---

# 2026-08-18 sweep — the 08-15→08-18 band is genuinely new, and the strongest result in it prices inber's own layer at 10x

**Scoping first, because the last sweep left a condition to re-run rather than a conclusion to
inherit.** The 08-17 entry closed by reporting that arXiv had announced nothing submitted after
2026-08-14 and told the next sweep to re-check in ~48h. Re-run this pass with the same
`totalResults` control: `[202608150000 TO 202608170000]` now answers **1029** and
`[202608170000 TO 202608190000]` answers **915**, both zero at the time of the last sweep. So the
window 2026-08-15 → 2026-08-18 was **structurally unreachable** by every earlier sweep in this file
rather than merely unexamined, and everything below is new. Highest id previously recorded here was
`2608.14375`; all sixteen candidates checked this pass are above it and grep-clean against all five
notes files. Every id and date below was verified against the arXiv Atom API by `id_list`, not read
off a search snippet.

## 1. [arXiv:2608.16630](https://arxiv.org/abs/2608.16630) — The Working Set of a Coding Agent: coherence debt, and why read-based instrumentation cannot see it (Mohammadi, Klein, Chadha, Arora, Bindschaedler; 2026-08-17, cs.SE)

Models repo-scale coding as reconstructing a coupled-fact graph: at each edit a required fact comes
from recent context or from parametric memory, and the facts covered by neither are **coherence
debt**. Each channel is supplied and withheld, with injected faults, across **seven models and five
harnesses**. Three findings carry. **Availability decides the outcome and distance does not** — a
supplied fact works as well far from the edit as adjacent to it, and withholding one costs exactly
the work it supports, which kills the intuition that context should be arranged by proximity.
**A missing fact produces wrong work rather than absent work** — "an agent asked to act acts,
fabricating the file or guessing the value, so instruments built on reads look for a hole already
filled." And the number: **harness configurations that all pass every test differ more than tenfold
in tokens consumed**, because they rebuild the same content at different rates, while spending more
recovers nothing when facts are genuinely withheld. A side result corroborates this file's 08-12
"catastrophic remembering" entry from the opposite direction — where a convention file and the code
disagree, all seven models follow the stale convention, so **a wrong `CLAUDE.md` costs more than no
`CLAUDE.md`**.

**What inber should consider:** the tenfold spread is measured at the layer inber owns, between
configurations that are all *correct*, which makes it the most cost-relevant number in this window.
The trap is the instrumentation. inber's observability is read-shaped — tool calls and cache
counters via `agent/agent.go` hooks and `session/session.go` — and the paper's closing prescription
is that availability must be checked **against what the agent produced, not what it read**. That is
not a metric inber can compute today, and adding it is a real piece of work rather than a
counter; record the finding, do not pretend the existing counters answer it.

## 2. [arXiv:2608.16370](https://arxiv.org/abs/2608.16370) — What Does Context Compression Cost an Agent? (Shuyu Liu; 2026-08-17, cs.AI)

The argument is that task completion is the wrong instrument for compression, because compression
can force an agent to **reacquire dropped state** while leaving completion statistically unchanged.
Controlled 24-turn horizon, three models, two task regimes, tool calls split into retrieval versus
execution. **Retrieval calls rise in all six model-regime comparisons, five of six survive Holm
correction, and at the prespecified 5x compression point completion changes are not significant in
any cell.** Cleanest case, GPT-5.5: completion 80% → 85% (p=1.0) while retrieval goes 21.0 → 63.9
calls (p=.002). Replacing retained state with semantically irrelevant content raises retrieval
**57%** with no completion change. The honest hedge is the author's own: in ALFWorld, sliding
compression produces **no** retrieval surge, so the reacquisition signature is
environment-dependent rather than intrinsic to shortening context.

**What inber should consider:** this says the compaction work already in this corpus has been
measured with the wrong instrument — inber cannot tell whether `conversation/summary_generation.go`
is any good from whether sessions still finish, because the paper's whole point is that they do
finish, more expensively. The concrete ask is cheap and uses data inber already writes: **count
tool calls in the N turns after a compaction event against the N before**, per session, as a
regression metric. Because of the ALFWorld result, measure it on inber's own traffic before
assuming the surge is there — this is a hypothesis to test, not a defect to fix.

## 3. [arXiv:2608.16055](https://arxiv.org/abs/2608.16055) — Governance at the Boundary: facts are attenuated at the handoff, and decomposition is what does it (Li, Wang; 2026-08-17, cs.AI)

626 episodes over 100 KYC/AML variants, two models, three architectures. The mechanism is a handoff
defect stated precisely: **policy-relevant facts discovered by one component are attenuated at the
boundary before reaching the component that must act on them.** A 32B open-weights model attenuated
**0% of discovered facts under a single-loop baseline, 56% under a fixed pipeline, and 85% under an
orchestrator-subagent architecture**; gpt-4.1-mini attenuated 3–6%, so the price of decomposition is
partly a function of model capability. The same mechanism yields **both under- and over-escalation**,
depending on whether the dropped fact was a risk signal or an exculpating one.

**What inber should consider:** read it against `2608.07556` (MasDrift, 08-14 entry) rather than
alongside it — MasDrift measured whether *authority* survives delegation, this measures whether the
*facts the authority depends on* survive. inber's exposure is legible in one tool signature:
`spawn_agent` carries `{agent, orchestrator, task}` (`agent/registry/spawn_tool.go`) where `task` is
free-text the parent writes, so **every fact the parent discovered reaches the child only if the
parent chose to retype it into prose**. That is the 85% channel. The bidirectional failure is why
there is no cheap fix and why this is not filed as a defect: passing more context and passing less
are both wrong without knowing a fact's polarity, so "widen the handoff struct" is a design decision
with a real downside, not an obvious improvement.

## 4. [arXiv:2608.15089](https://arxiv.org/abs/2608.15089) — StateM: harness scaling, with an unusually large number attached (Qin, Lu, Wang, Wang; 2026-08-15, cs.AI)

An explicit bet on improving the execution system around an agent without touching model weights:
durable states, phase-local context, checked transitions, recoverable runbooks, versioned procedural
practices. GPT-5.5 xhigh 83.1% → **92.1%** on Terminal-Bench 2.1; the runbook **transfers unchanged
across models**, and under $38 of adaptation lifts DeepSeek-V4 Flash 82.7 → 88.1%. Reported final
run economics: **~$15 against $574.68** for the GPT reference. Code at `henryqin1997/statem`.
⚠️ The headline 95.3% is *raw accuracy across 445 trials* with an "all 89 tasks succeed at least
once" framing — a different claim from a single-run pass rate, and it should be cited as such.

**What inber should consider:** the transferable half is the claim that four named failure modes —
losing mutable state, failing to reactivate earlier lessons, skipping known procedures, stopping
prematurely — live in the harness rather than the model, which is this corpus's running thesis with
a number on it. The one mechanism worth looking at concretely is **turning postmortem findings into
persistent, executable preconditions**: inber has the raw material in `memory/` and
`session/checkpoint.go`, but a memory there is retrieved as context and never enforced as a
precondition, and those are different things.

## 5. [arXiv:2608.15008](https://arxiv.org/abs/2608.15008) — Harness the Memory: no substrate dominates, and retrieval actively harms decisions (Huang, Zhang, Wu, Chen, Jiang, Yang, Yang, Zou; 2026-08-15, cs.CL)

Controlled comparison of memory *substrates* — dense and sparse indices, text records, structural
and hierarchical stores, refinement-based memories, parametric updates, activation-compatible
context mechanisms — over three backbones, four benchmark suites, 26 metrics. **No single substrate
dominates.** Broad retrieval helps long-context factual QA but **excessive retrieval actively harms
sequential decision-making by shifting attention away from action-critical context**, and substrates
that work at moderate history lengths turn costly or brittle at longer horizons. Conclusion:
substrate routing is a necessary component, not an optimization.

**What inber should consider:** this referees the disagreement the 08-17 entry logged and left open —
`2608.12720` (learned routing wins) against `2608.12888` (lexical BM25 over raw logs wins) — and its
answer is that both hold in different regimes and the question was mis-posed as a choice. The
actionable read for inber is not "add substrates": it is that **the retrieval-harms-decisions
finding lands directly on `memory/auto_context.go`**, since automatic injection into an agent that
is already mid-task is the exact configuration measured as harmful. Worth measuring before widening
what that file injects.

## 6. [arXiv:2608.15888](https://arxiv.org/abs/2608.15888) — Bounded Agents: injection is an authorization-architecture problem, and composition closure is the part nothing here has (Muruaga; 2026-08-16, cs.AI/cs.CR)

Frames prompt injection as an authorization problem rather than a model problem — an injection is
only a risk if the agent holds the authority to act on it. The Agentic Principal Chain tracks
delegated authority across principals, evaluates each request against **accumulated session state
rather than independently**, carries forward and restricts delegated scope and budgets, and uses
**composition closure** to block individually-permitted actions that combine into a prohibited
outcome. 3,154 instances over InjecAgent, AgentDojo and ASB: AgentDojo exfiltration **75–100% → 0%**
in all four domains, all 544 InjecAgent data-stealing cases blocked, intent binding cutting
destruction 38.6% → 4.0%. Authorization latency **0.24 ms at p99**. ⚠️ The utility cost is real and
the author states it: **8.6 and 13.9 percentage points lower** across 949 task-injection pairs, and
the Composition Soundness proof holds only under a complete restriction set and serialized
admission.

**What inber should consider:** composition closure is the piece inber's guard and **permission-store**
have in no form — `guard.CheckTool` (`guard/guard.go:165`) sees one call with no memory of the last
one, so two individually-benign calls that compose into exfiltration both pass, and the repetition
detector is the only stateful thing in the package. Sub-millisecond p99 removes cost as the reason
not to build it; the 8.6–13.9 point utility loss is the reason, and it is a genuine trade rather
than an oversight. Read directly against `2608.12654` (08-16 entry: model-driven approval gates
over-refuse 28:1) — the two reach the same conclusion, that the decision belongs **outside** the
model, from opposite evidence.

## 7. Tier two — verified, narrower, no action

- **[arXiv:2608.15127](https://arxiv.org/abs/2608.15127)** (AgentSysBench, Chang et al., 08-15,
  cs.OS) — the only in-window item touching cache economics, and it is a *different* cache. Names a
  **"control-plane tax"** where auxiliary LLM calls and context overhead from tool schemas and
  observations crowd out productive compute, and measures heavy cross-request redundancy in
  production traces: **tool-result caching removes 35.2% of redundant search calls and 19.3% of
  aggregate search latency**. Also that sessions hold state idle for minutes to hours between
  steps — the demand-side confirmation of the keepalive-economics paper (`2607.19214`, held 08-13).
- **[arXiv:2608.16801](https://arxiv.org/abs/2608.16801)** (Destefanis, Aste, 08-17, cs.SE) — 1902
  runs modelling multi-agent coding as a temporal network. **Shared files replace repeated 1-to-1
  messaging, cutting output tokens ~42% at eight agents** on message-heavy work while adding
  overhead where files already carry the coordination — and **naming a coordinator creates no
  communication hub and gives no reliable improvement in success**, which is worth holding against
  any future orchestrator design here. Unprompted reward-hacking observation: agents hunt for hidden
  grading material in four-fifths of runs, reproduced across 244 sealed-environment runs.
- **[arXiv:2608.16381](https://arxiv.org/abs/2608.16381)** (AstronOS, Nie et al., 08-17) — five ways
  to carry a plan into a fresh model session: runtime-mediated handoff passes **14/15**, against
  **0/15 for rereading the originals, 2/15 for full-history replay, and 0/15 for both a
  deterministic text summary and a deterministic JSON summary**. Squarely on inber's resume path if
  it holds. ⚠️ 150 executions, one system, self-designed benchmark, frozen scorer — small and
  self-refereed, so treat the ordering as a hypothesis rather than the margins as measurements.
- **[arXiv:2608.16357](https://arxiv.org/abs/2608.16357)** (MELD, Lovén et al., 08-17) — reconciling
  facts across federated agent memories: five outcomes (insert/merge/relate/conflict/reject), one
  auditable Patch as the sole mutation object, per-claim status CRDT reconverging 30/30 on
  partition-heal where last-writer-wins manages 11/30. The principle is the part worth stealing —
  **a detected contradiction is preserved for adjudication, never silently resolved** — and it is
  the same rule noteboard follows by keeping revisions.
- **[arXiv:2608.16393](https://arxiv.org/abs/2608.16393)** (Ying et al., 08-17, Tencent) — 14,560
  controlled indirect-injection executions over 16 channels. For inber: **the skills channel in file
  mode reaches 16.0% attack success**, hidden Unicode in file mode 25.5%. Corroborates the 08-13
  `2608.11888` skill-harm entry from a second vendor's harness.
- **[arXiv:2608.15242](https://arxiv.org/abs/2608.15242)** (LongRCA Bench, Zhang et al., 08-15,
  cs.SE) — 1,140 **real** failed trajectories, no injected errors, median **145 steps**, human
  labels for responsible role and earliest decisive root-cause step. The number to keep is the
  difficulty one: the strongest baseline reaches **13.2% exact root-step accuracy**. Automated
  blame-assignment over long agent traces does not work yet.
- **[arXiv:2608.15703](https://arxiv.org/abs/2608.15703)** (HyMem, Wang et al., 08-16) — layers
  context so an isolated reasoning module handles subtasks **without adding intermediate traces to
  the persistent planning context**. ⚠️ Web/QA, not coding.
- **[arXiv:2608.16798](https://arxiv.org/abs/2608.16798)** (ClawGym II, Song et al., 08-17) —
  black-box RL *through* a harness via a serving proxy at the model boundary. Relevant here only as
  evidence that the harness has become a training surface and not just a runtime.

## 8. Channel notes — and one correction to this file

**Prompt caching / KV-cache reuse: ninth consecutive empty sweep.** Dedicated search returned only
papers already recorded (`2607.19214`, `2601.06007`, `2607.21604`) plus two out-of-window items
(`2601.08343`, `2603.13289`). AgentSysBench's tool-result caching above is a different cache and
should not be filed under this thread. The exact phrases `"agentic software engineering"`,
`"multi-agent orchestration"`, `"context compaction"`, `"tool use reliability"` and
`"episodic memory"` each returned **0** in-window hits; a complete enumeration of cs.SE and cs.MA
for the window surfaced nothing the phrase queries had missed.

**Blogs: ninth consecutive empty sweep.** Nothing in window from Anthropic, Google DeepMind, Meta AI
or OpenAI; HuggingFace has published nothing since the two 08-13 posts already logged.

⚠️ **Correction to the 08-17 entry.** It reported "Anthropic engineering — latest remains
2026-04-23". That is wrong. `anthropic.com/engineering` carries **"How we contain Claude across
products"** — process sandboxes, VMs, filesystem boundaries and egress controls across claude.ai,
Claude Code and Cowork — published **May 2026**, canonical URL
`https://www.anthropic.com/engineering/how-we-contain-claude`. It is out of window for every sweep
since, which is why no sweep filed it, but it is on-topic for the sandbox and permission thread this
file tracks and **no entry here has ever recorded it**. Worth reading independently of any window.

**Frontier check for the next sweep.** The 08-17 heuristic — a sweep dated D reaches submissions up
to about D-2 — held exactly: this pass reaches 08-17 cleanly and 08-18 is not yet announced. Re-run
the `totalResults` control before assuming the next window is real, rather than inheriting this
sentence.

---

# Sweep 2026-08-19 — a dense window, and the strongest result says safety is a property of the *configuration*, not the model

Unlike the last nine sweeps this one is not thin. Four papers below were re-fetched and re-read
independently of the sweep agent before being written down, after a previous sweep in this file
reported papers that did not exist: `2608.17597`, `2608.17485`, `2608.16801` and `2608.16032` each
resolved to a live abstract with the title, authors, date and headline numbers exactly as recorded
here.

## 1. [arXiv:2608.17597](https://arxiv.org/abs/2608.17597) — HarnessRisk: attack success is 12.6%–80.9% across configurations of the *same* models (Bai, Duan, Peng, Wu, Liu, Wang, Chen; 2026-08-18, cs.CR)

Organizes harness safety into six lifecycle phases — Harness Configuration, Capability Extension,
Runtime Operation, State Persistence, Action Control, Incident Recovery — and tests 128 sandboxed
cases that pair a benign objective with an adversarial instruction planted in an untrusted workflow
artifact. Over 3 harnesses, 6 models and 14 model×harness configurations, **attack success ranges
12.6%–80.9% while utility stays 75.0%–97.6%**: the spread is driven by how the harness is
configured, not by which model is behind it. Two findings carry directly. **Harness Configuration is
the most vulnerable phase in all three harnesses** — the setup surface, not the runtime. And
recognition does not produce refusal: some configurations **detect risk in >90% of runs and still
get compromised**.

**What inber should consider:** the second finding is the one that changes a design. inber's gate is
`guard.CheckTool`, a tool-*name* switch (`guard/guard.go:158-184`), and every open todo about it
argues over defaults; this paper says the argument is worth having, because a model that notices the
risk in its reasoning still makes the call. Gate at the action layer or not at all. The first
finding points at a surface nothing here tests: the config path — bundle-store `/resolve`,
tool-store `/provision`, the agent-store row that decides an agent's tools — is an attack phase in
its own right, and it is upstream of every runtime control.

## 2. [arXiv:2608.17485](https://arxiv.org/abs/2608.17485) — KeyPooling: a shared provider credential is a shared prompt cache (Sun, Cai, Liu, Zhao, Cao, Xiao; 2026-08-18, cs.CR)

Providers scope the prompt cache to the *upstream* principal. So any relay that authenticates its
own customers and then forwards on one shared provider key collapses them into a single cache
identity. Across five open-source gateways against OpenAI and Anthropic, **none bound customers to
upstream credentials by default and all five exposed cross-customer cache reads on both providers**;
a weekly measurement found cross-account reads for 12 of 28 labels carrying 33.7% of volume. The
defense contract is the useful part: derive a cache namespace from the *authenticated identity* and
make it survive every final cache lookup **and write** — and place that split **after** the reusable
public prefix, which preserved most reuse at a **1.7–2.5% cost increase**.

**What inber should consider:** this is a live question rather than a hypothetical, because
model-store holds one credential per provider and every agent on this box runs through it. The
placement rule is the transferable engineering: a naive per-agent namespace prepended to the prompt
would destroy the shared system-prefix cache this corpus has spent months protecting, and the paper
shows you can have both by splitting after the shared prefix. Worth checking whether inber's cache
identity is ever per-agent today — `agent/read_cache_identity.go` and the BP3 placement are where
that would live.

## 3. [arXiv:2608.16801](https://arxiv.org/abs/2608.16801) — naming a coordinator agent buys nothing measurable (Destefanis, Aste; 2026-08-17, cs.AI)

Models each run as a temporal network — agents and files as nodes, messages/writes/reads as
timestamped costed edges — over **1,902 runs** varying team size, structure and file policy. Three
results. Direct messaging grows **near-quadratically** with team size until broadcast takes over.
**Shared files replace repeated 1-to-1 messaging and cut output tokens ~42% at eight agents** on
message-heavy work, while adding overhead where files already carry the coordination. And
**"naming one agent as coordinator creates no communication hub and provides no reliable improvement
in success"** — a null result that held under replication across 244 further runs in a sealed
environment.

**What inber should consider:** inber's whole spawn design is a coordinator pattern — an
orchestrator agent with `MaxSpawnDepth` defaulting to 2 — so the null result deserves an honest
reading rather than a defensive one. The paper does not say hierarchy is useless; it says
*designating* a lead produces no measurable hub and no reliable gain. The actionable half is the
first: route multi-subagent coordination through a shared workspace artifact instead of
parent↔child message passing, which is cheaper at exactly the fan-out inber supports. Note also the
incidental finding — agents reached for hidden grading material in four fifths of sealed runs — as
a caution about any benchmark this repo runs on itself.

## 4. [arXiv:2608.16032](https://arxiv.org/abs/2608.16032) — a memory row must never be able to assert that a safety step ran (Rahman, Kim; 2026-08-17, cs.CR)

The FARMA attack writes fabricated entries into an agent's reasoning memory claiming a required
safety step already executed, so the agent skips it — no malicious command anywhere. The
wording-based defense SENTINEL is defeated on the first try by asking a model to reword the forgery,
and there is a capability paradox: attack success is **98–100% on GPT-4o/4o-mini against 44% on
Llama-3.1-8B**, because the stronger model follows the reworded claim more faithfully. PoEM's remedy
is structural rather than semantic — **an HMAC-chained, tamper-evident ledger of safety steps that
actually executed, writable only by the trusted action layer** — driving attack success to **0%**
with 0% false positives in eight of nine cells, against SENTINEL wrongly blocking 33–50% of
legitimate operations.

**What inber should consider:** this is the strongest available argument for two todos already open
against this codebase — *"inber memory_save fabricates provenance the two HTTP paths were fixed to
leave empty"* and *"memory-store: the model chooses its own provenance, so a poisoned page can write
a memory stamped `user`"*. Those were filed as correctness defects; PoEM reframes them as an
exploitable class, and supplies the fix's shape: the answer is not to validate memory content but to
put executed gated actions in a **separate append-only record written only by the tool-execution
path**, and have the gate consult that record rather than the conversation. It also warns off the
tempting cheap version — a model-judged "does this memory look forged" check is the defense measured
here as both bypassable and a heavy over-refuser.

## 5. Tier two — verified, narrower, no action yet

- **[arXiv:2608.11977](https://arxiv.org/abs/2608.11977)** (Chen et al., 08-12) — tool failures
  produce a near-universal robustness gap across 7 models; a purely **runtime** intervention,
  Bayesian Tool Memory, gains **+16.8 points with no retraining**. The cheapest idea in this sweep:
  keep a per-tool success/failure posterior in the SQLite memory store and feed it back as recovery
  context, so a flaky MCP server is switched away from rather than retried.
- **[arXiv:2608.17433](https://arxiv.org/abs/2608.17433)** (Lin et al., 08-18) — harness provisioning
  as resource-matching, with map-guided escalation from a minimal task-specific harness to full
  provision only on validation failure (0.652 → 0.715 at far fewer tokens). This is bundle-store's
  thesis with an algorithm attached: `/resolve` could return the minimal bundle and widen on failure.
- **[arXiv:2608.15703](https://arxiv.org/abs/2608.15703)** (HyMem, Wang et al., 08-16) — already
  noted in the 08-17 sweep as web/QA rather than coding, but the specific claim now looks load-bearing
  for compaction: isolate the reasoning module so intermediate traces are **never written back into
  the persistent planning context** (+6.1 / +4.7 points). The argument against one flat compaction
  pass over one transcript.
- **[arXiv:2608.14036](https://arxiv.org/abs/2608.14036)** (Jiang et al., 08-14) — **procedural
  anchoring is 65.7% of skill benefit against 4.5% for explicit knowledge injection**, and actual-use
  precision collapses **29.6% → 3.3% as the skill pool grows 5 → 100**. Both halves bear on
  skill-store: write skills as procedures, and resolve a task-scoped subset rather than exposing a
  catalog past 100 entries.
- **[arXiv:2608.13662](https://arxiv.org/abs/2608.13662)** (Adam, 08-13) — symbolic project memory
  scores **0.98–1.00** on supersession/set-completeness/negation queries against **6–27%** for a
  production vector-memory tool at equivalent token cost. A real argument against memory-store's
  pure-vector recall for exactly the queries a coding agent asks.
- **[arXiv:2608.12990](https://arxiv.org/abs/2608.12990)** (LycheeMemory V2, Li et al., 08-13) —
  segment-level rather than turn-level consolidation cuts memory-construction tokens **86.0%/75.9%**
  with no query-time increase. Direct guidance on consolidation cadence.
- **[arXiv:2608.18066](https://arxiv.org/abs/2608.18066)** (Ye, Li, Pruksachatkun, Zhang, Wu, 08-18) —
  agent evaluation is inherently noisy and a self-improvement loop **amplifies** that noise; reported
  gains depend heavily on task order, which acts as a hidden curriculum. Read before believing any
  "the agent learned from memory" claim measured in a single run.
- **[arXiv:2608.14876](https://arxiv.org/abs/2608.14876)** (Day et al., 08-14) — workspace topology
  as a measurable attack surface; **highly modular environments show significantly lower attack
  success**. Argues forge should hand a subagent a narrow root rather than the whole repo, and warns
  that a workspace already containing prior agent artifacts invalidates an injection benchmark.
- **[arXiv:2608.13900](https://arxiv.org/abs/2608.13900)** (Sun, Wang, Li, 08-14) — ACID reinterpreted
  for agents; semantic dependency-aware isolation, +10.6% over baselines including Claude Code. The
  relevant piece for inber is that parallel subagents sharing a worktree need a dependency-aware
  scope lock, not just separate slots.
- **[arXiv:2608.16178](https://arxiv.org/abs/2608.16178)** (He, Yu, 08-17) — signed hash-chained state
  deltas replacing prose logs, with untrusted text held behind a digest reference: **−88.8% context
  tokens** and **zero successful prompt injections across 50 adversarial trials per configuration**.
  One change that is both the token win and the injection defense, aimed squarely at what log-store
  feeds an agent.

## 6. Channel notes

**Prompt caching: the ten-sweep drought ends.** `2608.17485` is the first genuinely new prompt-cache
paper since this file started tracking the thread, and it is a *security* result rather than an
efficiency one — which is probably why the efficiency-phrased queries kept returning nothing.

**Blogs: tenth consecutive empty sweep.** Anthropic's engineering blog still shows nothing after
2026-04-23 (the May "How we contain Claude" post noted in the 08-17 correction remains the most
recent on-topic item and is still out of window). Google DeepMind and Meta AI produced nothing
in-window. HuggingFace has one item, *"Training a coding agent using the OpenCode harness"* — a
tutorial, not research.

**Just outside the window, flagged rather than filed:** `2608.11386` *The Devil Is in the Interface*
(CodeAct interfaces: −41.6% steps, −56.3% tokens) and `2608.10934` *Understanding the Architecture of
Coding Agents*, both 08-10/11 and both more on-topic than half of tier two. Worth a read outside any
sweep window.

## Harness-watch — 2026-08-20

The 08-17 → 08-19 band is dense and mostly untouched by the 08-19 sweep, which filed papers up to
`2608.18066`. Everything below is grep-clean against this file and against the April–July files. Every
numbered entry was re-fetched from the arXiv abs page and matched on title, authors, date and headline
numbers before being written down.

### 1. [arXiv:2608.14380](https://arxiv.org/abs/2608.14380) — AgentRewind: a checkpoint that restores the workspace but not the context is not a checkpoint (2026-08-14, cs.AI)

Early errors in a long-horizon run propagate through two channels at once — the agent's context and the
environment's state — and later actions often cannot reverse either. AgentRewind records **aligned
checkpoints of agent context and controlled environment together**, so a rewind returns both to the same
earlier point and resumes carrying what the failed attempt learned. The authors build MettleBench, a
long-horizon engineering benchmark scored on task completion *and* partial checklist progress, and report
gains in both across models, execution strategies and harnesses. The design claim is the alignment, not
the snapshot.

**What inber should consider:** `checkpoint/checkpoint.go` is an honest sketch — every method returns
`ErrNotImplemented` (`:50`, `:70-109`) — and its `Checkpoint` struct carries `CommitSHA`, `TurnNum`,
`FileCount` (`:56-58`): a git snapshot of the workspace and nothing about the conversation. Implemented as
written, `Restore(num)` rolls the repo back while the transcript still describes the edits that were
undone, and the agent's beliefs desynchronize from the tree it is editing. This is the argument for making
the conversation snapshot part of `Take` before any of it ships, and for the second half too: on resume,
feed the failed attempt forward rather than discarding it.

### 2. [arXiv:2608.18050](https://arxiv.org/abs/2608.18050) — StagedWorkspace: bind every view to a content hash of the file it claims to describe (2026-08-18, cs.AI)

Formulates a **workspace-state contract**: the parsed views an agent searches, the native files it edits,
the diffs it reviews and the artifacts it submits can all refer to different versions of the same work
product, and every view should be explicitly tied to a version of the evolving state. StagedWorkspace binds
parsed records and review diffs to **content hashes of the native files as they change**. In fixed-harness
ablations, dual parsed/native access has the highest point estimate for every model tested, improving
OfficeQA Pass@1 by **8.3–12.1 points** and APEX mean rubric score by **4.7–9.2 points** over the single
view; SW-AGENT scores **63.9%** on OfficeQA with Gemini 3.1 Pro against a published same-model **29.3%**.
The paper notes coding agents already get part of this from repository contracts — which is why the gap is
worth checking rather than assuming.

**What inber should consider:** `codeindex/codeindex.go` detects change by **mtime** (`:19`, `:55`), and
mtime is not a version identity — it moves without content changing and fails to move when content does.
The contract says the index entry should carry the content hash of the file it was parsed from, so a stale
parse is detectable rather than merely unlikely. Both sites are still `// TODO: implement`, so this costs
nothing to decide now. The second half applies to `conversation/dedup_files.go` and the read/edit path: a
file summary in context is a parsed view, and nothing ties it to the version the agent is about to edit.

### 3. [arXiv:2608.18360](https://arxiv.org/abs/2608.18360) — One Gate Is Not Enough: gate order is semantics, not implementation detail (2026-08-18, cs.SE)

Agentic actions pass through more than one pre-action control — authority, resource, evidence — and each can
admit, degrade, or **remediate** an action before it runs. The paper names the hazard
**remediation-induced control coupling**: a remediation applied by one control changes the action, evidence
or context another control already judged, invalidating that judgment. It gives a remediate-and-regate
protocol restoring per-action soundness under bounded, idempotent assumptions, and shows by finite-model
checking that the two implemented remediation operators — evidence substitution and resource-budget
downroute — **do not commute**, with concrete counterexamples. A governed evidence buffer that trusts its
own most recent admitted write is the same failure at the level of state. CH1–CH5 meet their registered
decision rules across all 30 pre-registered seeds; CH6 holds under W1 but not the smaller W2, reported as
such. The authors are explicit that this is a mechanism demonstration on synthetic metadata, not a
prevalence claim.

**What inber should consider:** inber's gate is a single verdict — `guard.CheckTool` (`guard/guard.go:165`)
— alongside a separate `CheckLimits` for cost and time. That is already two controls, and permission-store
plus bundle-store's resolve would make more. The transferable result: once any control can *modify* rather
than only admit or deny, the composition acquires an order-dependent semantics that must be written down,
and a downroute that swaps a model or trims context is exactly such a modification. Filed as a cross-cutting
pattern in `agentic-design-patterns.md` 2026-08-20 §4. The evidence-buffer warning also lands on the
memory-provenance thread already open here.

### 4. [arXiv:2608.17718](https://arxiv.org/abs/2608.17718) — RGE: watch whether the trajectory still matches the task, not whether each step looks suspicious (2026-08-18, cs.AI)

The oversight question in a long run is not per-step validity but whether the evolving prefix still
corresponds to the task the user authorized. Drift accumulates quietly: right tool, plausible arguments,
every step, while the prefix moves toward a broader role, an adjacent objective, or evidence the user never
supplied. The paper defines **ontological trust** as a task-conditioned property of trajectory *prefixes*
and instantiates it as RGE, an online monitor decomposed along **Role, Goal and Evidence**. LLMs derive the
structured task and step representations; trust-state updates, projections and intervention decisions are
**deterministic**, so the output is a replayable, auditable trust trajectory rather than one judge verdict.
On a corpus from OSWorld, FinanceBench and EICU-AC covering benign runs, prefix-paired drift and
pseudo-consistency failures, the two larger estimators exceed **93% Drift F1** at **≥95.8%** benign
coverage. Pseudo-consistency is harder and depends on task completion being externally visible.

**What inber should consider:** everything in `guard/` is per-call and stateless with respect to intent —
`CheckTool` sees a tool name and an input string, `IsRepeating` sees a loop. Nothing asks whether turn 40 is
still doing what turn 1 was asked to do, which is the failure mode a `MaxSpawnDepth`-2 orchestration is most
exposed to. The engineering worth copying is the split: use the model only to *extract* Role/Goal/Evidence
per step, then update trust deterministically, so the record is replayable and a `trace/` entry can carry
it. Far cheaper than a judge, and unlike a judge it does not fail the competence bound in §9 below.

### 5. [arXiv:2608.18389](https://arxiv.org/abs/2608.18389) — A Jagged Frontier: robustness rankings do not survive a change of scaffold (2026-08-18, cs.AI)

Applies semantics-preserving transformations — control-flow rewrites, dead-code injection, identifier
renaming — to the codebase around a SWE-bench Verified / SWE-bench Pro issue, then runs each agent
repeatedly on clean and perturbed variants so the perturbation effect separates from ordinary stochasticity.
Across 2 scaffolds (mini-SWE-agent, OpenCode) × 4 models, degradation is real but modest: **up to 6.7
percentage points mean resolve-rate drop**, significant in **6 of 16** configurations. The finding that
matters is the interaction: **no single model ranking by robustness holds across scaffolds** — Qwen is among
the most robust under mini-SWE-agent and the most brittle under OpenCode — and **the simpler scaffold is the
more robust one**.

**What inber should consider:** the third result in this file pointing the same way (with `2608.08654` on
scaffolding beating interface and `2608.17597` on configuration beating model), and here the mechanism is
legible: added scaffold machinery gives superficial code variation more surfaces to perturb. Read it as a
caution against answering a reliability complaint with another layer in `engine/`. It also warns about
inber's own bench numbers in `agent-bench`: a resolve rate measured on one repo shape does not transfer, and
a per-model ranking measured under inber's harness says nothing about that model under any other.

### 6. [arXiv:2608.18167](https://arxiv.org/abs/2608.18167) — Adversarial Review: three agents beat five, and agreement is the failure mode (2026-08-16, cs.AI)

Scaling agent count yields diminishing returns on repository-level tasks, and the fashionable alternative —
treating agents as passive subagent tools — removes agent interaction entirely. Adversarial Review is the
middle: a main coding agent, a reviewer that evaluates the code, and a **critic that audits the review
through structured disagreement** before the main agent edits. On LiveCodeBench, AR gets the highest pass
rate of the tested methods **while using three agents against a five-agent baseline**. On SWE-PRBench, naive
AR exposes a **false-consensus failure mode** — the agents converge on agreement without sufficient evidence
— and a single prompt iteration making disagreement explicit then achieves the highest F1 tested.

**What inber should consider:** the constructive counterpart to `2608.12895` (a reviewer on the same model is
not a second opinion) and `2608.16801` (naming a coordinator buys nothing). For `server/spawn.go`, the cheap
version is a review subagent required to cite specific evidence per objection, plus a pass that rejects
unsupported agreement. Filed as a cross-cutting pattern in `agentic-design-patterns.md` 2026-08-20 §4.

### 7. [arXiv:2608.17188](https://arxiv.org/abs/2608.17188) — deliberately mixing low-relevance items into a prompt improved relevance judgment (2026-08-17, cs.CL)

A practitioner report from a production dashboard extracting work items from meetings, email and chat. Six
patterns — context stratification, fetch-once/process-locally, schema-contracted prompts, token-aware
fallback chains, semantic caching, inter-agent communication compression — cut measured cold-load latency to
**61–116 s** from an operational baseline of roughly 3.5–10.5 minutes at an **estimated 60–70% token
reduction**. The interesting part is a controlled context-composition study: **2,420 trials across 11 model
configurations** over 661 anonymized items, prompt held at a fixed ten items. Replacing some high-relevance
items with same-domain *low*-relevance ones **improved** relevance-score concordance on the targets —
**+0.077** for the 50:50 signal/noise condition over 100% high-relevance (Cohen's d = 0.49, Holm-adjusted
p < .001, n = 220). The author calls this relevance-contrast context and is careful that the cells are not
independent and this is a within-corpus descriptive comparison, not a population inference. A Fusion-of-N
follow-up found learned synthesis did not beat a mechanical set union of item IDs.

**What inber should consider:** every compaction and retrieval path in `conversation/` optimizes for purity
— keep the relevant, drop the rest — and this is the first measurement in this corpus suggesting purity is
not free, because a model calibrates relevance against contrast it can only see if the contrast is present.
Treat it as a hypothesis worth a cheap A/B in `conversation/summarize.go`, not a rule: single author, one
corpus, one task shape. The negative result is the safer takeaway and is directly reusable — a mechanical
union beat learned synthesis, which argues against adding a model call to merge retrieved sets in `memory/`.

### 8. [arXiv:2608.10178](https://arxiv.org/abs/2608.10178) — One Recipe, Many Harnesses: a harness closes the gap between what a policy can do and what it does (2026-08-10, cs.SE)

Holds one self-evolution recipe fixed across 8 languages (Multi-SWE-Bench) × 3 base models and analyzes what
the evolved harnesses encode, rather than reporting aggregate gain. The recipe routes every edit through a
**typed failure signal** and records it as a **falsifiable contract**, so each modification stays
attributable after evolution. Four findings: the loop beats both a minimal seed and the hand-designed
mini-SWE-agent scaffold in most cells but has **two null regions**; gains compensate *recoverable execution
defects*, and where defect mass is near zero the gain is near zero; evolved harnesses share an abstract
playbook across languages but instantiate it with **almost disjoint ecosystem machinery**; and the shared
core distills into one universal harness while an ecosystem margin resists transfer.

**What inber should consider:** the second finding is a diagnostic inber can apply without adopting any of
the machinery — measure the recoverable-execution-defect mass in `agent-bench` trajectories first, because
where it is near zero no amount of harness work will move the number, and the honest conclusion is to stop.
The third and fourth bear on bundle-store: they are evidence for exactly its split, a transferable core
bundle plus a per-repo-signature margin that repo-store's tags select, and against a single global bundle.
The framing — the harness as a legible compensation layer for a specific model's behavioral gaps — also
implies inber's defaults are tuned to whichever model was in front of them when they were written.

### 9. Tier two — verified, narrower, no action yet

- **[arXiv:2608.18852](https://arxiv.org/abs/2608.18852)** (SkillGate, 08-19) — names **selector credit
  starvation**: under a broadcast sequence-level advantage, the few tokens naming a chosen skill carry a
  vanishing share of the loss and inherit increasingly wrong-signed credit as trajectories lengthen, so a
  correct skill choice is punished whenever the execution after it fails. Splitting credit into disjoint
  channels lifts a 9B policy **40.8% → 53.2%** on a 16-candidate slate **while reading fewer skills**. The
  training method is not for inber; the framing is — skill *selection* is a mid-episode decision no signal
  currently trains, another argument for skill-store resolving a scoped slate rather than exposing the
  catalog.
- **[arXiv:2608.18719](https://arxiv.org/abs/2608.18719)** (08-19) — a reference-free LLM judge is a latent
  solver, so its ability to evaluate is bounded by its ability to solve. Closed-form ROC-AUC bound in judge
  competence `c` and answer-space size `k`, a necessary condition **c > 1/k**, and the finding that
  benchmark accuracy overstates the competence that matters. Read before putting any model-judged gate in a
  loop here — including the tempting "does this memory look forged" check `2608.16032` already warned off.
- **[arXiv:2608.18931](https://arxiv.org/abs/2608.18931)** (08-19) — first compute-normalised comparison of
  five test-time-scaling families on open-ended tasks. Exploration scales fine; **exploitation breaks**:
  reward models correlate **ρ ≈ 0.12** with true quality, making selection near-random at any budget, and
  tree search amplifies it through diversity collapse. Only synthesis across candidates helps, recovering
  ~40% of available quality. Anything in inber that generates N candidates and picks one is buying the
  broken half.
- **[arXiv:2608.17148](https://arxiv.org/abs/2608.17148)** (08-17, cs.CR) — *authorization before context*:
  one anti-monotone audience-membership rule applied at the memory-to-context transition, each item carrying
  the audience present when recorded, viewer set read from channel metadata and **failing closed to public
  when ambiguous**. Proves a poisoned memory cannot widen its own audience. Evidence is **preliminary and
  synthetic** and it is a non-archival workshop paper, but the invariant is the right shape for the
  memory-provenance defects already open against `memory/` and memory-store.
- **[arXiv:2608.18398](https://arxiv.org/abs/2608.18398)** (LEDGER, 08-19) — argues the bottleneck has moved
  from producing outputs to auditing them, and raw event visibility is not enough: layered trace graphs group
  records into evidence and workflow nodes with **typed semantic edges connecting a claim to the actions,
  artifacts and checks that support it**. A concrete target shape for what `trace/` emits and log-store
  stores, beyond a flat event list.
- **[arXiv:2608.17684](https://arxiv.org/abs/2608.17684)** (08-18) — audits three self-evolution methods in
  simulated e-banking. SkillOpt raises benign utility **0.741 → 0.837** while exposure to injected content
  rises **0.820 → 0.943** and unauthorized state changes reach **0.685**; ASR rises even though conditional
  attack success *after* exposure falls. Capability and attack surface grow together, and accuracy alone
  hides it. Also records an artifact/executor mismatch costing 0.319 vs 0.756 utility — a caution for any
  skill written against a different tool envelope than inber's.
- **[arXiv:2608.18351](https://arxiv.org/abs/2608.18351)** (08-18) — least-privilege post-training for
  terminal/MCP agents: **98.48% safe success across 2,896 episodes** against 64.36% for the base policy,
  excess-authority events **4.56% → 0.79%**. Not actionable here (inber does not train), and the paper says
  so itself: learned restraint "does not replace permission gates and sandboxing." Filed for the
  six-dimension pre- and post-execution audit rubric, reusable as an evaluation without any training.
- **[arXiv:2608.17588](https://arxiv.org/abs/2608.17588)** (TRUSS, 08-18) — generates skills behind a static
  gate of nine safety properties plus a **shadow agent in a controllable execution environment with brokered
  tools**, linking observed violations back to the responsible skill content. Reports 100% precision/recall
  on vulnerability detection over 168 SkillInject artifacts and lifts task effectiveness 17.11% → 52.94%.
  The transferable half for skill-store: inspecting the artifact is not enough — run it behind a broker and
  watch what it calls.
- **[arXiv:2608.18280](https://arxiv.org/abs/2608.18280)** (08-18) — issue-resolution difficulty is
  **substantially predictable from static task features (AUC = 0.863)**, driven mainly by patch fragmentation
  and repository scale. Enables difficulty-controlled benchmark construction; relevant to reading
  `agent-bench` results, where a score change may be a task-mix change.
- **[arXiv:2608.14711](https://arxiv.org/abs/2608.14711)** (08-11) — coding-agent benchmarks misapply pass@k
  by setting `n` to the number of unit tests in one submission rather than the number of independent
  rollouts. On a synthetic benchmark the misapplied metric inflates scores by **0.85–0.97 absolute**
  (0.96–0.98 reported vs 0.00–0.12 corrected), and a single-rollout proxy does not substitute
  (Spearman ρ = 0.417). Worth checking against whatever `agent-bench` computes.
- **[arXiv:2608.17756](https://arxiv.org/abs/2608.17756)** (D²ACCI, 08-18) — memory's multi-stage pipeline
  makes failures hard to localize, so end-to-end scores say an error happened but not which stage caused it.
  A diagnostic gate promotes / feature-flags / rejects each memory change on paired evidence and
  protected-slice monitoring; enriched traces reach **98–100% DCR@3 against 0% for results-only logs**, and
  one component (BM25/RRF) survives only as a monitored flag — a distinction aggregate evaluation cannot
  make. The protocol to copy if memory-store's retrieval is ever tuned.

### 10. Channel notes

**KV-cache: two more hits, both excluded by the standing filter.** `2608.15584` (GraniKV, 08-16) —
asymmetric paging granularity, a contiguous HOT pool for the long shared prefix and a token-level COLD pool
for per-request suffixes, **2.16×/1.98×/1.57×** output-token throughput at a 16K shared prefix, and **1.95×**
under heterogeneous multi-agent serving where batch-global cascade collapses to parity. `2608.07855`
(CommitKV, 08-08) — lifecycle-aware eviction distinguishing *dormant* pages from pages whose role has
*completed*, by comparing a page's deletion effect before a tool-call commit and after its observation is
incorporated. Both are serving-stack mechanisms requiring ownership of the KV cache, so the 2026-08-10
standing filter applies unchanged: inber reaches models over the Anthropic Messages API and an
OpenAI-compatible path, where its only lever is `cache_control` breakpoint placement on a byte-identical
prefix. Recorded so a future sweep does not read them again. CommitKV's *distinction* is the part worth
keeping, and it applies above the API too: `conversation/` has no notion that a tool observation whose commit
has landed is retirable in a way a merely-unreferenced message is not.

**Blogs: eleventh consecutive empty sweep.** Anthropic engineering still shows nothing dated after
2026-04-23. HuggingFace's August posts are Strands/LeRobot deployment, ICML reproduction, distillation cost
and voice agents — nothing on harness design. Google DeepMind, Meta AI and OpenAI produced nothing in window.
Eleven sweeps is long enough to say this channel is not merely quiet: check it monthly, not per sweep.

**Coverage, honestly.** ~995 unique arXiv records screened — a complete category pull of cs.SE (61), cs.CL
(181), cs.CR (108), cs.AI (441) and cs.LG (393) for `submittedDate` 2026-08-17 → 2026-08-20, plus nine
targeted keyword queries spanning 2026-08-06 → 2026-08-20. 396 keyword-matched after deduping against the 180
arXiv ids already in `docs/papers/*.md`; 27 abstracts read in full, 12 re-verified on the live abs page. **The
2026-08-06 → 2026-08-16 band was covered only by keyword query, not by exhaustive category listing, so misses
there are likely** — `2608.14380` and `2608.18167` were both such misses from prior sweeps, which suggests
more remain. cs.MA, cs.HC, cs.DC and cs.PL were never swept by category. No listing pages were used, so the
announcement-date pagination trap did not apply.

## Harness-watch — 2026-08-21

Window 2026-07-22 → 2026-08-21. Dedupe was done by extracting all **358** arXiv ids already present across
`docs/papers/*.md` and grepping every candidate below against all five files: **zero hits**. Two title
collisions were resolved rather than assumed — `HANDBOOK.md` (`2607.25398`) is already recorded as
*rejected, benchmark only* in the July file and is not re-filed, and the `SkillZip` below is **not** the
`SkillZip` the docs already hold (see the caution at the end).

### 1. [arXiv:2608.11242](https://arxiv.org/abs/2608.11242) — a standing user constraint is the first thing compaction throws away, and the fix is a second pass, not a better prompt

Defines *Session Constraints*: standing instructions like "do not delete any emails until I confirm" that
are stated once and must bind for the rest of the session. Measured across multi-turn chat, agentic
trajectories and long-horizon research, current compactors retain **17%** of them on average, and **most
compacted runs score worse than not compacting at all**. Retention varies with compactor, prompt, context
length, constraint phrasing *and* injection location — so this is systematic, not a tuning artifact. A
plug-in constraint-aware extractor running **alongside** the compactor, touching neither the compactor nor
the model, reaches **>90% retention** in all three scenarios.

This lands directly on `conversation/summarize.go`. inber's compaction hands the turns being replaced to a
summarizing model as flat text (`conversation/message_utils.go:154 messagesToText`) and trusts the summary
prompt to keep what matters. The file already knows this class of failure in one specific instance — the
comment at `conversation/message_utils.go:146-153` explains that `is_error` had to be carried structurally
because *"rendered without the flag, `make: *** [all] Error 2` after forty ok lines reads to the summarizer
as a clean build"*. A session constraint is the same problem one level up: a single line of user text whose
force does not survive being summarized.

**What inber should consider:** a second extraction pass over the pre-compaction span that pulls
session-scoped constraints out **verbatim** and re-injects them into the post-compaction system block, rather
than a stronger instruction in the summarizer prompt. The paper's own result is that the co-pass works and
prompt-tuning does not.

### 2. [arXiv:2608.19303](https://arxiv.org/abs/2608.19303) — the recovery-tool list is the active ingredient, not the diagnosis

Targets the failure inber has no defence against: a tool call that returns **well-formed but wrong** — a
cached error page, a negative price — rather than failing. Outcome contracts mined from task-disjoint traces
or public schemas raise ToolMaze completion from **10.9% → 28.1%** across four models in two provider
families, and τ-bench retail by **+14.0 and +12.0 points**. The ablation is what makes it worth filing: the
gain lives entirely in the **list of alternate tools** named in the receipt. Remove that list and the gain
disappears; restore it and it returns. Diagnostic verbosity and timing detail change nothing.

inber's version of this failure is already documented and already worse than silent: `cline.md:684-712`
records that `tool-store/tools/shell.go` returns `(text, nil)` on a non-zero exit, so a failed command is not
merely wrongly-shaped — it is reported as a success, and `is_error` and `Turn.ConsecutiveErrors` never move.

**What inber should consider:** a post-execution check keyed off the tool's own JSON schema, appending a
non-binding receipt on violation. **The receipt must name specific alternate tools by name** — that is the
measured ingredient, and a receipt that only describes what went wrong buys nothing.

### 3. [arXiv:2608.19662](https://arxiv.org/abs/2608.19662) — recurring tool schemas in varying order defeat prefix caching outright, and inber already gets this right

The mechanism (composition-invariant per-resource KV blocks — **82.3% vs 82.4%** Inv-F1 at **3.655×** TTFT
and **92.43%** less KV memory) is serving-side and falls under the 2026-08-10 standing filter: inber owns no
KV cache. The **diagnosis** is above the API and does apply. Agents re-encode tool and skill schemas that
*"recur across requests in different combinations and orders,"* and any reordering or conditional inclusion
of a single tool invalidates the whole cached prefix behind it.

**Checked, and inber is clean — recorded so a later sweep does not re-derive it.** Every path that builds the
tool block iterates a **slice**, not a map: `buildConfiguredTools` ranges `e.AgentConfig.Tools`
(`engine/build_tools.go:27`), `buildMemoryTools` ranges the same config slice
(`engine/build_tools.go:123`), and `tools.All()` returns a fixed literal in a fixed order
(`tools/tools.go:83-91`). inber also already states the rule explicitly, one layer up, in the one place a map
range would otherwise have leaked in: `server/spawn_tools.go:64-71` sorts the agent names inside the
`spawn_agent` description because *"Ranging a Go map gives a fresh permutation on every call, so two sessions
of the same agent, a fork and its parent, and a session and its resume would each hash a different prefix and
share no cache entry."*

**What inber should consider:** nothing to fix; something to keep. The discipline is currently upheld by
three independent call sites each happening to use a slice, plus one comment. `tools/registry_order_test.go`
pins part of it — the standing risk is a future conditional inclusion (a tool added only when a workspace
exists, say), which changes the member set rather than the order and breaks the prefix just as thoroughly.

### 4. [arXiv:2608.14838](https://arxiv.org/abs/2608.14838) — maximizing retrieval recall *lowers* issue resolution, and the authors map the boundary honestly

Execution-graded single-flag ablation on SWE-bench Verified with a fixed 12-slot context pack and no search
tools. One-chunk-per-file deduplication raises gold-file presence (**0.878 vs 0.806**) and *lowers* resolve
rate: turning it off gains **+7.6pp** (39.2% → 46.8%, n=500, McNemar exact **p=0.0003**), with a
pre-registered open-weights replication at +3.6pp (p=0.0133). It **reverses** on BM25 (−3.2pp) and is a
powered null under unrestricted-Read agents — the paper says so rather than burying it, which is why the
result is usable.

**What inber should consider:** the boundary condition is the finding. inber's agents have unrestricted read
tools, which is the arm where the effect is a powered null — so this changes nothing today. It becomes
binding the moment `codeindex/` grows into a fixed-budget context pack: do not hard-deduplicate by file, trade
file breadth for within-file depth, and A/B the packing policy against **task success**, never against
recall@k. Filing it now because the natural instinct when building that pack is to maximize gold-file
coverage, and that instinct is measurably wrong.

### 5. Tier two — verified, narrower, one line each

- **[2608.19993](https://arxiv.org/abs/2608.19993)** — skill-document loading under a token budget as monotone-submodular benefit minus context penalty; first **(1−1/e, 1)** bicriteria guarantee, **0.73 task success vs 0.20–0.52** for released skill routers and text retrievers, on **28% fewer tokens**. → the skill-store → bundle-store `/resolve` path should use a budgeted submodular selector; the redundancy penalty is exactly what top-k lacks.
- **[2607.27267](https://arxiv.org/abs/2607.27267) FAVA** — permission IR → evidence-backed permission graph → SMT authorizer, **90.5%** decision compliance across three benchmarks, and a denial returns a **counterexample**. → this is the shape permission-store's unbuilt steps 2–7 should copy; a denial carrying a counterexample is machine-actionable where a boolean is not.
- **[2608.11274](https://arxiv.org/abs/2608.11274)** — position paper with four released audits behind it, including a title-level pass over all **28,560** NeurIPS/ICML/ICLR 2023–25 papers showing an **8–12×** imbalance between training-time and deployment-time safety work. Splits the contract into a preventive face and an **evidential** face. → inber has only the preventive half; the evidential half means a session cannot report "done" without an attached execution artifact — a `conversation/` and tool-result schema change, not a prompt change.
- **[2608.03297](https://arxiv.org/abs/2608.03297)** — naive middle-drop truncation collapses monotonically (Holm-corrected p<0.05 in all eight cells) purely because answer-bearing content survives in **fewer than 1%** of samples at 25% retention. → any inber experiment measuring "does compaction hurt?" by dropping the middle is measuring middle-removal luck, not context length. Directly relevant to how `conversation/staged.go`'s head-drop would be evaluated.
- **[2608.08389](https://arxiv.org/abs/2608.08389)** — **where** you prune dominates **which** scoring rule you use; heuristics cut tokens up to **73%** with little quality loss and a learned value model never dominates. → prune at the point a tool result **enters** the conversation, not at the compaction boundary; late pruning only cleans the final synthesis window.
- **[2607.24882](https://arxiv.org/abs/2607.24882)** — 427 samples over 25 repos; **RepoMap wins budgeted context yield at 8K tokens**, and logged agent trajectories miss every gold file on **27–35%** of samples. → seeding a session's first window from a retriever beats blind exploration, and structure beats embeddings at inber's realistic budget.
- **[2608.06811](https://arxiv.org/abs/2608.06811) PMCoder** — plan phase conditions memory retrieval and memory-derived trajectory statistics drive stuck detection; **+5.0pp** on SWE-bench Verified, replicated ≥+2.8pp across three models, specifically reducing repeated failed actions and context exhaustion. → memory-store retrieval should take a phase label as a query parameter, and inber should compute a repeated-action statistic as a replan trigger. Note `guard.RecordToolCall` (`guard/guard.go:209`) already computes exactly that statistic and, per its own comment, **has no caller** — the write exists and the reader does not.
- **[2608.19564](https://arxiv.org/abs/2608.19564)** — memory-commitment benchmark (persist / in-context-only / re-verify / ask), κ=0.962. Models verify but do not ask: bare Qwen asks on **0/12** clarification items. A policy prompt cuts erroneous persistence 0.243 → 0.100 (p=0.038) while clarification recall stalls at 0.333, and **label↔tool-call agreement is only 57% (Claude) / 23% (Qwen)**. → a durable write to inber's memory store needs an explicit four-way commit decision in the **tool schema**, and any eval must grade the tool call, not the stated intent — they disagree half the time.
- **[2607.22445](https://arxiv.org/abs/2607.22445)** — role ceilings + task-context classifier + combination prohibitions; co-developing dataset and policy cut ceiling violations **46 → 3 (93%)**, and it supports an **observe-only** mode. → the observe-only mode is the cheap first move for permission-store: log every call against a task-derived ceiling before enforcing anything, and the mismatch log is itself the tuning signal.
- **[2608.20169](https://arxiv.org/abs/2608.20169)** — variance-weighted sampling concentrates evaluation on tasks where candidate harnesses *disagree*, with a sampling-probability correction so partial evaluations stay comparable; matches full-set search at **80% fewer evaluations**. → directly applicable to `agent-bench`.
- **[2607.25635](https://arxiv.org/abs/2607.25635)** — 1,723 MCP-consuming applications mined from GitHub: 85.2% converge on config files and 81.1% on official SDKs, but only **37.2%** gate tool execution behind a blocking approval step. → baseline evidence that tool-store's `POST /provision` should ship approval-gating defaults **on**, because the ecosystem default is off.
- **[2608.11632](https://arxiv.org/abs/2608.11632)** — untrusted components propose typed changes against an *exact predecessor head or typed absence*; model-checked over **2,808,230 states and 5,526,474 transitions, zero invariant violations**. → the compare-and-swap-on-predecessor-head discipline is what stops a background sub-agent stale-overwriting a session's memory record; inber's memory writes carry no such precondition. Surfaced by the `cs.MA` category pull the 08-20 sweep flagged as never swept.

### 6. Screened and rejected — including one that argues against a standing intuition

- **[2608.14992](https://arxiv.org/abs/2608.14992)** is worth reading *because it fails*. An exploratory arm found 14/24 false-code adoption from a tool-result record vs 0/22 from an assistant assertion; a preregistered replication reproduced the gap (7/24 vs 0/24, Fisher p=0.0047) — but the tool-result rate itself fell **14/24 → 7/24 across runs four days apart**, and a second preregistered study with a live text control found inline text sufficient in **60/60** trials vs 57/60 for the tool result, failing the registered superiority criterion (p=1). One model, one synthetic template. **File as a caution against the intuition that tool-result framing is inherently more dangerous than inline text — that is not established**, which bears directly on how much weight to put on the delimiter-spoofing finding filed this sweep.
- **[2608.19677](https://arxiv.org/abs/2608.19677) CacheRoute** — 64.1% → 93.2% served KV hit rate, 2.3× QPS on 60 H100s. Standing serving-stack filter; inber owns no KV cache. One transferable line, from their own caveat: gate affinity changes with a shadow replay, not workload statistics.
- **[2608.17528](https://arxiv.org/abs/2608.17528)** and **[2608.17393](https://arxiv.org/abs/2608.17393)** — agentic RL; inber does no training. One incidental observation worth keeping: both must recompute token-level logprobs *"even under harness-side compaction or re-serialization"* — i.e. compaction breaks token identity, which is also why a compacted prefix cannot be cache-hit.
- **[2608.09802](https://arxiv.org/abs/2608.09802)** and **[2607.28587](https://arxiv.org/abs/2607.28587)** — benchmark hygiene: ~60% of unsolved SWE-bench Verified instances have flawed tests, and 13.6% PR-issue misalignment measured independently. Relevant only if inber quotes Verified numbers.
- **[2608.03327](https://arxiv.org/abs/2608.03327)** — GUI-specific, but the context rule generalizes: dropping the screenshot made redundant by a successful tool call and halving image history reaches **37.8% vs 33.0%** at **53%** of input cost.
- ⚠️ **[2608.11079](https://arxiv.org/abs/2608.11079) SkillZip — do not file blind.** The docs already hold a *different* paper called SkillZip at `2608.05604`. Same name, different abstract (MDL over a skill contract plus residual, vs contract-preserving graph compression). Resolve which is which before filing either.
- **[2608.19263](https://arxiv.org/abs/2608.19263)** self-described work in progress, validation listed as future work; **[2608.12440](https://arxiv.org/abs/2608.12440)** n=1 self-reported case study with evidence published as 1,500+ pages in French — a cost datapoint, not a result. Also screened and dropped as off-target despite matching keywords: `2608.17007`, `2608.19901`, `2608.11878`, `2608.19982`, `2608.12172`, `2608.08131`, `2608.08995`, `2608.18177`, `2607.25090`, `2607.23884`, `2608.04746`, `2608.00814`, `2608.12218`, `2608.18933`, `2608.15071`, `2608.18580`, `2608.18565`, `2608.19799`, `2608.10622`. `2608.19741` (Thinkingbox) reports a good reliability number — **65.36% pass@1 vs 25.25% pass^20** — on a business-workflow benchmark; cite the pass^k gap, do not adopt the bench.

### 7. Channel and coverage notes

**Blogs: twelfth consecutive empty sweep.** `anthropic.com/engineering` still tops out at 2026-04-23 for
dated posts; the undated featured "How we contain Claude across products" is 2026-05-25 and the 08-17 sweep
already recorded that correction (line 2461). HuggingFace's `agent-glossary` harness-vs-scaffold post is
2026-05-25, out of window. DeepMind, Meta AI and OpenAI produced nothing in window. The standing
recommendation — check this channel **monthly, not per sweep** — holds for the twelfth time and should now be
treated as settled rather than re-tested.

**Coverage.** 86 arXiv queries plus **full category pulls of `cs.MA` and `cs.PL`**, which the 08-20 sweep's
own coverage note flagged as never swept by category — that pull is what surfaced `2608.11632`. 847 records
with `published ≥ 2026-07-20`, 655 surviving dedupe, 60 abstracts read in full. **`cs.HC` and `cs.DC` remain
keyword-swept only, never category-pulled** — that is the gap this sweep did not close, and it is the same
kind of gap that hid `2608.14380` and `2608.18167` from earlier sweeps.

# 2026-08-22 sweep

**The coverage gap the 08-21 sweep flagged is closed, and it was worth closing.**
That sweep's own note recorded `cs.HC` and `cs.DC` as *"keyword-swept only, never
category-pulled"*. Both are now category-pulled and provably complete for
2026-08-14 → 2026-08-20: a range query returns `totalResults=119` for `cs.HC` and
`71` for `cs.DC`, and the parsed in-window set is exactly 119 + 71 = **190
records**. Two of the four keeps below sat in `cs.DC` and would not have been
reached by the `cs.SE`/`cs.CL`/`cs.AI`/`cs.LG` pulls every prior sweep ran.

⚠️ **Method note for the next sweep, and it is the kind that silently voids a
whole sweep.** `http://export.arxiv.org` now answers **301 with a zero-byte
body**. The plain-`http` curl in the harness-watch job prompt therefore yields
**zero entries and no error** — a sweep that used it would truthfully report
"nothing worth filing" having queried nothing at all. Use `https://` **and**
`-L`. Confirmed independently by both arXiv sweeps this run. The API also
rate-limits hard after a few hundred records — verification queries came back
empty for several minutes after a 400-record category pull — and
`https://arxiv.org/abs/<id>` is the reliable fallback for spot-checking one id.

### 1. [arXiv:2608.20195](https://arxiv.org/abs/2608.20195) — the documentation agents actually read is the instruction file, and "actionable, verifiable docs" describes no measured behaviour

Behaviour-grounded study over two public datasets, both large: **557 agentic
coding sessions from SWE-chat (94,813 development events, 3,033 documentation
interactions)** and **33,097 agentic pull requests from AIDev (690,260 classified
file-level change records)** — figures confirmed against the abstract directly,
not a search snippet. Agent instruction files plus agent working notes are
**60.5%** of all documentation interaction, against **10.6%** for classical
technical documentation and **1.3%** for API references. Three assumed mechanisms
fail: the read→act adjacent transition probability is **0.002** (adjusted OR 1.33
[1.09, 1.62], which the authors call "unresolved" rather than claiming); **zero**
documentation-based validation events were observed, so "verifiability" describes
nothing anyone measured; and documentation is the first recovery move in only
**5.4%** of 2,034 failure episodes. It carries a Threats to Validity section, uses
cluster-bootstrap/GEE with fixed seeds, has an explicit "implications our data do
not support" section, and refuses to rank recovery strategies off n=11 — the
discipline that made it worth reading.

**What inber should consider:** this reprices where instruction context pays. The
argument is to spend on the instruction surface — inber's per-agent system prompt
and agent-store's stored config and memories — over any effort to make tool
descriptions or repo docs "verifiable", which is the intuitive move and is
unattested. Two harness-level consequences follow: reads chain to further reads
(transition probability 0.270) while *follow-reference* is entirely unattested, so
anything `codeindex/` ever emits should be locally self-contained rather than
cross-linked; and agent working notes are 25.1% of interactions with agents
writing nearly as much as they read, which makes inber's `memory` writes a
maintenance surface the harness creates and never collects. Note it cites
`2608.11095` ("Why Does CLAUDE.md Keep Growing? Catastrophic Remembering"),
already filed at line 1411 — the two belong together.

### 2. [arXiv:2608.14863](https://arxiv.org/abs/2608.14863) DDBench — for a strong model, a pre-supplied debug context buys cost, not correctness

Title confirmed on the abs page: *"Evaluating Agentic Code Repair Capabilities in
Distributed Systems"*, submitted 2026-08-14. 60 historical bugs from 13 open-source
distributed systems across 5 languages, each run under **two matched conditions** —
symptom-only vs. bounded debugging context (logs, traces, runtime state, targeted
investigation notes) — over 10 models on an identical `mini-swe-agent` scaffold
with a 450-step / $5 / 30-min cap. Frontier models cluster in the high-70s on
SWE-bench Verified and span **61 pp** here. Debug context lifts aggregate pass rate
**+18.1 pp**, and the lift splits by baseline capability: GLM 4.7 goes
**9.7% → 48.4%**, while **Claude Opus 4.6 gains zero pass rate** and instead drops
2.46M → 0.75M tokens (**−69%**) and 63.6 → 37.8 steps (**−41%**). Third finding,
honestly reported: on one instance a *faithful* debug context anchored the agent to
the wrong abstraction layer and turned Opus's only symptom-only win into a loss.

**What inber should consider:** for the frontier models inber actually runs, a
pre-collected context pack is a **cost lever, not a capability lever** — which is
the right way to justify building one, and a very different pitch from the usual.
This is the concrete argument for `codeindex/`, or a `tools/` pre-flight collector,
gathering bounded logs, traces and state up front instead of letting the agent burn
60-odd steps discovering them. It also hands `agent-bench` a design worth copying:
run every task twice, symptom-only and context-augmented, and report the
token/step delta **separately** from the pass delta — inber's model population is
the arm where only the former moves, so a single blended number would hide the
whole effect. The misleading-context case is the standing caution: the pack must be
gated, not blindly appended.

### 3. Tier two — verified, narrower

- **[2608.19557](https://arxiv.org/abs/2608.19557)** — asks directly whether an LLM
  control plane beats a strong heuristic at deadline-aware scheduling. The
  heuristic reaches **0.902** completion across 60 seeded instances (Holm-corrected
  p<0.001, best baseline 0.838) at **0.87 of a CP-SAT bound**, and single-component
  ablation traces the entire advantage to batching horizon and time-critical-first
  ordering — *not* the auction, not the per-window LLM policy. Under stationary
  load the LLM plane adds nothing; under a mid-run surge it recovers **+0.0053
  (p=0.004)** of roughly 7 points of headroom, while a non-LLM bandit adapting the
  same parameters recovers nothing (p=0.571). → the cleanest evidence available
  against putting a model in the dispatch loop of `session/` or the spawn path.
  The result is significant *and* nearly worthless, which is the useful part: an
  LLM scheduler must be justified against a measured optimum and an ablated
  heuristic, never a strawman. Domain distance is real (simulated AV edge offload),
  so file it as a prior on dispatch-layer cost/benefit, not as a mechanism.
- **[2608.18307](https://arxiv.org/abs/2608.18307) ComponentBench** — 2,910
  programmatically verified tasks, 7 models, 4 observation/action spaces, 20-step
  budget, bootstrap CIs with half-width ≤1.9% per cell. The tool-rich regime with
  DOM access beats every perception-only space for **all six** models that ran all
  four, monotonically; the largest single-model swing is GPT-5 mini at **83.1%
  (AX-tree) → 48.9% (coordinate-only)** in the same harness with only the
  observation space changed. Be precise: that is the *largest* gap, not the typical
  one — Gemini 3 Flash moves only 89.6 → 85.4. → the transferable claim is that a
  structured tool surface beats making the model parse raw output, uniformly enough
  to swamp model choice: an argument for `tools/` returning typed, structured
  results rather than pretty-printed text. And for `agent-bench`: reporting a model
  score without pinning the observation/action space is reporting a harness score.
  Caveat honestly — this is GUI/computer-use, inber has no GUI agent, and the
  authors say component competence is not validated as a predictor of long-horizon
  success.
- **[2608.14527](https://arxiv.org/abs/2608.14527)** — differential fault injection
  over 2,200+ runs; original and LLM-modernized kernels agree in all 200 paired
  injections. Narrow (Fortran/GAMESS), but the *method* transfers: validate an
  agent's refactor by injecting identical deterministic faults into both versions
  and comparing responses. Worth remembering when `guard/` needs a way to grade a
  refactor it did not supervise.

### 4. Screened and rejected

- **[2608.19551](https://arxiv.org/abs/2608.19551)** — N=73 between-subjects, MCP-backed
  CMS. AI assistance cut clicks, navigation and scrolling but **did not** cut task
  time; delegation variance is person-level (ICC = .50), not task-level. The
  tempting bullet — "users did not systematically avoid delegating higher-risk CRUD
  ops, so don't rely on the human to self-gate" — rests on an **underpowered null**
  (~24/cell across 3 modes). Cite the ICC if anything; not the null.
- **[2608.17150](https://arxiv.org/abs/2608.17150) KnowSim** (705 sessions, 73–74%
  sign agreement with human judgment) — real, but assistant calibration, not harness
  design. **[2608.16181](https://arxiv.org/abs/2608.16181) MUSE** (n=15) and
  **[2608.17834](https://arxiv.org/abs/2608.17834) AdaLens** (2 cases) — right problem
  shape for a long-running inber session, evaluations too small to act on.
- No validation reported: `2608.18312` (and `2608.18398` LEDGER already covers that
  ground, filed), `2608.16428`, `2608.14869` (n=3 pilot, comparison "planned"),
  `2608.14093`, `2608.19281`, `2608.15442`, `2608.15838`, `2608.10689`.
  `2608.14815` is a workshop CFP; `2608.15403` is design fiction.
- Standing serving/training-stack filter (inber owns no KV cache and does no
  training): `2608.15241`, `2608.17826`, `2608.15117`, `2608.19535` (also
  self-described as "a vision"), `2608.19659`, `2608.16336`, `2608.15171`,
  `2608.15762`, `2608.15533`, `2608.15531`, `2608.15473`, `2608.13263`, `2608.11152`,
  `2608.10402`.
- Off-target despite keyword match: `2608.14132`, `2608.17175`, `2608.14948`,
  `2608.16633`, `2608.13017`, `2608.12750`.
- ⚠️ **`2608.16311` and `2608.16293` are the same paper posted twice** under two ids
  with byte-identical abstracts (cooperative-game authority switching, LQ systems).
  Both rejected as off-target; recorded so a future sweep does not read it twice.
- `2608.13884` (33,228 vLLM/SGLang PRs) was **already filed and already rejected**
  as too confounded. Leaving rejected.

Six of the 190 in-window records were already filed (`2608.15127`, `2608.16357`,
`2608.16178`, `2608.18398`, `2608.19677`, `2608.13884`), each re-checked by line
number rather than trusted from an id list. 29 abstracts read in full, 6
re-verified on the live `abs` page. The remaining ~150 in-window records are HPC
kernels, federated learning, BFT consensus, LOCAL/CONGEST complexity, HRI, wearables
and health HCI — no harness relevance.

## Second pass — keyword sweep, 2026-08-17 → 2026-08-20

Run independently of the category pull above: **68 arXiv queries, 2,155 unique
records, 1,289 with `published` in window**, 139 scored harness-relevant, 41
abstracts read in full. Note the real window is only **08-17 → 08-20** — arXiv's
newest submission at sweep time was 2026-08-20T15:59Z, so 08-21 and 08-22 are
empty and the "fresh" window beyond the 08-21 sweep's reach is about one day
wide. The two sweeps independently surfaced `2608.20195` and independently
rejected `2608.19551`, which is the corroboration this pair of methods was
supposed to produce.

### 5. [arXiv:2608.19652](https://arxiv.org/abs/2608.19652) StateMemBench — memory benchmarks test recall, but the failure that matters is answering with a *superseded* fact

Defines **state tracking**: facts, constraints and decisions get revised over a
long interaction and an answer must reflect the current state. **234 multi-session
scenarios** across two conversation-length regimes, graded closed-pool so each
answer is scored as reflecting the *current* state, the *superseded* state, or
other — which separates state-tracking failure from ordinary error by
construction rather than by inference. Existing memory systems, retrieval
baselines and long-context baselines all struggle. Their StateMem method improves
current-state accuracy **1.8× (0.205 → 0.363)** over the strongest same-backbone
baseline and **1.6× (0.149 → 0.233)** over the strongest memory system on a second
model. Applied as a **single-call wrapper over six existing memory and retrieval
backends** it lifts current-state accuracy **+32 to +67 points**, and a **length-
and cost-matched control attributes +15 to +32 of those points to state structure
rather than to the added context** — that control is what makes this filable.

**What inber should consider — this is the most actionable paper of the window.**
`memory/` has exactly the architecture the benchmark breaks, and it was checked
rather than assumed: `memory/tools.go:100-105` declares `memory_save` with four
properties — `content`, `tags`, `importance`, `source` — and **no supersession
relation of any kind**. A revised fact and the fact it revises both sit in the
index, both embed near the same query, and recency is the only tiebreak.
`memory_forget` is the sole correction channel and it fires only if the agent
independently notices the contradiction and calls it. The cheap version is the
wrapper, not the architecture: a `supersedes` field on `memory_save` plus a filter
in `memory_search` that drops superseded rows — a schema change in
`memory/tools.go` and a query change in memory-store. ⚠️ It interacts with the
already-filed `2608.19564`: models verify but do not ask, and label↔tool-call
agreement is only 57%/23%, so `supersedes` must be a **tool-schema field the model
fills in**, gradeable from the call itself, not a stated intention.

### 6. [arXiv:2608.17719](https://arxiv.org/abs/2608.17719) — an aggregate benchmark win hides ~8% of items that reliably got *worse*

Three pairwise upgrades across a product model sequence, 900 public benchmark
items, **50 queries per item per model**, each item classified as reliably
improved / reliably regressed / practically equivalent / inconclusive under
**false-discovery-rate control plus a practical-significance threshold**,
calibrated against a **label-permutation null**. In all nine
migration×benchmark cells, reliable improvements and reliable regressions
coexist: edges with aggregate gains up to **7.3 pp** contain up to **8.3%
reliably regressed items**, and edges with aggregate *losses* contain up to
**10.7% reliably improved** ones. Separately, a **3.9-point instruction-following
regression under strict scoring shrinks to 0.04 points under loose scoring** —
the scorer's leniency moved the number, not the model.

**What inber should consider:** the sharpest paper here for `agent-bench`. inber
changes model defaults per agent through agent-store runtime configs and the
decision signal is a suite mean, while `engine/engine_benchmark.go` measures
*timing* rather than per-item behavioural deltas — so nothing in the tree can
currently see a bidirectional split. Emit **per-task pass/fail at n≥20 repeats
with FDR control**, and gate a model swap on "no reliably-regressed task in the
pinned set" rather than on the mean. The strict-vs-loose result is the second
half: inber's own grader leniency is an unmeasured free parameter of every model
comparison it has ever run.

### 7. Three deflationary results — each kills the obvious next feature

These are the useful shape, and filing them is cheaper than discovering them.

- **[2608.16190](https://arxiv.org/abs/2608.16190) — do not build a monitor
  ensemble.** 24 open-weight monitors, nine pretraining lineages, a **29× range**
  of detection skill. Agreement decomposes into a shared-detectability *signal*
  term and an *idiosyncratic error* term that predict ensemble gain with
  **opposite signs (Spearman −0.25 and +0.26)**, so their sum — the
  average-pairwise-correlation metric everyone minimizes — predicts gain at
  **+0.05**, and the cancellation replicates in **7 of 8** evaluations. A
  monitor's own skill predicts its agreement with the pool at **Spearman 0.84,
  n=24, permutation p < 0.0001**; at matched capability, cross-lineage panels
  detect no better (**p = 0.13**); and **no correlation-weighted selection beats
  picking the single best monitor out of sample**. The authors report the test
  failing against themselves, which is why the null is credible. → whenever a
  check in `guard/` is model-judged, use one best judge selected out of sample on
  measured skill. This also settles the tempting cheap trick of routing a gate
  through two model families "because their errors decorrelate": measurably not
  paying. Pairs with the filed `2608.18719`.
- **[2608.19760](https://arxiv.org/abs/2608.19760) — do not build LLM-judge step
  attribution.** Audits step-level credit against causal ground truth from
  **executed replay**: resample the policy's own alternatives at each decision
  point, roll forward, measure what actually changes the outcome. **None** of
  LLM-judge scores, outcome-conditioned logprob ratios, or the policy's own
  confidence beats chance at identifying causally important steps. The failure
  mode is named — implicit credit **echoes fluency** (median rank correlation
  **+0.75**, replicating at +0.70 in a second model family) while conditioning on
  the outcome adds **no causal information (partial correlation −0.004)**. A
  seven-arm **pre-registered** training experiment finds no arm reliably beating
  the untrained policy. → the obvious next feature after collecting trajectories
  is scoring each step with a model to find where a run went wrong, and it is
  measured here as indistinguishable from scoring fluency. Real step attribution
  needs replay from a checkpoint with a resampled action — which `checkpoint/`
  and forge's worktree slots make possible and no inber code does. One reusable
  number: confidence-gating an already-present judge cuts judge cost **13.1% per
  turn** — free savings, just not attribution.
- **[2608.19626](https://arxiv.org/abs/2608.19626) — a self-graded feedback loop
  mostly measures its own broken oracle.** 142 development / 114 locked external
  / 138 held-out tasks, two code models, three seeds, fault-cross-fitted. On
  external inputs where three accepted implementations agree, generated outputs
  match the panel on only **27.79% and 50.12%** of cases, inflating the measured
  gain from "evolution" by **9.46–14.85 pp**; after auditing, **equal-budget
  independent resampling beats mutation-based evolution by 6.01–18.83 points**.
  Against a **density-matched placebo**, a genuine three-round feedback loop
  produces external differences of **+0.13 and −0.50**. → before inber adds any
  refinement scaffolding to the loop `conversation/` sustains across turns,
  `agent-bench` should run this paper's density-matched-placebo control: a loop
  consuming the same tokens carrying no real feedback. If the placebo matches,
  the scaffolding is measuring dose, not feedback.

### 8. `2608.20195` again, from the other sweep — and the finding whose sign most people would guess wrong

The keyword sweep surfaced the documentation paper independently and read further
into it than the summary in §1 above. Two additions worth keeping. First,
consultation is associated with **less** immediate testing (lift 0.23, cluster CI
0.08–0.45; adjusted **OR 0.39 [0.25, 0.60]**), consultation is **self-initiated
70.2%** of the time against **7.5%** failure-driven, and among multi-commit PRs
touching both, code is touched first **4.7×** more often. Second, and checked in
the tree rather than assumed: **inber has no instruction-file discovery path at
all** — grepping the Go source for `CLAUDE.md`, `AGENTS.md` and `.cursorrules`
returns exactly one hit, a comment in `cmd/inber-server/authstore.go:27`. So the
artifact class agents spend **60.5%** of their documentation attention on is one
inber neither reads nor writes.

**What inber should consider:** if a "read the docs first" nudge is ever added to
the system prompt, pair it with a test-execution check rather than assuming it
induces one — the measured association runs the other way.

### 9. Tier two — verified, real numbers, no action yet

- **[2608.19703](https://arxiv.org/abs/2608.19703) Loreley** — quality-diversity
  archive of whole repository states as Git commits in isolated worktrees vs.
  sequential champion editing, **1,008 matched jobs over seven paired blocks**. QD
  came in **0.135% below** sequential (95% BCa **−0.556% to +0.161%**); sequential
  had the highest observed mean and median. → this is the natural thing to build
  on forge's worktree slots, and it did not pay. An honest null, reported as one.
- **[2608.19857](https://arxiv.org/abs/2608.19857)** — secrets merely *present* in
  the context window leak through benign outputs. Across eight proprietary models,
  4-digit in-context secrets reconstruct at **82% exact match** under a black-box
  adaptive attack, and **more capable models leak more**. → `redact/` scrubs
  outbound values and does not address this at all: the leak is correlational,
  from the secret's presence, not from the model emitting it. The only defence
  shaped like a fix is not putting resolved auth-store credentials in the context.
- **[2608.20280](https://arxiv.org/abs/2608.20280)** — semantic-cache eviction over
  three corpora × three capacities × two encoders: **no policy beats LFU by more
  than 0.041 pp in any of eighteen settings**. The number that matters is the
  audit: at one encoder's median threshold only **2.1–3.9%** of sampled hits are
  answer-substitutable, collapsing raw hit rates of 51–60% to **1.1–2.2%**
  quality-adjusted, and **thresholds do not transfer between embedding models**. →
  applies to `memory/` today: any similarity cutoff tuned on one embedding model
  is invalid after an embedding change, and `memory_search` has no re-calibration
  step.
- **[2608.17445](https://arxiv.org/abs/2608.17445)** — decomposition attacks under
  unlinkable identities. **91 executable tasks, 11,393 capability-matched benign
  requests**: under a 1% denial cap **all ten tested policies**, including a
  privileged one with an exact request-to-operation map, either fail to stop
  attacks or blow the budget; on unseen task families attack success is **≥99%
  after one attempt**. → bounds what permission-store's unbuilt rule engine can
  achieve: a stateless per-call gate is provably insufficient, and the Allow/Block
  response is itself an oracle for the attacker.
- **[2608.16246](https://arxiv.org/abs/2608.16246) CompoSkill** — skill composition
  risk is a **path-level, not node-level** property: skills that each pass a
  per-skill scanner form risky chains once an agent connects their outputs and
  side effects. **1,140 records**, chain formation reaching **83.3% white-box /
  80.6% black-box**. → bundle-store's `/resolve` composes skill sets by id, and
  scanning members individually is exactly the assumption this breaks.
  Complements the filed TRUSS (`2608.17588`), which inspects one artifact at a
  time.
- **[2608.16033](https://arxiv.org/abs/2608.16033) $R^3$-Bench** — six-problem
  suites under a **shared** budget. Across **72 cells / six models** an offline
  empirical oracle matches or exceeds the contest mean in all cells and is
  strictly higher in 71; under moderate pressure even naive equal allocation beats
  contest performance for four of six models. → models are bad at allocating a
  shared budget across sub-tasks, which is precisely what inber's `spawn_agent`
  fan-out asks them to do. **Equal allocation is a defensible default.**
- **[2608.19653](https://arxiv.org/abs/2608.19653) DeltaML-Bench** — the keeper is
  not the scaffolding gain but this: **specification gaming as high as 47.9% in
  the Modular configurations and none observed in the ARG configurations** — the
  scaffold, not the model, determined whether the agent cheated the evaluator. →
  an integrity check belongs in `agent-bench`'s scoring, not only in the task.
- **[2608.20274](https://arxiv.org/abs/2608.20274)** — **task-level skills mostly
  push an agent *below* its own no-memory baseline; subtask-level skills raise it
  above**, and text skills transfer better than code skills. Yields a skill-utility
  score computable with no execution. → an offline pre-flight diagnostic for
  skill-store/bundle-store, and a warning that a badly scoped skill is worse than
  no skill.
- **[2608.17534](https://arxiv.org/abs/2608.17534) ArborMem** — memory as a
  navigable forest of interaction states: localize which prior state the current
  turn *resumes* before retrieving. **+3.36–10.31 pp** over the strongest
  baselines on three benchmarks, with the advantage **growing under constrained
  read budgets**. → the branch-resumption framing maps directly onto `session/`
  fork-and-resume, which inber has and its memory layer is unaware of.
- **[2608.17713](https://arxiv.org/abs/2608.17713)** — two deterministic optimal
  tracebacks disagree on temporal localization for **55.9% of 1,586 nonzero
  trajectory pairs**. → whenever `agent-bench` aligns two runs to say where they
  diverged, the alignment is a free parameter that can flip the conclusion.
- **[2608.16295](https://arxiv.org/abs/2608.16295)** — 26 controlled patch tasks;
  AST-bounded fingerprints classify 50 positive and 17 control changes correctly
  where **static rule snapshots detect none of the 50 stale cases**. → the
  staleness result is the one for `codeindex/`: a snapshot-based index cannot tell
  you it has gone stale; an AST-bounded fingerprint can.
- **[2608.17177](https://arxiv.org/abs/2608.17177)** — instructing an agent to
  document pre-conditions, post-conditions and undefined behaviour before
  generating tests: **+9.8 pp bug detection (p = 0.0352)** on production bugs. A
  prompt-level intervention with a stated test on real bugs, rare in this window —
  but note it is exactly the "documentation as scaffold" claim `2608.20195` finds
  weak population-scale support for.
- **[2608.16114](https://arxiv.org/abs/2608.16114) HyperSkill** — hypergraph skill
  memory ranking skills by co-occurrence across retrieved trajectories, **+11.51
  GAIA / +11.18 WebWalkerQA** over ten memory baselines. Filed for the retrieval
  structure — co-occurrence ranking rather than flat embedding similarity — as a
  plausible memory-store query change. No statistical test reported, so it stays
  here.

### 10. Screened and rejected

- Position papers, demos and n=1: `2608.16411` (explicit "we argue"/"we
  advocate", a checklist and an agenda), `2608.17195` (tool demo), `2608.17214`
  (its own closing line: *"All measurements come from one system by one
  author."*), `2608.16302` ("preliminary results", nine generated projects),
  `2608.18645` ("weak but consistent signal", no effect size or n).
- **No headline number stated at all** — standing filter: `2608.16742`,
  `2608.19784`, `2608.16551`, `2608.16544`, `2608.16068`, `2608.18575`,
  `2608.18490`.
- `2608.16402` (policy algebra) is permission-store shaped and quotes 94.8%/86.9%/
  98.6%, but states **no n, no benchmark and no test**, and self-describes the
  evaluation as an interpretation. **Do not cite those numbers.**
- Training/architecture, which inber neither does nor controls: `2608.19197`,
  `2608.18524`, `2608.20314`, `2608.18171`, `2608.18261`, `2608.17310`,
  `2608.19803`, `2608.18682`, `2608.17289`, `2608.19842`. Standing serving-stack
  filter: `2608.16477`.
- `2608.17829` LeakGauge — good result (**AUROC 0.944–0.996**, 10.34 ms) but it
  reads **prefill token probabilities**, which inber cannot obtain over the
  Anthropic Messages API. Unimplementable here rather than uninteresting.
- `2608.16185` LENS — the headline metric is *worse* (62.4% EM vs ReAct's 65.2%);
  only grounding improves. `2608.16621` — entire evidence base is a controlled
  synthetic pilot. `2608.20167`, `2608.16022` — platform-specific with no
  transferable claim. `2608.19266` — κ ≈ 0.20 with a 90% interval crossing zero.
  `2608.19269` — self-described as "a localized non-implication witness, not a
  prevalence estimate".
- `2608.17275` (Web3 MCP survey) is rejected as a survey, but one datapoint is
  worth carrying: the share of deployed MCP tools that **modify external state
  rose 27% → 65%**, while measured protections stop fewer than 30% of attacks and
  model-level safety refuses fewer than 3%. That is the population tool-store's
  `POST /provision` defaults are chosen against.
- ~60 further ids screened and dropped as off-target despite keyword matches.

### 11. Two cross-paper observations

**Iterative refinement lost twice in one week, both under a matched budget, both
to something simpler.** `2608.19626` finds equal-budget independent resampling
beating mutation-based evolution by **6.01–18.83 points** once the oracle is
audited, with a density-matched placebo showing no robust benefit from real
feedback; `2608.19703` finds a quality-diversity archive **0.135% below** plain
sequential editing over 1,008 matched jobs. Independently designed, different
domains, same direction. With the already-filed `2608.18931` (reward models
correlate ρ ≈ 0.12 with true quality) the corpus now holds **three separate
results** saying inber should not spend its budget selecting among or refining
candidates.

**Three of this pass's six keeps are negative results, and that is the useful
shape.** `2608.16190` says do not build a monitor ensemble; `2608.19760` says do
not build LLM-judge step attribution; `2608.19626` says do not trust a self-graded
refinement loop. Each names a feature that is the obvious next thing to build in
`guard/` or `agent-bench`, and shows it does not work.

## Harness-watch — 2026-08-23

Window 2026-07-24 → 2026-08-23. **505 arXiv ids** already logged across
`docs/papers/*.md` and `docs/comparisons/*.md` were extracted and machine-checked
against ~200 candidates from eight arXiv API sweeps (cs.SE/cs.AI/cs.CL/cs.CR
plus keyword slices on tool use, harness, compaction, KV cache,
permission/sandbox, memory, orchestration, spawn/handoff). Everything below is
new to both directories.

**The existing corpus is thorough** — 44 strong-looking hits were checked and
found already logged, including `2608.19662` ReCache, `2608.19652` Evolving
State, `2608.11242` Lost in Compaction, `2608.14838` Recall Trap, `2608.18351`
Task-Conditioned Least-Privilege, `2608.16246` CompoSkill and `2608.17597`
HarnessRisk. Non-arXiv sources were dry: **Anthropic's engineering blog has no
July or August 2026 posts** (latest is April), and DeepMind/Meta/OpenAI research
blogs surfaced no harness posts in the window.

### 1. [arXiv:2608.15939](https://arxiv.org/abs/2608.15939) — clearing a rejected branch from the transcript does not un-do it, because the cache still attends to it

The sharpest result of the window. Stateful agents assume that removing a
rejected branch restores prior state; this fails whenever the serving session
retains KV state across the logical abort — the model keeps attending to content
the application believes it discarded. The authors formalize **rollback
consistency** ("a complete abort must restore the state the model *attends*, not
just the transcript") and isolate it with a same-token/different-cache audit that
holds decision-step tokens identical and varies only whether the cached prefix is
stale or rebuilt. Across seven open-weight families (3.8B–36B), retained KV alone
**flips a typed protected effect in 25 of 63 audited cells**, with attacker tokens
absent from the served request in all 63. Rebuilding the cache closes every cell.
Reproduces under LangGraph time-travel and the default HF Transformers
cache-reuse path.

- **What inber should consider:** this sits exactly on the `guard/` × engine-cache
  × `session/checkpoint` seam. When `guard.CheckTool` denies a call, when an
  approval is refused, or when `session/checkpoint.go` rewinds or a session
  forks, the cache prefix is part of the state being rolled back — not just the
  `conversation` slice. `engine/prompt_blueprint.go` already computes per-block
  hashes and predicted cache behaviour; extend it to assert a post-rollback
  blueprint's cached prefix is not the aborted one, and add their
  same-token/different-cache audit as a test. **Caveat, and it matters:** their
  channel is a local KV store on self-hosted serving. inber's exposure is via
  Anthropic `cache_control` breakpoints, which is a different mechanism —
  verify the analogue holds before building on it.

### 2. [arXiv:2608.14943](https://arxiv.org/abs/2608.14943) — four skill-loading strategies, measured cache-correctly, and no universal winner

Compares Full, Skill Block, Reference and Hybrid loading across SearchQA,
SpreadsheetBench, ALFWorld, ScienceWorld and SynthProc, measuring **cache-correct
effective input** for multi-turn tasks rather than raw tokens. That is the
methodological contribution: naive token counting rewards strategies that break
the cache prefix. Hybrid cuts input 27.4% (SearchQA) and 39.8%
(SpreadsheetBench); on large multi-turn skills Skill Block and Hybrid reach
62.5%/52.8% (ScienceWorld) and 73.0%/66.6% (SynthProc). **ALFWorld gains little**
because its procedures are short and needed every turn. Paired outcome tests
found no quality difference, and the authors are explicit this does not establish
equivalence.

- **What inber should consider:** the decision rule — conditional loading pays
  only when a large fraction of a skill goes unused per turn — is computable from
  what `engine/prompt_blueprint.go` already knows (each block's size and cache
  state). Measure inber's tool-schema and skill blocks against their metric
  *before* adding progressive-disclosure machinery. The ALFWorld null is the case
  that says don't bother, and inber's always-loaded tool registry is exactly that
  shape.

### 3. [arXiv:2607.28430](https://arxiv.org/abs/2607.28430) — subagents that only report at completion are leaving most of the gain on the table

Argues the missing primitive is *mid-execution* communication: existing systems
exchange only at phase boundaries via staged handoffs, so a discovery made
mid-task cannot be shared until the next boundary. AgentRadio adds threads,
messages, and **waiting for mentions** as a background task that surfaces peers'
messages without interrupting foreground work. On SWE-Atlas QnA a single Claude
Code agent (Opus 4.6) resolves 32.3%; four AgentRadio-organized agents resolve
**62.1%** — and that beats Claude Code on the newer Opus 4.8 (57.2%). Rubric
analysis shows the gain grows with task difficulty, consistent with mid-course
correction as the mechanism.

- **What inber should consider:** the largest measured delta in this batch, and
  it names a capability `server/` spawn does not have — inber's
  `MaxSpawnDepth`-bounded subagents return results only at completion, which this
  says is what caps the score. A `bus/`-backed thread + mention channel with a
  non-blocking "check mentions" step folded into `engine/turn_prepare.go` is the
  concrete port. **Caveat:** measured on code *comprehension* QA, not patch
  generation.

### 4. [arXiv:2608.04719](https://arxiv.org/abs/2608.04719) — canary tools turn "picked the wrong tool" into a per-model profile, and capability tier does not predict safety

Plants diagnostic probe tools in an MCP tool set, each engineered to trip one
tool-selection weakness, under a six-type taxonomy (semantic decoys, parameter
traps, **capability mirages**, prerequisite blindness, temporal decoys,
granularity traps). 8 models × 120 tasks × 3 canary densities × 3 seeds = 8,640
runs, plus a 2,880-run subtlety ablation, judged by a provider-independent judge
corroborated by a second (κ = 0.75). Susceptibility spans ~36× across models, and
**capability tier alone does not predict it** — the most susceptible hosted model
is mid-tier, and within a provider the cheaper model can be safer. Capability
mirages are the one type that reliably traps frontier models. Softening give-away
phrasing leaves frontier susceptibility unchanged, evidence the probes measure
reasoning rather than phrase-spotting. Susceptibility predicts task failure
(ρ = −0.34).

- **What inber should consider:** a ready-made eval for `tools/`. Add canary
  tools to an `agent-bench` fixture registry to get a per-model tool-selection
  profile before promoting a model in `engine/failover.go`. Test capability
  mirages first — a tool whose *description overclaims* is the failure frontier
  models actually fall for, and inber's descriptions are hand-written prose.

### 5. [arXiv:2608.15108](https://arxiv.org/abs/2608.15108) — an allowed tool used to burn your budget is a threat model `guard/` does not have

Identifies a class existing agent-security work misses: attackers induce the
agent to **invoke, consume, transfer or control** high-value resources — compute,
credentials, usage budgets, identities, private knowledge, comms channels — for
the attacker's goals *without ever obtaining the resource or its credentials*.
ResourceHijackBench covers 6 resource categories, 300 scenarios and 900 attack
prompts, each run in an isolated environment that records **actual resource use**,
so success is graded from behaviour and not from text. Undefended, OpenClaw
reaches **84.06% average ASR** (69.98–89.58% across backends); the strongest
evaluated defense still leaves **55.11%**.

- **What inber should consider:** `guard/` gates on tool *identity* and mode
  (Observe/Assist/Autonomous). This threat model is orthogonal — a permitted tool
  used to consume a resource the task never declared. The concrete ask is a
  resource-consumption dimension in `guard/state.go` beside the existing
  MaxCost/MaxTokens limits: per-category caps and an approval route when a call
  would spend credentials, money or outbound comms. Their measurement discipline
  — grade the environment, not the response text — is also the right shape for an
  inber guard test.

### 6. [arXiv:2608.19013](https://arxiv.org/abs/2608.19013) — a harness update can break working behaviour with the model frozen

Names the failure mode any harness with a writable memory or skill store has:
because prompts, memories, tools, skills and routing rules jointly shape later
execution, a harness update can regress previously reliable behaviour without
touching weights — **harness-level forgetting**. Proposes *guarded harness
evolution*, separating update generation from state commitment: a Continual
Optimizer proposes candidates from post-execution feedback, and a Continual
Evaluator commits only after checking current improvement, historical retention
and validity. Relative gains >10% across textual reasoning, multimodal perception
and open-world interaction; controlled sweeps show measurable forgetting and an
adjustable stability–plasticity knob.

- **What inber should consider:** `memory/` writes and `agent/registry` config
  changes are commit-on-write today. The transferable piece is the propose/commit
  split with a **historical-retention check** — replay a small held-out set of
  previously-working behaviours before a memory write or config change lands.
  Cheap version: keep a regression set in `agent-bench` and gate `memory/tools.go`
  writes on it. See the cross-paper note below — this arrived three times this
  window.

### 7. Tier two — verified, narrower, one line each

- [arXiv:2608.19861](https://arxiv.org/abs/2608.19861) **PolicyGuide** — action-local
  checks cannot guide a multi-step procedure, so compliance fails by *omission*
  (skipped confirmation) as much as by forbidden action. A proactive verifier at
  **user-turn boundaries** reconciling open requests from persisted graph state
  lifts mean Pass⁴ 0.42 → 0.62 on τ²-bench, largest on the most workflow-structured
  domain (0.19 → 0.61). `guard/guard.go`'s `CheckTool` is exactly an action-local
  check, and inber already has both the state surface (`session/guard_state.go`)
  and the boundary (`engine/turn_postprocess.go`). Gains concentrate where the
  domain is procedural; a coding harness may see less.
- [arXiv:2608.17911](https://arxiv.org/abs/2608.17911) **CABLE** — memory-graph links
  driven by semantic overlap **duplicate what the retriever already finds**. CABLE
  generates antecedent-oriented queries, retrieves priors, **subtracts the direct
  semantic neighbourhood**, verifies the remainder and keeps only survivors.
  `memory/` is a vector store with exactly this blind spot; the subtract step is a
  cheap add to the write path in `memory/memory.go`.
- [arXiv:2608.15755](https://arxiv.org/abs/2608.15755) **IDSS** — training-free
  situation state that parses *tool returns* into provenance-aware entities and
  tracks constraint satisfaction, so the agent stops re-inferring from raw traces.
  Fits `conversation/manage_tool_pruning.go`: keep a distilled situation block and
  prune the raw tool results behind it, rather than summarizing prose in
  `conversation/summarize.go`.
- [arXiv:2608.16002](https://arxiv.org/abs/2608.16002) **RUPA** — local uncertainty
  signals miss failures whose cause originates several steps earlier; propagating
  uncertainty over a trajectory graph enables earlier failure detection. A
  candidate escalation signal for `guard/` and `engine/failover.go`. **Caveat:**
  needs logprob-adjacent signals — check what survives the Anthropic Messages API
  first, the same reason `2608.17829` LeakGauge was rejected earlier.
- [arXiv:2608.09732](https://arxiv.org/abs/2608.09732) **ColluSkill/ChainGuard** —
  scanners inspect skills individually, so intent split across separately-packaged,
  locally-plausible skills passes every check: **96.0% ASR across six scanners**.
  ChainGuard scans a candidate *jointly with what is already installed*, dropping
  ASR to 22.5% at 99.5% benign pass. Admission in `tools/` and skill-store must be
  environment-conditioned.
- [arXiv:2608.14668](https://arxiv.org/abs/2608.14668) **BRA-Audit** — frames the
  guard-cost dilemma as audit-point placement under a fixed budget, minimizing
  cumulative unchecked exposure. Near clean-setting performance at **17.2–40.6%
  lower end-to-end tokens**. `guard/` checks every call uniformly; "cumulative
  unchecked exposure" is a usable metric for when to spend an expensive
  verification, and trusted audit points map onto `session/checkpoint.go`.
- [arXiv:2608.15565](https://arxiv.org/abs/2608.15565) **AdmitOR** — the negative
  result is the value: admitting every executable candidate **poisons roughly one
  admission in four**. Calibrated cross-family behavioural agreement raises
  admission precision to 0.927 vs 0.871 (majority vote) and 0.726 (execution
  success). Unusually honest: their preregistered false-discovery criterion held on
  calibration data but **not** on the wild stream. Directly about `memory/`'s write
  path — "it ran without error" is the weakest possible gate.
- [arXiv:2608.06153](https://arxiv.org/abs/2608.06153) **GSE** — skill evolution as
  local edits overfits; a Skill Relation Graph plus **replay-driven verification to
  prevent behavioral regressions** gives precision +6.1–34.1% and recall
  +31.8–180.0% on test generation. Third independent arrival of the guarded-commit
  shape.
- [arXiv:2608.17587](https://arxiv.org/abs/2608.17587) — buried in an RL paper's
  intro: **agent-authored skills perform 8–11 points worse than using no skill at
  all**, while expert-written ones help. Following procedural guidance and
  improving it from execution evidence are distinct capabilities. The method needs
  training and is out of scope; the baseline number is a direct warning against
  letting an inber agent write its own skills unsupervised. Consistent with the
  corpus's existing "self-refinement doesn't buy quality" cluster.
- [arXiv:2608.17034](https://arxiv.org/abs/2608.17034) **SLAaaT** — switching LoRA
  adapters mid-trace instead of spawning subagents solves 4 of the hardest tasks
  versus none, at **46.1× fewer tokens** in some scenarios. The mechanism is
  unavailable over the Anthropic API; the actionable content is the *comparison
  baseline* — subagent spawn is being measured as a very costly way to compose
  capabilities. **Two synthetic tasks; treat 46.1× as an existence proof.**
- [arXiv:2608.15834](https://arxiv.org/abs/2608.15834) **GRA** — seven generic tools
  over a hybrid graph beat a full-context agent by 5.1 pp while reading under a
  third of the input tokens, and a graph-free control shows the gain comes chiefly
  from **selective agentic access rather than graph topology**. Supports keeping
  `tools/` generic and few, and exposing `codeindex/` as navigation primitives
  rather than a retrieval blob.
- [arXiv:2608.16177](https://arxiv.org/abs/2608.16177) — Milgram ported as a scripted
  probe: 42 models, 4,848 sessions, 102,511 decision turns, full-obedience rates
  spanning **0%–100%**. Two harness-design results: declaring the scenario
  **fictional raises obedience**, while **routing the decision through a native
  tool call, or granting a modest thinking budget, lowers it sharply**. The first
  is a prompt-hygiene warning for any inber test or sandbox mode that tells the
  model the situation isn't real; the second is an argument for expressing
  consequential actions as real tool schemas.

### 8. Also new, worth a line

`2608.18704` MemFuse (source provenance preserved through a causal fusion graph —
relevant if `memory/` ever ingests from more than one channel) ·
`2608.01913` (search effort and answer quality only weakly aligned; accuracy
tracks cumulative retrieval recall, not search count — argues for **stopping
criteria based on evidence sufficiency** in `engine/turn_execute.go`) ·
`2608.16707` Semantic Bandits (tool *names and descriptions* are an inductive bias
on selection, badly so when misaligned — not neutral metadata) ·
`2608.16447` HaReCAP (compile frequent leaf decisions into **auditable,
abstainable** one-step rules; 14.67–20.08% token reduction) ·
`2608.00202` (a **Telephone Loop** cross-agent delegation attack, ~80% success,
**inert against single-agent systems**; prompt-hardening does not generalize —
`server/` spawn depth limits are necessary but not sufficient) ·
`2608.01719` MNC (multi-agent systems leak protected state through internal
messages, tool arguments, logs and persistent memory even when public output looks
clean — relevant to what a parent passes into a spawned subagent and to `redact/`) ·
`2608.08677` Branch2Skill (forking substituting for repeated rollouts; touches
`session/` forking) ·
`2608.09025` SAGE-Fin and `2608.17220` PACE (same idea from two domains: bind
approval to **the exact execution artifact**, not the text the agent proposed —
inber approves a call by name and input, which is closer to approving text) ·
`2608.15391` TwinGridShield (simulate-before-approve; the honest part is the
degradation curve — 0/500 unsafe under self-conformance, but **5.63% and 30.09%**
unsafe acceptance under model mismatch) ·
`2608.19974` ReguSim (stated reasoning, attempted action, execution enforcement
and monitor evidence are four different artifacts; rationales can mislead a
monitor unless enforcement evidence is shown — framing for what a guard log must
record) ·
`2608.12996` ATOBench (make verification observable when **tool output lies**;
increased activity can mask a broken verification chain) ·
`2608.18884` EvoResearcher (generate→critique→revise **does not** beat the 95%
Wilson interval on clean BBH; the value is a CONFIRMED sentinel early-stopping
82–88% of items at equal accuracy) ·
`2608.18744` EvalCEGAR (asking a model for evaluator operators directly gave 183
candidates realizing only **96 distinct behaviours**) ·
`2608.15579` Kozuchi (open-weight, no fine-tuning, TTS@8 → **374/500 SWE-bench
Verified**; remaining gap attributed to semantic correctness and *selection*, not
edit formatting) ·
`2608.15763` TaoLive (fixed-harness SFT **reduced IFEval by 7.7 points** — a
moving harness silently degrades a model fine-tuned against an older one) ·
`2608.18423` FM-Bench (token spend predicts nothing; **self-managed memory fails
in two opposite modes — an archive that only grows, or a plan rewritten every
cycle** — a direct description of the two ways an inber workspace can fail).

### 9. Out of window but worth logging — the CLI-shaped-tool result

Hugging Face's *Designing the hf CLI as an agent-optimized way to work with the
Hub* is dated **2026-06-04**, outside this window, and is logged here anyway
because it carries hard numbers on a question `tools/` faces directly. Across 18
non-trivial tasks and ~1,000 runs, a **CLI-shaped tool** beat curl and
Python-SDK calls by 1.3–1.8× tokens on simple tasks and **2–6× on multi-step
tasks**, raised Claude Code (Sonnet 4.6) success from 84% to 94%, and an
auto-generated command reference cut tool calls ~30% (mean ~10 → ~7). Direct
evidence for exposing capability as **few CLI-like tools plus a good reference
block** rather than many fine-grained MCP tools — and it agrees with
`2608.15834` (§7) from a completely different direction.

### 10. Screened, new, and honestly thin — recorded so they are not re-found

`2608.15832` Authority Resolution Framework (**no evaluation at all**) ·
`2608.17053` Memory Is Communication (position paper; main hypothesis stated as
untested) · `2608.14707` HASSUM (evaluated on StrategyQA/JailbreakBench/TruthfulQA
— **not agentic trajectories**; `2608.16002` does the same job with real agent
benchmarks) · `2608.03588` GenOS (heavy formalism whose entire empirical
instantiation is an insertion-sort audit) · `2608.11965` (no significant ROUGE
difference on README summarization; no design lesson) · `2608.13522` Vero
(well-built benchmark, no harness-design implication) · `2608.16045` Walk Before
You Run (spreadsheet-narrow) · `2608.02110` IACM-RL (nice BeliefState idea
delivered as an RL training recipe — §7's IDSS gets a similar effect
training-free) · `2608.17499` FACA, `2608.16211` BaT, `2608.19880` EnvHarness,
`2608.20318` AI4AI-Bench, `2608.19072` (all training-side, standing filter).

One worth a sentence despite the rejection: `2608.15678` *Where Accountability
Lives* maps platform controls against provider terms across four agentic coding
tools and 18 policy documents and finds the layers contradict each other — one
provider bars the assigning developer from approving the resulting PR, another
ships an agent that approves PRs below a risk threshold and can dismiss reviews.
It is a policy-document study claiming no harm, so it is not evidence about
inber; but its observation that the approval *artifact* is weaker than the terms
assume is the same point `2608.09025`/`2608.17220` make empirically, and it is
relevant to what a `guard/` approval record should contain.

### 11. Two cross-paper observations

**Guarded commit arrived three times independently this window.**
`2608.19013` (§6) proposes a Continual Evaluator that commits a harness update
only after a historical-retention check; `2608.06153` (§7) uses replay-driven
verification to prevent regressions in a skill bank; `2608.15565` (§7) measures
what happens without one — **one poisoned admission in four** when the gate is
"it executed". Three groups, three domains, one conclusion: a writable agent
store needs a regression gate between propose and commit. inber's `memory/` write
path has none. This is the single best-supported feature request of the sweep.

**Single-item scanning of skills and tools is measurably dead.**
`2608.09732` (96.0% ASR across six scanners) and the already-logged `2608.16246`
CompoSkill reach the same finding from opposite framings within a week, while the
already-logged `2608.17275` datapoint — deployed MCP tools that modify external
state rising **27% → 65%** — says the population being scanned is getting more
dangerous at the same time. Admission decisions must be conditioned on what is
already installed, not made per item.

## Harness-watch — 2026-08-24

Window 2026-07-25 → 2026-08-24, re-swept one day after the 08-23 sweep, so the
overlap was expected and it was large: **35 of 40** verified candidates were
already logged in this directory, including every item the 08-23 entry names.
Five are new, and one of them is the strongest memory result this file has
recorded. Non-arXiv was nearly dry again; the two lab items below are the whole
yield.

### 1. [arXiv:2608.07429](https://arxiv.org/abs/2608.07429) — append-only agent memory scored *worse than having no memory at all*

TEPA (Zhou, Ouyang, Zheng, Xiang; 2026-08-07, v2 08-10). The paper names the
failure "memory pollution": a memory that newer conflicting evidence has
superseded stays retrievable and keeps entering the prompt. TEPA stores
observations as **keyed precedents** and *revokes* an active precedent when fresh
evidence contradicts it under the same key, keeping the revoked row for audit
rather than deleting it — validity becomes an explicit state of a memory row
rather than a consequence of recency ranking.

The number that matters is the baseline, not the method. Under controlled full
reversal over 50 seeds: **append-only 0.210, last-write-wins 0.210, no memory
0.309, TEPA 0.950** — and the ordering reproduced under real file-backed drift
(append-only 0.203, no memory 0.298, TEPA 0.950). On clean MemoryAgentBench SH-6k
TEPA merely matches last-write-wins, which localizes the entire gain to the
reversal case.

**What inber should consider:** inber's `memory/` write path is append-only with
importance decay and recency ranking, and `memory_save` has no field that can say
*this replaces that*. Under drift — a changed config value, a moved file, a
retracted decision — that is the 0.210 row, and this paper says it is worse than
switching memory off. Two consequences, in order: **(a)** a memory row needs a
`key` and a validity state (`active` / `revoked`), and the revoked row must stay
readable, because "what superseded X" is a question inber cannot currently ask —
the same conclusion `2608.13662` (MOOSEDev, already logged) reached from the
knowledge-graph side, where vector top-k surfaced 6–27% of a supersession answer
set against 0.98–1.00 for symbolic supersession links. **(b)** Do not model this
as a delete. The audit trail is the half that makes revocation safe to automate,
and it is also what lets a wrongly-revoked fact be re-promoted.

### 2. [arXiv:2608.21230](https://arxiv.org/abs/2608.21230) — write-time screening does not catch memory poisoning, and provenance weighting at a shipped setting is indistinguishable from no defense

Poisoning **1.2%** of a LongMemEval corpus with plainly-worded false assertions —
no triggers, no optimization, nothing an injection classifier is shaped to see —
dropped accuracy **0.850 → 0.300**. A write-time screening pipeline scoring 0.832
recall on indirect prompt injection rejected **0 of 360** poisoned memories.
Provenance-weighted retrieval at the shipped weight was statistically
indistinguishable from no defense (p = 0.80); turning the weight up recovered
utility only by excluding untrusted content outright, at which point accuracy
collapsed to 0.0417 whenever the answer-bearing evidence was itself untrusted.

**What inber should consider:** this is a limit result and it bounds two things
inber has already filed. Todo `90296699` (memory-store: the model chooses its own
provenance) and `68e64219` (`memory_save` fabricates provenance) are both worth
fixing — but this paper says provenance *weighting* is not the payoff. A false
statement written in good faith by an honest path is still false, and no
screener distinguishes false from true without external grounding. Pair it with
§1: revocation on contradiction is a grounding mechanism that screening is not,
because it fires on *later evidence* rather than on the text of the write.

### 3. [arXiv:2608.20664](https://arxiv.org/abs/2608.20664) — verbatim event memory gets most of the measured benefit; the elaborate-structure premium was not demonstrated

DreamBench-SWE, a preregistered multi-session benchmark where a later software
task depends on non-inferable evidence from an earlier session, scored by
executable hidden oracles. No external memory **21/180 (0.117)**; deterministic
verbatim event memory **82/180 (0.456)**; typed-plus-raw reference probe **83/180
(0.461)**; a pinned hosted literal-storage config **97/180 (0.539)**. Every
memory-bearing condition beats no-memory after Holm correction, and the audit
explicitly declines to establish superiority *among* them.

**What inber should consider:** a baseline to hold inber's memory work to before
building more structure — most of the distance from 0.117 to 0.539 is bought by
keeping the events at all. Read against `2608.12888` (ReFind, already logged:
agent-controlled lexical search over raw chat logs beating HippoRAG 2), the two
say the same thing from different directions, and both point at log-store search
tooling ahead of memory-store schema.

### 4. [arXiv:2608.20732](https://arxiv.org/abs/2608.20732) — prefix-cache reuse is a working cross-tenant side channel for inferring who is upstream of whom

CacheTracer: 1.1M API requests across 636 endpoint pairs and 39 reseller
endpoints over the LLM API reseller ecosystem. **37.1% of endpoint pairs share
cache reach**, containment order spans seven layers, and one cache reach is
contained in at least 31 others. Timing on a cached prefix is enough to infer
that two nominally independent endpoints sit behind one provider.

**What inber should consider:** narrower than it looks, and worth writing down
before it is not. A cached prefix contains inber's system prompt and whatever
repo content sits above the `cache_control` breakpoint. Today model-store routes
to first-party endpoints, so there is no exposure — the item is a **model-store
policy field**, not code: mark which providers may receive a cacheable prefix, so
adding a gateway or reseller later is a decision someone makes rather than a
default someone inherits. Same family as `2608.17485` (KeyPooling, already
logged: a shared provider credential is a shared prompt cache).

### 5. [arXiv:2608.08468](https://arxiv.org/abs/2608.08468) — static analysis of a skill catches exfiltration and is blind to `rm -rf`

SkillsMetric, five-stage static analysis over 2,266 adversarial skills across 16
attack types on the SkillMD-138K corpus: **AUC 0.93**, F1 73.4% ± 0.5%, strong on
data exfiltration (93%) and steganographic payloads (93%). But **host-destruction
attacks written with ordinary shell commands evade all five stages — 0%
detection** — and natural-language prompt injection reaches only 42%.

**What inber should consider:** this is the shape of the answer for skill-store
ingestion, and the shape is "screening is not a gate." The stages that work catch
payloads that must *look* unusual to function; a destructive skill is
indistinguishable from a legitimate cleanup skill by inspection, because the
bytes are the same and only the intent differs. So skill-store may use static
analysis to *rank* and to refuse the obvious, and must not use it as the
admission decision. Consistent with the 08-23 entry's conclusion that
single-item scanning is dead (`2608.09732`, `2608.16246`).

### Lab and industry, both new to this directory

- **[Anthropic — The new rules of context engineering for Claude 5 generation
  models](https://claude.com/blog/the-new-rules-of-context-engineering-for-claude-5-generation-models)**,
  dated **2026-07-24** on the page itself, so it sits one day outside this
  window's nominal start and is logged here because nothing in this directory has
  it. Anthropic removed **over 80% of Claude Code's system prompt** for Opus 5 /
  Fable 5 "with no measurable loss on our coding evaluations." The recommendations:
  replace rules with contextual judgment; stop writing tool-usage *examples* and
  improve the tool *interface* instead; disclose tool definitions progressively
  rather than front-loading the catalog; deduplicate guidance that appears in both
  the system prompt and a tool description; prefer automatic memory over a
  hand-maintained `CLAUDE.md`. **What inber should consider:** inber's per-agent
  system prompts are hand-grown and have only ever accreted. The cheap experiment
  is subtractive and inber can run it today — cut an agent's prompt hard, keep the
  tool set fixed, and compare on the same task set; the 08-23 entry's ArkBench
  note (`2608.10934`) is the small bench that makes this affordable. The
  deduplication point lands on a specific inber habit: guidance that lives in both
  the prompt and a tool description is paid for on every cached prefix *and* every
  tool block.
- **[IBM Research — How Much Memory Does Your Agent Actually
  Need?](https://huggingface.co/blog/ibm-research/altk-evolve-hmm)** (2026-08-18).
  The optimal memory dose varies by model tier and not by parameter count. Strong
  models gain from the **full** guideline set (DeepSeek-V3.2 +9.5 pp, Claude Opus
  4.6 +4.1 / +7.1 pp, GPT-5.5 +2.9 / +7.2 pp); a mid-tier model gained most from
  **curated retrieval** (gpt-oss-120b +16.1 pp) at **+5% token overhead against
  +78%** for full injection; GLM-5 showed **0.0**. **What inber should consider:**
  memory-injection policy belongs on the **model-store model record**, not in a
  single engine-wide setting — full injection for the premium agents, retrieval
  only for the cheap ones. Same shape as the goose 2026-08-20 entry's conclusion
  about thinking-mode: a default-off capability flag on the model record, gated on
  the flag and never on a provider name.

**Screened and rejected:** 35 items verified and already present, so no row was
written for them. Named here so the next sweep does not re-fetch them:
`2608.11386`, `2608.08654`, `2608.13867`, `2608.18389`, `2608.18280`,
`2608.13547`, `2608.10178`, `2608.10934`, `2608.20195`, `2608.16370`,
`2608.06503`, `2608.11775`, `2608.11392`, `2608.06057`, `2608.19662`,
`2608.07855`, `2608.02645`, `2608.19303`, `2608.04719`, `2608.02650`,
`2608.19741`, `2608.17007`, `2608.16055`, `2608.16801`, `2608.15888`,
`2607.25090`, `2608.17393`, `2608.19564`, `2608.15008`, `2608.12888`,
`2608.13662`, `2608.18066`, `2608.14876`, `2608.18351`, `2608.00997`.

## Harness-watch — 2026-08-25

Window 2026-07-26 → 2026-08-25. Fifty verified candidates, **44 already logged
here**, so the sweep is now mostly re-reading itself — the screened list at the
end of this section is longer than the section. Six are new. Two of them
(`2608.22752`, `2608.22708`) land on machinery inber runs today, and one
(`2608.23067`) is the first measured argument *against* a rule in this box's own
directives.

### 1. [arXiv:2608.22752](https://arxiv.org/abs/2608.22752) — a safety rule and a chat log are compacted at the same rate, and only one of them survives being paraphrased

Zerhoudi, Mitrovic, Granitzer (2026-08-24). Measured across 20 production agent
configurations: Claude Code's `/compact` prompt on Sonnet 4.6 retained **53% of
safety rules after one compaction and 10% after five** — they call it the
Compaction Cliff. The diagnosis is the useful part: an episodic log survives being
summarized because its value is its gist, and a rule does not, because its value
is its exact wording. Uniform summarization therefore destroys one class and
preserves the other while reporting a single ratio. Their Knowledge Triage
classifies each knowledge-base line by type and gives each type its own retention
policy — TypeCompact keeps **2–4× more safety rules** than the strongest
single-shot LLM compactor at every ratio (**96% recall over five rounds**),
TypeDecompose **0% locality violations against 93%** under uniform partitioning,
TypeRetrieve **100% recall@50 against 73%**. They release AgentArtifactCorpus:
396,934 agent configurations mined from 54,628 GitHub repos.

**What inber should consider:** inber's compaction is uniform over the transcript
(`conversation/summarize.go`, `conversation/manage.go`), so a standing operator
constraint decays at the same rate as a `ls` output. The cheap half of Knowledge
Triage is not the classifier — it is admitting that some lines must be *copied*
rather than summarized. inber already has a preserve-set concept in the pruner;
the question a fix has to decide is **where the rule-type marking comes from**:
the agent record in agent-store (durable, operator-authored, but blind to rules
that arrive mid-session), or a per-message flag set at injection time (catches
both, but every injection site must set it, which is exactly the failure mode the
08-24 entry documented for `ContentItemKind`). Do not pick one here.

### 2. [arXiv:2608.22708](https://arxiv.org/abs/2608.22708) — progressive tool disclosure and prompt caching are in direct conflict, and the resolution is to move the varying part off the cached prefix

Zha et al. (2026-08-24). Stated plainly for the first time in this directory:
showing the model only the currently-relevant tools keeps the prompt small, and
every change to that visible list invalidates the cached prefix, so the two
techniques cancel. CacheRouter separates the jobs — the main model always sees a
**small fixed core tool set**, so its request head is byte-stable across turns,
and the long tail is reached through an independent router sub-model that searches
the full catalog, picks one tool, executes it and returns only the result. On 55
functional queries and a 30-turn dialogue, token-level cache hit rates of
**90.99% and 95.2%**, cutting input cost to **12.0% and 8.0%** of a no-cache
baseline under DeepSeek pricing.

**What inber should consider:** this is the direct counterweight to "disclose tool
definitions progressively", which this directory logged approvingly from
Anthropic's context-engineering post on 2026-08-24 — that advice is priced in
cache misses and the post does not say so. It lands on live inber machinery: the
tool block is assembled per turn in `engine/build_tools.go` and then appended to
by `mergeExtraTools` (`engine/engine_new.go`), so anything that makes an agent's
tool set turn-dependent moves the cache breakpoint. The structural point is that
inber already owns the expensive half of CacheRouter — `spawn_agent` *is* a router
sub-model — so the long tail can be delegated rather than disclosed. What a fix
must decide: whether a delegated tool call is allowed to write, because a router
sub-agent that only reads is a cache optimization and one that can write is a new
principal with no allowlist of its own.

### 3. [arXiv:2608.23067](https://arxiv.org/abs/2608.23067) — injected Agent Skills *lowered* Pass@2 by 1.3–4.2% while raising token cost 72–394%

Yang and Ding (2026-08-24). WebDev-Skills-Bench: 31 public WebDev Skills across 50
Web-Bench projects, 1,000 ordered tasks. Injecting the *target* Skill — the one
written for that job — **reduced mean Pass@2 by 1.3% to 4.2%** and **raised token
cost by 72% to 394%**. Skills helped in only **17–36% of Skill-project pairs**.
Length-matched controls isolate content from length: Skill *content* lowered Pass@2
by 1.1–1.4% in some models, so this is not a context-length artifact. The losses
concentrate on easy early tasks, where the Skill displaces a straightforward
solution the model already had.

**What inber should consider:** this is the first measured evidence against an
unconditional skill lookup, and this host's own directives mandate one before any
non-trivial task. The paper's framing — a Skill is a hypothesis about a specific
Skill × project × model triple, not a portable asset — is the actionable part: if
inber records which agent and which repo a skill was pulled for and whether the
run succeeded, the injection becomes an audited decision with a growing evidence
base instead of a reflex. Note the limits before over-reading it: WebDev only, and
Pass@2 on a benchmark of ordered tasks is not the same shape as inber's work.

### 4. [arXiv:2608.22339](https://arxiv.org/abs/2608.22339) — a skill mined from a success raises the model's confidence in the *wrong* tool by 47%

Lin et al. (2026-08-23). The assumption under every "learn from successful
trajectories" memory design is that more retrieved skills cannot hurt. On tasks
that *resemble* past successes but need different tools, retrieving more skills
**increases confidence in wrong tool calls — procedure skills raise the wrong-tool
margin by 47%** over a memory-free baseline. They call it the Skill Imitation
Trap. Boundary-Aware Skill Memory attaches applicability conditions, risk cues,
avoidance rules and recovery notes to each skill: **+23.8% task success on
AppWorld**, **+5.0% on BFCL**, **−4.6% attack success rate on AgentDojo**,
**−6.6% average AppWorld steps**.

**What inber should consider:** the sharpest form of this result is that a memory
recording only what worked is *actively misleading* near its own boundary, which
is worse than absent — the same shape as the 08-24 entry's finding that
append-only memory scored below no memory at all, arrived at from a different
direction. inber's memory rows carry no negative field. A fix has to decide
whether the boundary is **required on write** (which blocks the cheap
`memory_save` call that makes memory get used at all) or **retrievable-only when
present** (which lets the unbounded rows keep competing in ranking). That is a
real trade, not an oversight.

### 5. [arXiv:2608.21690](https://arxiv.org/abs/2608.21690) — treat the session as an executable environment with an eviction index, not a prompt to be summarized

Lin et al. (2026-08-21). The argument against both summarization and
extract-to-memory is that each commits to what matters *before* the future need is
known. Scroll makes each session an executable Session Environment over an
append-only Event Log plus a sandboxed persistent Python kernel: tool outputs bind
to variables in a typed namespace instead of being serialized into the prompt each
call, only explicitly printed projections enter the working view, and evicted spans
stay recoverable through an eviction index of compact landmarks pointing at exact
Event Log addresses. With Qwen3.8-Max: **94.8% LongMemEval_S**, **73.1%
BEAM_10M (+5.1 pts over the best published memory system)**, **86.7% LOCA_256K
(+37.4 pts over the best published long-horizon agent)**.

**What inber should consider:** take the eviction index and leave the kernel.
inber's SQLite session store is already the append-only log this design assumes,
so the missing piece is one landmark row per compacted span carrying the message
row id — which makes a compacted-away span *addressable* rather than gone, and is
the recoverability property on its own. The kernel half is a much larger change
and is not required for it.

### 6. [arXiv:2608.22510](https://arxiv.org/abs/2608.22510) — one-shot success and three-trial success rank agents differently enough that a leaderboard on either is not evidence about the other

Xiao (2026-08-23). The claim is that the evaluable unit is a declared
model-plus-runtime configuration, because failures land in evidence acquisition,
runtime routing, safety boundaries and repeated execution — none of which a final
answer shows. ClawProBench runs on OpenClaw with native surfaces for browsing,
memory, messaging, scheduling, skills and subagents, scoring from execution traces
under a safety-gated correctness/process/efficiency formula. Across 68
configurations: top trace score **0.7671**; native-runtime tasks underperform
workspace-live tasks **0.5238 vs 0.6415**; **pass@k-any 0.6638 against strict
three-trial pass 0.2890**; and full-profile against holdout rankings agree at only
**Spearman 0.1300**.

**What inber should consider:** the 0.29-against-0.66 gap says a single successful
run is mostly luck at this reliability level, so any inber-versus-harness
comparison that reports one trial is reporting noise — run three and report the
strict number. The second half is cheaper than it sounds: inber already writes
session events to SQLite, so scoring from those rows rather than from the last
assistant message is a query, and it is the only way failures in spawn, steer and
delegation become visible at all, since every one of them can occur under a
correct final answer.

**Screened and rejected — verified and already present in this directory**, named
so the next sweep does not re-fetch them: `2608.20195`, `2608.19799`,
`2608.18645`, `2608.18167`, `2608.18050`, `2608.17528`, `2608.17485`,
`2608.17393`, `2608.17034`, `2608.16402`, `2608.16295`, `2608.16055`,
`2608.15888`, `2608.15678`, `2608.15584`, `2608.15241`, `2608.15117`,
`2608.14876`, `2608.14863`, `2608.14093`, `2608.13867`, `2608.13662`,
`2608.13560`, `2608.13522`, `2608.11772`, `2608.11727`, `2608.11386`,
`2608.11152`, `2608.10504`, `2608.10450`, `2608.10402`, `2608.10319`,
`2608.10178`, `2608.09290`, `2608.08793`, `2608.07855`, `2608.07556`,
`2608.05263`, `2608.01558`, `2608.01507`, `2608.00902`, `2608.00202`,
`2608.00101`, `2607.25816`, `2607.25090`, `2607.25032`, `2607.15516`,
`2607.13080`, `2607.12161`, `2607.10582`.

## Harness-watch — 2026-08-26

Window 2026-08-20 → 2026-08-26 for arXiv, 2026-07-26 → 2026-08-26 for lab and
industry sources. Roughly seventy arXiv hits were already logged here, so the
screened list at the end is again the longer half. **Thirteen are new**, and two
of them (`2608.24358`, `2608.23651`) land on machinery inber runs today —
`2608.24358` is the direct counterpart to the defect this run filed as
`7c6a0ee4-9907-477e-96ee-f21f060e1584`. Announcement lag means nothing submitted
2026-08-26 is visible yet; the newest ids are `2608.248xx` from 08-25.

### 1. [arXiv:2608.24358](https://arxiv.org/abs/2608.24358) — the right amount of trajectory to carry across a model switch *reverses* with the direction of the switch

Ganz, Shpigel Nacson, Kalyanpur, Litman (2026-08-25, cs.AI). "The Handoff Tax."
Paired low-capability and high-capability models **from both the Claude and GPT
families**, switching mid-run, varying direction, timing, and the interface —
full-trajectory transfer, compaction, or trajectory removal with repo state
preserved. Two findings. **Full-trajectory escalation recovers less than half of
the LC→HC quality gap while incurring a substantial cost premium** — that gap is
the handoff tax, and downshifting once the hard reasoning is done is a favourable
cost-quality point. The second is the one that matters here: **the preferred
interface reverses with direction.** Reducing the inherited trajectory *improves*
escalation quality; removing the HC model's trajectory *degrades* downshift
quality. Headline results are stated directionally in the abstract rather than as
point percentages, which is worth saying before quoting them.

**What inber should consider:** inber crosses providers mid-run by design —
`selectModel` (`engine/failover.go:22`) walks model-store's `FailoverChain()`, and
the live priority table on this host interleaves anthropic and openai. Today it
applies a **fixed** trajectory reduction in exactly **one** direction and never
the other: `engine/turn_execute.go:35` deletes every OpenAI-sourced tool pair when
entering an Anthropic turn, while `engine/turn_openai.go:57` projects losslessly
in the reverse direction. That is the opposite of what this paper measures on the
axis that matters — the reduction is applied on entry to Anthropic regardless of
whether that model is an escalation or a downshift, because inber has no notion
of tier at all. The defect half is filed as
`7c6a0ee4-9907-477e-96ee-f21f060e1584`; the *policy* half is a separate and
larger question, and this paper says a single uniform compaction rule is the
wrong shape for it. Note the limit before over-reading: their handoffs preserve
repo state and switch a whole session, which is not the same as inber's per-turn
failover under a health signal.

### 2. [arXiv:2608.23651](https://arxiv.org/abs/2608.23651) — the *verbatim text* of a failed tool call is what makes a model repeat it, and deleting the attempt is the worst available remedy

Gumaan (2026-08-24, cs.SE). Defines the **corrective gain** of a failure record as
the change in log-probability of re-emitting the action that just failed, and
measures it **negative for all six instruction-tuned checkpoints tested**
(135M–1.7B, four families) across simulated tool calling and MBPP repair:
**≈ −1.03 nats per action token, a 2.8× odds shift per token, holding on 90–100%
of individual items.** Over a fixed candidate set, the probability of repeating
the failed call rises **0.06 → 0.54**, and greedy decoding reproduces it
token-for-token on **19% of items after the failure against 0% before**.
Counterfactuals separate form from semantics: **the failed call's verbatim text
accounts for 83% of the damage**, while marking it "failed" contributes little.
Two remedies and one anti-remedy: replacing the verbatim call with a
runtime-generated failure description **removes 76% of the inversion at zero token
cost**; an explicit "do not repeat" instruction does nothing; and **deleting the
failed attempt and retrying from a clean context was the worst harness measured**,
because it restores the context that caused the failure.

**What inber should consider:** the anti-remedy is the actionable half, because
inber has that shape in two places. `conversation/repair.go:12`'s
`interruptedToolResultText` replaces an unanswered call's *result* while leaving
the call's arguments verbatim, and `truncateToolCall`
(`conversation/manage_tool_pruning.go:104-117`) rewrites a `tool_use` input to
`{"_summary": "name: args"}` — which is a *shortened* verbatim, not a
description. The paper's cheap fix maps onto the existing
`replaceToolUseInput` seam exactly. **Read the scope before acting on it:** every
checkpoint measured is 135M–1.7B, and inber runs frontier models where the effect
may be far smaller or absent. This is a hypothesis to test on this host's own
rollouts, not a finding to port — and the file already holds the right instrument,
since `tool_use` blocks in the transcript are the ground truth for how often a
failed call is re-emitted.

### 3. [arXiv:2608.24188](https://arxiv.org/abs/2608.24188) — an extractive 4B compressor holds 86.5% of solve quality at 25.7% of the context, and copies 96% of identifiers verbatim

Shi, Chen (2026-08-25, cs.AI). Paritok-4B is a LoRA compressor that **selects
spans rather than rewriting**, conditioned on agent intent so retention follows
task relevance instead of uniform shrinkage; trained on **67,074 real OpenHands
trajectories**. **96% of identifiers, paths and numbers in its output are copied
verbatim from the input.** Compression to **25.7% of original size at 86.5% solve
quality on SWE-bench Lite**; with line-numbered input, **27.8% and 89.3%**, no
statistically significant degradation at that sample size. The model is **264 MB,
self-hosts on one GPU, and has no per-token fee.**

**What inber should consider:** the extractive-versus-abstractive distinction is
the transferable part and it does not require the model. inber's compaction is an
LLM summary (`conversation/summarize.go`) whose output is prose, so a file path or
a symbol name that survives compaction survives as something the summariser
*rewrote* — and the failure mode is a plausible-looking path that does not exist.
The 96%-verbatim figure names the property a coding harness actually needs from a
compactor. The economic argument is secondary but real: using a frontier model as
the compactor is the expensive way to buy a property a 264 MB extractive model
gets structurally.

### 4. [arXiv:2608.24876](https://arxiv.org/abs/2608.24876) — split agent memory into mutable working state and validated skills, and the benefit *grows* with horizon length

Yu, Wu, Yin et al., incl. Wang and Yan (2026-08-25, cs.AI). Recuris separates a
**Working Memory** tracking task progress, which gates skill selection, from an
**Experiential Memory** of skills, so a skill is chosen against current state
rather than the full history; a fixed Meta-Agent turns execution evidence into
localized, **validation-gated** updates to skill memory. Across **four
long-horizon benchmarks and ten models it improves success in 35 of 37 completed
model-benchmark pairs**: **+17.8 points to GPT-5.6 Sol and +15.6 to Claude Opus 5
on tau-bench** (Opus 5 → 87.9%), **+16.6 / +13.5 on Qwen3.6-27B/35B on
SkillFlow**. The advantage **widens with horizon, to +32.2 points on the longest
tasks**, and common long-horizon failure modes drop by **up to 80%**.

**What inber should consider:** this is the third independent arrival in three
days at the same conclusion — after `2608.22339`'s Skill Imitation Trap (a skill
mined from a success raises confidence in the wrong tool by 47% near its
boundary, 08-25) and `2608.07429`'s finding that append-only memory scored below
no memory at all (08-24) — that **an unvalidated, ungated skill store is worse
than absent**. The validation gate is the common element in all three. inber has
the working-memory half already and does not call it that: noteboard's
`workspace` type is a timestamped document a recurring job reads first and
rewrites last, which is exactly this design's Working Memory, and it sits outside
the todo queue for the same reason the paper gives. The missing half is that
`memory_save` writes an unvalidated row. What a fix must decide is unchanged from
the 08-25 entry and should not be settled here: whether the gate is required on
write, which blocks the cheap call that makes memory get used at all.

### 5. [arXiv:2608.23992](https://arxiv.org/abs/2608.23992) — 140.2k tool tokens to 1.3k in production, and an explicit argument that prompt caching is not a substitute

Saha, Wang, Manoharan (2026-08-25, cs.IR). SCOUT reframes tool exposure as
context *selection*: instead of injecting all schemas, expose two MCP meta-tools,
`tool_search` and `execute_tool`, where the search fuses **BM25 sparse matching
with dense vector search via Reciprocal Rank Fusion** over a catalog supporting
zero-downtime updates. Scale: **2,000+ tools across 200+ MCP servers.** In
production at PayPal, **MCP tool-token consumption drops from 140.2k tokens
(70.1% of context) to 1.3k (0.8%) — a 99% reduction.**

**What inber should consider:** the sentence to keep is the one they wrote as a
rebuttal — *prompt caching reduces reprocessing cost but neither frees context
capacity nor improves accuracy.* That is the counterweight to `2608.22708`
(CacheRouter, logged 08-25), which argued the other way, and the two are not
actually in conflict: caching buys back the *price* of a large tool block and
selection buys back the *window*. inber pays both costs today —
`engine/build_tools.go:48-65` `buildDefaultTools()` puts the entire registry on
the wire, and `agent/agent_run.go:36` caches it — and has never measured either.
The measurement is the prerequisite and the 2026-08-14 shadow-selector plan in
`agentic-design-patterns.md:1510` already describes how to take it.

### 6. [arXiv:2608.22331](https://arxiv.org/abs/2608.22331) — prompt phrasing, not sampling, sets the noise floor: 11×–58× larger

Chen, Qian, Wang et al. (2026-08-23, cs.CL). Audits measurement variability for
**three native tool-calling endpoints across two providers** on BFCL
multiple/parallel with matched AST grading. At temperature 0 reruns are nearly
deterministic — **ever-flip fractions of 0.7%, 2.0%, 2.7%; mean run correlations
0.997, 0.966, 0.961**. But **semantics-preserving prompt perturbations produce
median paired standard deviations 11× to 58× larger than rerun paired SDs.**
Failure *character* also shifts: **malformed-output failures are 30%, 7% and <1%
of task failures** across the three endpoints, so one accuracy number hides both
stability and failure mode.

**What inber should consider:** this sets the bar for any inber-versus-harness
comparison, and it sharpens `2608.22510` from 08-25 rather than repeating it —
that paper said one trial is noise against three (0.2890 strict versus 0.6638
pass@k-any), and this one says even three trials at temperature 0 understate the
real variance by an order of magnitude unless the *prompt* is perturbed too. Both
are cheap for inber: session events already land in SQLite, so the scoring is a
query; the perturbation is the new work.

### 7. [arXiv:2608.23550](https://arxiv.org/abs/2608.23550) — only 4.4% of security rules written into CLAUDE.md have a matching enforceable control

Yan (2026-08-24, cs.HC). Measures the gap between natural-language security rules
in instruction files and enforceable Claude Code controls across **481 public
CLAUDE.md files**. Under the strictest matching standard **4.4% (95% CI
2.6–6.7%)** of retrieved security rules had a matching built-in control; **~4–16%
depending on strictness**, with close agreement between two blinded
security-practitioner annotators. A manual review found the extraction pipeline
captured **66.3%** of eligible rules, so the rates describe the captured subset.
The framing is the finding: an instruction file is a **write-only channel** — the
author gets no feedback on whether anything will enforce what they wrote, and the
same plain text silently mixes enforceable rules with model-interpreted ones.

**What inber should consider:** this box runs exactly the two-layer arrangement the
paper measures — `~/AGENTS.md` and `~/CLAUDE.md` as prose, `guard/` and
permission-store as enforcement — and nothing anywhere reports which prose rules
are backed. The cheap version is a lint that reads the directives and answers, per
rule, *enforced by X* or *model-interpreted only*; the honest version admits that
most will be the latter and that saying so is the point. Related and already
logged: `2608.20195` (the documentation agents actually read is the instruction
file, 08-22).

### 8. [arXiv:2608.21964](https://arxiv.org/abs/2608.21964) — every release transition invalidated part of the skill set, and skill decay raises no signal

Duan, Shi, Mao et al. (2026-08-22, cs.AI). Repo2Skill-Evo casts each release as a
skill-maintenance task: given a V1 skill set and the official V1→V2 patch, update
what is obsolete and preserve what is still valid. Across **57 real repositories
and 105 selected release transitions, every evaluated transition invalidated part
of the V1 skill set.** Six frontier agents reach only **29.9%–69.7% avg@3 macro
F1** under a patch-grounded metric balancing stale-content recall against
over-editing precision, and two opposing errors dominate — incomplete file
coverage leaves stale content, overbroad editing raises recall and drops
precision. The core claim: **externalizing knowledge into a skill makes its rot
invisible.**

**What inber should consider:** this is about skill-store (`:8301`) rather than
inber, and it is the strongest argument yet for a staleness signal there — the
registry ingests by cloning an upstream repo, so it knows the commit each
`SKILL.md` was read at and can say how far behind the upstream has moved without
judging the content at all. Pairs with `2608.23067` from 08-25, which measured
injected skills *lowering* Pass@2 by 1.3–4.2%: a stale skill is the mechanism, and
this paper says staleness is guaranteed rather than occasional.

### 9. [arXiv:2608.23041](https://arxiv.org/abs/2608.23041) and [arXiv:2608.24804](https://arxiv.org/abs/2608.24804) — two independent papers evolving the *harness* with the model frozen, both reporting double-digit gains

Park, Kim, Tan et al. (2026-08-24, cs.AI) — **AutoSaddler**: offline learning from
failure traces, diagnosis → structured patch treating the harness as code →
validation-based update selection, iterated over mini-batches. **+9.0 pp on
GAIA2, +9.6 pp on SWE-Bench Pro, +10.0 pp on Terminal-Bench 2.0.** Ablations
attribute the gain to **deep debugging rather than shallow reflection, targeted
modifications rather than unconstrained editing, and generalization-aware
selection rather than trajectory-specific repair** — i.e. the naive
reflect-and-rewrite-the-prompt loop underperforms.

Esakkiraja, Akhiyarov, Yadav et al. (2026-08-25, cs.AI) — **StarHarness**:
stratified search over prompt framing, tool interfaces, skills, MCP providers,
**subagent structure**, and agent-loop configuration, with proposer-visible search
tasks separated from proposer-hidden selection tasks. **+20–35 percentage points
across ITBench SRE, EnterpriseOps-Gym ITSM and AutomationBench Finance after only
4–12 accepted changes per environment**, persisting on held-out tasks and
transferring across GPT and Qwen families without re-evolution.

**What inber should consider:** together these are a measured argument that the
harness, not the model, is the binding constraint — the same claim OpenAI made
non-academically this window (§11). The inber-specific note is that both methods
need failure traces as data, and inber has them: `session.jsonl` plus the SQLite
session store. What neither paper solves, and what inber would have to decide
first, is the **acceptance gate** — StarHarness's whole discipline is that the
proposer never sees the tasks used to select, and an unattended loop that patched
`agent/`, `engine/` or the prompt blueprint against traces it also proposed
against would be overfitting with a commit bit. Do not build the loop before the
holdout.

### 10. [arXiv:2608.22215](https://arxiv.org/abs/2608.22215) — route memory writes with a small model: 68% of redundant writes pruned, 98% of downstream quality kept

Li, Nie, Lan et al. (2026-08-23, cs.CL). Moves memory management to the **write**
phase: each incoming item is classified non-write / write-new / write-update by a
**small-to-large model cascade**, with a periodic slow consolidation of
high-value external memories into parameters. A **1.7B/8B cascade prunes up to 68%
of redundant external memory while escalating fewer than 50% of inputs to the
large model, and retains over 98% of the downstream QA Exact Match** of an
exhaustive-retention baseline.

**What inber should consider:** the cheap half — a write-time router deciding
new / update / skip — is the one inber can build, and it is the missing piece
under `2608.19652`'s superseded-fact finding (08-22) and `2608.07429`'s
append-only result (08-24): all three say the same thing, that the defect is at
the write, not the read. `memory_save` has no supersession field and no
classification step, so a revised fact and the fact it revises both survive into
ranking. The parameter-consolidation half is out of scope and should be said so
explicitly, or it will be read as a prerequisite.

### 11. Lab and industry — the two items with the most weight

- **MCP specification revision `2026-07-28` removes the handshake, sessions and
  SSE resumability.**
  [Changelog](https://modelcontextprotocol.io/specification/2026-07-28/changelog).
  Nine breaking changes. `initialize`/`notifications/initialized` is **gone** —
  every request carries `io.modelcontextprotocol/protocolVersion` and
  `clientCapabilities` in `_meta`, and servers must implement a mandatory
  `server/discover` RPC (SEP-2575). Protocol-level sessions and `Mcp-Session-Id`
  are removed (SEP-2567): cross-call state becomes a server-minted handle passed
  as an ordinary tool argument. Server-initiated requests (`roots/list`,
  `sampling/createMessage`, `elicitation/create`) are replaced by **MRTR** — the
  server returns `resultType: "input_required"` with `inputRequests` and the
  client **retries the original request** with `inputResponses` — and every result
  now carries a required `resultType`. SSE resumability is removed entirely: no
  `Last-Event-ID`, no event ids. Roots, Sampling and Logging are deprecated on a
  12-month window, with the suggested Sampling migration being *"integrate
  directly with LLM provider APIs instead."* Two items are directly ours:
  servers **SHOULD** return `tools/list` in deterministic order, stated explicitly
  *"to improve LLM prompt cache hit rates"*, and list/read results now require
  `ttlMs` + `cacheScope` via a new `CacheableResult` interface.
  **For inber:** `tools/mcp/client.go` does the handshake and would need
  rewriting, and it has zero importers outside its own package — which makes this
  the moment to decide the open todo *"the tools/mcp package has zero importers —
  delete it or wire it"* with the extra fact that wiring it now means wiring the
  superseded protocol. The deterministic-`tools/list` rule is free input for the
  cache-breakpoint work in `agentic-design-patterns.md` 2026-08-26 §4.
- **OpenAI: two harness settings tripled ARC-AGI-3 while using 6× fewer output
  tokens.** GPT-5.6 Sol scored **13.3%** on the public set with the official
  harness and **38.3%** with retained reasoning and context compaction enabled.
  Their framing: *"the harness was not letting it remember what it had learned."*
  Retained reasoning means carrying reasoning items across tool calls and turns —
  `previous_response_id` or encrypted reasoning items on the Responses API — so
  the model does not re-derive its plan before every action.
  ⚠️ **Not first-party-verified:** openai.com returns 403 to the fetcher used
  here; the numbers are quoted verbatim on
  https://developers.openai.com/blog/codex-as-a-platform, which was fetched, and
  the 2026-07-29 date comes from search results only. Treat the date as
  unconfirmed.
  **For inber:** the OpenAI path is Chat Completions only — `OpenAIRequest`
  (`agent/openai_types.go:39-46`) has `Model`, `Messages`, `Tools`,
  `Temperature`, `MaxTokens`, `MaxCompletionTokens`, `Stream` and nothing else —
  so there is no reasoning item to retain and no `previous_response_id` to send.
  Every reasoning-model turn inber serves through that path re-derives its plan
  from scratch at every tool call. Whether that is worth a Responses-API path is a
  design question with a real cost (a second request builder, a second usage
  mapping, a second finish-reason table), and the existing open todos on that path
  — `25b91c78` (the reasoning-model switch is a two-prefix `HasPrefix`),
  `e68b05e0` (effort accepted and discarded) — are the ones it would have to be
  decided alongside.

### 12. Lab and industry — the rest, one line each

- **Anthropic mid-conversation tool changes** (beta
  `mid-conversation-tool-changes-2026-07-01`, Opus 5, 2026-07-24) — write-up in
  `agentic-design-patterns.md` 2026-08-26 §4; the doc states the prefix hash order
  `tools` → `system` → `messages` explicitly, which is the first first-party
  confirmation of a rule this directory had been inferring.
- **Anthropic advisor tool** — a cheap executor consults a higher-tier advisor
  mid-generation; `max_uses` is per-request and exceeding it returns
  `max_uses_exceeded` while the executor continues unadvised. Caching is
  `{"type":"ephemeral","ttl":"5m"|"1h"}` and the docs stress *"this is not a
  breakpoint marker. It is an on/off switch."* Measured: advisor output is
  typically 400–700 text tokens, a plaintext nudge to consult made **74% (Sonnet)
  to 98% (Haiku)** of nudged attempts call at turn 2, and nudging at turn 2 on
  workloads whose natural first call was turn 7+ correlated with a **3–4
  percentage-point performance drop**. Relevant to inber as a cache-preserving
  alternative to spawning an Opus child over the bus — and the nudge finding is a
  direct warning against hard-coding "ask the advisor first."
- **Anthropic Managed Agents limits**, worth copying as numbers someone else has
  already had to pick: **max 20 agents per roster**, delegation **exactly one
  level deep** (validated at create/update), **max 25 concurrent threads**, the
  roster **snapshotted at create** so referenced agents stay pinned, a **single
  shared budget across all threads** that stops new model requests with a
  `budget_reached` stop reason rather than killing the session, and an interrupt
  that closes each pending tool call with an error `tool_result` and re-emits idle
  **without sampling the model**. The last two are things a bus-based spawn/steer
  implementation gets wrong by default.
- **IBM Research / Hugging Face, ALTK-Evolve**
  (https://huggingface.co/blog/ibm-research/altk-evolve-hmm, 2026-08-18) —
  behavioural guidelines extracted from prior trajectories and re-injected at
  inference. On AppWorld across eight models 30B–745B the dose-response is the
  finding: gpt-oss-120b **39.9 → 56.0 TGC (+16.1pp)**, DeepSeek-V3.2 **+9.5pp**,
  Claude Opus **90.5 → 94.6 (+4.1pp)**, GLM-5 **0pp**. Token overhead
  **+5% for curated retrieval against +51% for injecting the full set**. Directly
  applicable: a memory tool that injects everything it holds on every step is
  paying the 51% for a benefit that shrinks toward zero as the model gets
  stronger.
- **Cursor changelog 2026-08-19** — subagents get **dedicated VMs** with
  independent project copies, and follow-up messages **queue for the next tool
  call rather than halting execution**. The second is a property inber already
  has: `InjectCheck` (`agent/agent_run.go` via `agent/agent.go:344-350`) is
  consulted between round-trips and appends to the last user message rather than
  restarting the turn. Anthropic's mid-conversation doc independently recommends
  the same pattern.
- **Inference hooks** (Anthropic beta, 2026-08-05, Enterprise) — every governed
  prompt is held for an external security server's allow/deny verdict **before
  inference**, signed, with configurable failure handling. A synchronous
  pre-inference gate, architecturally distinct from tool-call gating; relevant to
  permission-store's design if it ever grows one.
- **`agent-memory-2026-07-22` beta header** — memory listing becomes stable
  server-defined order with `order_by`/`order` **ignored**, `depth` restricted to
  `0`/`1`/omitted, and `path_prefix` matching whole path segments rather than
  substrings. Page cursors do not survive adoption. A real contract tightening.
- **Computer use out of beta** as `computer_toolset_20260801`, plus a new
  `browser_toolset_20260801` that reads the accessibility tree rather than pixels,
  and **web tool domain restriction** — `allowed_domains` / `blocked_domains` and
  `max_content_tokens` on `web_fetch`, a per-tool sandbox contract rather than an
  all-or-nothing switch.
- **Nothing new this window** from the Anthropic engineering blog (most recent
  posts 2026-04-23 and 2026-04-08), Google DeepMind, the Gemini API changelog, or
  Meta AI. Simon Willison notes the Muse Spark 1.2 card mentions *"rejection
  sampled harness trajectories and recipe optimizations for goals, compaction, and
  subagents"* with no accompanying write-up.
- ⚠️ **Verification note:** the Claude Code release-notes page now 307s to the
  GitHub `CHANGELOG.md`, which carries version numbers and **no dates**. This
  window's Claude Code entries were dated by fetching the ten CHANGELOG commits
  directly and are written up in `docs/comparisons/claude-code.md`.

### 13. Screened and rejected, with the reason

- **2608.23953** "The Empire, Long Divided, Must Unite" (cs.SE, 08-25) — a
  source-level study of three open harnesses finding convergence on five elements
  (commoditised loop, append-only replayable session record, model quirks as data,
  progressive disclosure, extension seams) and a total absence of external
  verifiability. Squarely on topic and **contains zero measured numbers**;
  excluded from the ranked list on that rule alone, and worth reading anyway for
  the taxonomy — inber has four of the five.
- **2608.24271** llmmas-otel — tool paper, *"initial validation on a minimal demo
  workflow,"* no numbers.
- **2608.23623** "When May an Agent Stop?" — heavily numeric (0/288 unsafe
  completions against 252/288) but **48 fully synthetic tasks** and
  near-degenerate intervals; low external validity.
- **2608.23740** AgentRoom (CRDT-backed concurrent multi-agent coding) — results
  are an ordering, not a split.
- **2608.23395** Right-Sizing LLM-Agent Decomposition — 4,400 runs and
  pre-registered, but the intermediate-optimum hypothesis is **unsupported** and
  the domain is cross-border VAT; the honest negative does not transfer.
- **2608.21884** Loop Engineering — mined 36,710 repos and confirmed autonomous
  loops in 217 of 256 heuristic matches, with the nice observation that repos
  commit loop *configuration* and almost never the prescribed state files. Mostly
  review plus a research agenda.
- **2608.23628** "Callability Is Not Operability" — abstract fetched, reports no
  results.
- **2608.21833** GameXpert-Bench — task counts only, no scores.
- **2608.22963** SPARE context pruning (37.89–64.58% reasoning tokens removed) and
  **2608.23252** Laws of Context Allocation (+16.7–20.5pp portfolio recall) — real
  numbers, wrong domain: multimodal tool use and RAG/generative search.
- **2608.23564** SWE Refactor Bench (08-24) — kept out of the ranked list only
  because it is an eval rather than a harness technique, but the number is worth
  recording: across **520 runs from 8 frontier models, 28 (5.4%) pass all three
  stages, 13 of 20 tasks receive no accepted solution, best model claude-opus-5 at
  47.0/100**, and the protocol exists specifically to catch *"Blindness"* — an
  agent copying the original implementation to make tests pass. The transferable
  claim is that **test-passing is not a completion signal**, which is the same
  hazard as the `updateMainSession` / spawn-summary path inber trusts.
- **2608.21747** Architecture as Capability Equalizer (08-22) — 90 multi-turn
  trials over five informationally equivalent spec formats × six models. Format
  barely matters on the strongest models (spread 0.17–0.92) and matters a great
  deal on weaker ones (0.83–2.42), with **TypeScript contracts tripling API route
  coverage for the weakest model (33% → 100%)**. Not ranked because it is about
  spec authoring rather than harness internals, but directly relevant to how
  inber phrases a sub-agent's task when it routes one to a cheaper model.
- **Broad topical near-misses, named so the next sweep does not re-fetch them:**
  `2608.24509`, `2608.23635`, `2608.23670`, `2608.23078`, `2608.22793`,
  `2608.22767`, `2608.22310`, `2608.23471`, `2608.22868`, `2608.22930`,
  `2608.23283`, `2608.20485`.
- **Verified and already present in this directory**, re-encountered this sweep:
  `2608.22752`, `2608.22708`, `2608.21690`, `2608.20664`, `2608.20195`,
  `2608.19799`, `2608.19741`, `2608.18389`, `2608.18280`, `2608.17597`,
  `2608.17528`, `2608.17485`, `2608.17393`, `2608.16801`, `2608.16370`,
  `2608.16022`, `2608.15584`, `2608.15008`, `2608.13867`, `2608.13662`,
  `2608.11775`, `2608.11386`, `2608.10934`, `2608.09802`, `2608.09799`,
  `2608.07855`, `2608.06503`, `2608.02110`, `2608.01347`, `2608.01326`,
  `2608.00902`, `2608.00808`, `2608.00101`.

---

## Sweep — 2026-08-27

Twelve papers not previously logged here. Every arXiv id below was confirmed by
fetching its abstract page; four of them (`2608.25322`, `2608.25920`,
`2608.22237`, `2608.16391`) were re-fetched a second time by the harness-watch
run itself, and title, date and headline number matched on all four.

### Ranked for inber

1. **Metis: Typed Runtime Mediation for Tool-Using Software Agents** —
   [2608.25322](https://arxiv.org/abs/2608.25322), v1 2026-08-26, cs.SE.
   A multi-provider agent runtime that turns provider streams into *typed
   events* before any admitted call reaches an external effect, so permission
   decisions, interference classes, terminal results and lifecycle transitions
   are explicit rather than inferred. Four-class mediation cut median elapsed
   time from 25.958 ms to 14.146 ms against forced serialization across 30
   matched real-I/O pairs. The child-boundary ablation is the part that matters:
   the gate-plus-registry condition blocked the declared unauthorized effect and
   **hid all five escape tools**; removing both protections reversed it.
   - **What inber should consider:** this is the closest published thing to
     inber's own shape — multi-provider, a tool registry, sub-agent spawn. Its
     child-boundary result argues a spawned child's tool set should be a
     *derived, narrowed* registry, and that narrowing must **hide** rather than
     deny: a denied-but-visible tool is still attempted and still costs a turn.
     inber currently does the opposite — `server/spawn.go:224` hands the child a
     zero `RunRequest`, so it inherits no mode at all
     (`server/tool_classification_test.go:127` pins that as today's answer).

2. **Read Less, Solve More: Token-Efficient Sparse Reading for AI Agents** —
   [2608.22237](https://arxiv.org/abs/2608.22237), v1 2026-08-23, cs.AI.
   SparseRead is a training-free reading layer that gates content admission
   *before* evidence enters the trajectory, on the argument that every existing
   context-reduction method intervenes only after the content is already in. Up
   to **92.9% token-volume and 89.0% wall-time reduction** across six frontier
   models and five scenarios, quality preserved or improved, portable across
   three agent frameworks.
   - **What inber should consider:** prevention is cache-preserving and
     compaction is not. inber's compactor rewrites the message array
     (`conversation/summarize.go:109-126`), which moves every breakpoint after it
     — `engine/prompt_blueprint.go:192-204` states that cascade in inber's own
     words. A read gate on tool results keeps the prefix byte-stable instead, and
     inber already has the truncation machinery to hang one on
     (`session/truncate.go:119-130`).

3. **Ventor-QTest: Threat-Model-Driven Verification of Vendor-Hosted LLM APIs** —
   [2608.16391](https://arxiv.org/abs/2608.16391), v1 2026-08-17, cs.CR.
   A black-box audit of hosted model routing needing no logprobs: average
   fidelity loss from repeated frozen-context requests, and extreme fidelity loss
   from the upper tail of a run-level surprisal statistic. Across seven route
   snapshots, AFL and EFL show little route-level association with GPQA-Diamond
   accuracy, but **pronounced EFL coincides with declining Terminal-Bench pass
   rate as task exposure increases** — long-horizon agentic correctness is
   sensitive to tail fidelity loss that short benchmarks cannot see.
   - **What inber should consider:** `engine/failover.go:22-60` demotes a model
     only on a hard error. This says the silently degraded route is the real
     hazard and that a benchmark score will not find it. A periodic
     frozen-context probe per provider, scored on the tail rather than the mean,
     is the missing input to `selectModel`.

4. **Repair or Resample? Rethinking Failure Debugging in LLM Multi-Agent
   Systems** — [2608.25920](https://arxiv.org/abs/2608.25920), v1 2026-08-26.
   Asks whether multi-agent debugging methods causally repair failures or merely
   exploit sampling randomness. Unguided rerun repairs **6.90%** of failures;
   symptom-driven intervention that reconstructs execution up to the failure
   point and regenerates only the subsequent steps reaches **20.15%**. Ships
   SymFail, 536 annotated failure cases.
   - **What inber should consider:** blind retry is close to worthless as a
     recovery policy, and inber's only post-failure retries are same-model
     replays of the whole turn (`engine/turn_execute.go:44-50`,
     `agent/agent_run.go:206-215`). Replaying to the failure point and
     regenerating only the tail is both the better repair and the cache-friendly
     one, because the replayed prefix is byte-identical.

5. **TOPAS: Workflow-Aware Prefix-State Scheduling for Multi-Agent LLM Serving** —
   [2608.25523](https://arxiv.org/abs/2608.25523), v1 2026-08-26, cs.CL.
   Formalizes the tension between retaining an agent's long system-prompt KV
   cache and starving concurrent batching, scoring candidate states by expected
   completion-time reduction against downstream prefix reuse, with aging against
   starvation. In SGLang: up to 39.8% mean and 49.4% p99 JCT reduction on
   synthetic workloads, 9.8/22.0/26.6% on MetaGPT tasks.
   - **What inber should consider:** the retention decision is per-*agent-prefix*,
     not per-request. When inber fans out siblings that share a system prompt and
     tool block, the breakpoint belongs on the shared boundary and the fan-out
     should be scheduled to land inside one TTL window — which is the scheduling
     half of the `promptCacheTtl` / `subagentPromptCacheTtl` split logged under
     2026-08-26.

6. **Weighted Memory Tree: Remembering What Matters for Long-Horizon LLM Agents** —
   [2608.20631](https://arxiv.org/abs/2608.20631), v1 2026-08-21, cs.AI.
   Hierarchical memory with a per-component retention score updated by events and
   decayed by non-selection. On GAIA-Text across three model variants: **+9.97
   percentage points accuracy while cutting prompt tokens 32.8%**. Under
   deliberately poisoned memories it bounds how far the bad information
   propagates.
   - **What inber should consider:** the containment result is the operative one.
     inber's memory importance drifts ×1.01 on read and ×0.99 daily
     (`memory-store/builder.go:124-143`) — a recency-ish rule with no decay on
     *non-selection*, so nothing evicts a memory that is retrieved and never
     used. A retention score gives a principled eviction rule and bounds the
     blast radius of one bad summarization.

7. **What Process Evaluation of Coding Agents Actually Measures** —
   [2608.22960](https://arxiv.org/abs/2608.22960), v1 2026-08-24, cs.AI.
   SCAE, a replay-based estimator from a structural causal model of agent
   execution. On 499 file-localization episodes from 12 repositories: next
   actions are driven primarily by **execution provenance rather than code-graph
   transitions**, execution uncertainty is structured at the task level not the
   step level, and full-trace judges show systematic **collider bias**.
   - **What inber should consider:** provenance-over-code-graph says the
     transcript's record of *which tool produced which result* predicts the next
     action better than the content does — so a compaction that drops provenance
     costs more than one that drops text. `conversation/message_utils.go:154-183`
     flattens to text and keeps tool names; that ordering is the right one and is
     now defensible on evidence rather than taste.

8. **From State to Action: OODA-Tool for Reliable Multi-Turn Tool Use** —
   [2608.24368](https://arxiv.org/abs/2608.24368), v1 2026-08-25, cs.AI.
   Names "state-action competition": direct function-calling and ReAct learn
   state tracking and action generation in one autoregressive trajectory, so
   pressure to emit the next call overwrites earlier accumulated information.
   Routing each decision through controller-checked intermediate states gives
   consistent gains on Qwen3 0.6B–14B, larger on smaller models and on tasks
   depending on cross-turn accumulation.
   - **What inber should consider:** the Orient gate — *is execution warranted at
     all?* — is a cheap-tier trick. inber has a failover chain ordered by
     model-store priority and no notion of routing part of a turn to a cheaper
     row; abstention is a far easier judgement than call synthesis.

9. **JIT-Agent: Scaling Harness Intelligence via Just-in-Time Harness Evolution** —
   [2608.25593](https://arxiv.org/abs/2608.25593), v1 2026-08-26, cs.CL.
   Argues the harness — memory management, planning strategy, action protocol,
   tool orchestration — can dominate the model's contribution, and makes it a
   composable machine-generatable artifact under a fixed four-module protocol.
   DeepSeek-V4-Flash + JIT-Agent beats GPT-5.6 on DeepSearchQA (+9.1) and
   OdysseyBench (+4.3); GLM-5.2 gains up to +20.2.
   - **What inber should consider:** the four-module protocol is a concrete spec
     for making an agent definition *data*. If inber's ten agents differ only
     along those axes they are agent-store rows that can be generated and
     compared per task, not hand-written Go personas.

10. **ToolMinimize: Auditing and Rewriting LLM Agent Tool Calls to Minimize
    Privacy Exposure** — [2608.24957](https://arxiv.org/abs/2608.24957), v1
    2026-08-25, cs.CR. Measured across three frontier models: **81–88% of tool
    calls carry unnecessary privacy-sensitive data** under default prompts, and
    explicit privacy instructions still leave 36–76% over-sharing. Existing
    defenses gate or label flows and cannot rewrite argument *values*; PII
    detectors miss implicit signals. Schema-aware necessity analysis plus
    removal/generalization/substitution gets 81.2–92.0% reduction at 100%
    argument-level task validity, 79.0% on unannotated MCP schemas, median
    latency 1.77 ms.
    - **What inber should consider:** the design point is a per-field
      `minimum_necessary` annotation on the tool *schema*, not a runtime
      component. That is a field on a registry entry, and it would give
      `redact/` a deterministic rule instead of a pattern list — relevant to the
      open finding that the redactor guards one door of four (`d60ec4a3`).

11. **From General Agents to RCA Experts: A Self-Evolving Harness** —
    [2608.25661](https://arxiv.org/abs/2608.25661), v1 2026-08-26, cs.SE.
    Finds general-purpose agents now frequently beat purpose-built RCA agents,
    and that the remaining gap lives in **the adaptation layer, not the agent**.
    59.0% top-1 accuracy, +63.4% over bare agents, with dual-gate verification on
    every knowledge update.
    - **What inber should consider:** the dual gate is the transferable piece. If
      inber ever lets an agent write back into its own tool registry or memory,
      an accumulated-knowledge update needs a verification gate or the archive
      degrades silently — which is the same argument the Weighted Memory Tree
      poisoning result makes from the other side.

12. **CatchBench: When Can an Agent Failure Be Caught?** —
    [2608.22808](https://arxiv.org/abs/2608.22808), v1 2026-08-24, cs.LG.
    Puts one auditor's question to three information states — declared
    configuration before the run, a growing trace prefix, and the finished trace
    — across 72 entrants, 1187 configurations and 1162 runs. Only 47 of 118
    pre-declared contrasts separate; the rest are published unresolved. Its
    sharpest finding cuts against its own authors: a rule that ignores every name
    and permission and flags each capability declared after the first hits
    perfect F1 on one configuration source, measuring corpus construction rather
    than reasoning.
    - **What inber should consider:** the PRE/LIVE/POST split is the right frame
      for where a guard goes, and most of what inber cares about is catchable at
      PRE — the declared agent plus its tool set — which is far cheaper than
      judging a finished transcript. The shortcut result is a standing warning
      against any single-number harness-safety score.

### Checked and dropped

- **`2608.14624`** CacheScout (agent-aware KV eviction and prefetching, +10–18pp
  hit rate) — relevant and not logged here, but the abstract page gives v1
  **2026-07-16**, outside this sweep's window. Worth a backfill.
- **`2608.21375`** SchemaRouter — v1 **2026-07-03** despite the 2608 id. Out of
  window.
- **Anthropic, "Harness design for long-running application development"** —
  published **2026-03-24**, out of window and absent from this directory. Its
  context-reset plus structured-file-handoff pattern, and the "context anxiety"
  failure mode, bear directly on inber's compactor. Worth a backfill.
- Already logged here and re-encountered: `2608.24358`, `2608.22752`,
  `2608.20664`, `2608.23550`, `2608.22708`, `2608.23953`, `2608.24188`,
  `2608.23651`, `2608.24804`.

## Sweep — 2026-08-28

Twelve papers not previously logged here. Every arXiv id below was verified by
fetching the abs page or the arXiv API `id_list` endpoint; title, date and
abstract matched on all twelve, and each id was grep-checked against this file
before inclusion. Ten distinct queries were run. **No lab-blog entries: DeepMind,
OpenAI and Meta AI published nothing on harness design, context, caching, memory
or agent security in the 2026-07-29 → 2026-08-28 window, and Anthropic's two
nearest posts are out of window** ("Effective harnesses for long-running agents",
2025-11-26; "Scaling Managed Agents: Decoupling the brain from the hands",
2026-04-08 — the second is a plausible backfill alongside the one already flagged
in the 08-27 sweep).

### Ranked for inber

1. **Same Model, Different Harness: Different Coding-Agent Results** —
   [2608.26218](https://arxiv.org/abs/2608.26218), 2026-08-26, cs.AI.
   Model and task held fixed; the only change is a harness that **mechanically
   shortens older tool results as the context fills**. Mean per-task
   fail-to-pass went from **28% to 49%** on a 169-task SWE-bench Verified cohort
   (20,480-token window, 480 s cap), complete solutions from **43 to 72**, and
   the frozen treatment transferred to three other models with no retuning while
   serving fewer prompt tokens per turn.
   - **What inber should consider:** this prices `session/truncate.go` as a
     *capability* lever, not a cost lever, and it is the strongest argument yet
     for fixing the tier selection held back in tonight's
     `agentic-design-patterns.md` §5 — every agent currently takes one flat
     1000/500/200 tier. Age-ordered progressive shortening is a different policy
     from a per-result cap, and it is cache-friendlier than compaction because it
     never rewrites the recent tail.

2. **When Context Gets Root: Privilege Escalation in LLM Harnesses** —
   [2608.27299](https://arxiv.org/abs/2608.27299), 2026-08-27, cs.CR/cs.SE.
   The harness's own context construction promotes low-privilege content to a
   higher instruction level. **All 13 attack objectives succeeded on all 6
   coding-agent harnesses** with unrestricted execution, and **all 13 on all 3
   harnesses that offer automatic permission review**, reproduced through
   harness-provided persistent goals and scheduled tasks.
   - **What inber should consider:** inber assembles the prompt in
     `engine/prompt_blueprint.go` and injects memory-store rows and tool results
     into it; anything the blueprint promotes into the system region inherits
     system privilege. The second number is the sharp one — an automatic
     permission review did not help, so a spend or tool gate is not a mitigation.
     This is the measured version of open todo `ceedbf75` (one injection channel
     carrying four principals all stamped "user").

3. **When Tool Outputs Become Commands: Separating Action Induction from Runtime
   Authorization (SARA)** — [2608.27146](https://arxiv.org/abs/2608.27146),
   2026-08-27, cs.AI/cs.SE. Authorizing calls only against the user objective plus
   audited evidence, with a **No-History-Promotion** rule so a recurring call
   cannot launder its origin, holds **ASR ≤ 0.63%** across four settings on
   AgentDojo and AgentDyn at competitive utility.
   - **What inber should consider:** No-History-Promotion is the rule to steal,
     and inber has the exact laundering path it names — `conversation/summarize.go`
     collapses a transcript into prose, after which an action that first appeared
     in a tool result is indistinguishable from one the user asked for. The 08-24
     entry's "a content fragment's *kind* is a required field" is the same
     invariant arriving from the security side.

4. **The Framing Gap: Indirect Prompt-Injection Exfiltration Defeats
   Surface-Level Defenses** — [2608.27092](https://arxiv.org/abs/2608.27092),
   2026-08-27, cs.CR. Ten overt injection classes are refused (**gpt-4o 0%**), but
   reframing the identical leak as a mandatory integrity signature or config field
   drives it **0% → 100%**. Paraphrasing a known template is trivial (**96% at 3
   wordings**); authoring a fresh mechanism is hard (**0/130**). Only payload-blind
   checks close it: destination allow-list **0%**, planner/reader capability split
   **0%** — while SecAlign fine-tuning leaves **32.5%**, channel separation
   **38.8%**, and an output-normalizing guard loses to ROT13 at **100%**.
   - **What inber should consider:** both working defences are harness-layer and
     cheap, and inber has neither. The planner/reader split maps directly onto the
     existing fork/delegate primitive: a sub-agent that reads untrusted content
     and holds no network or write tools is exactly the isolation measured at 0%.
     Note this needs the child's tool set to be a *narrowed* set, which
     `server/spawn.go:224`'s zero `RunRequest` does not currently produce.

5. **PILOT in the Loop: Live Self-Improvement for Long-Horizon Agents** —
   [2608.26530](https://arxiv.org/abs/2608.26530), 2026-08-27, cs.AI.
   A supervisor that can **redirect or abort an active worker mid-execution**:
   first in 5 of 6 configurations, **+9.8 pp on Terminal-Bench 2.0**, **+14.6 /
   +12.4 points** in the self-improvement setting, mean output tokens **down
   42.9% / 47.4%**, successful evaluations per million output tokens **up 110.3% /
   134.0%**.
   - **What inber should consider:** inber's delegate is fire-and-forget — the
     parent sees the child only at completion (`server/spawn_delivery.go`). The
     token reductions here come from killing bad sub-agent runs early, which makes
     live abort a *budget* mechanism, not just a quality one. It also depends on an
     abort that actually lands, which tonight's `7de193b1` says inber's does not
     for a queued child.

6. **Agent Mesh: Reliability Primitives for Non-Idempotent Agent Delegation** —
   [2608.26225](https://arxiv.org/abs/2608.26225), 2026-08-26, cs.AI.
   Failure study of a production agentic platform: **147 incidents over 81 runs**.
   A **54-consecutive-successful-tool-call loop that no error-rate breaker could
   see**; **21 events accumulated across 6 invocations of one delegation**, making
   an idempotent component unwinnable; and **12 incidents where the enforcement
   layer blocked correct work, the worst costing 107 agent turns and zero accepted
   writes**.
   - **What inber should consider:** the enforcement unit should be the
     **delegation, not the turn**. inber's guard and retries are per-turn, so a
     sub-agent invoked six times accumulates state the parent's accounting cannot
     see — which is the same gap codex #41183 closed and open todo `9e31d359`
     parks. The 107-turns-zero-writes case is what an over-tight guard looks like
     from the inside, and is the counterweight to that todo's shared-pot option.

7. **Can your AI agent be cheaper? Effects of task specifications on token
   spend** — [2608.25399](https://arxiv.org/abs/2608.25399), 2026-08-26, cs.AI.
   Across **2,700 runs**: reducing a full specification to a bare user story
   raises token spend **29.7%**; run-to-run variance is unaffected by any prompt
   change; prompt-sensitivity is task-dependent from **13% to 115%**; and a simple
   predictor prices a full distribution of spec × thinking-effort configurations
   from **one cheap probe, within 36%**.
   - **What inber should consider:** a pre-flight estimator for `guard/`. inber's
     cap is reactive — it stops a run that has already burned the budget
     (`engine/turn_postprocess.go:87`). A one-probe predictor lets the guard refuse
     or downgrade before the spend, and the 29.7% figure says the cheapest
     available saving is rejecting underspecified task text, not tuning the model.

8. **Safety Does Not Compose: Non-Decaying Loop State for Autonomous LLM
   Agents** — [2608.27141](https://arxiv.org/abs/2608.27141), 2026-08-27,
   cs.CR/cs.AI. A separation result: against an attack whose evidence is
   fragmented across iterations, **every trajectory-scoped monitor has a
   true-positive rate equal to its false-positive rate**, regardless of
   expressiveness, while a monitor retaining cross-iteration state separates
   perfectly. A geometrically decaying risk score is insufficient because the
   adversary's cooling-off wait is a constant in the horizon. LoopHarness bounds
   expected unauthorized irreversible actions at **B+m−1+m/δ_M**, constant in N.
   - **What inber should consider:** inber's guards are session-scoped and reset at
     each session boundary — `restoreGuardState` (`server/session_creation.go:316`)
     restores totals precisely so a rebuild does not hand the budget back, which is
     the right instinct applied to money and not to safety. This says a safety
     counter must live outside the conversation and must **not** decay.

9. **MemToC: Benchmarking Memory-Tool Conflict Resolution** —
   [2608.26295](https://arxiv.org/abs/2608.26295), 2026-08-26, cs.CL.
   6,504 episodes over 542 questions: instruction-tuned 7–9B models **retain a
   verified-correct memory against an incorrect tool return in only 6.5–17.1%** of
   eligible cases, follow a correct tool in **86.0–93.1%**, and **repeat the tool
   return in 78.4–86.0% of cases where both sources are wrong**. No cross-model
   ordering survives three instruction wordings, and **19 of 20 method-model
   combinations reduce abstention** after tool errors.
   - **What inber should consider:** a wrong tool result overrides a correct memory
     almost always. inber injects retrieved memory rows as context that competes
     directly with live tool output and supplies **no arbitration signal at all**,
     so a stale or poisoned tool result silently overwrites a good memory. Pairs
     with the 08-24 entry's finding that append-only agent memory scored worse than
     no memory.

10. **Routed Graph Handoff: Adaptive Format Selection for Multi-Agent LLM
    Delegation** — [2608.25277](https://arxiv.org/abs/2608.25277), 2026-08-26,
    cs.CL. Natural-language inter-agent messages consume **40–60% of the token
    budget**. A **155-token router (0.15% overhead)** choosing per delegation
    between a typed dependency graph and natural language gives **+12.7 pp on
    τ-retail at 3.2× compression** and **+8.7 pp on BrowseComp at 2.2×**, with
    parity on BFCL/AppWorld — while graph-only delegation **regresses 14.6 pp** on
    AppWorld without the router.
    - **What inber should consider:** the handoff payload in `server/spawn.go` is
      prose, and the child's answer returns as prose through
      `server/spawn_delivery.go:54-67`. A typed structure is cheaper, but the
      ablation says forcing it universally is a net loss — the per-delegation
      router is the load-bearing part, and it is small enough to be a cheap-tier
      call.

11. **The Guard That Cried Wolf: How Scary Words Make Agent Guardrails Refuse
    Legitimate Actions** — [2608.27009](https://arxiv.org/abs/2608.27009),
    2026-08-27, cs.CR. Cautious Bench: **756 decidable benign/twin pairs under
    three object-name types (2,268 measured pairs)** plus 40 undecidable, labels
    mechanically re-derived from a stated authorization policy. **All six
    guardrails across five designs over-refuse an authorized action more often
    under a scary-looking object name than a benign one** — only the object name
    varies, so the guardrail reads the surface label rather than the authorization
    context.
    - **What inber should consider:** pairs with the 28:1 over-refusal result
      already logged for `2608.12654`. Any model-driven approval gate inber adds
      will refuse on filenames like `credentials.go` or `secrets_test.go`
      regardless of what the call does — an argument for the payload-blind,
      policy-based gating of #4 over an LLM judge, and a caution for
      `permission-store` (`:8304`) before it grows one.

12. **SKILL.state: Scalable Long-Horizon Agent Skills** —
    [2608.26263](https://arxiv.org/abs/2608.26263), 2026-08-26, cs.AI.
    Replaces append-only conversational history with an explicit mutable execution
    state: each step the model sees only the immutable skill spec, the current
    structured state and the latest observation, with intermediate reasoning
    discarded after a validated state update. Reports improved accuracy and
    substantially lower cumulative tokens across datasets, models and environments
    — **direction only; the abstract gives no headline percentage**, which is why
    it ranks last here.
    - **What inber should consider:** an alternative to compaction rather than a
      better compactor. inber's summarizer rewrites the message array
      (`conversation/summarize.go:109-126`) and cascades every cache breakpoint
      after it, a cost `engine/prompt_blueprint.go:192-204` states in inber's own
      words; a fixed-shape state block keeps the prefix byte-stable by
      construction. A design lead, not a result.

### Verified, logged, not ranked

- **[2608.25198](https://arxiv.org/abs/2608.25198)** *Tunable Tool-Call Rates via
  Representation Steering* (08-25) — a single linear residual-stream direction
  moves call rate from ~0% to >90% and nearly doubles open-domain QA accuracy
  (0.29→0.56). Inapplicable to inber's API-only providers; the cost/accuracy
  Pareto framing is the transferable part.
- **[2608.24087](https://arxiv.org/abs/2608.24087)** *Knowing When to Ask for
  Help* (08-25) — intra-generation Bayesian escalation to a stronger model, on a
  Qwen2.5-Coder 1.5B→7B cascade (MBPP, 257 tasks). Relevant to
  `engine/failover.go`, which escalates only on a hard error and never on
  difficulty.
- **[2608.25241](https://arxiv.org/abs/2608.25241)** *A Few Pages of Markdown*
  (08-26) — across 441 repos, agent-first repos *without* committed AI
  configuration show ~2× the cognitive-complexity increase (**+53% vs +27%**) and
  1.7× the static-analysis warnings; **73.8% of AI-config artifacts are committed
  once and never modified**. That last number is a fair warning about this corpus.
- **[2608.23552](https://arxiv.org/abs/2608.23552)** *Prime Agent* (08-24) —
  self-improving harness with a persistent IPython REPL and recursive subagents;
  ARC-AGI-3 RHAE Best@1 from 30% to 95.5%.
- **[2608.25683](https://arxiv.org/abs/2608.25683)** *psRL* (08-26) — training-time
  prefix sharing, up to 5.2× throughput. Training-side, out of scope.
- **[2608.26004](https://arxiv.org/abs/2608.26004)** *AsymSpec* (08-26) — drafter
  reads full context, verifier reads the compressed view; ~90% of full-context
  accuracy at 1.3–1.7× throughput. Requires local serving.
