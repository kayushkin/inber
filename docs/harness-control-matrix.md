# Harness Control Matrix

A coverage audit of inber against an external taxonomy of harness control
boundaries, taken from the Mojo AI Studio "Harness Skills" board
(<https://mojoaistudio.com/harnesses/>, captured 2026-07-13).

Companion doc: `llm-bridge-server/HARNESS-CONTROL-MATRIX.md` runs the same audit
against llm-bridge, and carries the fuller explanation of the taxonomy.

## The taxonomy in one paragraph

A harness is the control wrapper around model work: it decides what goes in, what
is allowed to happen, what must come out, what proof is captured, and how the
workflow recovers. The board enumerates **39 boundaries**, each with three
orthogonal control axes — capability, performance, cost. Boundaries divide into a
**control plane** (Orchestrator, Proof Authority, Tool Registry, Session Manager,
Memory, Git Control, Release Readiness, Review Queue, Project Scope, Health
Monitor, Interface, File Navigation, Guardrails, Network Protocol, Sub-agent
Dispatch) and a **per-model-call layer** (Provider, Identity, Select, Route, Model
Router, Fallback, Prompt, Context, Goal, Policy, Tools, Response, Evidence, Trace,
Payload, Verify, Trust, Cost, Latency, Privacy, Safety, Compare, Knowledge
Retrieval, Browser Capture).

The board's 117 downloadable `SKILL.md` files are template-generated boilerplate
and not worth installing. The boundary list and its ~250 levers are worth using as
an audit checklist, which is what this doc does.

## Headline finding

**Four of inber's packages that the taxonomy would score as distinctive assets —
`guard`, `trace`, `checkpoint`, `codeindex` — are TODO stubs that the engine calls
on the hot path and that silently do nothing.**

- `guard/guard.go` — `CheckTool()` has zero callers. `Mode` is hardcoded to
  `Autonomous`; `ApprovalFunc` is never set; `RecordToolCall`/`IsRepeating` are
  dead. Only `CheckLimits` (turns/tokens) is live. `guard/classification_test.go:42`
  says so in prose: *"The classifiers are unreachable today — CheckTool has no caller."*
- `trace/` — `engine.go:176` calls `NewRecorder("", …)`, which returns `nil`.
  `RecordTurn`/`WriteSummary` are no-ops. The real tracing lives in `session/timeline*.go`.
- `checkpoint/` — `Take()` is called **every turn** and does nothing. `session/checkpoint.go`
  is write-only: `LoadCheckpoint`/`ListCheckpoints` have zero callers, and
  `pruneOldCheckpoints` is `return nil`.
- `codeindex/` — `Open()` returns an empty struct; `Search`/`RepoMap` are TODO and
  never called.

They appear in neither `ARCHITECTURE.md` nor `BACKLOG.md` (which has 88 `[x]` and
zero `[ ]` — it is a refactor log, not a product backlog). They are doc comments
describing systems that were never built, wired into `RunTurn` so they read as
live. That is worth knowing before anyone plans on top of them.

## Coverage

Tally: **0 COVERED · 27 PARTIAL · 12 ABSENT**.

| Boundary | Verdict | Owner | Missing levers |
|---|---|---|---|
| Browser Capture | PARTIAL | `tools/tools.go:Browser()` → tool-store `browser.go` | all 8 levers. Worse: `browser.go:145` returns the screenshot as a base64 **text** block, never an image block — the model cannot see it |
| Compare | ABSENT | — | all; zero hits for juror/vote/consensus/ensemble |
| Context | PARTIAL | `engine/turn_prompt.go:BuildSystemPrompt`, `turn_context.go:contextBudget` | pluggable packing strategy (it's a hardcoded if-ladder). ~~per-model window map~~ **FIXED `c4ad0fa`** — `GetModelInfo` looked the model up by display *name*, which no Anthropic row matches, so every model took a hardcoded 200k. It now resolves by id through `Store.ResolveModel` and takes `Model.MaxTokens`, capped at what the client can request (no long-context beta — todo `52c9b341`) |
| Cost | PARTIAL | `session/timeline_cost.go:CalcCostWithCache` | budget pre-check; per-project/task budgets; alert/halt thresholds. `MaxCost` is dead in 3 places (`guard.Config`, `EngineConfig:53`, `server.RunRequest:237`) and `Guard.RecordCost()` has zero callers. **Pricing is always $3/$15 flat** — five call sites pass `store=nil` and four of them also pass `model=""` (`server/spawn.go:284`, `server/spawn_delivery.go:44`, `server/server.go:351,373`, `session/timeline_jsonl.go:128`). The registry lookup underneath them was fixed in `c4ad0fa`; these callers still ask it nothing. Filed as todo `9317ef2b` |
| Evidence | ABSENT | — | all |
| Fallback | PARTIAL | `engine/failover.go:selectModel/fallbackChain` | retry policy; warm standby; tier cap. The fast-fail timeout **is computed then discarded** — `turn_execute.go:18` drops the second return value and the ctx gets no deadline |
| File Navigation | PARTIAL | tool-store `repo_map`/`recent_files` | `codeindex/` is a stub (above). No ownership graph, capability map, path cache, incremental reindex |
| Git Control | PARTIAL | `engine/workflow_git.go`; `forge` via `server/forge_iface.go` | branch leases; risk scoring; **main-write gate** — `finishSessionGit` auto-`git push`es the current branch, including main, at session close; CI gating; conflict prediction |
| Goal | ABSENT | — | all. Spawn `Task` is a free-form string; no objective schema |
| **Guardrails** | **ABSENT** | `guard/guard.go` (stub) | **everything.** No tool gate of any kind on a harness whose `shell_commands` is `bash -c` with the full `os.Environ()` |
| Health Monitor | PARTIAL | `server/session_reaper.go`; `session/db_sessions.go:DetectInterrupted` (PID liveness) | heartbeat; stall detection; anomaly window; auto-recovery; alert routing; probe backoff |
| Identity | PARTIAL | `agent/clients.go:NewModelClient` → model-store `ResolveModel` | version allowlist (`Model.Enabled` is never checked); identity-record schema. **`resp.Model` is never read or persisted** — you cannot audit what actually served a request, and failover can swap it silently |
| Interface | PARTIAL | `server/api.go`, `api_bridge.go` (SSE), `inber-cli`, `server/events.go` → NATS | panels/widgets plugin; layout presets; virtualization; render throttle. No operator UI in-repo |
| Knowledge Retrieval | PARTIAL | tool-store `web_search`/`web_fetch`; `memory-store/search.go` | a real embedding model (it is a **256-bucket hash of term frequencies**, self-labeled a placeholder); any index at all (full table scan + **O(n²) selection sort**); rerank; top-k/chunk tuning; embedding cache; lineage |
| Latency | PARTIAL | `engine/failover.go:timeoutFromHealth`; model-store `AvgResponseMs` | SLO config; availability probes; slow-route demotion; batch mode. Latency data is collected but **no lever consumes it** |
| Memory | PARTIAL (deepest asset) | `memory/` → memory-store (Save/Search/BuildContext/Compact/DecayImportance) | recall index; **dedup on write** (fresh `uuid` per save → duplicates accumulate); conflict resolution; expiry (`ExpiresAt` exists, inber never sets it; rows are never hard-deleted); size caps; write batching; scheduled decay sweep; provenance depth |
| Model Router | PARTIAL | `engine/failover.go` + model-store `FailoverChain()` | route scoring weights; route learning; tier-down; per-route token ceiling (`MaxTokens: 16384` hardcoded at `agent_run.go:62`); cache-hit reuse; warm pool. Provider adapters are a hardcoded `switch` |
| Network Protocol | PARTIAL | `bus/client.go`, `server/bus.go` (WS subscribe + HTTP publish, NATS-backed) | node-trust policy; capability advertisement; packet signing; transport adapters; handshake cache; discovery; remote-call budgeting. It is a chat relay, not a node protocol |
| Orchestrator | PARTIAL | `server/queue.go:Queue` (lane semaphore + per-session mutex; main:4, subagent:8) | arbitration strategy — it is FIFO/blocking, there is no "why this next"; priority weights; preemption; attention slice; attention budget; idle reclaim; operator override |
| Payload | PARTIAL | `engine/prompt_blueprint.go` (per-block SHA-256 + cache-hit prediction), `turn_prompt.go:hashStrings` | configurable hash/sign scheme; signing; a request fingerprint stored for audit. The hashing exists but serves **cache prediction, not proof**, and is off by default (`INBER_BLUEPRINT=1`) |
| Policy | ABSENT | — | all |
| **Privacy** | **ABSENT** | — | **all.** Zero egress redaction. The only `redact` hits are Anthropic's own `redacted_thinking` (`conversation/repair.go:207`). `shell_commands` is `bash -c` with `os.Environ()`; `read_files` has no denylist |
| Project Scope | PARTIAL | `AgentConfig.Projects`; `forge` workspaces; repo-store | objective schema; dependency connectors; handoff templates; drift signals; scan frequency |
| **Prompt** | PARTIAL (strongest area) | `agent/agent.go:315 addHistoryCacheBreakpoint`, `engine/turn_prompt.go:183 buildSystemBlocks` (SHA-256 → byte-identical prefix reuse), `conversation/dedup_files.go` | template library (plugin); injection-order rules (hardcoded); template precompile. **The OpenAI path has zero caching.** Two dated, self-assigned actions in `docs/cache-optimization.md` (2026-05-11 BP3 retarget; 2026-07-01 volatile-blocks-to-tail) are unactioned |
| Proof Authority | ABSENT | — | all |
| Provider | PARTIAL | `agent/clients.go:newClientFromKey` (anthropic native; openai/google/openrouter/ollama via OpenAI-compat); `aiauth.ResolveKey` | pluggable adapters (hardcoded switch); pooling/keepalive; region selection; **batch endpoints** (no Message Batches API); compression. The `google` and `zhipu` paths look broken (`clients.go:126,152`) |
| Release Readiness | ABSENT | `engine/workflow_deploy.go` exists with **zero callers** | all. `VerifyDeployment` is unreachable and the `VerifyDeployed` config flag does nothing |
| Response | PARTIAL | `agent.Run` streaming; `api_bridge.go` SSE; `session/jsonl.go`, `timeline_jsonl.go`, `logstack.go` | artifact schema; post-process hooks; delta-only storage; compression |
| Review Queue | ABSENT | — | all |
| Route | PARTIAL | `server/queue.go` lanes | route table; dispatch hooks; warm route; per-route token cap |
| **Safety** | **ABSENT** | `guard/guard.go` (same stub) | all. `isDangerous`/`isReadOnly` exist and are unreachable |
| Select | ABSENT | — | all. Model is a static `AgentConfig.Model`; there is no selection step, no capability catalog, no tier default |
| Session Manager | PARTIAL (strong) | `session/*`, `server/session_creation.go`, `session_forking.go`, `session_reaper.go`; fork/resume/compact/interrupt/stop; `forge` worktrees | idle-**suspend** (the reaper *closes*, never suspends/resumes); snapshot retention (`pruneOldCheckpoints` = `return nil`); state compaction; lifecycle hooks; recovery scripts; resume prefetch. `checkpoint/` is a stub (above) |
| Sub-agent Dispatch | PARTIAL (strong) | `server/spawn.go`, `spawn_tools.go`, `ForkAndSpawn`, `agent/registry/`, agent-store agent types, `steer_agent` | result-schema enforcement (task and result are free-form strings); planning strategy; pipeline-vs-barrier stages; per-stage worker tier; agent-count ceiling beyond the lane cap |
| Tool Registry | PARTIAL | `agent/registry/tools.go:ToolRegistry`; per-agent allowlist (`registry.go:220`); tool-store | permission policy; dry-run; **sandboxing** (`bash -c` + unjailed `os.WriteFile`); reversibility/rollback; result cache; parallel exec; tool-call ceilings. **`tools/mcp/` is complete and has zero importers** — the plugin path from `docs/plugin-research.md` was built and never wired |
| Tools | PARTIAL | per-agent allowlist; `engine/build_hooks.go:buildLimitCheck` | parallel safe calls (`agent_run.go:224` is a strictly sequential loop); tool-result cache (`read_cache.go` is a *re-read suppressor*, not a result cache); a real tool-call ceiling — `maxAPICalls=50` caps round-trips, not tool calls, so one response carrying 30 `tool_use` blocks runs all 30 |
| Trace | PARTIAL | `session/timeline*.go`, `db_turns.go`, `jsonl.go`, `logstack.go` — *not* `trace/` | `trace/` is dead (above). Missing: payload hash; evidence; trace schema config; sampling; retention; async write |
| Trust | ABSENT | — | all. model-store "health" is liveness, not trust |
| Verify | PARTIAL (thin) | `engine/workflow_build.go:buildAndTest{Go,Node,Rust}` — injects test failures back into the conversation | verification checks (hardcoded per-language); evidence schema; sampling; risk tiering. No provider/route/response verification at all |

## Ranked build list

1. **Guardrails / Safety — wire `guard.CheckTool()` into the tool loop and emit
   `EventApproval`.** One fix closes three boundaries. Today there is no tool gate
   of any kind on a harness that runs `bash -c` with the full environment. It is
   also load-bearing across three repos: because inber never emits `EventApproval`,
   llm-bridge-server's `ask`/`plan`/`block_all` permission modes are
   **unenforceable against inber sessions** (`BRIDGE_PARITY.md`, "Remaining" item 1).
2. **Cost — a pre-dispatch budget check, and fix the pricing bug.** Every dollar
   figure inber reports is `tokens × $3/$15` regardless of model, because five
   call sites hand `GetModelInfo` a nil store and four of them an empty model id
   (todo `9317ef2b`). The registry lookup itself is fixed (`c4ad0fa`); the
   callers are not. `max_cost` is accepted over the API and silently ignored.
3. **Privacy — any egress redaction at all.** Zero exists. `cat .env` goes straight
   into `params.Messages`. The cheapest high-severity fix on this list.
4. ~~**Context — a real per-model window map.**~~ **Done, `c4ad0fa`.** There was no
   map to write: the model-store already records each window as `Model.MaxTokens`,
   filled from each provider's own API by `ms sync`. What was missing was reading
   it — the lookup keyed on the display name, matched nothing, and fell through to
   a hardcoded 200,000 that drove the auto-prune threshold (`contextWindow/2`).
   The five 128k OpenAI rows now arm the guard earlier than the hardcode did.
5. **Memory — recall index, dedup on write, actual expiry.** The store is inber's
   deepest asset and it is a full table scan with an O(n²) sort over 256-bucket hash
   pseudo-embeddings, with no dedup and no hard delete. It degrades quadratically and
   grows forever.
6. **Tool Registry — wire the MCP client that already exists.** `tools/mcp/{client,adapter}.go`
   is complete, real, and imported by nothing. This one wiring unlocks the plugin
   capability lever across Tool Registry, Knowledge Retrieval, Browser Capture, and
   Evidence.
7. **Trace — build `trace/` or delete it.** It is called from `RunTurn` and writes to
   nothing. Building it also gives Payload an audit sink.
8. **Checkpoint — build `Take`/`Diff`/`Restore`.** Called every turn, does nothing.
   Real git-backed checkpoints give Session Manager rollback *and* Tool Registry
   reversibility, which is the prerequisite for ever running in Assist mode.
9. **Orchestrator — an arbitration strategy.** `server/queue.go` is a semaphore;
   there is no "why this session next," no preemption, no attention budget. For a
   framework whose selling point is multi-agent orchestration, this is the biggest
   conceptual gap.
10. **Select / Model Router tier-down.** No task→model selection exists (model is
    static per agent). A tier-down policy is the single largest cost lever available
    and costs little on top of the existing model-store integration.
11. **Health Monitor — stall detection.** The reaper only evicts idle *bridge*
    sessions on a TTL. There is no heartbeat and no stall detection, so an unattended
    agent that hangs mid-turn hangs forever.
12. **The two unactioned items in `docs/cache-optimization.md`.** Both are
    self-assigned "Action:" lines in inber's own doc, both cheap, both directly reduce
    token spend in the area where inber is otherwise best-in-class.

## Honest summary

Inber's real strengths are **prompt caching** (three breakpoints, hash-based
byte-identical prefix reuse, a blueprint differ that predicts hit/miss — genuinely
better than most harnesses), **context management** (stash → repair → LLM summarize
→ staged prune → file-ref dedup → head/tail truncate), and **sub-agent dispatch**
(config-driven agent types, forge worktree isolation, an orchestrator-only merge
gate).

The seven packages one would expect to be its distinctive assets split cleanly in
two: `memory` and `bus` are real (if unindexed and un-hardened), `registry` is
real-but-thin, and `guard`, `trace`, `checkpoint`, and `codeindex` are scaffolding.
The single highest-value change is #1: inber currently has no tool authority
boundary at all, and that hole propagates outward into llm-bridge's permission
modes.
