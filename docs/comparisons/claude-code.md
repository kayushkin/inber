# Claude Code Comparison

**Claude Code** — Anthropic's official CLI coding assistant. A terminal-based agent that can read, modify, and execute code across multiple files with sophisticated permission controls and session management.

## Architecture Overview

Claude Code follows a **tool-centric architecture** where individual tools (file operations, shell commands, etc.) are mediated through a comprehensive permission system. The agent operates through interactive sessions that can be resumed and continued, with project context maintained through CLAUDE.md files.

## Key Features for Inber

### 🔐 Tool Permission Model

**What it does well:**
- **Hierarchical permission system**: User → Project → Local settings with clear precedence
- **Three permission modes**: Allow (no prompt), Ask (confirm first), Deny (blocked)
- **Tool-specific controls**: Can allow/deny specific commands or entire tool categories
- **Session modes**: `default`, `acceptEdits`, `plan`, `dontAsk`, `bypassPermissions`
- **Granular rule syntax**: `Tool(specifier)` allows fine-grained control (e.g., `Bash(npm install)`)

**What inber should adopt:**
```go
type PermissionPolicy struct {
    Rules []PermissionRule
    Mode  SessionMode
}

type PermissionRule struct {
    Tool      string    // "bash", "file_write", etc.
    Specifier string    // optional: specific command/path pattern
    Action    Action    // Allow, Ask, Deny
}
```

**Current inber state:**
Inber currently prompts for all potentially dangerous operations but lacks fine-grained policy control. Users can't pre-approve specific commands or establish project-level policies.

### 🔄 Session Resume & Continuity

**What it does well:**
- **Session IDs**: Every conversation gets a persistent ID
- **Multiple resume modes**: `--resume` (interactive picker), `--resume <id>` (specific), `--continue` (most recent)
- **Resume in different modes**: Can resume interactively or in one-shot mode (`-p`)
- **Session metadata**: Shows timestamps, project dirs, conversation summaries

**What inber should adopt:**
```go
type SessionManager interface {
    SaveSession(id string, state SessionState) error
    ListSessions(limit int) ([]SessionMetadata, error)
    ResumeSession(id string) (*SessionState, error)
    GetMostRecent() (*SessionState, error)
}
```

**Current inber state:**
Inber's memory system provides some continuity but lacks explicit session IDs and resume capabilities. When an inber session ends, there's no direct way to pick up exactly where you left off.

### 📁 Multi-file Edit Strategy

**What it does well:**
- **Project-aware context**: Uses CLAUDE.md files for persistent project knowledge
- **Auto-discovery**: `/init` command analyzes codebase and generates starter configuration
- **Hierarchical context**: Project root → parent dirs → home folder for monorepos
- **Integrated tooling**: Understands build systems, test runners, and project structure

**CLAUDE.md example:**
```markdown
# Project Context
FastAPI REST API for user authentication and profiles.

## Key Directories
- `app/models/` - database models  
- `app/api/` - route handlers

## Standards
- Type hints required on all functions
- pytest for testing (fixtures in `tests/conftest.py`)

## Common Commands
uvicorn app.main:app --reload # dev server
pytest tests/ -v # run tests
```

**What inber should adopt:**
- **Project configuration files** (`INBER.md`?) that provide context automatically
- **Smart project detection** that recognizes common patterns (Go modules, package.json, etc.)
- **Context inheritance** for monorepo setups

**Current inber state:**
Inber reads some project files but lacks a systematic way for users to provide project context. Each session starts "cold" regarding project conventions and structure.

## Architectural Insights

### Permission System Design
Claude Code's permission model is **layered** (user/project/local) with **precedence rules** (deny → ask → allow). This allows both organizational policies and individual developer customization while maintaining security.

### Session vs Context Separation
Claude Code separates **conversation state** (session IDs, history) from **project context** (CLAUDE.md files). This allows resuming conversations while keeping project knowledge persistent across all sessions in that directory.

### Tool Abstraction
Their tool system appears to be **capability-based** — each tool declares what it can do, and the permission system mediates access. Tools seem to be self-contained with their own schemas and execution logic.

## What Inber Does Better

### Memory System
Inber's memory system (conversation + repo-map + stash) is more sophisticated than Claude Code's simple CLAUDE.md approach. Inber automatically captures and summarizes context rather than requiring manual configuration.

### Go Ecosystem Integration
Inber understands Go-specific patterns (modules, build tags, etc.) more deeply than Claude Code's generic approach.

### Context Management
Inber's smart truncation and context loading is more automatic — users don't need to configure what files are important.

## What Inber Should Steal

### 1. Permission Policy System
```go
// In engine/permissions.go
type PermissionEngine struct {
    policies []PermissionPolicy
    mode     SessionMode
}

func (pe *PermissionEngine) CheckTool(tool string, specifier string) Permission {
    // Implement deny → ask → allow precedence
}
```

### 2. Session Resume
```go
// In engine/session.go
func (e *Engine) SaveSession(id string) error {
    // Serialize conversation + context state
}

func (e *Engine) ResumeSession(id string) error {
    // Restore exact conversation state
}
```

### 3. Project Configuration
```go
// Load INBER.md files from current dir → parent → home
func LoadProjectContext() (*ProjectConfig, error) {
    // Similar to CLAUDE.md but Go-aware
}
```

## Implementation Priority

**High Priority:**
1. **Permission policies** — Solves real user pain points with repeated approvals
2. **Session IDs and resume** — Makes inber more professional/tool-like
3. **Project configuration files** — Reduces repetitive context explanations

**Medium Priority:**
1. **Tool interface standardization** — Enables permission system
2. **Enhanced session metadata** — Better session management UI

## Harness-watch — 2026-06-02: asyncRewake hook, tiered security review, path-sensitive permission escalation

The `security-guidance` plugin update (commits `441892ec`, `ccadef7d`) plus
CHANGELOG 2.1.160 surfaced several harness primitives.

### 1. `asyncRewake` — background review re-wakes a stopped agent with findings

The plugin's `hooks.json` uses a PostToolUse entry with `if: "Bash(git commit:*)"`,
`asyncRewake: true`, `rewakeMessage`, and `rewakeSummary`: on `git commit`/`push`
it kicks off a background agentic security review *without blocking the turn*, and
when results land the harness **re-wakes** the (possibly already-stopped) agent by
injecting `rewakeMessage` + findings as a new turn. A true async-feedback primitive,
distinct from a synchronous PreToolUse gate, guarded by a TTL loop counter
(`stop_hook_fire_count`, max firings, content-based dedup) to prevent
fix→re-review→re-fire loops.

**What inber should consider:** add an async post-tool hook channel — a hook can
return "pending" and later re-wake a stopped/idle session by injecting a synthetic
user turn (summary + payload), guarded by a per-session fire-count TTL to prevent
re-wake loops.

### 2. Three-layer cost-tiered security review

security-guidance v2 layers three escalating reviewers: (1) instant regex warnings
on Edit/Write for ~25 dangerous patterns (`yaml.load`, `pickle.load`, raw
`innerHTML`, hardcoded secrets); (2) a fast LLM diff review on the **Stop hook**
feeding high-severity findings back so Claude self-fixes before the user sees the
reply; (3) an SDK-driven agentic reviewer on `git commit` that uses Read/Grep/Glob
to trace cross-file data flow (IDOR/auth-bypass/SSRF a single diff misses). Org
policy is concatenated `claude-security-guidance.md` at user→project→project-local
scope with an 8KB budget that truncates project-local first. The escalation ladder
(cheap-always / medium-on-stop / expensive-on-commit) is reusable.

**What inber should consider:** model the PreToolUse prehook as a tiered ladder
rather than a single gate — cheap regex/heuristic on every edit, a Stop-time
fast-model diff review that self-corrects before responding, and an expensive
cross-file agentic review only on commit-class tools — sharing a layered
user/project/local policy file.

### 3. Path-sensitive permission escalation (the write *is* the execution vector)

CHANGELOG 2.1.160: even in `acceptEdits`/auto modes, Claude Code now prompts before
writing files that can silently cause code execution — shell-init (`.zshenv`,
`.zlogin`, `.bash_login`), `~/.config/git/`, build-tool config (`.npmrc`,
`.yarnrc*`, `bunfig.toml`, `.bazelrc`, `.pre-commit-config.yaml`, `.devcontainer/`).
A "safe" edit-only mode isn't safe when the edited file is itself an execution
vector. Also: a single-file grep/egrep/fgrep now satisfies the read-before-edit
precondition (any tool that observed current contents counts, not just Read).

**What inber should consider:** give the prehook a path-pattern escalation list —
writes to shell-init / VCS-config / build-tool config files always escalate to Ask,
overriding acceptEdits/auto, because the write itself is an execution primitive. And
if inber enforces a read-before-edit invariant, let any content-observing tool
(grep/search returning file lines) mark the file "seen" for the turn, not just the
dedicated Read tool.

> ⛔ **WALKED 2026-08-07 (nightly worker) — the read-before-edit half is LATENT and needs no
> further thought until that changes.** inber enforces no read-before-edit invariant at all:
> `grep -rn "read.before.edit\|readBeforeEdit\|has not been read"` over every `.go` file
> returns zero. The sentence is conditional ("if inber enforces") and the condition is false,
> so there is nothing to widen. The path-escalation half is a genuine feature proposal, not a
> defect, and belongs with the guard-mode work (`9eeba694`), not on the finding shelf.

The permission system would have the highest impact on daily usability while being relatively straightforward to implement given inber's existing tool architecture.

## Harness-watch — 2026-07-13: memory is the *non-derivable complement* of the codebase — a falsifiable rule for what to cut

[claude-code 2.1.206](https://github.com/anthropics/claude-code/commits/main/CHANGELOG.md) adds "a `/doctor` check that proposes trimming checked-in `CLAUDE.md` files by cutting content Claude could derive from the codebase." Small line, good idea. Every harness tells you to keep memory files short; nobody ships a *rule* for what to cut — so in practice memory files only grow. This criterion is falsifiable and automatable: **if the agent could recover the fact by reading the repo, it does not belong in memory.** Memory is for what is *not* in the source — preferences, decisions, landmines, non-obvious invariants, things that cost someone an afternoon to learn. It reframes memory from "helpful notes" to the non-derivable complement of the codebase. Note the deliberate posture: it **proposes**, it does not auto-delete.

> ⛔ **WALKED 2026-08-07 (nightly worker) — the premise below is FALSE twice over. Do not act on it.**
> (a) **inber has no top-level `MEMORY.md`, and never has.** `git log --all -- MEMORY.md` returns
> nothing and `git ls-files | grep -i memory` lists only source files. (b) **`:8160` is not
> memory-store.** `ss -tlnp` shows `:8160` is the `llm-bridge` gateway binary; memory-store is a
> library, not a service with a port (see repos CLAUDE.md). The accreting MEMORY.md the entry is
> actually describing is **Claude Code's own auto-memory on this host**
> (`~/.claude/projects/-home-kayushkincom-repos/memory/MEMORY.md`) — measured at **1,489 bytes**,
> 13 one-line index entries, and **zero** `FIXED`/`RESOLVED` markers. So the bloat the proposed
> pruning job would attack does not exist here, in either repo. The *idea* (memory is the
> non-derivable complement of the codebase) is still worth keeping; the inber-specific
> justification under it is not.

**What inber should consider:** inber has both a `memory-store` (:8160) and a large top-level `MEMORY.md` that only accretes (several entries are already marked FIXED/RESOLVED and are now pure context cost, paid on every session). A scheduler job that walks each memory entry with a cheap model and asks *"could you have derived this by reading the repo?"* — then files the candidates as a **proposal** rather than deleting — is roughly a day of work and attacks context bloat on every single session thereafter. Keep the propose-don't-apply posture; the same reason the noteboard decision backlog uses it. The complementary entry is 07-13 in `agentic-design-patterns.md`, which covers the *authority* half of claude-code's window (consent provenance, transcript tampering, headless consent banking).

Also worth noting from the same window, as independent corroboration rather than a new idea: 2.1.204 fixed "worktree-isolated subagents sometimes running shell commands in the parent checkout instead of their own worktree" — the **same scope-leak class** as codex #32197 in the 07-12 entry (sandbox cwd moved, writable roots didn't). Two harnesses shipped the identical bug in adjacent windows, which is a strong signal that inber's `forge` worktree slot deserves an explicit test that a worktree-confined agent *cannot* write to the parent checkout, rather than an assumption that it doesn't.
## Harness-watch — 2026-07-30 (CC 2.1.219): text the user already saw must survive the error that killed the turn — and a subagent tree deeper than one level needs event routing to match

[CC 2.1.219](https://github.com/anthropics/claude-code/blob/main/CHANGELOG.md) (2026-07-24) is
mostly Opus 5 plumbing, but two entries name defects inber has in its own equivalents. Neither is
about the CLI: inber talks to the Messages API directly through `anthropic-sdk-go`, so the
`--forward-subagent-text`, `mcp_server_errors` and `sandbox.*` settings themselves are unreachable
here. The *bugs behind them* are not.

### 1. "Fixed `claude -p` text output dropping the answer already produced when a turn dies on a mid-stream API error" — inber has this, and worse

inber streams deltas to the user and then throws the accumulated message away if the stream ends
badly. `agent/agent_run.go:153-170`: the loop calls `accumulated.Accumulate(event)` and fires
`a.hooks.OnTextDelta(...)` per chunk — so the user watches the answer arrive — but
`resp = &accumulated` happens **only** in the `else` branch of `streamResp.Err()`. On a mid-stream
error `resp` stays `nil` and `:188` returns `nil, fmt.Errorf("api call failed: %w", apiErr)`. The
discard then cascades: `agent/agent.go:265` returns early, so `processResponse` never counts tokens
and `*messages = append(*messages, resp.ToParam())` never runs — **the partial assistant turn never
enters conversation history**, making the work unrecoverable on the next turn. `engine/engine.go:228`
returns `nil`, skipping `postProcessResult` (the session log), `Guard.RecordTurn`, `Trace.RecordTurn`
and `Checkpoint.Take`. `server/spawn.go:227-250` guards on `result != nil`, so the request row
records status `error`, an empty summary, and **0 tokens → $0 cost for a call Anthropic already
billed**. Note the inconsistency that makes this clearly a bug and not a policy: the ctx-cancel and
max-API-calls paths (`agent/agent.go:203-214`) both write a placeholder into `result.Text`; only the
stream-error path writes nothing. The same discard sits at `agent_run.go:179-185`, where the
context-length retry re-issues non-streaming and abandons `accumulated` too.

**What inber should consider:** set `resp = &accumulated` before recording `apiErr`, and let
`agent.go:265` fall through to `processResponse` and the message append when `resp != nil` even with
a non-nil error. The invariant to state once and enforce everywhere: **bytes shown to the user are
already part of the conversation** — a later failure can annotate the turn, never un-say it. Usage
accounting rides along, which independently fixes billing a real call as $0.

### 2. "Subagents can now spawn nested subagents up to depth 3 (was 1)" — inber permits depth 2 and drops every event from it

CC pairs the depth raise with an escape hatch (`CLAUDE_CODE_MAX_SUBAGENT_SPAWN_DEPTH=1`) and, in
stream-json, forwards depth-2+ subagent text **keyed by its spawning Agent `tool_use` id** — the id
being the part that makes a deep tree addressable rather than merely permitted. inber already allows
the depth and has none of the routing. `MaxSpawnDepth` defaults to **2** (`server/config.go:23`,
`server/server.go:56-58`, enforced `server/spawn.go:57`), so root→child→grandchild is legal today.
But forwarding is hard-coded flat: `server/spawn.go:172` snapshots `parentOnEvent`, and the child's
filter at `:177` admits only `ev.Kind == "status" || ev.Kind == "thinking"`. When the child spawns a
grandchild, `parentOnEvent` resolves to *the child's own filter closure*, and the grandchild's
events arrive as `agent_update` / `agent_spawned` / `agent_done` — none of which match. **Every
depth-2 subagent is silently invisible on the root session's live stream.** Two related gaps: the
subagent identity and depth that `spawn.go:185-188` does put in `StreamEvent.Data` are discarded by
`streamEventToBridge`'s `default:` branch (`server/api_bridge.go:884-888`), which copies only
`Subtype` and `Message`, so an SSE client cannot tell which subagent or depth produced an update;
and `Store.SpawnChildren` (`server/store.go:266-272`) is `WHERE parent_request_id = ?`, one level,
so a grandchild's spend rolls up into no ancestor at any depth.

**What inber should consider:** either lower the default to 1 until routing is recursive, or make it
recursive — forward by re-emitting the child's event kinds rather than filtering to two, and carry
the `agent`/`depth`/`session_key` payload through `streamEventToBridge` instead of dropping it. The
transferable half of CC's design is the keying: an event from depth N is only useful if the consumer
can attribute it, which needs a stable id per spawning call. inber drops the tool id one layer
earlier — `engine/build_hooks.go:105` receives `toolID` and calls `d.OnToolCall(name, input)`
(`engine/display.go:15` has no id parameter), so `msg.ToolCallEvent.ToolID` is left empty at
`server/api_bridge.go:840-845` even though the bridge defines it. Same missing-identity root cause as
the goose #10716 entry.

> ⛔ **WALKED 2026-08-07 (nightly worker). This entry is now SPENT. Four claims, four different
> fates — do not re-derive any of them.**
>
> 1. **"Every depth-2 subagent is silently invisible" — FIXED, no longer true.** `13d87d1` made
>    the forwarder pass a descendant's envelope through unchanged; `server/spawn.go:96` is
>    `case eventKindAgentUpdate, eventKindAgentSpawned, eventKindAgentDone: emit(ev)`. Todo
>    `c81c4b63`.
> 2. **`msg.ToolCallEvent.ToolID` always empty — FIXED tonight, inber `0d72bc3`.** The id was
>    dropped only on `DisplayHooks`, which took `(name, input)`; it now takes `toolID`, and
>    `StreamEvent.ToolID` carries it to `api_bridge`. The live cost was larger than this entry
>    says: bridge-ui pairs call to result by `tool_id`, so an empty id put them in unrelated
>    rows and the tool **output was discarded**, not merely unlabelled.
> 3. **"`Store.SpawnChildren` … a grandchild's spend rolls up into no ancestor at any depth" —
>    REFUTED. Do not file this.** `server/spawn.go:262-266` resolves `ActiveRequest(req.ParentKey)`
>    and `:281` writes it as the child's `parent_request_id`, so a grandchild IS linked — to its
>    immediate parent. The query (now at `server/store.go:572-579`, not `:266-272`) is a normal
>    one-hop adjacency lookup; a caller wanting a subtree recurses. And there is no caller:
>    `SpawnChildren` has exactly one reference in the repo, `server/store_test.go:130`. No
>    aggregate over `requests` exists anywhere (no `SUM`, no `GROUP BY`), and the only dollar cap,
>    `guard/guard.go:287`, is fed solely by the session's own turns via `RecordCost`. There is no
>    rollup for a grandchild to fall out of.
> 4. **The `agent_*` attribution drop — CONFIRMED and FILED as `e37fc5f4`.** `Data` carries
>    `agent`/`session_key`/`parent_key`/`depth` (and `status` on `agent_done`), and
>    `streamEventToBridge`'s `default:` branch copies only `Subtype` and `Message`. Left open
>    because mapping it onto `msg.SystemEvent`'s `TaskID`/`SubagentType`/`TaskStatus` is a
>    protocol question spanning three repos.
>
> Note the doc's line numbers were stale in every case (`store.go:266-272`, `server.go:56-58`,
> `spawn.go:57`, `build_hooks.go:105`, `api_bridge.go:840-845`). Re-locate by symbol, not line.

### 3. Two settings with no inber analogue, recorded so the gap is named

`sandbox.network.strictAllowlist` denies non-allowlisted hosts for sandboxed commands **without
prompting** — i.e. egress is a policy axis separate from command approval. inber has no egress axis
at all, and no command axis either: `guard.CheckTool` has zero non-test callers and the mode is
hardwired `guard.Autonomous` (`engine/engine.go:169-171`), whose `default:` branch returns
`Allowed`, while `tools/tools.go:45-52` registers `Browser`, `WebSearch`, `WebFetch` and a
`bash -c` shell that inherits the full `os.Environ()`. That absence is already ranked in
`docs/harness-control-matrix.md:66,78`; what is new is the *shape* worth copying when it is built —
a host allowlist consulted without a prompt, so the sandbox is not merely advisory. Separately,
`mcp_server_errors` in the init event exists so a server dropped by config validation is visible
rather than silently missing; inber emits no init/system event and `msg.SystemEvent.MCPServers` is
never populated, which is moot only because `tools/mcp/` still has zero importers
(`harness-control-matrix.md:91`) — worth wiring at the same time as the client.

> ⛔ **WALKED 2026-08-07 (nightly worker). The load-bearing claim here is STALE — do not repeat it.**
> "`guard.CheckTool` has zero non-test callers and the mode is hardwired `guard.Autonomous`" was
> true when written and is not now. `engine/build_hooks.go:94` is
> `switch e.Guard.CheckTool(tool, input)`, and the mode comes from config:
> `engine/engine.go:201` `mode, err := guard.ParseMode(cfg.Mode)` feeding
> `:205` `e.Guard = guard.New(e.Limits.GuardConfig(mode))`. The gate is wired. What is genuinely
> still open is *which tools are classified* — nine names reach the model unclassified, so Assist
> allows them unasked. That is filed as `9eeba694`, not here.
>
> The egress-axis and `mcp_server_errors` halves stand as written: both are absent, and both are
> latent while `tools/mcp/` has no importer. The worktree-confinement point in the 2026-07-13
> entry above is already filed as `d967400a` (an A/B/C/D policy call), so it needs no new todo —
> `forge` IS live in inber (`server/spawn.go`, `engine/engine_new.go` import it), so that one is
> live rather than latent.

## Harness-watch — 2026-08-20 (CC 2.1.236–2.1.237): a subscriber needs an identity and a lifetime — inber's SSE fan-out is one mutable slot, so the next turn unsubscribes the viewer and the viewer's exit mutes the turn

Two releases landed in this window: [2.1.236](https://github.com/anthropics/claude-code/commit/084ca20b)
and [2.1.237](https://github.com/anthropics/claude-code/commit/770933ea). Both are CLI features inber
cannot use directly, but one line in each names a rule inber breaks. 2.1.236 adds `notify_when_idle`
to cross-session `SendMessage` — "opt-in, one-shot, no polling" — and, in the same release,
"`SendMessage` now refuses further messages to a session up front once a rapid burst would exceed
what that session's inbox accepts, **instead of reporting them sent while they were dropped**." The
pair states one principle twice: a subscription is a registered thing with an owner and an end, and
delivery that cannot happen must be refused rather than silently discarded.

### 1. The bridge's "persistent SSE subscription" is a single mutable callback slot — two different events unsubscribe it, and one of them mutes an unrelated live turn

`handleBridgeEvents` (`server/api_bridge.go:336`, commented "persistent SSE subscription") subscribes
by *overwriting* a field: it takes `sess.mu`, saves `origOnEvent := sess.onEvent`, installs a closure
that calls the old one and then fans out to its own channel, and restores the saved value on exit
(`server/api_bridge.go:364-384`). There is no subscriber list. `Session.onEvent` is one slot, and its
own comment says what it was built for — `// current request's event callback (updated per-turn)`
(`server/session.go:67`). `setOnEvent` (`server/session.go:72-76`) overwrites it unconditionally, and
`getOrCreateSession` calls `setOnEvent` on **every** run and stream against a live session
(`server/session_creation.go:26`, `:34`), after which `Session.turn` rebuilds the engine's display
hooks from whatever is in the slot (`server/session.go:151-152`). Three producers install callbacks
this way — the HTTP run path, the NATS bus (`server/bus.go:114`) and spawn delivery
(`server/spawn_delivery.go:114-116`) — and each one silently evicts whoever held the slot.

That gives two failures from one root cause. **A viewer goes deaf:** an SSE client attached while the
session is idle stops receiving anything the moment any other caller starts a turn, which is the only
moment it exists for; the HTTP connection stays open and streams nothing, so the client cannot tell
subscription from silence. **A viewer's exit mutes someone else:** the deferred restore at
`server/api_bridge.go:379-384` writes back the callback captured at subscribe time. If a turn started
after that, closing the SSE tab replaces that turn's live writer with a dead closure from a finished
request, and `updateHooks()` on the next line pushes it into the running engine — the streaming
caller stops receiving output mid-turn, for a reason that happened in a different connection. Nothing
logs either transition.

**What inber should consider:** make the subscriber plural — a map of id → callback on `Session`,
with subscribe and unsubscribe by that id, so a restore can never clobber a stranger. **What a fix
must decide:** whether the per-request writer stays privileged. Today "the current request's writer"
and "a long-lived observer" are the same field, and the run path depends on last-write-wins to point
the hooks at the caller now streaming. A subscriber set has to say which callbacks a new turn
replaces (the request writer) and which survive it (observers), or the bus, spawn delivery and the
bridge start delivering each other's turns.

### 2. A slow SSE client loses events silently, and the frames carry no id, so it cannot detect the gap

The fan-out closure sends to a 100-deep buffer with a bare `default:` and no logging, counter or
marker (`server/api_bridge.go:371-374`). The frames it feeds are written as `event:`/`data:` only
(`server/api_bridge.go:397-399`), with no `id:` field, so a client that fell behind during a burst of
deltas receives a transcript with a hole in it and no way to know. This is exactly the shape 2.1.235
and 2.1.236 closed on `SendMessage` twice — refuse up front rather than report a delivery that did
not occur. The neighbouring drop in this repo is not the same: `offerToTurnInFlightLocked`
(`server/session.go:280-287`) also selects with a `default:`, but it logs a warning, returns `false`,
and `deliver` falls the message back to the pending queue, so nothing is lost.

**What inber should consider:** number the frames and let the client see the gap — an `id:` per event
plus a drop counter in the next frame costs nothing and turns silent loss into a visible one. **What
a fix must decide:** what to do when the buffer is genuinely full. Blocking is not free here: the
closure runs on the token-streaming goroutine, so a stalled reader would stall the turn itself. Drop
with a marker, coalesce deltas, or disconnect the slow client are three different answers with
different blast radii, and this entry does not pick one.

### Checked and not worth a finding

- **"Fixed prompt caching for sessions using an LLM gateway or custom base URL" (2.1.237)** — inber
  never sets a base URL on the Anthropic path (`agent/clients.go:103-129` passes auth and headers and
  no `option.WithBaseURL`), so there is no inber-side gateway path to break. Worth recording, since it
  is not obvious: the SDK still honours `ANTHROPIC_BASE_URL` from the environment
  (`anthropic-sdk-go@v1.35.0/client.go:34`), so a gateway *can* be interposed on this host without a
  line of inber config — and the OAuth branch would send its `sk-ant-oat01-` token there in a header,
  which body-side egress redaction cannot see by construction.
- **`ANTHROPIC_DEFAULT_MODEL` vs `ANTHROPIC_MODEL` (2.1.236)** — a two-tier default where an explicit
  pick outranks the environment. inber resolves the model per agent through model-store already; no
  competing environment tier exists to disambiguate.
- **Sandbox wildcard read-deny precedence (2.1.236)** — `guard.CheckTool` is a tool-*name* switch with
  no path rules (`guard/guard.go:158-184`). No shape to check, as previously recorded.
- **Recap text capped at 400 characters (2.1.236)** — bounding a generated artifact so it cannot grow
  without limit; the same rule as the compaction-prompt entry of 2026-08-16, already filed.
- **"Concise" output style, `/model` picker sizing, fullscreen renderer fallback, spellcheck** — CLI
  presentation, no analogue.
