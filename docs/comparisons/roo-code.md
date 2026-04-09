# Roo Code Comparison

**Project**: [Roo Code](https://github.com/RooCodeInc/Roo-Code)
**Language**: TypeScript (VS Code extension, Cline fork)
**Focus**: Multi-mode AI coding assistant — different agent behaviors via mode switching rather than separate agents
**Key Strengths**: Mode system (Code/Architect/Ask/Debug/custom), codebase semantic indexing, skills as slash commands, auto-approval policies

## Architecture Overview

Roo Code is a VS Code extension (forked from Cline) with a single agent that can switch between modes. Each mode configures which tools are available and what system prompt components to use. The agent can switch modes mid-conversation via `SwitchModeTool`.

```
VS Code Sidebar → Task loop → Mode-configured tools + prompts
    → Code mode:      full file ops + shell + MCP
    → Architect mode:  read-only + planning tools
    → Ask mode:        conversation only, no tools
    → Debug mode:      read + execute + diagnostics
    → Custom modes:    user-defined tool groups + prompts
```

Inherits Cline's core: 25+ tools, MCP client, tree-sitter AST parsing, embedding-based codebase search, git-based checkpoints, multi-provider LLM support.

## What Roo Code Does Well

### 1. Mode System for Agent Specialization ⭐️

Instead of separate agents, Roo Code uses modes — configurations of tool groups and system prompt components. Five built-in modes plus custom user-defined modes. Each mode is a `ModeConfig` with `slug`, `groups` (tool group sets), and prompts.

This is a lightweight alternative to multi-agent: one agent, multiple personalities, switchable mid-conversation.

**Inber connection**: Inber uses separate named agents with distinct identities. Roo Code's mode pattern could complement this — an agent could have modes (e.g., "research mode" with read-only tools, "build mode" with full access) without needing separate agent definitions.

### 2. Auto-Approval Policies

Configurable per-tool auto-approve policies. Read operations can be auto-approved while write/execute operations require confirmation. More granular than a binary "approve all" or "approve nothing."

**Inber connection**: Inber's agents have tool allowlists but no per-tool approval levels. A tiered system (auto/ask/deny per tool) would be useful for the execution mode system discussed in the Commander comparison.

### 3. Skills as Slash Commands

Reusable prompt-based workflows saved as files and exposed as `/command` slash commands. The agent can invoke them via `RunSlashCommandTool`. Simple extensibility without code changes.

### 4. Tool Repetition Detection

A `ToolRepetitionDetector` prevents infinite loops where the agent repeatedly calls the same tool with the same arguments. Practical guard against a common failure mode.

**Inber connection**: Inber doesn't explicitly detect tool repetition. Adding this would be a simple improvement to the agent loop.

## What Inber Should Adopt

### 1. Agent Modes (MEDIUM PRIORITY)

Add a per-session mode that controls tool availability without changing the agent identity:

```
Modes:
  observe  → read-only tools (file reads, search, memory)
  assist   → reads + writes with confirmation
  autonome → full tool access (current default)
```

Modes could be set per-request via the server API, allowing the same agent to operate at different trust levels depending on context.

### 2. Tool Repetition Detection (HIGH PRIORITY)

Simple guard: if the agent calls the same tool with identical input N times in a row, inject a warning or force a different action. Prevents stuck loops.

### 3. Auto-Approval Tiers (LOW PRIORITY)

Per-tool approval levels in agent config: auto (always execute), ask (require confirmation via bus/UI), deny (blocked). This complements the mode system — modes select which tools are available, approval tiers control how they're gated.

## What's Different

| Aspect | Roo Code | Inber |
|--------|----------|-------|
| **Agent model** | Single agent with mode switching | Multiple named agents |
| **Runtime** | VS Code extension | Go server |
| **Specialization** | Modes (tool groups + prompts) | Separate agent identities |
| **Context** | AST + embeddings (from Cline) | Memory store + token budget |
| **Multi-agent** | No (one agent, multiple modes) | Yes (NATS bus, concurrent agents) |
| **Tools** | 25+ hardcoded + MCP | agentkit library + MCP |
| **Loop guard** | ToolRepetitionDetector | None currently |

## Key Takeaway

Roo Code's mode system is the most relevant pattern for inber. **Modes are a lightweight complement to multi-agent** — instead of spawning a separate research agent, you could put the current agent into "research mode" with read-only tools. Combined with the tool repetition detection, these are practical improvements that address real failure modes. The mode pattern maps cleanly to the execution tiers discussed across several comparisons (Commander, Cline, Autohand).
