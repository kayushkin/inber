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

The permission system would have the highest impact on daily usability while being relatively straightforward to implement given inber's existing tool architecture.