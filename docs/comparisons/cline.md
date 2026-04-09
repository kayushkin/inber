# Cline Comparison

**Project**: [Cline](https://github.com/cline/cline)
**Language**: TypeScript (VS Code extension)
**Focus**: Autonomous coding agent in-editor with strict human-in-the-loop approval for every action
**Key Strengths**: AST-based context management, checkpoint/restore, browser automation, MCP extensibility, ~60k stars

## Architecture Overview

Cline is a VS Code extension where the agent interacts with the codebase through the editor's APIs — file system, terminal, diagnostics, diff views. Every action (file write, terminal command, browser action) surfaces in the VS Code GUI for explicit user approval.

```
VS Code Sidebar (React webview)
    → Task execution loop
    → Tool calls (file ops, terminal, browser, MCP)
    → Human approval gate (VS Code diff view / dialog)
    → LLM (Anthropic, OpenAI, Google, Bedrock, Vertex, OpenRouter, etc.)
```

The checkpoint system takes workspace snapshots at each step, enabling diff-and-restore to any prior state. MCP support enables dynamic tool extension — the agent can even create new MCP servers on the fly.

## What Cline Does Well

### 1. AST-Based Context Management ⭐️

Cline uses tree-sitter to parse source code structure and builds a semantic codebase index with embeddings. Instead of naively stuffing files into context, it extracts symbol definitions and uses targeted searches. This enables working on large projects without exhausting the context window.

**Inber connection**: Inber's context system builds prompts from memory entries with token budgets (`turn_context.go`), but doesn't analyze code structure. AST-based context would significantly improve coding task efficiency.

### 2. Checkpoint System with Diff/Restore

Git worktree-based snapshots at each step. Users can compare any checkpoint against the current state and restore to any prior point. Finer-grained than git commits — captures intermediate states within a single task.

**Inber connection**: Inber saves messages to workspace JSON and logs to JSONL, but doesn't capture workspace state snapshots. For agents with file write access, checkpointing would provide safety and undo capability.

### 3. Browser Automation

Launches a headless browser (Puppeteer/Playwright), interacts with web pages (click, type, scroll), captures screenshots and console logs. Enables visual debugging and end-to-end testing workflows.

### 4. MCP as Extensibility Primitive

Rather than a custom plugin system, Cline uses MCP for tool extension. It can connect to MCP servers and even create new ones on-the-fly ("add a tool that fetches Jira tickets"). This bets on ecosystem effects and interoperability.

### 5. Mention System for Context Injection

`@file`, `@folder`, `@url` (fetches and converts to markdown), `@problems` (workspace diagnostics) give users fine-grained control over what the agent sees. Simple but effective context management UX.

## What Inber Should Adopt

### 1. Codebase Indexing for Context (MEDIUM PRIORITY)

Tree-sitter parsing + embedding-based semantic search for code context. When an agent is working on code, retrieve relevant symbols and definitions rather than relying on the agent to manually read files.

This would enhance `turn_prompt.go`'s context building for coding tasks without changing the overall architecture.

### 2. Workspace Checkpointing (MEDIUM PRIORITY)

For agents with file write access, take git snapshots at turn boundaries:
- Before RunTurn: snapshot workspace state
- After RunTurn: if files changed, create a checkpoint commit
- Expose checkpoint list via session history endpoints

This provides undo capability without requiring the agent to manage git itself.

### 3. Linter/Compiler Monitoring (LOW PRIORITY)

Cline watches for diagnostic errors after file changes and proactively fixes issues. Inber's workflow hooks already run build/test after tool calls (`workflow_build.go`), but feeding compiler output back as context for the next turn would close the loop.

## What's Different

| Aspect | Cline | Inber |
|--------|-------|-------|
| **Runtime** | VS Code extension | Standalone Go server |
| **Approval model** | Every action needs GUI approval | Tool allowlists per agent |
| **Context** | AST + embeddings + mentions | Memory store + token budget |
| **Multi-agent** | Single agent per task | 10 named agents, bus-based |
| **Persistence** | VS Code workspace state | SQLite agent-store |
| **Extensibility** | MCP-first | agentkit + MCP |
| **Deployment** | Desktop only (VS Code) | Server, headless, CLI |
| **Target** | Individual developer | Multi-agent infrastructure |

## Key Takeaway

Cline's most transferable innovation is **AST-based context management** — using code structure analysis to build efficient, relevant context within token budgets. This would directly improve inber's coding task performance. The checkpoint system is also valuable for any agent with file write access. Cline's strict human-in-the-loop model is the opposite of inber's autonomous approach, but the underlying tools (code indexing, checkpointing, compiler monitoring) are universally useful.
