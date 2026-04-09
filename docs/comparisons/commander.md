# Commander Comparison

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

**Inber connection**: Inber uses the Anthropic SDK directly for its agents, and OpenAI-compatible APIs via `agent/openai.go`. Commander's multi-protocol approach is relevant if inber ever needs to orchestrate external agent CLIs rather than running agents in-process.

### 2. Git Worktree Isolation for Agents

Each agent task can get its own git worktree under `.commander/<workspace-name>/`. The agent operates on an isolated branch without touching the main working tree. The History view renders a commit DAG with lane assignment and supports diffing workspaces against main.

**Inber connection**: Inber's forge library does the same thing — manages git worktree slots for agent sessions. Commander's implementation validates this pattern. Their commit DAG visualization in the UI is something inber-party could adopt.

### 3. Execution Mode Tiers

Three modes with clear security boundaries:
- **Chat** — read-only, no filesystem writes
- **Collab** — asks for approval before writes
- **Full** — auto-execute, maps to `--full-auto` or bypass-approvals flags per agent

This maps cleanly to different trust levels for different tasks.

**Inber connection**: Inber doesn't have explicit execution modes — agents always have full tool access based on their allowlist. A mode system could be useful for spawned sub-agents that should be more restricted.

### 4. Agent Status and Tool Event Display

Real-time agent availability display with classified tool event badges. The UI shows what tools agents are using (Read, Write, Execute), plan breakdowns, and session status. All agents share the same event taxonomy despite using different protocols underneath.

### 5. IDE/Editor Integration

Commander detects installed editors (VS Code, Cursor, Zed, Sublime, Xcode, JetBrains, etc.) and can open projects in them directly. Combined with the commit graph and diff viewer, it provides a complete developer workflow without leaving the app.

## What Inber Should Adopt

### 1. Commit Graph Visualization (MEDIUM PRIORITY)

Commander's History view with commit DAG, lane assignment, and workspace-vs-main diffing is something inber-party could benefit from. When multiple agents work on worktrees, visualizing their branches and changes in the dashboard would help the user understand what's happening.

### 2. Execution Mode System (MEDIUM PRIORITY)

Inber's agents currently have a flat tool allowlist. A tiered execution mode system would add safety for different contexts:

- **Observe** — read-only tools only (file reads, search, memory)
- **Assist** — writes require confirmation via bus/UI
- **Autonomous** — full tool access (current behavior)

This could be per-session, not per-agent — the same agent could run in different modes depending on trust level.

### 3. Classified Tool Events (LOW PRIORITY)

Commander classifies tool calls by kind (Read, Write, Execute, Think, etc.) for consistent UI display regardless of the underlying agent. Inber's `StreamEvent` has tool name and text but no classification. Adding a `ToolKind` field would help inber-party render tool events more meaningfully.

## What's Different

| Aspect | Commander | Inber |
|--------|-----------|-------|
| **Architecture** | Desktop GUI wrapping external CLIs | Server-side framework, own agent loop |
| **Agent execution** | Process spawning (PTY/RPC/ACP) | In-process via Anthropic SDK |
| **Multi-agent** | Multiple CLI tools (Claude, Codex, Gemini) | Multiple named agents (same SDK) |
| **Isolation** | Git worktrees under `.commander/` | Forge library for worktree slots |
| **Persistence** | Local JSON files, tauri-plugin-store | SQLite via agent-store/model-store |
| **Communication** | Tauri events (frontend ↔ backend) | NATS JetStream pub/sub |
| **Target** | Individual developer desktop workflow | Multi-agent server, distributed |
| **UI** | Built-in React desktop app | Separate dashboard (inber-party) |
| **Model management** | Per-agent settings in UI | Centralized model-store service |

## Key Takeaway

Commander is a well-executed UI layer for orchestrating existing coding agents. It validates the git worktree isolation pattern that inber already uses via forge. The most relevant insights for inber are: **execution mode tiers** (observe/assist/autonomous) as a safety mechanism for different contexts, **classified tool events** for better dashboard display, and **commit DAG visualization** for multi-agent workspace management in inber-party.
