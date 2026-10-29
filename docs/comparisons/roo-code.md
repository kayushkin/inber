# Roo Code Comparison

**Last reviewed**: 2026-10-29 against RooCodeInc/Roo-Code@b867ec914 (2026-05-15).

**Project**: [Roo Code](https://github.com/RooCodeInc/Roo-Code)
**Status**: **Shut down.** The README says the extension was shut down on 2026-05-15, the repo is archived on GitHub, and the last release is v3.54.0. The README points users to [Zoo Code](https://github.com/Zoo-Code-Org/Zoo-Code), a community continuation that is active, and back to Cline. This file now records the ideas, not a live project; there is nothing upstream to watch.
**Language**: TypeScript (VS Code extension, a fork of Cline)
**Focus**: One coding agent whose behaviour changes by mode rather than by switching agents.
**Key Strengths**: Mode system (Code/Architect/Ask/Debug/custom), codebase semantic indexing, skills as slash commands, auto-approval policies, tool repetition detection.

## Architecture Overview

Roo Code was a VS Code extension with a single agent that switched between modes. Each mode set which tool groups were available and which system prompt parts were used. The agent could change mode mid-conversation through `SwitchModeTool`.

```
VS Code Sidebar → Task loop → Mode-configured tools + prompts
    → Code mode:      full file ops + shell + MCP
    → Architect mode:  read-only + planning tools
    → Ask mode:        conversation only, no tools
    → Debug mode:      read + execute + diagnostics
    → Custom modes:    user-defined tool groups + prompts
```

At shutdown `src/core/tools/` held about 25 tools, among them `CodebaseSearchTool` (embedding search), `NewTaskTool` (subtasks), `SkillTool`, `RunSlashCommandTool`, `UseMcpToolTool`, `SwitchModeTool` and `ToolRepetitionDetector`.

## What Roo Code Did Well

### 1. Mode System for Agent Specialization ⭐️

Instead of separate agents, Roo Code used modes: a `ModeConfig` with a `slug`, tool `groups` and prompts. Five built-in modes plus user-defined ones. One agent, several behaviours, switchable mid-conversation.

**Inber connection**: inber uses separate named agents from agent-store, each with its own tool allowlist. It also has per-session execution modes in `guard/guard.go` (`observe`, `assist`, `autonomous`), set per request, which restrict tools without changing the agent. Those modes control trust, not prompt or role, so they cover half of Roo's idea.

### 2. Auto-Approval Policies

Per-tool auto-approve settings: reads could run unasked while writes and commands asked first.

**Inber connection**: inber's `assist` mode sends dangerous calls to approval, but no approver exists, so they are refused (`engine/build_hooks.go`, `buildToolRefusal`). There are no per-tool approval levels.

### 3. Skills as Slash Commands

Reusable prompt workflows saved as files and exposed as `/command`s, which the agent could call through `RunSlashCommandTool`.

### 4. Tool Repetition Detection

`ToolRepetitionDetector` stopped loops where the agent called the same tool with the same arguments again and again.

**Inber connection**: `guard` has a repetition counter (`RecordToolCall`, `IsRepeating`), but nothing outside the package's tests calls `RecordToolCall`, so it never fires. The package doc in `guard/guard.go` says so.

## What Inber Should Adopt

### 1. Wire the Repetition Check (HIGH PRIORITY)

The code exists; call `Guard.RecordToolCall` from the tool-execution hook and act on `IsRepeating` (warn the model, then stop the turn). Roo's detector is the model for what "same call" means.

### 2. Approval Tiers (LOW PRIORITY)

Per-tool levels in agent config: run, ask (through the bus or llm-bridge), deny. This needs the approver that `assist` mode lacks.

### Done: Agent Modes

This file once proposed `observe` / `assist` / `autonome` modes set per request. inber now has them as `observe`, `assist` and `autonomous` (`guard/mode.go`, `ParseMode`), checked on every tool call (`engine/build_hooks.go`).

## What's Different

| Aspect | Roo Code | Inber |
|--------|----------|-------|
| **Agent model** | Single agent with mode switching | Several named agents from agent-store |
| **Runtime** | VS Code extension (shut down) | Go server |
| **Specialization** | Modes (tool groups + prompts) | Separate agent identities; trust modes per session |
| **Context** | Embedding codebase search + Cline's context tools | Memory store + token budget |
| **Multi-agent** | Subtasks through `NewTaskTool` | Spawned sub-agents (depth 2) over a NATS bus |
| **Tools** | About 25 built in + MCP | tool-store tools; MCP client present but not wired (`tools/mcp/`) |
| **Loop guard** | `ToolRepetitionDetector` | Counter exists, never called |

## Key Takeaway

Roo Code's mode system remains the most useful idea here, and inber has built the trust half of it. The cheapest remaining win is wiring the repetition check that already sits unused in `guard/`.
