# Inber Overview

## What is Inber?

**Inber** is a Go-based agent orchestration framework designed for building intelligent, context-aware AI agents with persistent memory and flexible tool access.

### Core Architecture

Inber is built around several key subsystems that work together:

```
┌─────────────────────────────────────────────┐
│           Agent Orchestration               │
│  ┌─────────────────────────────────────┐   │
│  │   Registry (Multi-Agent Manager)    │   │
│  └─────────────────────────────────────┘   │
└─────────────────────────────────────────────┘
           │              │
           ▼              ▼
┌──────────────┐   ┌──────────────┐
│   Context    │   │   Memory     │
│   System     │   │   System     │
│  (Tagged     │   │  (SQLite +   │
│   Chunks)    │   │   Search)    │
└──────────────┘   └──────────────┘
           │              │
           └──────┬───────┘
                  ▼
           ┌──────────────┐
           │    Tools     │
           │ (Shell, File,│
           │   Memory)    │
           └──────────────┘
```

### Key Features

1. **Tag-Based Context System**
   - All content (files, messages, memories) becomes tagged chunks
   - Chunks compete for limited context space based on relevance
   - Token-aware budgeting prevents context overflow

2. **Persistent Memory**
   - SQLite-backed storage survives across sessions
   - Semantic search using TF-IDF embeddings (swappable)
   - Importance scoring, decay, and compaction
   - Agent-facing tools: search, save, expand, forget

3. **Multi-Agent Registry**
   - Define specialized agents with custom tool access
   - Isolated sessions and contexts per agent
   - Markdown identity + JSON configuration
   - Dynamic agent creation and composition

4. **Flexible Tool System**
   - Built-in: shell, file operations (read/write/edit/list)
   - Memory tools: search, save, expand, forget
   - Tool scoping per agent (researcher gets read-only)
   - Extensible: add custom tools easily

5. **Session Management**
   - JSONL logging with full request/response payloads
   - Persistent default session (resumes where you left off)
   - `--new` flag for fresh sessions
   - `--detach` for one-off commands without affecting default

6. **Lifecycle Hooks**
   - 8 event types: session_start, turn_start, tool_use, etc.
   - Gatekeeper mode for validation/approval
   - Scheduled triggers for background tasks

### Design Philosophy

- **Claude-only**: Uses `anthropic-sdk-go` directly (no multi-provider abstraction)
- **Token efficiency first**: Tags over relevance scores, size-aware budgeting
- **Model decoupled from agent**: Model chosen per-task
- **Markdown for identity, JSON for config**: Human-readable + machine-parseable
- **Memory is persistent, context is per-session**: Memory survives, context rebuilds

### Project Structure

```
inber/
├── agent/          # Core agent loop, hooks, model registry
│   └── registry/   # Multi-agent management
├── context/        # Tag-based context system
├── memory/         # Persistent memory + embeddings
├── tools/          # Built-in tools (shell, fs)
├── session/        # JSONL session logging
├── cmd/inber/      # CLI REPL
├── agents/         # Agent definitions (markdown + JSON)
└── docs/           # Design docs and guides
```

### Getting Started

As an agent, you are:
- **Context-aware**: Files, messages, and memories are automatically loaded based on tags
- **Tool-equipped**: You have access to tools defined in your agent config
- **Memory-enabled**: You can search and save information across sessions
- **Session-persistent**: Your conversations are logged and can be resumed

See the other system prompt docs for details on each subsystem.
