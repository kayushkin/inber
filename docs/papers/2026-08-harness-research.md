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
