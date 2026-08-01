# Write-gated-differently-from-its-read: the audit

One question, asked of every artifact this repository persists:

> **Name the read that consumes it, and confirm the write gate and the read gate are
> the same expression.**

The failure it hunts has no symptom. State is written under condition A and read
under condition B — or under no condition, because no reader exists — and nothing
errors, nothing logs, and a success divider sits over the top of it. cline #12563's
second bug was a compacted context persisted under one condition and read under
another that defaulted false. `docs/comparisons/agentic-design-patterns.md`,
2026-07-28 §3(b), asked for this sweep and named the first pair.

Walked 2026-08-01. Every line below was read, not inferred. **Do not re-derive
these; extend them.**

## Fixed

### The compaction archive — `conversation/summarize.go`
Write gated on `cfg.SaveToMemory && memStore != nil`; **no reachable read at all**.
The archive is tagged out of the automatic context by design, so the only way back
is `memory_expand` by id, and the id went to the caller, a log line and the session
log — never anywhere the model could see. Fixed in `69f9e7b`: `summaryFooter` names
the archive, gated on the write having happened *and* on `memory_expand` being on
the wire. Two bugs in the read itself fixed in memory-store `52724ae`.

### The stash pointer — `conversation/stash.go`
Same shape, one file over, and it had been *cited as the good example* in the
compaction fix's own comment ("StashLargeContent … has always left its pointer
inline"). It does leave a pointer. The pointer names `memory_search` and
`memory_expand` unconditionally, which is a claim about this repository rather
than about the session.

What decides the truth of that claim is the agent's configured tool list:
`engine/buildMemoryTools` keeps only the `memory_` names the config asked for, and
`SetDisabledTools` can take one away afterwards. The stash was gated on none of it
— `cfg.Enabled && memStore != nil` is entirely about the write.

Measured against the ten agents configured on this host:

| agents | recall tools held | what the pointer said |
|---|---|---|
| task-manager, fionn, party, logstack, researcher | search + expand | correct |
| scathach, oisin, bran, orchestrator | search only | named `memory_expand`, which they cannot call |
| worker | none | named two tools it does not hold |

For `worker` the stash was a deletion with a receipt: the block leaves the message,
which is the whole point of stashing, and the memory row is then unreachable.

Fixed here. `StashConfig.RecallToolNames` carries the wire set, the engine fills it
per turn from `EnabledToolNames` (per turn, not at construction — `SetDisabledTools`
moves it mid-session, and `summarizeIfNeeded` reads the same set at the same moment
for the same reason), the pointer names only what is there, and a session that can
recall nothing does not stash. The id is now named in full: memory-store resolves a
prefix onto a row, but two of 35,000 uuids sharing their first eight characters is
likelier than not, and that read answers with whichever row SQLite reaches first.

## Unpaired, and each one is a decision

Found by this sweep, not fixed — each needs a call on **wire the read or delete the
write**, and that call is the owner's.

| artifact | write | read |
|---|---|---|
| `messages.json` in the session log dir **KEPT, error no longer discarded** | `engine/lifecycle.go`, every turn | none in production — `session.LoadMessagesFromDir` has no production caller. But it is the only *lossless* copy in that directory: `session.jsonl` is append-only, so reconstructing from it drops any tool call the process died inside, which is why `LoadMessagesFromDir` prefers the snapshot when it is there (`resume_pairing_test.go`: 14 of this host's 23 session dirs carry one). Deleting the write would make every future log dir the lossy kind and would foreclose the wire-it option in todo `875aa19e`, which owns that reader. So: write kept, all three errors in `saveResumableState` now logged, and the turn counter is written only after the transcript it counts |
| `checkpoint.json` **write repaired, wire-vs-delete still open** | `session/checkpoint.go`, every 20th user turn | Still none — `LoadCheckpoint`/`ListCheckpoints` have zero callers, and that call is todo `78403a2a`'s child. Three things the write got wrong are fixed, because they are wrong under either answer. It stamped `Session.turn`, the API round-trip counter, while the gate fires on the engine's user-turn counter, so a checkpoint taken at user turn 20 filed itself as turn 87 — the caller now passes the number the gate used, and the field is `UserTurn`/`user_turn` so the two clocks cannot be confused again. `MaxCheckpoints: 5` and its `return nil` `pruneOldCheckpoints` stub are deleted: one filename is written and overwritten, so the knob read as a retention policy that retained nothing. And the token totals were read outside the mutex that `LogAssistant` writes them under — a real race, confirmed by `-race`. `session/checkpoint_test.go` pins all three |
| ~~`timeline.md`~~ **RESOLVED, write deleted** | was `session/session.go:258`, every turn, by re-parsing the whole JSONL | Still none, and now nothing produces the file either. The one consumer, `GET /api/sessions/{key}/timeline`, calls `ReadTimelineFromJSONL` and regenerates the same markdown from `session.jsonl` — it had already chosen the source of truth over the derived copy. `appendTimelineEntry` is deleted; `session/timeline_artifact_test.go` pins that `EndTurn` writes no artifact **and** that the timeline is still answerable from the log. Todo `4efefbc0` |
| `turns.cost` / `in_tokens` / `out_tokens` / `tool_calls` / `stop_reason` | `session/db_turns.go:19`, every turn | none reachable — `GetTurns`, `ListSessions`, `ListActive` have zero production callers. `IncrementToolCalls` has none at all |
| ~~workspace `system/*.md`~~ **RESOLVED, doc side** | `engine/turn_prompt.go:155,169` — `RemoveAll` + rewrite each turn | Still none, and now that is what the code says. `Workspace.ReadSystem` is deleted (it had zero callers, tests included) and the type comment no longer promises a read-back it never did; `session/workspace_test.go` pins that an edit and a user-added file are both discarded on the next turn. The write itself is unchanged and its error is no longer discarded. Making the directory genuinely editable stays open as a design call — it is keyed on the agent, not the session, so two concurrent sessions share it and a read-back would need a per-write manifest plus an answer to the sharing. Todo `9def7d6b` |
| `Session.truncateRefs` | `session/session_logging.go:84`, holding the **full** output of every truncated tool result | none — `GetFullToolResult` has zero callers. It defeats the truncation that populates it, in memory, for the life of the process |
| session summary | `engine/lifecycle.go:273`, every session that had messages | none that names it. `memory_search` applies no tag filter so the row is reachable, but nothing searches for it, no id is left behind, and `memory/auto_context.go:15` already concedes it "leaves none, and it is the one that cannot" |

The one that cost real work per turn was `timeline.md`: it re-parsed and
reformatted the entire session log on every `EndTurn` to slice out the turn that
had just finished, so the cost of ending a turn grew with the length of the
session. That write is gone. The log-dir `messages.json` also rewrites the whole
transcript each turn, but it is a single marshal already done for the workspace
copy and one `WriteFile`, and it buys the only lossless record in the directory —
so it stays, and the case for deleting it now belongs to `875aa19e` along with the
reader.

## Paired — confirmed, do not re-check

- **Guard state.** Write `server/session_management.go:157` gated on
  `s.Engine.Guard != nil`; read `server/session_creation.go:195` gated on the same
  thing. Same directory, same expression. The CLI path persists and loads *neither*,
  so a CLI session resumed with `--max-cost` gets a fresh budget — absent at both
  ends, symmetric by omission rather than drift.
- **Workspace `messages.json`** (`engine/lifecycle.go:247` ↔ `engine_new.go:164`)
  and **server `messages.json`** (`session_management.go:150` ↔
  `session_creation.go:230`). Both paired. Three files share this name; only the
  third, in the log dir, is unread.
- **`bus/`.** Persists nothing locally. `PublishOutbound` goes to JetStream, whose
  consumer is another service. One dead API (`Client.Reply`, no callers) and one
  genuine drop worth its own todo: `Subscribe`'s `default:` branch discards an
  inbound message when the 64-slot channel is full, `log.Printf` only, no error to
  any caller.

## Not writes at all — the "hot-path stub" claim, verified

`docs/harness-control-matrix.md` records checkpoint, trace and codeindex as
hot-path stubs. Confirmed, and the distinction matters for this audit: an unread
write is a defect, a package that never writes is only unfinished.

- **`trace/`** — `RecordTurn` is called every turn from `engine.go:287`, but the
  recorder is built with a hardcoded empty dir (`engine.go:201`, `// TODO: enable
  via config`) and `NewRecorder` returns nil for an empty dir. `WriteSummary` is
  `return nil`. No write primitive exists in the package.
- **`checkpoint/`** — `Take` is `return nil, nil`. `m.points` is never appended to,
  so `List` is always nil and `Diff` always `""`. The gate reads
  `e.Checkpoint != nil`, which is *always true*: a call site that looks like a
  feature flag and is not one.
- **`codeindex/`** — `Open` returns a struct and a TODO; `Refresh`, `Search` and
  `RepoMap` have zero callers.

## The method, for the next artifact

1. Find the write and write down its gate as an expression.
2. Name the read. Not "it could be read" — the function, and its callers.
3. Compare the two expressions. If the read has no callers, the write is unpaired
   whatever the gate says.
4. When the read is a *tool call*, the gate includes whether that tool is on the
   wire, and only the engine knows that. Both defects fixed above were this case.
5. Test it at the call site. A test that sets the gate by hand passes just as well
   when nothing ever sets it.
