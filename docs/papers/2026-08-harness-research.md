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
