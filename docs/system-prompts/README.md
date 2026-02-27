# System Prompts Documentation

This directory contains comprehensive documentation about the **inber framework** that should be included in agent system prompts or loaded as context.

## Purpose

These documents provide agents with:
- Background knowledge about inber's architecture
- Tool usage instructions and best practices
- Understanding of how context and memory systems work
- Session management and continuity patterns
- Workspace organization

## Files

### [overview.md](overview.md)
**What is inber?**
- Core architecture and design philosophy
- Key features and subsystems
- Project structure and components
- Getting started as an agent

**Use in:** All agent system prompts (baseline knowledge)

---

### [tools.md](tools.md)
**Available Tools**
- Built-in tools: file operations (read/write/edit/list), shell
- Memory tools: search/save/expand/forget
- Tool usage patterns and best practices
- When to use each tool

**Use in:** Agent system prompts to explain tool capabilities

---

### [context-system.md](context-system.md)
**Tag-Based Context**
- How tagged chunks work
- Token budgets and prioritization
- Context builder API
- Auto-loading at session start
- Chunking strategies

**Use in:** Advanced agents that need to understand how context is assembled

---

### [memory-system.md](memory-system.md)
**Persistent Memory**
- Memory structure and lifecycle
- Semantic search with TF-IDF
- Importance scoring and decay
- When and how to save memories
- Best practices for memory hygiene

**Use in:** All agents that have memory tools

---

### [workspace.md](workspace.md)
**Workspace Files**
- `.inber/workspace/{agent}/` structure
- Editable system prompts (overrides)
- Agent-specific state files
- Workspace vs memory (when to use each)

**Use in:** Agents that maintain state files (task-manager, orchestrator)

---

### [sessions.md](sessions.md)
**Session Management**
- Default (persistent) sessions
- New sessions (--new flag)
- Detached sessions (--detach)
- JSONL logging format
- Session continuity guarantees

**Use in:** Agents that need to understand conversation continuity

---

## Usage Patterns

### For Agent System Prompts

**Minimal (researcher):**
```markdown
# Research Agent

[Role description]

## About Inber

You are part of inber, a Go-based agent orchestration framework with:
- Tag-based context system
- Persistent memory (SQLite + search)
- Session continuity across runs

See: docs/system-prompts/overview.md for details.

## Your Tools

[Tool descriptions - refer to tools.md for details]
```

**Full (task-manager, coder):**
```markdown
# Task Manager

[Role description]

## Framework Knowledge

You should understand:
- What inber is and how it works (overview.md)
- Available tools and when to use them (tools.md)
- How context is assembled (context-system.md)
- How memory works and when to save (memory-system.md)
- How sessions persist across runs (sessions.md)
- Your workspace at .inber/workspace/task-manager/ (workspace.md)

[Rest of agent identity]
```

### For Dynamic Loading

Agents can also **load these docs as context** dynamically:

```json
{
  "path": "docs/system-prompts/memory-system.md"
}
```

This allows:
- Just-in-time learning about subsystems
- Reduced system prompt size (load on-demand)
- Always up-to-date documentation

---

## Maintenance

### When to Update

- **overview.md**: Architecture changes, new subsystems
- **tools.md**: New tools added, tool behavior changes
- **context-system.md**: Context builder changes, new tagging patterns
- **memory-system.md**: Memory ranking changes, new memory features
- **workspace.md**: Workspace structure changes
- **sessions.md**: Session management changes, new flags

### Style Guide

- **Be concise but complete** - agents need facts, not fluff
- **Use examples** - show don't just tell
- **Provide patterns** - "when to use X" not just "X exists"
- **Keep up-to-date** - docs that lie are worse than no docs
- **Cross-reference** - link related concepts

---

## Auto-Loading

These docs can be auto-loaded into agent context at session start:

```go
// Load system prompt docs for task-manager
store.AddFile("docs/system-prompts/overview.md", "framework", "docs")
store.AddFile("docs/system-prompts/tools.md", "framework", "docs", "tools")
store.AddFile("docs/system-prompts/memory-system.md", "framework", "docs", "memory")
```

With tags like `["framework", "docs"]`, agents can request this context when needed.

---

## Future Enhancements

- **Interactive tutorials** - Step-by-step guides for complex workflows
- **Troubleshooting guides** - Common issues and solutions
- **Performance tips** - How to optimize token usage, memory, context
- **Integration examples** - Real-world agent configuration patterns
