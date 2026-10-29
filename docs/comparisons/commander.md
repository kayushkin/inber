# Commander Comparison

**Last reviewed**: 2026-10-29 against autohandai/commander@6b09422 (2026-06-09). Not archived, but quiet: the last commit is a dependency bump, and before that the last feature work was in March 2026. The executors, `ToolKind` and Chat/Collab/Full modes described below are all still in the code (`src-tauri/src/services/executors/`, `src-tauri/src/models/protocol.rs`).

**Project**: [Commander](https://github.com/autohandai/commander)
**Language**: Rust (Tauri v2) + React/TypeScript
**Focus**: Desktop app for orchestrating multiple CLI coding agents (Claude Code, Codex, Gemini) against Git projects
**Key Strengths**: Multi-agent UI, git worktree isolation, unified protocol layer across different agent CLIs, commit graph visualization

## Architecture Overview

Commander is a native desktop app (macOS/Windows) that wraps external CLI coding agents via process spawning. It doesn't implement its own agent loop — it orchestrates existing CLIs (Claude Code, Codex, Gemini, custom agents) through a unified interface.

```
Desktop UI (React + Tauri)
    → ExecutorFactory
        → PtyExecutor (terminal spawning for Claude/Codex/Gemini)
        → RpcExecutor (JSON-RPC for structured agents)
        → AcpExecutor (Agent Communication Protocol for ndJSON agents)
    → Git worktrees for workspace isolation
    → ProtocolEvent (unified event stream to UI)
```

Three executor backends handle different agent communication protocols. All emit a common `ProtocolEvent` type to the frontend, so the UI doesn't care which agent is running. Tool events are classified by kind (Read, Write, Execute, Think, etc.) for consistent display.

## What Commander Does Well

### 1. Unified Multi-Agent Protocol Layer ⭐️

Commander supports three different communication protocols and abstracts them behind a common event interface:
- **PTY** (pseudo-terminal) for CLI agents that output plain text streams
- **JSON-RPC 2.0** over stdin/stdout for structured request/response
- **ACP** (Agent Communication Protocol) using ndJSON for typed message envelopes

The `ExecutorFactory` detects the protocol via a cache and selects the right backend. All backends emit `ProtocolEvent` with classified tool events (`ToolKind::Read`, `Write`, `Edit`, `Execute`, `Think`, etc.).

**Inber connection**: Inber calls models itself: the Anthropic SDK for Claude, and an OpenAI-compatible client (`agent/openai.go`) that can also go through the Codex CLI's ChatGPT plan (`agent/codex_subscription.go`). Commander's multi-protocol approach is relevant if inber ever needs to orchestrate external agent CLIs rather than running agents in-process.

### 2. Git Worktree Isolation for Agents

Each agent task can get its own git worktree under `.commander/<workspace-name>/`. The agent operates on an isolated branch without touching the main working tree. The History view renders a commit DAG with lane assignment and supports diffing workspaces against main.

**Inber connection**: Inber uses the forge library for the same thing — a git worktree workspace per spawned session (`server/workspace_roots.go`, `server/session_creation.go`). Commander's implementation validates this pattern. Their commit DAG view is something an inber dashboard could adopt.

### 3. Execution Mode Tiers

Three modes with clear security boundaries:
- **Chat** — read-only, no filesystem writes
- **Collab** — asks for approval before writes
- **Full** — auto-execute, maps to `--full-auto` or bypass-approvals flags per agent

This maps cleanly to different trust levels for different tasks.

**Inber connection**: Inber now has the same three tiers: `observe`, `assist` and `autonomous` (`guard/guard.go`), chosen per session (`Mode` in `engine/engine_types.go`, parsed in `server/session_creation.go`) and enforced on every tool call by `buildToolRefusal` (`engine/build_hooks.go`). The gap is assist mode: it has no approver to ask, so a call that needs approval is refused rather than held. *(Updated 2026-10-29: this said inber had no execution modes.)*

### 4. Agent Status and Tool Event Display

Real-time agent availability display with classified tool event badges. The UI shows what tools agents are using (Read, Write, Execute), plan breakdowns, and session status. All agents share the same event taxonomy despite using different protocols underneath.

### 5. IDE/Editor Integration

Commander detects installed editors (VS Code, Cursor, Zed, Sublime, Xcode, JetBrains, etc.) and can open projects in them directly. Combined with the commit graph and diff viewer, it provides a complete developer workflow without leaving the app.

## What Inber Should Adopt

### 1. Commit Graph Visualization (MEDIUM PRIORITY)

Commander's History view with commit DAG, lane assignment, and workspace-vs-main diffing is something an inber dashboard could benefit from (inber-party, which this first named, has since been retired). When multiple agents work on worktrees, visualizing their branches and changes in the dashboard would help the user understand what's happening.

### 2. Execution Mode System (MEDIUM PRIORITY)

Inber's agents currently have a flat tool allowlist. A tiered execution mode system would add safety for different contexts:

- **Observe** — read-only tools only (file reads, search, memory)
- **Assist** — writes require confirmation via bus/UI
- **Autonomous** — full tool access (current behavior)

This could be per-session, not per-agent — the same agent could run in different modes depending on trust level.

*(2026-10-29: mostly done, per-session, as described above. What remains is an approval path for assist mode: an event that llm-bridge or a chat can answer, so a held call can go ahead.)*

### 3. Classified Tool Events (LOW PRIORITY)

Commander classifies tool calls by kind (Read, Write, Execute, Think, etc.) for consistent UI display regardless of the underlying agent. Inber's `StreamEvent` has tool name and text but no classification. Adding a `ToolKind` field would help a UI render tool events more meaningfully. (Still true on 2026-10-29: `StreamEvent` in `server/server.go` carries `Tool` and `ToolID`, no kind.)

## What's Different

| Aspect | Commander | Inber |
|--------|-----------|-------|
| **Architecture** | Desktop GUI wrapping external CLIs | Server-side framework, own agent loop |
| **Agent execution** | Process spawning (PTY/RPC/ACP) | In-process; Anthropic SDK or OpenAI-compatible client |
| **Multi-agent** | Multiple CLI tools (Claude, Codex, Gemini) | Multiple named agents, plus spawned subagents |
| **Isolation** | Git worktrees under `.commander/`; Chat/Collab/Full modes | forge-library worktrees; observe/assist/autonomous modes |
| **Persistence** | Local JSON files, tauri-plugin-store | SQLite via agent-store/model-store |
| **Communication** | Tauri events (frontend ↔ backend) | NATS JetStream pub/sub |
| **Target** | Individual developer desktop workflow | Multi-agent server, distributed |
| **UI** | Built-in React desktop app | None of its own; llm-bridge-compatible routes (`server/api_bridge.go`) for the llm-bridge chat UI |
| **Model management** | Per-agent settings in UI | Centralized model-store service |

## Key Takeaway

Commander is a well-executed UI layer for orchestrating existing coding agents. It validates the git worktree isolation pattern that inber already uses via forge. The most relevant insights for inber were: **execution mode tiers** (observe/assist/autonomous), which inber has since built apart from an approval path; **classified tool events** for better UI display; and **commit DAG visualization** for multi-agent workspace management.
