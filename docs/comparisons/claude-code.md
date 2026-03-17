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

The permission system would have the highest impact on daily usability while being relatively straightforward to implement given inber's existing tool architecture.