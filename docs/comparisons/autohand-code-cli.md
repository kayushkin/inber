# Autohand Code CLI Comparison

**Last reviewed**: 2026-10-29 against autohandai/code-cli@79a26ef (2026-10-28). Still under active development.

**Project**: [Autohand Code CLI](https://github.com/autohandai/code-cli)
**Language**: TypeScript (Bun)
**Focus**: Autonomous coding agent CLI with ReAct pattern, multi-provider support, and modular skills
**Key Strengths**: 40+ built-in tools, auto-mode with cost/time caps, git worktree isolation, team mode, subagents, read-only plan mode, community skill registry, importers for other agents' settings

## Architecture Overview

Autohand is a terminal-first coding agent using the ReAct (Reason + Act) loop: the agent observes, reasons about what to do, selects a tool, executes it, and repeats. Built on Bun for fast TypeScript execution.

```
User input → ReAct loop (observe → reason → act → observe)
    → 40+ tools (file ops, git, shell, memory, planning)
    → Multi-provider LLM (about 17 providers in src/providers/, incl. Anthropic, OpenAI, Bedrock, Vertex, Ollama, llama.cpp, MLX)
    → Session persistence (~/.autohand/sessions/)
```

Supports interactive REPL, single-shot command mode (`-p`), RPC mode, and ACP (Agent Client Protocol) over stdio for editor integration. VS Code, Zed and JetBrains connect back to the CLI.

Added since the first version of this doc:
- **Sign-in to account plans**: ChatGPT/Codex sign-in by browser or device code (`src/providers/openaiAuth.ts`) and xAI sign-in (`xaiAuth.ts`), alongside API keys.
- **Subagents**: `delegate_task` and `delegate_parallel` hand work to focused agents with their own context (`src/core/toolManager.ts`); 15 built-in agent definitions ship in `src/agents/builtin/`.
- **MCP client** (`src/mcp/McpClientManager.ts`) and **hooks** (`src/core/HookManager.ts`).
- **Context compaction**, on by default (`--cc`; `src/core/context/`).
- **Memory**: `save_memory` / `recall_memory` tools over an event log and summary tree (`src/memory/`).
- **Read-only plan mode** (`--plan`) that blocks writing tools until the user accepts the plan.
- **Importers** that read settings, permissions and hooks from Claude Code, Codex, Cursor, Gemini, Cline and others (`src/import/importers/`).
- **Extensions**: declarative packages of tools, agents and skills, installable without changing CLI source.

## What Autohand Does Well

### 1. Auto-Mode with Safety Caps ⭐️

Fully autonomous operation with configurable limits: max iterations (default 50), cost cap (default $10), runtime cap (default 120 min), and periodic git checkpoints. The agent runs until it declares completion or hits a limit. This is well-designed for "fire and forget" tasks.

**Inber connection**: Inber now has all of these limits except git checkpoints: `MaxTurns`, `MaxInputTokens`, `MaxCost` (dollars) and `MaxDuration` (seconds) in `engine/engine_types.go`, enforced by `guard/guard.go`. *(Updated 2026-10-29: this paragraph said inber had no cost or runtime cap.)*

### 2. Skill System with Auto-Generation

Skills are composable instruction sets that augment the agent's capabilities. The `--auto-skill` flag analyzes a project and generates tailored skills (e.g., "nextjs-component-creator"). Skills can be shared through a community registry (`--skill-install`, `/skills install`).

**Inber connection**: Inber's agent identities are defined in agent-store with static system prompts. Auto-generating project-specific context (similar to auto-skill) would improve first-turn quality.

### 3. Team Mode via Git Worktrees

The `/team`, `/tasks`, `/message` commands enable parallel agent sessions displayed in tmux panes, each in an isolated git worktree. Lightweight multi-agent without containers or message buses.

**Inber connection**: Validates the git worktree approach inber uses through the forge library (`server/workspace_roots.go`). The tmux view is a nice pattern for a terminal UI.

### 4. Dry Run and Patch Generation

`--dry-run` previews changes without applying. `--patch` generates a git patch file. Both are useful for CI/CD pipelines and code review workflows.

## What Inber Should Adopt

### 1. Cost-Based Safety Caps (DONE)

Add a `MaxCost` field to EngineConfig alongside MaxTurns and MaxInputTokens. Track cumulative cost per RunTurn and abort when exceeded. Essential for autonomous sub-agents.

*(2026-10-29: done. `LimitConfig` has `MaxCost` and `MaxDuration` (`engine/engine_types.go`), and `max_cost_wiring_test.go` / `max_duration_wiring_test.go` in `engine/` check the wiring.)*

### 2. Auto-Skill / Project Analysis (MEDIUM PRIORITY)

On first session in a workspace, analyze the project structure and generate context-specific instructions. Similar to how Autohand's `--auto-skill` creates tailored skills. This could populate the initial memory store with project-relevant context.

### 3. Session Checkpointing with Git (LOW PRIORITY)

Autohand checkpoints to git every N iterations during auto-mode. Inber could adopt this for long-running agent sessions — auto-commit at checkpoint intervals provides easy rollback.

## What's Different

| Aspect | Autohand | Inber |
|--------|----------|-------|
| **Pattern** | ReAct loop, plus subagents and team mode | Multi-agent orchestration |
| **Runtime** | Bun (TypeScript) | Go server |
| **Tools** | 40+ built in, plus MCP and extensions | tool-store tools (`agent/registry/tools.go`) + MCP (`tools/mcp/`) |
| **Multi-agent** | Team mode (parallel sessions in tmux), `delegate_task` subagents | Named agents with NATS bus, spawn tool capped by `MaxSpawnDepth` |
| **Providers** | About 17, incl. ChatGPT and xAI sign-in | Anthropic SDK first; OpenAI-compatible path that can use the Codex ChatGPT plan (`agent/codex_subscription.go`) |
| **Persistence** | File-based sessions | SQLite (agent-store for agents, memory-store for memory) |
| **Safety** | Cost/time/iteration caps, permission modes, plan mode | Turn/token/cost/duration caps; observe/assist/autonomous modes (`guard/guard.go`) |
| **Deployment** | Per-user CLI | Shared server |

## Key Takeaway

Autohand is a well-executed single-agent CLI with strong autonomous operation features. The cost caps, auto-skill generation, and dry-run mode are the most transferable ideas. For inber, the main gap it highlighted was **cost-based safety limits** for autonomous sub-agents — turns and tokens aren't enough when models differ widely in price per token. Inber has since added cost and wall-clock caps; what is left to borrow is auto-skill style project analysis, git checkpoints, and a plan mode that waits for the user to accept.
