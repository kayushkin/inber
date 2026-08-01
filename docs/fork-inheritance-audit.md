# What a fork inherits: the audit

One question, asked of everything `forkSession` hands a child:

> **Name it, say whether the child reading it as a statement about itself is true,
> and say whether it should cross at all.**

The failure it hunts is a child steering on a claim it inherited. `docs/comparisons/agentic-design-patterns.md`, the 2026-07-28 entry: codex strips stale parent *usage* hints on fork while keeping developer messages. Inherited instructions are wanted. An inherited "you have used N tokens" is not — it is a claim about a budget the child does not have, and the model steers on it.

Walked 2026-08-01, against `server/session_forking.go` at `9166563`. Every line below was read or measured, not inferred. **Do not re-derive these; extend them.**

## The boundary

`forkSession` is 55 lines and hands over exactly six things:

| what | where | crosses? | should it? |
|---|---|---|---|
| the parent's messages, deep-copied | `session_forking.go:18–28` | yes | yes, **but not everything inside them** — see below |
| the parent's turn counter | `:19`, `:54` | yes | yes, and it is load-bearing — see below |
| the parent's workspace roots | `:41–42` | yes | yes, deliberate, documented at the site |
| spawn depth + parent key | `:55–56` | yes | yes |
| the fork notice, one appended user message | `:59–65` | n/a, written fresh | yes |
| the guard's caps and totals | not copied | **no** | that absence is two open todos |

Everything else is built fresh by `createSession`: a new engine, a new `Tokens` total, a new `injections` channel, no pending queue, no children, no read cache.

## The one thing that crosses and should not

**The parent's `[Context]` blocks are inside the copied messages, and nothing takes them out.**

`buildTurnPrompt` collects the volatile content into `e.Turn.VolatileContext` (`engine/turn_prompt.go:148–152`), and `Agent.buildRequest` inserts it into the last user message. The insert is not request-local:

```go
(*messages)[lastIdx].Content = newContent      // agent/agent_run.go:112
```

`messages` is `&e.Messages` (`engine/turn_execute.go:42`). So the block is appended to the conversation, persisted with it at the end of the turn, and deep-copied into every fork. Pinned by `agent/volatile_context_writeback_test.go`, both halves sabotage-verified.

What is in one of those blocks, from this host's live transcripts:

- `## Active Agents` — every other running session's **turn count** and **elapsed running time** (`engine/fleet_status.go:41–52`).
- `# Server Sessions` — every other session's key, status, **message count** and last-active age (`server/session_context.go:96–130`).
- `# Agent Fleet` — the whole fleet with per-agent status and current task.
- `Recently modified (N minutes ago): <path>` — the volatile memory rows.
- `[source: <ref>]`, `[Context]\nBuild: <ref>`.

Three of those five are counts. All five are true only of the moment they were built.

**Measured across the 95 persisted transcripts in `~/.inber/server/sessions` (4,958 messages): 1,218 baked `[Context]` blocks in 84 of them, a quarter of a block per message.** The busiest single transcript carries 29 in 62 messages. `agent:claxon:main`, the orchestrator a fork would most likely be taken from, holds 7 in 24 messages.

The sharpest form of the defect is not staleness, it is that **a fork usually cannot produce a replacement**. `contextInjectorsFor` returns nil unless the session's agent is the default agent (`server/session_context.go:15–17`), and a fork is normally a fork *to another agent* — every child on record here is `brigid` under a `claxon` parent. So the inherited block is not merely the oldest picture the child has of the fleet, it is the **only** one it will ever have, and nothing in the transcript says when it was taken.

Nothing strips or rewrites one either: `[Context]` appears in three places in the tree and all three write it. Only whole-message pruning can remove one, and it takes the turn's real content with it.

### The sibling's bill is in there too

`deliverResult` injects the child's completion message into the parent's conversation through the same write-back (`agent/agent.go:300–311`), and that message carries `describeSpawnTokens` — the whole prompt, the split, and the dollar figure. Live example from `agent:claxon:bridge-1776397122756535143`, message 10:

```
[Sub-agent completed]
Agent: brigid (agent:claxon:main:sub:28939)
...
Tokens: 14981 in / 5979 out ($0.577)
```

**58 such lines across 20 transcripts.** A fork taken after that turn inherits a *sibling's* spend as an unlabelled fact about its own conversation. This is the closest thing in the tree to the codex finding the audit was asked about, and it arrives by a route the todo did not name — not from the parent's own usage, but from a third session's, forwarded through the parent.

### What this is not

It is not a leak of the parent's *own* token or cost totals. `e.Tokens` is engine state, never rendered into a message; `guard`'s totals are never rendered either. The parent's spending crosses only as arithmetic the model could do from the sibling lines, which is a much weaker claim than "you have used N tokens" and is worth saying plainly rather than overstating.

**This is a decision, not a fix, and it is filed rather than shipped.** The options are not equivalent: strip every `[Context]` block on fork (loses the recently-modified file list, which is the one part a child plausibly still wants); keep them and stamp each with the moment it was taken (honest, costs tokens forever, and the model must be told to read the stamp); or give every session the injectors so the child overwrites the picture on its first turn (fixes staleness, and hands non-orchestrator agents a fleet listing they were deliberately not given).

## What does not cross, and the two todos that own the absence

`forkSession` calls `createSession(..., RunRequest{}, ...)` — a zero request. Every cap the server has is a field of that request (`applyRequestOverrides`, `server/session_creation.go:57–95`), so the child configures none. `restoreGuardState` then runs against the **child's own key** (`:167`), which is brand new and has no sidecar, so it returns at `recorded == guard.State{}` and restores nothing.

So the guard totals do not ride the fork, and the todo's third bullet — "a fork inherits its parent's spent turns and dollars" — **is falsified**. The child inherits neither the totals nor the caps. A forked child runs under `guard.Config{}`, and a zero cap reads as unlimited.

That absence is already owned: `9e31d359` (three `createSession` callers pass a zero `RunRequest`) and `610e0f4a` (spend ceilings are per-session, so a fork gets a fresh full allowance). Nothing here adds to them except the confirmation that the fork path reaches the second one through the first.

The engine's own limits are not affected: `NewEngine` fills them from the **child agent's** stored config, and the `Detach` defaults (25 turns / 500k input tokens, `engine/engine_new.go:406–413`) do not apply because a zero request sets no `Detach`. Those are the child's own numbers, correctly.

## The turn counter crosses, and it decides what the child spends

`child.Engine.RestoreSession(parentMessages, parentTurnCounter)` is deliberate and documented — the child's first turn is not a first turn. It is also the one inherited number with a live effect on the child's spending, which the comment does not say: `contextBudget` reads `e.Turn.Counter` (`engine/turn_context.go:23,34`), so an inherited counter means the child never gets the `Counter == 0` first-turn budget of 4,000 tokens, and a fork of a parent past turn 15 opens straight onto the 8,000 branch.

Left as is. The child really is that many turns deep in that conversation, so the budget it gets is the right one for the transcript it holds; this is recorded because it is the only place an inherited count changes what a child pays, and a future change to `contextBudget` needs to know it is reading a parent's number.

Note it is **not** the guard's turn count. `guard.turns` is separate, starts at zero for the child, and is what `MaxTurns` checks.

## Measured, then falsified: the 56-year fleet status

The transcripts hold 6,753 `## Active Agents` duration figures and **every single one is longer than a year** — `running 493443h38m`, about 56 years, i.e. a start time at the Unix epoch. 100% of them, across 80 transcripts, on the hot path the model reads every turn.

It is dead. `3ed4abf` (2026-07-13, "fix epoch-1970 start times and apply the SQLite pragmas the DSNs claimed") fixed it, with tests that re-inject the original bug. Every transcript on disk predates it.

Two things worth keeping from chasing it:

- The first explanation was wrong and looked certain. `sessions.started_at` holds Go's `t.String()` shape — `2026-03-25 05:35:45.192770721 +0000 UTC m=+0.019650163` — which reads like the known modernc trap. **Probed against the real driver: it parses that string correctly**, monotonic suffix and all, and a bound `time.Time` today writes RFC 3339, not `t.String()`. The trap did not apply. Ten minutes with a throwaway `main.go` beat a confident reading of the column.
- Before treating a 100%-wrong figure in old transcripts as a live defect, `git log` the file that prints it. The data is a recording, not a measurement of HEAD.

## The child's key is minted from a clock remainder

Not usage-shaped, but it is the mechanism by which foreign usage *could* ride into a child, so it belongs in this audit.

```go
suffix := fmt.Sprintf("%d", time.Now().UnixNano()%100000)   // session_forking.go:91
```

The key is generated and used with no uniqueness check (`spawn.go:179`, `api_bridge.go:605`). `createSession` then reads `sessions/<key>/` for that key — so a child whose suffix collides with a retired sibling's picks up **that sibling's persisted transcript and its recorded guard state**, caps, spent turns, dollars and all.

On the fork path the transcript half is masked, because `RestoreSession` overwrites it a moment later; the guard state is not overwritten and would stand. On the plain spawn path nothing is masked and the child inherits the whole foreign conversation.

Measured on the live store: 29 children on record, 27 of them under one parent, 29 distinct suffixes, **no collision has happened yet**. In a 100,000-wide space, 27 draws carry roughly a 0.35% chance of one. Small, and it is a name-derived join in a repository whose standing rule is to join on ids — the parent key plus a store-assigned id would cost nothing and close it.

### What a collision costs, re-measured (2026-08-01)

The audit priced the collision as transcript + guard state. Two corrections, both measured on the live box:

- **The store row is the worse half, and the audit did not name it.** `UpsertSession` leaves agent, lineage and workspace **alone** on conflict, so the second child's own agent name is not written — it is silently recorded as the first child's, and `agentForSession` (`api_bridge.go:982`) reads that row on every rebuild. That is exactly the defect `session_agent_resolution_test.go` exists to close, arriving by a different door. It crosses agents in practice: the 27 children of `agent:claxon:main` are brigid's, fionn's and manannan's.
- **The guard-state half has no material on disk yet.** `command find ~/.inber/server/sessions -name guard_state.json` returns **0** across all 95 session directories; every one holds `messages.json` and nothing else. The sidecar's only writer is `persistSessionState`, landed in `88780d7` (2026-07-31), and the box has barely spawned since (`3a968e87`). So today a collision costs the transcript, the turn counter and the row; it costs the recorded budget from the next spawn onwards.

**Fixed, option C of `704c5000`:** `mintChildSessionKey` proposes a key and then checks it — against the live session map, the store row (new `Store.SessionExists`, because `SessionAgent` cannot tell "no row" from "empty agent") and the transcript directory — reserving each proposal in `Server.pendingChildKeys` so two concurrent spawns cannot both be told one key is free. A check that cannot be completed refuses; 100 taken proposals in a row refuse. `sessionKeyForChild` is now a proposal, not an answer.

The identity question is untouched and still open: the key is still derived from the clock and still readable by `backfillSessionLineageFromChildKeys`. Options A and B of `704c5000` remain a choice for the owner.

## Do not re-derive

- The write-back is a fact of `buildRequest`, not of the engine, and it is pinned in `agent/volatile_context_writeback_test.go`. Both assertions were sabotage-verified by making the injection request-local; both failed.
- The guard does not cross. Read `9e31d359` and `610e0f4a` instead of re-measuring it.
- The epoch durations in the transcripts are pre-`3ed4abf` recordings. They are not a live defect and the driver is not the cause.
- `saveSpawnToMemory` writes no counts, so nothing usage-shaped reaches a child by the memory route. (Its own defect — an empty id, so every spawn result overwrites the last — is `a475c73c`.)
