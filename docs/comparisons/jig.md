# Jig Comparison

**Project**: [Jig](https://github.com/jdforsythe/jig)
**Language**: Go
**Focus**: Profile manager and launcher for Claude Code sessions — assembles configuration and invokes `claude` with the right flags
**Key Strengths**: YAML profile system with inheritance, TUI profile manager, per-project profiles, granular plugin component selection

## Architecture Overview

Jig is a Go CLI that manages and launches Claude Code sessions with pre-configured profiles. It does NOT run agents — it assembles the right environment and hands off to `claude`. Think of it as a "dotfiles manager" for Claude Code sessions.

```
jig run <profile>
    → Load YAML profile (project-local or global)
    → Resolve inheritance chain (extends: base-profile)
    → Scan filesystem for available skills, agents, MCP servers
    → Generate temp plugin directory with symlinks
    → Launch: claude --plugin-dir /tmp/jig-xyz [flags]
    → Clean up temp directory on exit
```

Built with cobra (CLI), bubbletea + lipgloss (TUI), and levenshtein (fuzzy matching). Standard Go toolchain, no heavy dependencies.

## What Jig Does Well

### 1. Profile Inheritance System ⭐️

Profiles use `extends: base-profile` with child settings taking precedence. This enables layered configuration: a base profile with common settings (model, permissions, MCP servers), and specialized profiles that override specific fields.

```yaml
# base.yaml
model: claude-opus-4-6
permissions: default
mcp_servers: [memory, search]

# coding.yaml
extends: base
skills: [code-review, testing]
allowed_tools: [shell, file_write]
```

**Inber connection**: Inber's agents in agent-store have flat configs (no inheritance). Profile inheritance would be useful for agent variants — a base agent with common settings and specialized versions that override specific tools or prompts.

### 2. TUI Profile Manager

Full terminal UI with vim-style keybindings: profile list with launch/create/edit/delete/preview, tabbed editor for profile fields, preview showing the fully resolved YAML. Well-executed developer UX.

### 3. Per-Project Profiles

`jig init` creates `.jig/profiles/` in the current directory. Project-local profiles take precedence over global ones and can be checked into version control.

**Inber connection**: Agent configs in agent-store are global. Per-project agent overrides (workspace-specific tool allowlists, prompts, or limits) would be useful.

### 4. Granular Plugin Component Selection

Can enable an entire Claude Code plugin or cherry-pick specific skills and agents from it. This avoids the all-or-nothing plugin model.

### 5. Dry Run and Doctor

`jig run --dry-run` shows the generated config and command without launching. `jig doctor` validates installation, directories, profiles, and MCP availability. Good operational hygiene.

## What Inber Should Adopt

### 1. Agent Config Inheritance (MEDIUM PRIORITY)

Add an `extends` field to agent definitions in agent-store. A child agent inherits all settings from the parent and overrides specific fields. Useful for creating agent variants:

- `fionn` (base builder) → `fionn-frontend` (adds React tools) → `fionn-frontend-testing` (adds test runner)

This is simpler than duplicating entire agent configs for minor variations.

### 2. Per-Project Agent Overrides (MEDIUM PRIORITY)

Allow a `.inber/agents.yaml` in a project directory to override agent settings when working in that workspace. For example, a Go project could restrict shell commands to `go` and `git`, while a Node project allows `npm` and `bun`.

### 3. Config Validation / Doctor Command (LOW PRIORITY)

A `GET /api/doctor` endpoint or equivalent that validates:
- All agent configs are valid
- Referenced workspaces exist
- Model-store has valid credentials
- NATS is connected
- Agent-store is accessible

The healthcheck service partially covers this, but a more comprehensive validation would help debugging.

## What's Different

| Aspect | Jig | Inber |
|--------|-----|-------|
| **Purpose** | Pre-session config + launch | Full agent lifecycle (run, communicate, remember) |
| **Execution** | Launches `claude` subprocess, exits | Long-running server with concurrent agents |
| **Config** | YAML profiles with inheritance | SQLite agent-store |
| **Scope** | Claude Code session configuration | Multi-agent orchestration |
| **Communication** | None (single process) | NATS JetStream pub/sub |
| **Persistence** | YAML files in filesystem | SQLite (agent-store, model-store, session DB) |
| **TUI** | Profile manager (bubbletea) | Interactive chat mode |
| **Language** | Go (shared!) | Go |

## Key Takeaway

Jig is a well-scoped tool that solves one problem well: composing Claude Code sessions from reusable configuration components. For inber, the transferable ideas are **config inheritance** (agent variants without duplication) and **per-project overrides** (workspace-specific agent settings). Jig operates at the pre-session layer — it's complementary to inber's runtime, not a competitor. The fact that it's also Go makes it a natural reference for CLI patterns and TUI design.
