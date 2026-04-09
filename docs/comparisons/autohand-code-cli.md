# Autohand Code CLI Comparison

**Project**: [Autohand Code CLI](https://github.com/autohandai/code-cli)
**Language**: TypeScript (Bun)
**Focus**: Autonomous coding agent CLI with ReAct pattern, multi-provider support, and modular skills
**Key Strengths**: 40+ built-in tools, auto-mode with cost/time caps, git worktree isolation, team mode, skill marketplace

## Architecture Overview

Autohand is a terminal-first coding agent using the ReAct (Reason + Act) loop: the agent observes, reasons about what to do, selects a tool, executes it, and repeats. Built on Bun for fast TypeScript execution.

```
User input → ReAct loop (observe → reason → act → observe)
    → 40+ tools (file ops, git, shell, memory, planning)
    → Multi-provider LLM (OpenRouter, Anthropic, OpenAI, Ollama, llama.cpp, MLX)
    → Session persistence (~/.autohand/sessions/)
```

Supports interactive REPL, single-shot command mode (`-p`), RPC mode, and ACP (Agent Communication Protocol) over stdio for editor integration. VS Code and Zed extensions connect back to the CLI.

## What Autohand Does Well

### 1. Auto-Mode with Safety Caps ⭐️

Fully autonomous operation with configurable limits: max iterations (default 50), cost cap (default $10), runtime cap (default 120 min), and periodic git checkpoints. The agent runs until it declares completion or hits a limit. This is well-designed for "fire and forget" tasks.

**Inber connection**: Inber has `MaxTurns` and `MaxInputTokens` limits but no cost cap or runtime cap. Adding cost-based limits would be valuable for spawned sub-agents.

### 2. Skill System with Auto-Generation

Skills are composable instruction sets that augment the agent's capabilities. The `--auto-skill` flag analyzes a project and generates tailored skills (e.g., "nextjs-component-creator"). Skills can be shared via a community marketplace with install/search/trending.

**Inber connection**: Inber's agent identities are defined in agent-store with static system prompts. Auto-generating project-specific context (similar to auto-skill) would improve first-turn quality.

### 3. Team Mode via Git Worktrees

The `/team`, `/tasks`, `/message` commands enable parallel agent sessions displayed in tmux panes, each in an isolated git worktree. Lightweight multi-agent without containers or message buses.

**Inber connection**: Validates the git worktree approach inber uses via forge. The tmux visualization is a nice UX pattern for the TUI.

### 4. Dry Run and Patch Generation

`--dry-run` previews changes without applying. `--patch` generates a git patch file. Both are useful for CI/CD pipelines and code review workflows.

## What Inber Should Adopt

### 1. Cost-Based Safety Caps (HIGH PRIORITY)

Add a `MaxCost` field to EngineConfig alongside MaxTurns and MaxInputTokens. Track cumulative cost per RunTurn and abort when exceeded. Essential for autonomous sub-agents.

### 2. Auto-Skill / Project Analysis (MEDIUM PRIORITY)

On first session in a workspace, analyze the project structure and generate context-specific instructions. Similar to how Autohand's `--auto-skill` creates tailored skills. This could populate the initial memory store with project-relevant context.

### 3. Session Checkpointing with Git (LOW PRIORITY)

Autohand checkpoints to git every N iterations during auto-mode. Inber could adopt this for long-running agent sessions — auto-commit at checkpoint intervals provides easy rollback.

## What's Different

| Aspect | Autohand | Inber |
|--------|----------|-------|
| **Pattern** | ReAct (single agent loop) | Multi-agent orchestration |
| **Runtime** | Bun (TypeScript) | Go server |
| **Tools** | 40+ hardcoded | agentkit library + MCP |
| **Multi-agent** | Team mode (parallel sessions in tmux) | Named agents with NATS bus |
| **Providers** | OpenRouter, Anthropic, OpenAI, Ollama, MLX | Anthropic-focused via model-store |
| **Persistence** | File-based sessions | SQLite via agent-store |
| **Safety** | Cost/time/iteration caps | Turn/token caps |
| **Deployment** | Per-user CLI | Shared server |

## Key Takeaway

Autohand is a well-executed single-agent CLI with strong autonomous operation features. The cost caps, auto-skill generation, and dry-run mode are the most transferable ideas. For inber, the main gap it highlights is **cost-based safety limits** for autonomous sub-agents — turns and tokens aren't sufficient when different models have wildly different per-token costs.
