# Herdr Comparison

**Project**: [Herdr](https://herdr.dev/) — [github.com/ogulcancelik/herdr](https://github.com/ogulcancelik/herdr)
**Language**: Rust (single binary, no Electron). Vendors `libghostty-vt` (terminal emulation) and `portable-pty`.
**License**: Apache-2.0
**Focus**: An **agent multiplexer** — "tmux for coding agents." It runs foreign agent CLIs in PTY panes, *infers each agent's lifecycle state from the outside*, and exposes the whole session over a Unix-socket API so agents can script each other.
**Key Strengths**: Two-channel agent-state inference (passive terminal scraping + injected lifecycle hooks), a JSON-RPC socket API with a server-owned `agent.wait`, live server handoff that hot-upgrades the daemon without killing agents, native session-identity capture for resume, detach/reattach over SSH.

> **Namesake warning (inverted from the first draft of this doc).** `herdr.org` is a
> closed, hosted "agent orchestration" marketing page with no public source or architecture —
> not worth a comparison. **This doc is about `herdr.dev`**, the open-source Rust project, which
> is a different kind of thing (a local terminal multiplexer, not a hosted orchestrator) but is
> actually inspectable and solves problems inber's llm-bridge ecosystem has. Do not merge the two.

## Why this is worth studying for inber

Herdr is not an orchestration server and is not trying to be inber. But it has independently built
production-grade answers to three problems inber's **harness-bridge** layer faces, precisely because
Herdr wraps *foreign* agent CLIs it did not write:

1. **Knowing an agent's state when the agent doesn't tell you** (PTY-mode / non-structured harnesses).
2. **Upgrading the long-lived server process without killing the sessions under it.**
3. **Capturing a foreign agent's resumable session id so you can restart its actual conversation.**

Inber solves (1) and (3) for harnesses it drives via stream-json (llm-bridge-claudecode reads
structured events), but has no answer for a harness running in raw PTY mode, and (2) is an open
landmine (see the "never restart the gateway unattended" directive).

## Architecture Overview

```
herdr (client, any terminal)  ──socket──▶  herdr daemon (server)
                                              ├── PTY panes (vendored portable-pty + libghostty-vt)
                                              │     └── foreign agent CLI runs here (claude, codex, pi, …)
                                              ├── detect/  — per-agent TOML manifests + rule engine
                                              │     classify pane → idle | working | blocked | done | unknown
                                              ├── integration/ — injected hook scripts (self-report channel)
                                              ├── api/  — Unix-socket JSON-RPC (agent.*, pane.*, events.*, layout.*)
                                              ├── worktree/ — git worktree per workspace
                                              └── handoff_runtime — hand PTY fds to a replacement server binary
```

Client and server are separate: `ctrl+b q` detaches, `herdr` reattaches, sessions survive restarts,
and you can reattach over SSH. Keyboard (tmux-style prefix) and mouse are both first-class.

## What Herdr Does Well

### 1. Two-channel agent-state inference ⭐️ (the standout)

Herdr classifies every agent pane into `idle | working | blocked | done | unknown` using **two
independent channels**, because it can't assume the agent cooperates:

- **Passive: a declarative terminal-scraping rule engine.** Each agent has a versioned TOML manifest
  (`src/detect/manifests/claude.toml`, `codex.toml`, `grok.toml`, …). Rules match *named regions* of
  the rendered screen — `osc_title`, `prompt_box_body`, `after_last_horizontal_rule`,
  `bottom_non_empty_lines(5)`, `whole_recent` — with `priority`, `regex`/`contains`/`any`/`all`/`not`
  predicates, and flags like `visible_blocker` / `skip_state_update`. Example: Claude's
  `bash_permission_prompt` rule fires `blocked` when the screen shows "do you want to proceed?" plus a
  "1. yes / 2. no" body; a transcript-viewer overlay is matched only to *suppress* a state change
  (`skip_state_update`). Manifests carry `min_engine_version` for forward-compatible updates and can be
  hot-updated (`manifest_update.rs`) without shipping a new binary.
- **Active: an injected lifecycle hook.** For "official" agents Herdr installs a small script into the
  agent's *own* hook system (`integration/assets/claude/herdr-agent-state.sh`). On session events the
  agent posts `pane.report_agent_session` back to Herdr's socket. This channel's real payload is the
  **native session id + transcript path** (for resume), not the coarse state — and it's carefully
  guarded (a `SubagentStop` event is dropped so a subagent finishing never revives an idle main pane).

The genuinely hard, transferable part is the *reliability* work visible in the CHANGELOG: lifecycle
reports are **serialized** so out-of-order events can't flip an idle pane to working (OpenCode); state
transitions wait for **settled** events (Pi); background work is **pinned** so a mid-turn redraw
doesn't fall back to idle (Grok); a prompt wait reports `agent_prompt_stalled` after 5s rather than
hanging. State inference from a TUI is a swamp, and Herdr has already mapped it.

**Inber connection**: inber's harness bridges (llm-bridge-claudecode et al.) get structured lifecycle
from stream-json, so they don't need this — *for agents they drive that way*. But inber has a **PTY
mode** path (the `user_message` dual-emit note records that PTY mode has only the OTel copy; the TUI
PTY-attach work exists) where no structured stream is available, and the bridge-server exposes an
`awaiting_user` "?" status + the herald `ask` channel that fundamentally need to answer "is this agent
blocked waiting for a human right now?" Herdr's manifest engine is a working design for deriving that
from the rendered screen when the harness won't say. This is the single most reusable idea here.

### 2. A socket API that makes agents scriptable ⭐️

The daemon exposes JSON-RPC over a Unix socket (named pipe on Windows). Methods:
`agent.start / prompt / read / send_keys / wait / get / list / rename / focus / explain`,
`pane.wait_for_output`, `pane.report_agent_session`, `events.subscribe`, `events.wait`, `layout.*`,
`notification.show`, `integration.install/uninstall`. A pane exists with or without an agent;
`agent.start` targets an *existing* shell pane and never changes layout.

The coordination primitive is **`agent.wait`** — a *server-owned* wait until a named agent reaches a
state. That's what lets "agents wait on each other": agent A can spawn agent B in a pane, prompt it,
and block until B goes `done` before continuing. Herdr ships this to agents as a Claude-style SKILL.md
gated on `HERDR_ENV=1`, so an agent inside a pane can drive the multiplexer.

**Inber connection**: this is a blackboard/coordination surface built on a socket instead of a bus.
inber has the stronger substrate (NATS JetStream, persistent + distributed) but no first-class
"block this agent until that agent reaches state X" verb. Herdr's `agent.wait` (with its
stall-timeout) is a clean model for a bus-level `wait_for_agent_state` that the team-orchestration /
kanban-blackboard work would use directly.

### 3. Live server handoff — hot-upgrade without killing agents ⭐️

`handoff_runtime.rs` transfers **server-owned session state to a replacement server binary**: it hands
the PTY **master fd**, child pid, size, keyboard-protocol flags, terminal title, and scrollback ANSI to
the new process, which imports them (`ImportedHandoffRuntime`) and keeps the agents running. It
deliberately does *not* carry transient coordination (in-flight requests, waits, subscriptions, client
sockets) — clients reconnect and retry. Plugin `[[startup]]` hooks restore plugin state after handoff.

**Inber connection**: this maps **directly** onto the "never restart the live llm-bridge gateway
unattended" landmine — a gateway restart kills every session, so the directive stops automation at the
restart step. Herdr's design is the missing mechanism: to ship a new gateway binary, pass the live
session fds/state to the replacement instead of tearing sessions down. It's harder for inber (sessions
are subprocess harnesses + SSE streams, not just PTYs), but the shape — "serialize durable
server-owned state, hand it to the successor, let clients reconnect transient state" — is exactly what
would let the gateway be upgraded without the manual-only restriction.

### 4. Native session-identity capture from foreign agents

The injected hook reports `agent_session_id` + `agent_session_path`; `agent_resume.rs` turns a
persisted `AgentSessionRef` (id *or* path — pi/omp use path, others use id) into an `AgentResumePlan`
(the argv to relaunch), with a dedupe key so a resumed agent isn't double-started. Only "official"
sources are trusted (`is_official_agent_source`).

**Inber connection**: this is what llm-bridge-claudecode already does via stream-json (session
resume/fork), but Herdr does it for agents it *didn't* spawn via an SDK — by injecting a reporter. The
id/path duality and the "never trust an unofficial source's session id" guard echo inber's
harness-session-id contract (HarnessSessionID must never equal BridgeSessionID). Worth noting as
convergent validation of that contract.

## What Inber Should Adopt

### 1. A manifest-style state classifier for PTY / non-structured harnesses (MEDIUM–HIGH)

Give the bridge layer a fallback that infers `working | idle | blocked | done` from a harness's rendered
output when there's no structured stream — a small versioned rule table per harness, matched against
screen regions. This is what fills the gap behind `awaiting_user` / the herald `ask` channel for
PTY-mode or uncooperative harnesses. Steal Herdr's *reliability* rules too: serialize reports, wait for
settled events, add a stall timeout, let overlays suppress rather than force transitions.

### 2. Live gateway handoff via fd/state passing (HIGH — retires a standing landmine)

Design an llm-bridge-server upgrade path that serializes durable per-session state and hands it to a
replacement process so sessions survive a binary swap, instead of the current "manual restart only"
rule. Herdr's `HandoffRuntimeState` (what to carry) vs "clients reconnect transient state" (what to
drop) is the template. Even a partial version — drain-and-resume harness subprocesses across an exec —
would remove the reason the restart is forbidden unattended.

### 3. A server-owned `wait_for_agent_state` coordination verb on the bus (MEDIUM)

Add an `agent.wait`-equivalent to the bus/team-orchestration layer: block a caller until a named agent
reaches a state, with a stall timeout. It's the primitive multi-agent handoff wants and inber doesn't
name today.

## What's Different

| Dimension | Herdr (herdr.dev) | Inber |
|-----------|-------------------|-------|
| **Kind** | Local terminal multiplexer (per-user, terminal-bound) | Distributed multi-agent orchestration server |
| **Agent relationship** | Wraps foreign agent CLIs it didn't write, in PTY panes | Owns agents + drives foreign harnesses via llm-bridge-* |
| **State source** | *Inferred* — terminal scrape + injected hook | *Known* natively / parsed from stream-json events |
| **Coordination** | Unix-socket JSON-RPC, server-owned `agent.wait` | NATS JetStream bus (persistent, distributed) |
| **Session survival** | Detach/reattach, SSH, **live server handoff** (fd passing) | SSE + subprocess; gateway restart kills sessions (manual-only) |
| **Isolation** | git worktree per workspace | forge worktree slots per session |
| **Extensibility** | Plugins (marketplace) + injected integrations | tool-store / skill-store / MCP |
| **Footprint** | One Rust binary, no runtime deps | Ecosystem of Go services |
| **Transparency** | Full source, Apache-2.0 | Internal, full source |

## Key Takeaway

Herdr is the wrong *shape* to copy — inber is a server, Herdr is a terminal tool — but it's the right
place to steal three mechanisms, all born from the fact that **Herdr manages agents it doesn't
control**, which is exactly inber's harness-bridge situation. In priority order: (1) **live server
handoff** (fd/state passing to a successor process) directly attacks the "can't restart the gateway
unattended" landmine; (2) a **manifest-style state classifier** gives the bridge a way to know
`blocked/idle/working` for PTY-mode harnesses that don't emit structured events; (3) a server-owned
**`agent.wait`** is the missing coordination verb for multi-agent handoff. The reliability engineering
around state inference (serialize, settle, stall-timeout, overlay-suppress) is as valuable as the idea.
Ignore herdr.org.
