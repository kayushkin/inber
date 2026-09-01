# Cline Comparison

**Project**: [Cline](https://github.com/cline/cline)
**Language**: TypeScript (VS Code extension)
**Focus**: Autonomous coding agent in-editor with strict human-in-the-loop approval for every action
**Key Strengths**: AST-based context management, checkpoint/restore, browser automation, MCP extensibility, ~60k stars

## Architecture Overview

Cline is a VS Code extension where the agent interacts with the codebase through the editor's APIs — file system, terminal, diagnostics, diff views. Every action (file write, terminal command, browser action) surfaces in the VS Code GUI for explicit user approval.

```
VS Code Sidebar (React webview)
    → Task execution loop
    → Tool calls (file ops, terminal, browser, MCP)
    → Human approval gate (VS Code diff view / dialog)
    → LLM (Anthropic, OpenAI, Google, Bedrock, Vertex, OpenRouter, etc.)
```

The checkpoint system takes workspace snapshots at each step, enabling diff-and-restore to any prior state. MCP support enables dynamic tool extension — the agent can even create new MCP servers on the fly.

## What Cline Does Well

### 1. AST-Based Context Management ⭐️

Cline uses tree-sitter to parse source code structure and builds a semantic codebase index with embeddings. Instead of naively stuffing files into context, it extracts symbol definitions and uses targeted searches. This enables working on large projects without exhausting the context window.

**Inber connection**: Inber's context system builds prompts from memory entries with token budgets (`turn_context.go`), but doesn't analyze code structure. AST-based context would significantly improve coding task efficiency.

### 2. Checkpoint System with Diff/Restore

Git worktree-based snapshots at each step. Users can compare any checkpoint against the current state and restore to any prior point. Finer-grained than git commits — captures intermediate states within a single task.

**Inber connection**: Inber saves messages to workspace JSON and logs to JSONL, but doesn't capture workspace state snapshots. For agents with file write access, checkpointing would provide safety and undo capability.

### 3. Browser Automation

Launches a headless browser (Puppeteer/Playwright), interacts with web pages (click, type, scroll), captures screenshots and console logs. Enables visual debugging and end-to-end testing workflows.

### 4. MCP as Extensibility Primitive

Rather than a custom plugin system, Cline uses MCP for tool extension. It can connect to MCP servers and even create new ones on-the-fly ("add a tool that fetches Jira tickets"). This bets on ecosystem effects and interoperability.

### 5. Mention System for Context Injection

`@file`, `@folder`, `@url` (fetches and converts to markdown), `@problems` (workspace diagnostics) give users fine-grained control over what the agent sees. Simple but effective context management UX.

## What Inber Should Adopt

### 1. Codebase Indexing for Context (MEDIUM PRIORITY)

Tree-sitter parsing + embedding-based semantic search for code context. When an agent is working on code, retrieve relevant symbols and definitions rather than relying on the agent to manually read files.

This would enhance `turn_prompt.go`'s context building for coding tasks without changing the overall architecture.

### 2. Workspace Checkpointing (MEDIUM PRIORITY)

For agents with file write access, take git snapshots at turn boundaries:
- Before RunTurn: snapshot workspace state
- After RunTurn: if files changed, create a checkpoint commit
- Expose checkpoint list via session history endpoints

This provides undo capability without requiring the agent to manage git itself.

### 3. Linter/Compiler Monitoring (LOW PRIORITY)

Cline watches for diagnostic errors after file changes and proactively fixes issues. Inber's workflow hooks already run build/test after tool calls (`workflow_build.go`), but feeding compiler output back as context for the next turn would close the loop.

## What's Different

| Aspect | Cline | Inber |
|--------|-------|-------|
| **Runtime** | VS Code extension | Standalone Go server |
| **Approval model** | Every action needs GUI approval | Tool allowlists per agent |
| **Context** | AST + embeddings + mentions | Memory store + token budget |
| **Multi-agent** | Single agent per task | 10 named agents, bus-based |
| **Persistence** | VS Code workspace state | SQLite agent-store |
| **Extensibility** | MCP-first | agentkit + MCP |
| **Deployment** | Desktop only (VS Code) | Server, headless, CLI |
| **Target** | Individual developer | Multi-agent infrastructure |

## Key Takeaway

Cline's most transferable innovation is **AST-based context management** — using code structure analysis to build efficient, relevant context within token budgets. This would directly improve inber's coding task performance. The checkpoint system is also valuable for any agent with file write access. Cline's strict human-in-the-loop model is the opposite of inber's autonomous approach, but the underlying tools (code indexing, checkpointing, compiler monitoring) are universally useful.

## Harness-watch — 2026-06-07: skills travel *bundled inside plugins*, discovered as extra search roots (no plugin-specific skill format)

[PR 11161](https://github.com/cline/cline/pull/11161) lets a plugin ship skills by
placing a normal `skills/<name>/SKILL.md` tree inside the package; the plugin
system contributes additional **skill search roots** rather than a plugin-specific
skill API. The same SKILL.md loader (frontmatter + body, identical to
workspace/global/managed skills) parses them — the only new logic is walking up
from each plugin entry file to the package root and adding `<root>/skills` when
present, gated by the same `plugins` config (disabled plugins are filtered before
their skill dirs are considered). [#11219](https://github.com/cline/cline/pull/11219)
groups discovered skills in settings **by owning plugin** using filesystem
ownership (so opening settings never executes plugin code), and
[#11220](https://github.com/cline/cline/pull/11220) restores enabled skills to the
`/` slash autocomplete alongside the searchable picker.

**What inber should consider:** inber's skill-store (`:8301`) is a centralized flat
registry (one row per SKILL.md, ingested by cloning whole repos). Cline's inverse
pattern offers two cheap wins: (1) **group-by-source** — skill-store already stores
`source` per skill, so a "these N skills came from upstream X" view in the
dash/bridge-ui Skills surface gives the same provenance UX without flattening it
away. (2) **co-distribution** — Cline ships skills next to plugin tools so one
install brings both; inber splits skill-store and tool-store (`:8302`) with no link
between a tool and its sibling skill from the same upstream. Worth deciding whether
to join them by shared source-group. The key reusable stance: keep bundled skills
**file-backed** (no imperative registration API) so they behave identically to
standalone skills — which matches inber's "everything is a SKILL.md row" model and
argues against ever inventing a plugin-specific skill format.

## Harness-watch — 2026-06-13: two-layer tool-output bounding + parallel-tool-calls as a cost lever

Two benchmark-driven context changes landed this window.

**1. Executor-level output caps with model-facing pagination** ([PR 11480](https://github.com/cline/cline/pull/11480), with [#11465](https://github.com/cline/cline/pull/11465)/[#11463](https://github.com/cline/cline/pull/11463)). Cline now caps `run_commands` combined stdout/stderr at 48,000 chars and `read_files` whole-file/oversized reads at 2,000 lines / 48,000 chars (plus a 2,000-char per-line cap to defang minified files). Two design details worth stealing: (a) truncation switched from keep-first-N to **head+tail sampling** (start and end preserved, middle elided) because build/test failures live at the *end* of output; (b) the cap is applied at the **executor** layer (bounds what enters session history at all — JSON files, memory, UI, summarizer inputs), sized deliberately *below* the MessageBuilder per-string backstop (50k) so capped content + truncation notice survives request-build instead of being re-truncated into a generic marker. The notice reports the *original* size and tells the model how to recover (grep/head/tail, or paginate with `start_line`/`end_line`) — a model-facing recovery path, not silent loss. This converges with the terminal-coding-agent harness research ([arXiv:2605.18747](https://arxiv.org/abs/2605.18747), already in `docs/papers/2026-06-harness-research.md`) and Pi's "[Showing lines 1-2000 of 50000. Use offset=2001]" nudge.

**What inber should consider:** inber's `smart-truncation`/`context-loading` bound context at request-build time. Add a *second*, earlier bound at the tool-execution edge so megabyte tool blobs never enter session history (memory-store, log-store, summarizer inputs) in the first place — and switch large-output truncation to head+tail sampling with an original-size + pagination notice, since the diagnostically-important bytes (stack traces, test failures) sit at the tail that keep-first-N discards.

**2. Parallel tool calls are a prompt problem, not a runtime problem** ([PR 11514](https://github.com/cline/cline/pull/11514)). Benchmark traces showed Cline emitting ~1 tool call per assistant turn vs OpenCode batching 2–4; because each turn resends the accumulated conversation, one-tool turns multiply the resend cost of every prior result. The fix was purely additive prompting — system-prompt + per-tool-description guidance to batch independent reads/searches/fetches/safe commands into one response — leaving runtime scheduling untouched. Note the open hypothesis: a **singular** tool surface (`read_file` vs `read_files`) may matter, because many models are trained to express parallelism as multiple separate tool-call blocks rather than an array arg.

**What inber should consider:** tool-calls-per-turn is a measurable cost lever independent of whether tools run concurrently. Audit inber's per-turn tool-call count on multi-read tasks; if it skews to one-per-turn, add explicit batch-independent-calls guidance to the agent system prompt and tool descriptions before touching the executor's concurrency config. Weigh exposing singular tool variants if traces show models reluctant to use array-shaped batch tools.

## Harness-watch — 2026-06-19: truncation becomes universal + `tool_use.input` joins the budget (the 50k backstop drops to 8k)

[PR 11475](https://github.com/cline/cline/pull/11475) reworks the `MessageBuilder`
backstop named in the 06-13 entry. Three contract changes worth stealing. **(1) The
truncation gate was an opt-in allowlist** — only seven built-in tools were capped, so
MCP/`createTool()`/`editor`/`apply_patch` results bypassed the per-result cap *yet still
counted toward the budget*, meaning one huge MCP blob made the budget loop uselessly
shrink other tools' results and still overflow. Now **every** tool result is truncated
(the `targetToolNames` param is gone). **(2) `tool_use.input` (model-generated tool
arguments) was invisible to the budget** — a 400KB `query` arg counted for nothing and
couldn't be reclaimed; now input strings count toward the aggregate *and* are truncated
as a last resort, so the budget is "always reclaimable, never one-sided." **(3) The
per-result cap drops 50,000 → 8,000 chars** (env-overridable for A/B), with binary
carrier blocks (`image`/`document`/`audio`/`video` with string `data`) protected by a
*known-type allowlist* so a textual `{type:"log",data}` payload can't dodge every cap,
and orphaned results fall back to `tool_result.name` when the paired `tool_use` is gone.

**What inber should consider:** inber's `smart-truncation` caps tool *results*. Close the
same two gaps: (a) make the request-build truncation **universal** — no per-tool
allowlist, or an unbounded MCP/custom-tool result both evades the cap and starves the
budget; (b) count and last-resort-truncate **model-generated tool-call arguments**, not
just results — a model that emits a megabyte `query`/`content` arg is an equal budget
risk and today inber only bounds the response side. Protect binary carriers by an
explicit type set, never by "has a `data` field."

## Harness-watch — 2026-07-19: a spawned teammate needs three things its lead has — a registry that hides tools it can't call, an errored run that reads as *failed*, and a mid-run credential refresh

Cline's SDK "teams" feature (a lead agent spawns parallel teammates and blocks in
`team_await_runs`) shipped a three-PR postmortem of one real July-15 incident, and each
PR is a transferable multi-agent lesson. This is inber's first team/teammate coverage.
**(1)** [PR 12371](https://github.com/cline/cline/pull/12371) — `team_spawn_teammate` is
lead-only, enforced by a runtime role check that *throws* `Only the lead agent can manage
teammates.` — but the tool was still in each teammate's registry. The teammate model
can't know it's forbidden, so it tries anyway: **every one of the seven teammate
transcripts made exactly two rejected spawn attempts (14 wasted reasoning turns) before
giving up.** Fix: build teammate toolsets with `includeSpawnTool:false` so the capability
isn't in the registry at all. **(2)** [PR 12370](https://github.com/cline/cline/pull/12370)
— a teammate whose model stream failed returned an `AgentResult{finishReason:"error"}`
rather than throwing; the queue only treated *thrown* errors as failures, so the run was
recorded `status:"completed"` with the real error buried in `resultSummary.textPreview`.
Consequence: the run-level retry machinery (`retryCount`/`maxRetries`) **never engaged for
model errors** — it only fed off exceptions — and the lead/human had to infer death from a
`"completed"` + `Unauthorized…` preview. **(3)** [PR 12369](https://github.com/cline/cline/pull/12369)
— a teammate died ~30 min in on a raw 401 (OAuth expired after 8 good iterations / ~1M
input tokens); no credentials were wiped, it just needed a refresh, yet the user had to
notice and manually re-dispatch. Fix: refresh expired OAuth and retry the run once.

**What inber should consider:** inber delegates to sub-agents (Irish-named fleet) and this
maps straight onto that surface. (a) **A capability a sub-agent can never use must be
absent from its tool registry, not merely rejected at execution** — a guard that throws
still costs the model reasoning turns discovering the tool is forbidden and can loop; scope
the delegate's toolset by role at build time. (b) **A sub-agent whose model call *returns*
an error (vs *throws*) must surface as `failed`, not `completed`** — echoes inber's
existing "status enums preserve granular truth" rule, and it's the precondition for
delegate-level retry to fire at all; a status collapsed to `completed` silently disables
the retry path. (c) **A long-blocked delegate should treat a mid-run credential expiry as
recoverable** — refresh and retry once before bubbling a failure the parent must hand back
to a human. All three failure modes only appear once delegates run *long and parallel*,
which is exactly inber's target regime.

## Harness-watch — 2026-07-31: cancelling a turn must reach the *process group*, resume must restore the state *derived from* the transcript, and compaction should be a projection whose fingerprint ignores transport identity

### 1. The shell/cancel contract — and inber's interrupt is wired to a context nothing observes

> **[Verified 2026-08-03 — STALE. Every defect this section names is fixed; do not re-derive.]**
> `RunTurn` takes a `context.Context` (`engine/engine.go:244`) and `server/session.go:175`
> passes one derived by `withoutCallerCancellation`, which keeps the caller's deadline and
> drops its cancellation on purpose — the doc comment at `:117-131` says why, and
> `engine/turn_cancellation_test.go` pins it. `s.cancel` is that context's cancel, so
> interrupt reaches a running turn.
> The shell half is fixed too, one layer down from where this section looked:
> `tool-store/internal/childprocess` now owns command construction — `processgroup_unix.go:15-18`
> sets `SysProcAttr.Setpgid`, and `childprocess.go:40,62` sets `WaitDelay`, with tests for the
> grandchild-holds-the-pipe case. `tool-store/tools/shell.go:76` calls
> `childprocess.NewCommand`, not `exec.CommandContext`. *(Path corrected 2026-08-07: this
> read `tools/shell.go`, which resolves to inber and does not exist. The shell tool is
> tool-store's, wrapped by `inber/tools/tools.go:49`.)*
> **Still open from this section:** the auto-proceed cap (a per-command wall-clock bound that
> returns partial output plus "still running" rather than blocking) and `OnToolProgress`. Both
> are features, neither is a defect.

Cline [PR 12696](https://github.com/cline/cline/pull/12696) found that `applyAbort` was exactly
`process.continue()`: cancellation only *detached Cline's listeners* and the spawned process kept
running. The fix adds `VscodeTerminalManager.sendInterrupt()`, which writes `\x03` (ETX, no newline)
so the pty line discipline delivers **SIGINT to the foreground process group**, then detaches — and
wraps the interrupt in try/catch, because cancellation must still succeed if the interrupt throws.
[PR 12712](https://github.com/cline/cline/pull/12712) adds the other half: a
`FOREGROUND_COMMAND_AUTO_PROCEED_MS = 300_000` timeout that turns the user's `detach()` into
`requestDetach(reason: "user" | "timeout")`, and on timeout **returns a tool result to the model** —
a sentence saying it auto-proceeded, the path of a log file still accumulating output, and the
partial output so far. Goose [#10808](https://github.com/block/goose/pull/10808) completes the
picture on the display side: `collect_tagged_lines` gained a `tokio::select!` with a 150 ms flush
feeding a `ShellOutputBatcher` (16 KB batches, 256 KB live cap, first line emitted immediately, one
terminal `truncated: true` notification), pushed as a `goose/developer_shell_output` notification on
a **best-effort `try_send`** emitter ("do not let a slow notification consumer delay tool
execution"). The regression test names the invariant:
`full_live_notification_channel_does_not_change_final_output` — **streaming is a side channel; the
tool result the model sees is byte-identical**.

**What inber should consider:** inber is a strictly worse position than cline's *pre-fix* state,
because it does not even detach. `engine/engine.go:228` calls `e.executeAgent(context.Background(), systemBlocks)`
and `RunTurn` (`engine/engine.go:201`) takes no `context.Context` at all — meanwhile
`server/session.go:116` does `ctx, s.cancel = context.WithCancel(ctx)` and `Session.interrupt()`
(`:169-173`) calls `s.cancel()`. That derived context is stored and then **dropped**:
`s.Engine.RunTurn(input)` at `server/session.go:139` never receives it. So `agent/agent.go:202`'s
`ctx.Err()` check and the shell tool's `exec.CommandContext(ctx, …)` both observe a context that is
never cancelled, and **the interrupt endpoint cannot stop a running turn or a running command**.
Threading the caller's `ctx` through `RunTurn` is the single highest-value line here; without it the
rest is unreachable. Then: `tool-store/tools/shell.go:51` runs `exec.CommandContext(ctx, "bash", "-c", c)`
with no `SysProcAttr{Setpgid: true}` anywhere in either tree, so Go's default cancel kills only the
direct child and orphans grandchildren — set the pgid and a `cmd.Cancel` that sends SIGINT to
`-pgid` then SIGKILL after a grace period, and copy cline's discipline that a failed interrupt must
not fail the cancellation. Set `cmd.WaitDelay` (go.mod is `go 1.25.0`): `shell.go:57` uses
`CombinedOutput()`, which waits on the copy goroutines, so a grandchild holding the pipe's write end
makes `Wait` block **indefinitely even after the kill**. Add cline's auto-proceed — a per-command
wall-clock cap that returns partial output plus a log path plus "still running" to the model rather
than erroring or blocking forever, which today is what `npm run dev` or `tail -f` does. And if
streaming is added (`engine/display.go:12-18` has `OnTextDelta` for model text but no mid-tool
progress event, so a ten-minute build shows nothing until it exits), take goose's two invariants
verbatim: best-effort send, and a regression test asserting the final tool result is unchanged.

### 2. Resume restores the transcript but not the state *derived from* the transcript

> **[Verified 2026-08-03 — the headline is STALE. `Turn.Counter` and `FrozenIdx` are both restored.]**
> `initSession` reads a turn count back with the transcript and holds it
> (`engine/engine_new.go:556-569`), and `initLimitsAndProfiling` installs both through
> `RestoreSession` once `e.staged` exists (`:670-691`) — its own comment says a bare assignment
> re-prunes the transcript and restarts the turn clock, which is exactly this finding.
> `engine/turn_counter_restore_test.go` pins it.
> **Still open and unchecked here:** the two turn clocks (`e.Turn.Counter` vs `session.turn`),
> the inert `MaxCheckpoints`, and the `metadata.kind` marker for synthetic user-role injections.
> The checkpoint half is todo `cf57e818`.

Cline [PR 12713](https://github.com/cline/cline/pull/12713) is two mechanically distinct bugs with
one theme. `local-runtime-host.ts:526` used `startInput.sessionMetadata ?? resumedArtifacts?.manifest.metadata`,
so supplying *any* fresh metadata silently discarded **all** resumed manifest metadata; it became a
per-key spread merge `{...resumed, ...startInput}`. And the checkpoint run counter was computed over
the **webview** message list, which omits hidden mode-switch and resume prompts; it now recomputes
over the **persisted SDK** list, counting every root user message except those tagged
`metadata.kind === "recovery_notice"`. [PR 12769](https://github.com/cline/cline/pull/12769) is the
mirror image — those same synthetic prompts must be *hidden* on rehydrate while still counting.

**What inber should consider:** `engine/engine_new.go:142` restores `e.Messages` from
`ws.LoadMessages()` and restores nothing derived from them. **`e.Turn.Counter` stays 0** (only write
is `e.Turn.Counter++` at `engine/engine.go:202`), so `engine/turn_context.go:23`'s
`if e.Turn.Counter == 0 { return 0, 4000 }` gives a resumed 200-message session the 4,000-token
*first turn* memory budget, and the `Counter > 15` → 8,000 branch at `:34` is unreachable for the
next fifteen turns. Worse, `engine/engine_new.go:571` reconstructs `e.staged` with **`FrozenIdx = 0`**
regardless of how much history was just restored, so on the first post-resume turn
`engine/lifecycle.go:91` treats the **entire restored history as the mutable staging zone** and
prunes it in place, `:97`'s `FrozenIdx > 0` guard skips `CrossZoneDedup` entirely, and
`saveMessages()` (`lifecycle.go:182`, via `turn_postprocess.go:39`) writes the rewritten array back
over `messages.json`. Persist and restore `Turn.Counter` and `staged.FrozenIdx` alongside the
transcript — the mechanism already exists, `engine/build.go:44-48` persists and reloads the prompt
blueprint across invocations — and adopt cline's merge shape (resumed as base, fresh input overriding
per key, never wholesale; `engine_new.go:571` is the `??`-equivalent today). Set
`FrozenIdx = len(restoredMessages)` on resume so the restored prefix is not rewritten and re-saved.
Separately, pick **one** turn clock for checkpoints: `engine/lifecycle.go:166` gates on
`e.Turn.Counter` (once per `RunTurn`) but `session/checkpoint.go:61` stamps the checkpoint with
`s.turn`, which `session/session_logging.go:109-114` increments **per API request**, so a five-tool
turn advances one by 5 and the other by 1 and the stamped turn is not the turn that triggered the
save. While there: `checkpointPath()` (`session/checkpoint.go:116`) is a fixed `checkpoint.json` and
`pruneOldCheckpoints` (`:122-128`) is a no-op returning nil, so every save overwrites the previous
one and `MaxCheckpoints: 5` is inert — implement it or drop it from the config. Finally, add cline's
`metadata.kind` marker: inber injects synthetic user-role content at `agent/agent.go:228`
(`[New message from user while you were working]`), `agent/agent.go:250` (`[BUDGET LIMIT REACHED]`)
and `conversation/summarize.go:104-109` (the summary message plus a canned assistant ack), and after
a round-trip through `messages.json` all three are indistinguishable from real user turns.

### 3. Compaction as a projectable sidecar over an intact source — and never fingerprint transport identity

> **[Verified 2026-08-03 — the `findTurnBoundary` tail is STALE, the rest holds.]**
> "`findTurnBoundary` counts every user-role message as a turn" is fixed (`d420980`):
> `conversation.StartsUserTurn` excludes messages whose blocks are all tool_results, and its
> doc comment carries the measurement — a 3.3× overcount on this host's transcripts, 6× on the
> most tool-heavy, which is how `KeepRecentTurns: 8` came to retain about two real turns. The
> unrecognised and empty cases answer true on purpose, erring toward keeping.
> The compaction-as-sidecar proposal itself is untouched and still the honest large change; the
> destructive-compaction half of it now has a measurement in the 2026-08-01 entry below.

Cline [PR 12747](https://github.com/cline/cline/pull/12747) fixed three separable rules.
**(a) Never hash transport identity into a validity fingerprint:** `session-compaction.ts:75-99`
dropped `id` and `ts` from `sourceMessageHashInput`, leaving role + content + durable metadata, and
bumped the seed `-source-v1` → `-v2`. The stated cause is that the codec regenerates ids and
timestamps on every storage round-trip — consolidated parallel tool results are "re-split with
freshly minted ids on resume" — so projection failed for semantically identical prefixes, the
sidecar was silently rejected *every turn*, and a full re-summarization (an extra LLM call) ran on
every turn past the trigger. **(b) Validate persisted derived state against the exact source it was
computed over:** `saveState` gained a `sourceMessages` parameter rather than falling back to
`session.agent.getMessages()`, because mid-turn the conversation store "can legally differ from the
runtime's working transcript, so validating against the store would spuriously reject the write."
**(c) A stale-write guard must not be able to wedge:** the count-based staleness check now applies
only when the stored state *still projects*, otherwise it falls back to comparing `updated_at` —
previously an invalidated sidecar permanently blocked its own replacement.

**What inber should consider:** inber cannot have cline's bug because it has no sidecar — compaction
is **destructive and irreversible in the durable snapshot**. `engine/lifecycle.go:59` does
`e.Messages = summarized`, and `saveMessages()` then overwrites both the workspace and session-dir
`messages.json` with the post-summary array, while `session/resume.go:181-189` **prefers
`messages.json`** and only falls back to the append-only `session.jsonl` — so the raw transcript
exists on disk and is never the resume source. The only other copy is a *lazy* memory row
(`conversation/summarize.go:56-65`, `IsLazy: true`) reachable only if the model calls `memory_expand`.
This interlocks with the already-documented silent fallback: `conversation/summarize.go:86` replaces
a failed LLM summary with `mechanicalSummary` (`conversation/summary_generation.go:73-126` — a
word-frequency list of words longer than six characters), and that word list then **permanently
replaces** the transcript. Keep `messages.json` as the source and store the compaction as a
projectable derivation (boundary index + summary + a fingerprint of the source prefix) applied at
request-build time; `conversation/staged.go`'s `FrozenIdx` is already the right shape for the
boundary half. If that is adopted, rule (a) is load-bearing for inber specifically:
`session/resume.go:17` **rewrites** tool IDs via `sanitizeToolID` and `:88-98`/`:123-133` **re-group**
tool_calls and tool_results into single messages on the JSONL path, so a fingerprint over ids or
block grouping would fail projection on literally every resume — the same failure cline shipped, and
the compaction-side instance of the existing "a tool id must never become a correlation key for a
message" rule. Fingerprint role + text content + durable metadata only, and version the seed. Rule
(c) generalizes past compaction: any "don't clobber newer state" guard needs a validity precondition
or an invalidated record deadlocks its own replacement. Independently, found while verifying:
`conversation/message_utils.go:13-30` `findTurnBoundary` counts *every user-role message* as a turn,
but in the Anthropic format tool-result batches are user-role messages — so `KeepRecentTurns: 8` can
mean eight tool-result round-trips **inside one user turn**, and the split point can land past the
user's actual request. `stripOrphanedToolResults` keeps the payload legal; the retention semantics
are not what the config name says. Fix the count or rename the knob.

## Harness-watch — 2026-08-01: a wrapper's own message is not the error — extraction must be ordered by proximity to the origin

> **[Verified 2026-08-03 — the max_tokens finding is REAL and its reporting half is SHIPPED.]**
> Confirmed live: `conversation/summary_generation.go` never read `response.StopReason`, so a
> summary the model was cut off mid-sentence returned `err == nil` and `engine/lifecycle.go`
> logged the same success line it logs for a complete one. One correction: the
> `mechanicalSummary` substitution this entry describes is already gone (`b04e998`), so the
> failure path no longer files a word-frequency list — but the truncation path was untouched.
> **Shipped:** `generateSummary` returns whether it was cut off, `SummarizeResult` carries
> `SummaryWasCutOffAtTokenLimit`, and the engine warns, naming the archive id as the only
> remaining copy of the turns. Behaviour is unchanged on purpose — reporting is decision-free,
> and the three answers this entry names (abort, retry with a larger ceiling, accept-and-mark)
> are not. Tests: `conversation/summarize_truncation_test.go`, with an end_turn control.
> **The decision is todo `2a0289a6`.** Do not re-derive the mechanism.

[#12800](https://github.com/cline/cline/pull/12800). When Vercel AI Gateway relays an upstream
rejection (their case: Alibaba Qwen refusing on context length), the object reaching
`extractErrorMessage` has `message: "Stream error occurred"`, a `cause` that is an unrelated
internal `ZodError` from the *gateway's own* parse failure, and the actual rejection JSON-encoded
inside `value.error_message`. Every rung of the extraction chain found something plausible before
reaching the real one, so the user was shown the gateway's generic wrapper and told nothing about
the context-length refusal that would have explained the failure. The fix teaches the structured
extractor to descend into `payload.value.error_message` — recursively, since it may be a JSON string
or a plain one — replaces a try/catch `JSON.parse` with `safeJsonParse` so *"not JSON"* and
*"parsed to undefined"* stop being conflated, and adds a final fallback that JSON-stringifies an
opaque object rather than rendering `[object Object]`.

**What inber should consider:** the ordering rule generalizes past provider errors —
**an extraction chain must be ordered by proximity to the origin, and every rung needs a fallback
that is still actionable.** The nearest inber instance is not an error string but a *success* that
masks an upstream condition: `conversation/summary_generation.go:59-76` never reads
`response.StopReason`. The only rejected response is an empty one (`:72-74`), so a summary the model
cut off at `max_tokens` returns `err == nil` and is treated as finished. The budget makes that the
expected case rather than the edge: `conversation/summarize_config.go:35-37` condenses 80 messages
(~40 turns) into `MaxSummaryTokens: 1024`, and `:42-44` condenses 40 messages into **800**.
Compaction is destructive — `engine/lifecycle.go:111` assigns `e.Messages = summarized` and
`saveResumableState` (`:241-256`) writes the post-summary array over both `messages.json` copies —
so a summary truncated mid-sentence becomes the session's only transcript, and
`engine/lifecycle.go:112` logs `"summarized %d turns → %d token summary"` exactly as it does on
success. The verbatim archive at `conversation/summarize.go:80-96` is reachable only if the model
calls `memory_expand`, which `summaryFooter` (`:150-158`) advertises only when that tool is on the
wire. What a fix has to decide is genuinely open: abort compaction on a `max_tokens` stop (safe, but
a session that consistently overruns then never compacts and hits the emergency flush at
`engine/lifecycle.go:174` instead), retry with a raised ceiling or a smaller slice (needs a retry
bound and a cost policy), or accept-but-mark so the injected block states its own incompleteness.

## Harness-watch — 2026-08-02: a restore that rewinds history but not files leaves a state that never existed

> **[Verified 2026-08-03 — the checkpoint-stub finding is REAL and its honesty half is SHIPPED.]**
> Confirmed as written: every method in `checkpoint/` returned a zero value and a nil error,
> and `Take()` ran on every `RunTurn`.
> **Shipped:** every entry point now returns `checkpoint.ErrNotImplemented`, the package doc
> says it is a design sketch and carries the three questions this entry names, and the engine
> no longer constructs it or calls it — the field and the per-turn call are gone.
> **Found while doing it, and not in this entry:** `EnableTrace`, `EnableCodeIndex` and
> `EnableCheckpoint` in `engine/engine_types.go` each occur exactly once in the tree, their own
> declaration. Nothing reads them, so all three are switches that turn on nothing. Documented
> in place, not deleted.
> **Build-or-delete is todo `0963afe5`**, which carries the three questions. The conversation
> half stays with `cf57e818`. Do not re-derive the stub.

[cline #12831](https://github.com/cline/cline/pull/12831) fixes two independent checkpoint bugs. The
transferable one: **restore only rewound tracked files**, so untracked files the agent created after
the checkpoint stayed on disk and "undo" produced a workspace that had never existed at any point in
the session. The fix captures untracked state as a **third parent** commit
(`createUntrackedParentCommit()`, `${ref}^3`); restore detects `^3` and runs `git clean -fd` before
applying the stash, and legacy two-parent snapshots deliberately leave untracked files alone "to
avoid unrecoverable data loss" — the fallback is asymmetric on purpose. The second bug is a
two-clocks error of the kind inber has shipped before: run numbering counted raw `role === "user"`
messages, but tool-result messages also carry `role: "user"`, so the picker offered run numbers the
core could not resolve and restore aborted. Fixed with span-aware counting — *a run warrants a
checkpoint when it **introduces** a new user turn*.

**What inber should consider.** inber has this asymmetry in its most extreme form, and the two halves
do not know about each other. `session/checkpoint.go` is real and writes *conversation* state
(`SaveCheckpoint`, `:65-112`, called from `engine/lifecycle.go:227`). The *file* half, the
`checkpoint/` package, is a 98-line stub in which every method is a TODO: `Take` (`:57-63`) returns
`nil, nil` and **`Restore` (`:91-97`) returns `nil` without touching a file** — while
`engine/engine.go:217` constructs it and `engine/engine.go:304` calls `Take()` on every `RunTurn`.
A `Restore` that reports success without doing anything is the most dangerous shape a safety feature
can take, and it corroborates `docs/harness-control-matrix.md:42-44` rather than contradicting it.
Note the conversation half is not restorable either — `session/checkpoint.go:41` says in its own doc
comment that `LoadCheckpoint` has no callers, which the open todo `cf57e818` already owns. So both
halves are write-only today. Three things a fix must decide, none of them mechanical: whether
"checkpoint" means conversation rewind, workspace rewind, or **the atomic pair** (`cline.md:56-61`
above assumes the third without saying so); whether checkpoints are per-turn or per-*user*-turn,
since inber's gate is `e.Turn.Counter % 20` (`session/checkpoint.go:137-142`) and a 50-round-trip
turn would otherwise produce 50 commits — the exact span-awareness #12831 had to add; and whether
untracked files are captured at all, since `git stash` semantics miss precisely the files an agent
most often creates.

[#12820](https://github.com/cline/cline/pull/12820) (a recoverable in-run notice is not a provider
API error — it inflated cline's measured error rate ~9× in a live A/B dashboard) is written up in
`agentic-design-patterns.md` (2026-08-02 §2), because inber's version *routes* on the answer:
`engine/turn_execute.go:54` records a **user cancellation** as a model error in a host-shared
model-store. Note the double-count half of that entry's 2026-08-01 predecessor is now **fixed** —
`agent/chain.go:330` passes `!isError` — so `agentic-design-patterns.md:2206` is stale on that point.

**Held back from the queue this pass (cap is 3/run), recorded so it is not lost.**
**FILED 2026-08-02 as todo `69f05f89` — do not re-file.** The passage is accurate; what it
missed is that the misclassification it hands off to §2 was already live and one-directional.
A `Client.Timeout` error satisfies `errors.Is(err, context.DeadlineExceeded)`, which
`errorIsEvidenceAboutTheModel` excluded outright, so a hung provider on this path recorded
**nothing** — the model never went unhealthy and failover never fired, the opposite of the
inflated-error-rate direction #12820 describes. Fixed in `4f50869`; the timeout's shape and
the discarded `timeoutHint` are the open half and live on the todo.
[#12839](https://github.com/cline/cline/pull/12839) raised cline's Ollama *response-start* timeout
default to 5 minutes because cold model loads exceed anything shorter, and [#12845](https://github.com/cline/cline/pull/12845)
retries empty Ollama responses at the model boundary. inber's version: `agent/openai.go:34` hardcodes
`Timeout: 120 * time.Second` on the `http.Client`, and `agent/clients.go:92` routes **ollama** through
that client — so inber is at 120s where cline needed 300s, and worse, `http.Client.Timeout` is a
**total-request** deadline, not a response-start one, so it kills a legitimately long generation as
readily as a hung connection. When it fires it surfaces as an `apiErr`, which then feeds the
model-health misclassification above. Meanwhile inber already computes a per-model timeout and throws
it away: `engine/turn_execute.go:18` reads `selected, _ := e.selectModel()`, discarding the second
return, which is `timeoutHint` — computed at all three `selectModel` exits (`engine/failover.go:30,37,51`)
by `timeoutFromHealth` (`:78-89`, 3× observed average, clamped 30s–5min) and consumed by **nothing**;
`selectModel` has exactly one non-test caller. A fix must decide whether to split response-start from
total-response deadline (cline's shape — only the former was raised), and whether the health-derived
hint should drive the timeout or be deleted, since its 5-minute ceiling and the flat 120s currently
disagree about what was intended.

## Harness-watch — 2026-08-15: not all JSON repair is equal, and the line is whether content was lost

[cline #13015](https://github.com/cline/cline/pull/13015) adds `hasUnterminatedString()` to
`parseJsonStream()` and rejects truncated tool-call arguments whose string values were cut off
mid-content — while **keeping** `jsonrepair` for single quotes, trailing commas and unclosed
containers. That split is the whole idea. An unclosed `{` or a trailing comma is a *formatting*
defect: every character the model emitted is still present, and closing the brace restores exactly
what it meant. An unterminated string is a *content* defect: the value is a prefix of what the model
was writing, and repairing it produces a syntactically valid object carrying a silently wrong
argument, which then runs. Structural repair is lossless and safe; content repair is a guess
presented as a fact.

**inber does not have cline's bug** — there is no repair library, no bracket balancing, no schema
coercion, and `json.Valid` is called nowhere in the repo. It has the other half of the problem: it
never *detects* truncation either. Three sites, all verified this sweep by reading the code:

- **`agent/agent.go:415`** treats `stop_reason: "max_tokens"` as identical to `end_turn` — returns
  `nil` error, sets no `Incomplete` flag, logs nothing. `engine/turn_openai.go:107-108` is the same
  two lines on the other turn loop. The sibling path fifteen lines up does the opposite and explains
  why: when the *stream* errors, `deliveredText` (`agent/agent_run.go:227-242`) keeps only text
  blocks because "carrying [a cut-off tool_use] into the conversation would leave a tool_use with no
  matching tool_result, which makes the next request invalid", and `agent/agent.go:397-405` appends
  `[response cut off: %v]` and returns the error. A clean stream ending on `max_tokens` is the same
  condition and gets none of it. The damage then splits: the missing `tool_result` **is** repaired
  loudly (`conversation/repair.go:23`, logged at `engine/turn_prepare.go:57-60`), but the truncated
  `Input` is never touched and blows up later at `json.Marshal` with `unexpected end of JSON input`,
  naming neither the tool nor the truncation. **Filed as `123a27c8`.**
- **`server/status_tools.go:35-36`** discards its `json.Unmarshal` error outright, so an unreadable
  `agent_slug` leaves the field `""` — the same value that means "no agent named" — and the tool
  returns the entire roster as a plausible answer to a call it never parsed. The only ignored
  unmarshal error on model-supplied input in the repo. **Filed as `9941a6aa`.**
- **`agent/chain.go:151-155` and `agent/sideband.go:98-102`** return an unparseable input with an
  empty `dropped` reason, the one branch in either function that reports nothing — against
  `extractChain`'s own doc comment at `chain.go:145-150`. **Filed as `a6767846`.**

**What inber should consider:** adopt cline's *distinction*, not its mechanism. inber has no repair
step to constrain, so the carry-over is that `stop_reason` is already recorded
(`engine/build_hooks.go:193-209`) and already returned by the oneshot API
(`server/api_oneshot.go:102,131`) — nothing branches on it as a problem. Making `max_tokens` a
condition the turn reports is a smaller change than it looks, and the three decisions it forces
(error or completed turn; drop the truncated block or repair it; whether `MaxTokens: 16384` should
come from model-store rather than being hardcoded at `agent/agent_run.go:58` and
`engine/turn_openai.go:71-74`) are on the todo, undecided.

**Held back from the queue this pass (cap is 3/run), recorded so it is not lost.** Two, both
adjacent to the above and neither independently filed. First, `server/api_oneshot.go:117` copies
`block.Input` straight into `out.Parsed`, and the loud check at `:123-126` fires only when there is
*no* `tool_use` — so a *partial* one passes, and `jsonResponse` (`server/api.go:73-76`) discards the
`Encode` error, handing the client a truncated body with a 200. That is the same finding one layer
out, and it should be settled by whatever `123a27c8` decides rather than separately. Second,
`agent/openai.go:62` is a single `c.client.Do` with no retry layer at all, so the openai, google,
openrouter and ollama paths get none of the bounded, exponential, jittered, `Retry-After`-honouring
retry the Anthropic SDK gives the other path (`MaxRetries: 2`, 0.5s×2ⁿ capped at 8s, jittered, and
cancellable on `ctx.Done()`). That is a real asymmetry but it belongs to the standing "OpenAI turn
loop is governed by less than the Anthropic one" decision (`5902f7b9`), not to a new todo.

## Harness-watch — 2026-08-19: a union-typed tool argument must be discriminated on *key presence*, and a binary must not be swapped out from under a live turn

### 1. Discriminating a union on a zero value — cline fails loudly, tool-store destroys the file

[cline #13336](https://github.com/cline/cline/pull/13336) fixes `run_commands`, which accepts each
command as a string *or* as `{command, args?}`. The executor branched on
`typeof command !== "string"` and handed `command` straight to `spawn(..., {shell:false})`, so the
schema-valid `{"command":"echo hello"}` died with `spawn echo hello ENOENT` and *any* command
containing a space failed. The fix is one predicate and its comment states the rule — **"Spawn
without a shell only when the args key is present (even empty), marking input the caller already
split"** — i.e. `directExec = typeof command !== "string" && "args" in command`. **Discriminate a
union on key presence, never on a zero value.** Note the blast radius the PR reports: whole sessions
became unable to run commands once the model settled on that schema shape.

**inber's tool set has the same shape and its failure is silent and destructive.** `write_files` —
registered at `tools/tools.go:35` → `agent/registry/tools.go:27`, implemented in
`~/repos/tool-store/tools/fs.go:163-200` — declares the identical union (a single `{path, content}`
arm or a `files[]` arm) and discriminates it at `fs.go:178-181` with `if in.Path != ""`, a zero-value
test that cannot tell an absent `content` key from an empty one. The loop then calls
`os.WriteFile(f.Path, []byte(""), 0644)`. Two facts make it reachable by construction rather than by
mistake: `fs.go:166` is `schema.Props([]string{}, ...)` — **zero required fields** — and
`schema.Parse` (`~/repos/tool-store/schema/schema.go:18-22`) is a bare `json.Unmarshal` that performs
**no JSON-Schema validation at all**, so the `required: ["path","content"]` published on the
`files[]` items at `fs.go:169` is decorative.

Measured by running the real `WriteFile().Run` against the live tree, not inferred:

```
{"path":"victim.txt"}                           → "wrote 0 bytes to victim.txt"          file now ""
{"files":[{v1,"content":"NEW"},{"path":"v2"}]}  → "wrote 3 bytes to v1\nwrote 0 bytes to v2"  v2 now ""
{"path":"v3.txt","content":null}                → "wrote 0 bytes to v3.txt"              v3 now ""
```

The batch case is the one that matters: one correct write and one destroyed file, returned to the
model as a two-line **success**. Nothing upstream catches it, because `guard.isDangerous`
(`guard/guard.go:329-334`) classifies `write_files` by **name only** — the guard sees a
dangerous-tool call and cannot see that the arguments are the destructive shape. That is the failure
already named at `agentic-design-patterns.md:2015`, "fail-closed on the tool name is not fail-closed
on the argument", now with a data-loss instance attached. `edit_files` (`fs.go:233`) has the same
zero-value discrimination and is saved by accident: an absent `old_text` makes
`strings.Count(content, "")` exceed 1 and the uniqueness check refuses. **Filed as a todo.**

**What inber should consider:** the fix lives in tool-store, not here, and there are two of them with
very different blast radii. Narrow — give `Content` a `*string` or `json.RawMessage` and discriminate
on presence, which forces an answer to what an explicit `{"path":"x","content":""}` means, since
truncating to empty is a legitimate request. Broad — make `schema.Parse` enforce the `required` list
it already publishes, which is one function covering every tool at once and correspondingly riskier,
since any caller relying on a tolerated partial payload starts erroring. inber's own move,
independent of either, is that a name-only dangerous-tool classification cannot see this and never
will.

### 2. The updater must ask whether anything is live

[cline #13233](https://github.com/cline/cline/pull/13233): the background auto-updater installed the
new package while cline was running and restarted the hub, killing live sessions mid-turn with
`code=1006`, because "the running process and the files on disk are now different builds". The rule
is one sentence — **"never install while cline is running"** — and the implementation is the
interesting half: defer the install into the exit sequence, gated on the hub confirming no client is
attached, so "the new binary doesn't exist on disk until every old process has exited". Mixed-version
processes become impossible rather than unlikely.

**inber's deploy never asks.** `deploy.sh:45-46` is an unconditional `systemctl --user stop`, with no
query of live session state anywhere in the script — although the server exposes exactly that
(`GET /sessions`, `server/api.go:38`). The graceful path is real but bounded: a turn runs
synchronously inside the HTTP handler, and `Queue.Enqueue` (`server/queue.go:39-56`) runs `work(ctx)`
on the caller's goroutine, so `server.Shutdown(context.Background())` (`server/api.go:53`, unbounded)
genuinely waits for it — until systemd's stop timeout expires. `deploy/systemd/inber-server.service.template`
sets no `TimeoutStopSec` and no `KillMode`, so it inherits the manager default and a long turn is
SIGKILLed: `defer g.Close()` (`cmd/inber-server/main.go:95`) never runs, `persistSessionState` never
runs, and tool calls the turn already made leave side effects with no transcript record.

Two smaller things fall out of the same read. `Server.Close()` (`server/server.go:166-172`) is the
**one of four sites** that acts on a session without reading `Status` first — `session_release.go:44`,
`session_reaper.go:66` and `api_bridge.go:778` all check `s.Status == Running`. And `close()` →
`stop()` (`server/session.go:329-343`) sets `Status = Completed`, so a killed turn lands in the
*success-shaped* terminal state. **Filed as a todo.**

Worth stating what inber already gets right, because it is the half cline was missing:
`Store.InterruptRunning()` at startup (`server/server.go:111`) means persisted requests do not sit at
`running` forever — they reconcile to `interrupted`. The residual harm is the unrecorded in-flight
turn and the `Completed` mislabel, not an unbounded leak.

### 3. Checked and not worth a finding

- **[cline #13226](https://github.com/cline/cline/pull/13226)** (fail-closed remote config) — the
  headline failure is a network hiccup deleting company policy while reporting success. inber does
  not have it: `reloadRegistry` (`server/api_agent_config.go:176-181`) returns early on error keeping
  the last good map, and `LoadFromAgentStore` (`agent/registry/config.go:94-96`) **errors on an empty
  result** rather than returning an empty map, so a reachable-but-empty store cannot wipe the
  registry. The residual concerns are already filed at `agentic-design-patterns.md:4079-4110`. One
  sub-idea worth holding for the day a permission-store round-trip gates a **deny**: a revocation must
  be evaluated locally *before* any network call, so losing the network cannot silently re-grant.
- **[cline #13293](https://github.com/cline/cline/pull/13293)** (preserve LiteLLM input token limits,
  "avoids fabricating maxInputTokens when no provider metadata exists") — inber already reasons this
  way and writes it down: `agent/models.go:18-27` sets non-zero unknown-model constants precisely
  because *"a zero context window means 'no overflow guard' … answering an unknown model with zero
  would turn a missing registry row into a silently unguarded request"*. Prices at `:94-95,102-103`
  pass through unguarded, which looks like the same asymmetry but is correct — a zero price is
  truthful for a local model, and `MaxCost` should never trip on something free.
- **[cline #13310](https://github.com/cline/cline/pull/13310)** (task-scoped settings overlay
  outliving its task view) — a scope-lifetime leak; inber's nearest analogue cleans up, since
  `Session.turn`'s deferred block resets `Status`/`cancel` and calls
  `requeueInjectionsTheTurnNeverReadLocked` (`server/session.go:167-173`), pinned by
  `session_injection_requeue_test.go`.
- **[cline #13227](https://github.com/cline/cline/pull/13227)**, **[#13230](https://github.com/cline/cline/pull/13230)**,
  **[#13231](https://github.com/cline/cline/pull/13231)**, **[#13075](https://github.com/cline/cline/pull/13075)**,
  **[#13245](https://github.com/cline/cline/pull/13245)** — already covered at
  `agentic-design-patterns.md:4601`, `:4689`, `:4780`, `:4856`, `:5045`, `:5105`.

## Harness-watch — 2026-08-20: a provider's terminal states are a union inber only half-names, and a non-zero exit is not an error anywhere in the pipeline

### 1. Two turn loops, opposite fail modes, for the same unmodeled stop reason

[cline #13300](https://github.com/cline/cline/pull/13300) fixes a regression where `emitAiSdkEvents`
diverted every AI-SDK part with `providerExecuted: true` off the normal pipeline, re-emitted the two it
recognised (`web_search`, `image_generation`) as observational events, and **silently dropped everything
else**. Because `ai-sdk-provider-claude-code` marks *every* tool the CLI runs as provider-executed, whole
sessions read and edited the workspace with no tool activity in events, transcripts or UI. The fix routes
all provider-executed parts onto the observational path and widens `toolName` from the closed
`ModelToolName` union to `string`. The rule underneath: when a provider hands back a variant your
allowlist does not name, the default branch decides whether you lose data or fail loudly — and picking
"drop" by omission is the worst of the two.

inber has no provider-executed tools, so the content-block half does not transfer. The **stop-reason**
half does, and inber holds both wrong answers at once. `agent/agent.go:415` returns on
`end_turn`/`max_tokens`, `:425` loops on `tool_use`, and `:441` is
`return result, fmt.Errorf("unexpected stop reason: %s", resp.StopReason)`. The SDK inber pins —
`anthropic-sdk-go v1.35.0`, `go.mod:6` — declares six, and `message.go:5325-5333` names the two the switch
omits: `StopReasonPauseTurn` and `StopReasonRefusal`. `refusal` is reachable today on any request;
`pause_turn` becomes reachable the moment a server tool is enabled, and its documented handling is
*resume the loop*, which is the branch inber is missing rather than an error. The assistant message is
already in history — `agent/agent.go:412` runs `*messages = append(*messages, resp.ToParam())` before the
switch — but `result.Text` is empty, because text is extracted only inside the `end_turn` branch
(`:416-419`) and `processResponse` (`agent/agent_run.go:263-299`) reads only usage and thinking blocks.
Note the asymmetry with the sibling path fifteen lines up: `agent/agent.go:397-404` works to keep
`deliveredText` when a *stream* errors. A refusal is the same condition and gets none of it.

Whether that error should also mark the model unhealthy is **already an open decision** — todo
`6b4a9ab5`, which `engine/failover.go:160-166` names in a comment beside the code, saying so outright:
"Whether a refusal, or a pause_turn inber has no branch for, is a provider fault, a model fault or a gap
in inber is an open question." Not re-filed.

The OpenAI-served loop makes the opposite mistake in the same place. `mapOpenAIFinishReason`
(`agent/openai_conversion.go:244-254`) is `case "stop"/"length"/"tool_calls"` and then
`default: return anthropic.StopReasonEndTurn` — so OpenAI's `content_filter` is reported to every layer
above as a normally completed turn. `ConvertOpenAIResponseToAnthropic` does it again at `:191-201`: zero
choices returns a message with no content and `StopReason: anthropic.StopReasonEndTurn`. Per
`agent/clients.go:92` that path carries ollama alongside openai/google/openrouter. One loop turns an
unmodeled terminal state into a hard error and a model-health penalty; the other turns it into a clean
success. Neither reports what actually happened.

**What inber should consider:** name the union exhaustively in both loops rather than allowlisting three
cases. **What a fix must decide:** whether `pause_turn` continues the loop (the provider's own contract)
or errors, and whether `refusal` is an error at all or a completed turn carrying `result.Incomplete`,
which `agent/agent.go:403` already sets on the stream path. On the OpenAI side the fix is smaller and
unambiguous: a `default` that silently returns `end_turn` fabricates a canonical value where the provider
supplied a real one.

### 2. A command that exits non-zero is a success everywhere except the prose

[cline #13358](https://github.com/cline/cline/pull/13358) sets `$ErrorActionPreference='Stop'` in the
`run_commands` PowerShell wrapper. Without it a pipeline raising a non-terminating error per item emitted
one error record per file — measured at 10,001 stderr lines and 1.3 MB on a 10,000-file tree — **and
still resolved as SUCCESS with exit 0**. Two separable claims: a failure must terminate the command rather
than continue through the rest of the work, and the harness must not report the result as success.

inber fails the second outright and the first for multi-command calls. `shell_commands` is tool-store's
`Shell()`, registered at `tools/tools.go:34,49` and reachable through `engine/build_tools.go:49`. At
`~/repos/tool-store/tools/shell.go:82-86` a non-zero exit is folded into the *text* — `out, err :=
cmd.CombinedOutput()`, then `result = fmt.Sprintf("%s\nexit: %s", result, err)` — and `Run` returns
`(text, nil)` at `:97`. Every consumer downstream derives failure from that discarded Go error and nothing
else. `agent/chain.go:406-410` is `primaryOutput, err := tool.Run(...)` / `if err != nil {...}`, so
`isError` is false; `:416` calls `hooks.OnToolResult(blockID, name, outcome.primaryOutput, false)`;
`engine/build_hooks.go:156-159` increments `e.Turn.ConsecutiveErrors` only `if isError`, so it never
moves; and `agent/agent_run.go:421-423` writes `anthropic.NewToolResultBlock(block.ID, finalOutput, false)`,
putting `is_error: false` on the wire. The counter that stays at zero drives the error-recovery context
ladder at `engine/turn_context.go:12-18` (1 error → 20k recall, 3 → 35k, 5 → 50k), which
`agent/chain.go:336-342` documents as the reason `isError` has to be truthful. So a session whose build
has failed five times running is, to every mechanism inber has, a session in which nothing has gone wrong.

The fail-fast half: `shell.go:68-96` loops `for i, c := range cmds` and the only `break` is `ctx.Err()` at
`:70-73`, so `commands: ["./configure", "make", "make install"]` runs all three after the first fails and
returns one aggregate string with a `nil` error.

**What inber should consider:** the tool-side fix is in tool-store, and **what it must decide** is what
"failed" means for a call carrying several commands — first non-zero stops and the tool errors (cline's
choice, which makes `commands[]` behave like `&&`), or every command runs and the tool errors if any did,
or a `continue_on_error` flag whose default has to be argued for. inber's own half is independent of that
answer: `is_error` and `Turn.ConsecutiveErrors` are derived *only* from a Go-level error, so any tool that
reports failure in-band is invisible to both. Whether the dispatcher may ever read a tool's output to
classify it, or whether every tool must signal failure structurally, is the question — the current
arrangement picks the second and ships a tool that does not obey it.

### 3. The guard's mode reaches the model only as a refusal after the call

[cline #13361](https://github.com/cline/cline/pull/13361) fixes a desktop session where
`autoApproveTools: true` made the generated system prompt describe Yolo mode while the core runtime ran in
Act mode — the prompt advertised a tool (`submit_and_exit`) the session did not have. The correction: the
explicit session `mode` drives both the system prompt and the runtime tool preset, and auto-approval goes
back to being an independent tool policy that changes neither.

inber's equivalent mismatch is total and live. `engine/engine.go:205` builds the guard from the mode the
create request asked for (`e.Guard = guard.New(e.Limits.GuardConfig(mode))`), and `guard.CheckTool`
(`guard/guard.go:165-187`) really enforces it: Observe returns `Denied` for anything not read-only, Assist
returns `NeedsApproval` for the dangerous set. But the mode reaches neither of the two things that tell the
model what it can do. `buildTools` (`engine/build_tools.go:16-21`) holds no guard reference and applies no
filter, so an Observe-mode session is handed `write_files`, `edit_files` and `shell_commands` in its tools
array with their full descriptions; and `engine/turn_prompt.go` contains **no occurrence of the string
`mode` at all** — `BuildSystemPrompt` (`:83`) never states which one is in force. The only path from mode
to model is `buildToolRefusal` (`engine/build_hooks.go:93-101`), returning `"%s mode allows read-only tools
only"` *after* the model has spent a call deciding to write.

The cost compounds rather than repeating. A refusal returns `isError = true` (`agent/chain.go:390-393`), so
unlike the shell case above it *does* increment `Turn.ConsecutiveErrors`, and `engine/turn_context.go:12-18`
widens memory recall from 6k to 20k to 35k to 50k tokens — each step rewriting the cached system-prompt
prefix and re-paying for the whole prompt. An Observe-mode session that keeps trying to write is charged an
escalating cache-busting penalty for failing to guess a policy it was never told.

**What inber should consider:** the mode is the session's contract and belongs where the model reads
contracts. **What a fix must decide:** state it as a named system block, which is cheap and cache-stable
since the mode does not change mid-session, or filter the tools array by mode, which is stronger — the
model cannot call what it cannot see — but makes the tools hash differ per mode, and Assist has no clean
answer because its dangerous tools are *conditionally* available. Either way, decide whether a guard
refusal should count toward `ConsecutiveErrors` at all: it is a policy outcome, not a failure, and it is
currently the one thing driving the recall ladder in a mode where every write is refused by design.

### 4. A settled turn has no terminal state that says it failed

[cline #13330](https://github.com/cline/cline/pull/13330): turns that settled through the event stream
rather than a blocking `send()` RPC had no finalization step — `chat_done` updated the composer status but
never cleared the streaming id or reconciled against the persisted transcript, so the last bubble shimmered
indefinitely and only healed when an unrelated follow-up nudged the hydration path. The shape: the settle
path and the happy path finalize differently, and only one of them was written.

inber's settle path is a `defer`, and it erases the one state the failure path sets.
`server/session.go:166-173` registers a deferred func that takes `s.mu` and sets `s.Status = Idle`;
`:176-183` then runs the turn and, on error, sets `s.Status = Error` before returning. Deferred functions
run after the return values are set, so `:178` is a dead store — **`Error` is written at exactly one place
in the package and unconditionally overwritten microseconds later.** `Error` is a declared `SessionStatus`
(`server/session.go:22`) with a `String()` case at `:33`, surfaced by `GET /sessions`
(`server/api_sessions.go:138`), `server/api_bridge.go:236,521,576` and `server/session_management.go:43` —
and no session has ever reported it. A turn that failed is indistinguishable from one that finished. Same
class as the `Completed`-on-SIGKILL mislabel recorded at `:600-604`, from the other end of the lifecycle.

**What inber should consider:** the mechanical fix (assign in the defer, or drop the early assignment) is
not the decision. **What a fix must decide** is what `Error` means for a session that will accept the next
turn regardless: a *sticky* attribute, which is what a distinct enum value implies and what a reader of
`GET /sessions` would assume, or a transient last-turn outcome belonging on the request record. If sticky,
the next successful turn has to clear it and nothing does. If not, the value should be deleted rather than
repaired — a status nobody can observe is worse than an absent one.

### 5. Checked and not worth a finding

- **[cline #13327](https://github.com/cline/cline/pull/13327)** (skill slash commands load via the skills
  tool instead of being pasted into the user message) — the named live angle, and inber lacks the
  machinery. `grep -ri skill --include=*.go` returns nothing outside vendored paths, and there is no
  slash-command parser: `redact/redact.go:226` is the only `HasPrefix(value, "/")` in the repo and it is
  path detection. Nothing expands a command body into the user message because nothing expands commands.
- **[cline #13331](https://github.com/cline/cline/pull/13331)** (agents create scheduled tasks) — inber
  already ships `scheduler` (`tools/tools.go:45`), and the hazard is **already written down and pinned**:
  `guard/classification_test.go:141-215` names `scheduler` in `unclassifiedToday` with the exact reason —
  an assist session can schedule shell work it is not allowed to run directly, and nothing asks an
  approver. It is not reachable today for a second reason that test does not cover: `agent_tools` in
  `~/.config/agent-store/agents.db` holds **0 rows**, so `e.AgentConfig.Tools` is empty for every agent and
  `buildTools` always takes the `buildDefaultTools` branch, which omits `scheduler` and `browser`. The hole
  opens the first time any agent config names a tool at all.
- **[cline #13336](https://github.com/cline/cline/pull/13336)** — covered 2026-08-19 §1 above (`:530`).
- **[cline #13391](https://github.com/cline/cline/pull/13391)** (@ file mentions break on paths with
  spaces) — inber has no mention syntax; the three `grep -i mention` hits are English prose in comments.
- **[cline #13369](https://github.com/cline/cline/pull/13369)** (strip the `<user_input>` envelope when
  copying a user message) — the principle, that a transport envelope is presentation and must not leak into
  what the user takes away, is the standing entry at `opencode.md:511-535` (2026-06-19) for inber's
  `"[New message from user while you were working]"` wrapper. No new surface: inber has no copy action, and
  `session/prompts_write.go:203` renders `[tool_use: %s]` at the display edge, which is where it belongs.
- **[cline #13413](https://github.com/cline/cline/pull/13413)** (work summary undercounts wall time when
  pre-tool thinking attaches to the answer) — a webview grouping bug plus a projection-ordering fix.
  inber's projection is already in live-stream order for the reason cline had to fix: `processResponse`
  emits thinking (`agent/agent_run.go:290-298`) before `executeTools` (`agent/agent.go:426`) emits any tool
  call. And no inber duration derives from timeline row timestamps — `server/api_oneshot.go:101` uses
  `time.Since(start)` — so there is no figure to undercount.
- **[cline #13179](https://github.com/cline/cline/pull/13179)** (stream run command output) — real gap, no
  finding, because there is nowhere to put it. `tool-store/tools/shell.go:82` is a single buffering
  `cmd.CombinedOutput()`, and inber's tool channel is one `OnToolResult` call at completion
  (`agent/chain.go:416`); there is no partial-tool-output event in `agent.Hooks`. Adding one is a protocol
  change, not a fix.
- **opencode**, whole window — the three non-model-churn candidates (#43099 oversized websocket fallback,
  #43248 malformed model costs, #43188 session request headers) are all covered by the 2026-08-19 entry at
  `opencode.md:570-591`. Everything else in that repo this window is model and pricing churn.

## Harness-watch — 2026-08-21: a synthetic delimiter inber emits is a delimiter a tool result can also emit, and a hook whose output nobody reads cannot honour the contract it advertises

### 1. The hook-context channel, and the injective escape that makes it forgeable-proof

[cline #13297](https://github.com/cline/cline/pull/13297) gives tool hooks a `appendContext` result and
flushes all collected hook text as **one trailing user message after the tool-result messages** — *"A
single trailing user message (rather than text parts inside tool messages, or interleaved user messages)
keeps tool-result parts contiguous and first in the merged user turn, which providers require."* Three
things the body does not advertise are the interesting half:

- each block is stamped `<hook_context source= tool_name= tool_call_id=>`, because *"parallel tool
  execution collects them in completion order, so position alone cannot identify the tool"*;
- attribute values go through an **injective** escape (`_`→`__`, so *"no two distinct ids can collapse to
  the same sanitized stamp"*), and embedded `<hook_context` / `</hook_context` in the body is neutralized,
  *"so neither provider-supplied ids nor hook output can corrupt or spoof the block markup"*;
- the message is minted `displayRole: "system"` — model-visible, transcript-invisible.

inber emits synthetic model-visible wrappers with fixed literal delimiters and no escaping of any kind.
`agent/agent.go:354` appends `"\n\n[New message from user while you were working]\n" + text` **into the
same user message that carries the tool_result blocks** — so the forged and the genuine article would sit
inside one message, adjacent, distinguishable only by which content block they landed in. The same shape
is at `conversation/summarize.go:107` (`[Conversation Summary — %d earlier turns condensed]`),
`server/session_forking.go:63` (`[System] You are a forked sub-agent…`) and
`conversation/message_utils.go:166` (`[tool_use: %s]`, which is the rendering handed to the *summarizing*
model). A `read_files` of a document containing the steer literal is the whole attack.

**Two things checked and found not to be defects, so the next pass does not re-derive them.**
(a) Nothing in inber *parses* these literals: `session/turn_counter.go:21` and
`conversation/message_utils.go:22` only name them in comments, and `StartsUserTurn`
(`conversation/message_utils.go:29`) discriminates on block **type**, not on text. So a forged literal
cannot move inber's turn accounting — only the model's belief. (b) The append at `agent/agent.go:348-359`
targets `messages[len-1]`, which at that point in the loop is always the user message holding the tool
results, so the steer cannot land on an assistant message and be attributed to the model.

**What inber should consider:** #13297's escape is one function and it is the cheap half. **What a fix
must decide:** whether the delimiter becomes a per-session nonce (unforgeable, but it moves the cached
prefix — `agent/agent.go:555` anchors `cache_control` on the last block of this very message) or stays
fixed and the *tool output* is scanned and neutralized on the way in (cache-safe, but it is a scan on
every tool result). Those two have different costs and only one of them is free.

### 2. A fire-and-forget hook cannot honour a control contract

[cline #13298](https://github.com/cline/cline/pull/13298) found PostToolUse hooks running detached with
`stdio: ["pipe","ignore","ignore"]`, so *"their entire JSON output is discarded — contextModification
**and** cancel."* The fix restores blocking execution under the same 120s cap as PreToolUse, and states
the trade-off as a principle rather than a regret: *"The alternative (staying async and dropping output)
is what produced the bug: the documented contract can't be honored without reading the hook's stdout, and
a detached process's exit can't be awaited without blocking anyway."* Note this is the argument in the
**opposite** direction from codex's async-hooks change recorded at
`agentic-design-patterns.md:4017-4019`, and it is this one that transfers, because inber's post-tool hook
already does real work.

inber's channel exists and is narrower than it looks. `agent.Hooks.PostToolResult`
(`agent/agent.go:65`) returns a string that becomes a model-visible injection
(`agent/agent_run.go:426-430`), and `engine/build_hooks.go:167-191` routes it to the workflow hooks
(auto-commit, auto-format, build/test) and the forge hook. But the forge hook's `action` is only
`Log.Info`'d (`engine/build_hooks.go:181-185`) — its `Kind` and `Reason` reach a log line and nothing
else — and the two provider loops disagree about when the hook runs at all. See the sweep entry below:
on the Anthropic path a failed tool call never reaches `PostToolResult`, which is also how the
`toolInputsCache` leak got in.

## 2026-09-01: an abort is a state, not an event — three PRs in one window, at three depths

[#13677](https://github.com/cline/cline/pull/13677) `fix(core): propagate parent aborts to
delegated subagents`, [#13647](https://github.com/cline/cline/pull/13647) `Propagate session aborts
to teammates`, [#13678](https://github.com/cline/cline/pull/13678) `fix(desktop): keep Stop
available for running child agents`.

The first two are the signal travelling down — a parent abort that left delegated children running.
The third is the one worth stealing, because it is not about the signal at all: a Stop control that
disappeared while a child was still running is a system deciding, on the user's behalf, that the
abort is over. Together they say an abort is a state the system has to keep being in, not an event
delivered once.

inber has the cascade and not the state. `StopSession` (`server/session_management.go:82-100`) reads
`s.Children`, recurses depth-first, and stops itself last — the #13647 half is done and correct. But
the child's *return path* has no cascade: `deliverResult` reaches
`server/spawn_delivery.go:101`, `injectIfRunning` returns false for a parent that is no longer
running, and the fallback starts a fresh turn on the session the user just stopped, under
`context.Background()`. Filed as `27dcb8e6-8844-41ef-8960-85feee45ae9d`. Its twin at the entry point
— a child stopped before its turn begins runs the whole task anyway, because `stop()` writes a
status `turn()` never reads — is `7de193b1`, and the two want one answer about whether `Completed`
gates `Session.turn`.

Also this window, and a different lesson:
[#13583](https://github.com/cline/cline/pull/13583) `fix(core): stop an empty capability list from
stripping image input` — an empty list read as a deliberate "supports nothing" rather than "not
configured". inber has no capability list and no image handling, so the subject does not transfer,
but the *shape* does with its polarity flipped: `FilterMessagesForAnthropic`'s assistant branch
(`agent/openai_conversion.go:320-327`) *writes* an empty block list as if it were a message, while
the user branch twenty lines below asks `len(newBlocks) > 0` first. Filed as
`b0f47ede-be6e-4d1d-af81-f5ca1e51943c`.

Routine in the same window, recorded so it is not re-read: desktop marketplace redesign (`#13653`),
shared attachment drop zone (`#13672`), Windows Authenticode signing (`#13607`, `#13021`),
searchable session history (`#13420`), agent-created schedules anchored in `.cline`
(`#13634`, `#13613`). The hook-crash fix (`#13422`) is the 2026-08-30 entry and adds nothing new.
