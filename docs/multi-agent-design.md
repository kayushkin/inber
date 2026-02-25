# Multi-Agent Design for inber

## Overview

Transform inber from a single-agent framework into a multi-agent orchestration system where agents can spawn sub-agents, delegate tasks, and maintain isolated contexts and sessions.

## Goals

1. **Multiple agent definitions** — Define agents via config files, each with its own role, tools, and context
2. **Orchestrator pattern** — A primary agent can delegate tasks to specialist agents
3. **Session isolation** — Each agent maintains its own conversation history and logging
4. **Context isolation** — Each agent has its own tagged context store
5. **Tool scoping** — Tools are agent-specific (shell access only for certain agents)
6. **Sub-agent spawning** — Agents can spawn temporary sub-agents for specific tasks
7. **Natural integration** — Fits cleanly into existing agent/, context/, session/ structure

## Architecture

### 1. Agent Definition (Config + Code)

Agents are defined in YAML config files:

```yaml
# agents/orchestrator.yaml
name: orchestrator
role: "task orchestrator and delegation manager"
system: |
  You coordinate complex tasks by delegating to specialist agents.
  You have access to spawn_agent tool to create sub-agents.
model: claude-sonnet-4-5
thinking: 2048
tools:
  - spawn_agent
  - read_file
  - list_files
context:
  tags:
    - identity
    - delegation
  budget: 20000

# agents/coder.yaml
name: coder
role: "software engineer"
system: |
  You are a coding specialist. You write, test, and debug code.
  Focus on code quality, tests, and clear documentation.
model: claude-sonnet-4-5
tools:
  - shell
  - read_file
  - write_file
  - edit_file
  - list_files
context:
  tags:
    - code
    - errors
    - tests
  budget: 50000

# agents/researcher.yaml
name: researcher
role: "research and analysis"
system: |
  You research topics, analyze data, and provide insights.
  No shell access — read-only operations.
model: claude-sonnet-4-5
tools:
  - read_file
  - list_files
context:
  tags:
    - research
    - data
  budget: 30000
```

### 2. Agent Registry

New package `agent/registry`:

```go
type Registry struct {
    agents map[string]*agent.Agent
    configs map[string]*AgentConfig
    contexts map[string]*context.Store
    sessions map[string]*session.Session
}

type AgentConfig struct {
    Name     string
    Role     string
    System   string
    Model    string
    Thinking int64
    Tools    []string  // tool names
    Context  ContextConfig
}

type ContextConfig struct {
    Tags   []string
    Budget int
}

func NewRegistry(configDir string) (*Registry, error)
func (r *Registry) Get(name string) (*agent.Agent, error)
func (r *Registry) Spawn(parentName, childName string, task string) (string, error)
```

The registry:
- Loads agent configs from YAML files
- Creates agent instances on-demand
- Manages per-agent context stores and sessions
- Tracks parent-child relationships for spawned agents

### 3. Sub-Agent Spawning Tool

New tool in `tools/`:

```go
func SpawnAgent(registry *Registry) agent.Tool {
    return agent.Tool{
        Name: "spawn_agent",
        Description: "Spawn a sub-agent to handle a specific task. " +
            "The sub-agent runs independently and returns its final result.",
        InputSchema: props([]string{"agent", "task"}, map[string]any{
            "agent": str("Agent to spawn (coder, researcher, etc)"),
            "task":  str("Task description for the sub-agent"),
        }),
        Run: func(ctx context.Context, input string) (string, error) {
            // Parse input
            // registry.Spawn(currentAgent, childName, task)
            // Wait for result
            // Return sub-agent's output
        },
    }
}
```

This tool:
- Takes agent name and task description
- Spawns a new agent instance with isolated session/context
- Executes the task (runs agent loop until completion)
- Returns the final result to the parent agent
- Logs the parent-child relationship

### 4. Session Isolation

Enhanced session package to support agent hierarchies:

```go
type Session struct {
    // existing fields...
    agentName  string
    parentID   string  // parent session ID (empty for root)
    sessionID  string  // unique session ID
}

func New(logsDir, model, agentName, parentID string) (*Session, error)
```

Session logs include:
- Agent name in every entry
- Parent-child relationships
- Per-agent token/cost tracking
- Sub-agent spawn/completion events

Log structure:
```
logs/
  orchestrator/
    2026-02-24_220900.jsonl        # root session
  coder/
    2026-02-24_220905-sub1.jsonl   # spawned by orchestrator
    2026-02-24_220912-sub2.jsonl   # another spawn
```

### 5. Context Isolation

Each agent gets its own `context.Store`:

- Orchestrator has chunks tagged `identity`, `delegation`
- Coder has chunks tagged `code`, `errors`, `tests`
- Researcher has chunks tagged `research`, `data`

Context inheritance:
- Sub-agents can optionally inherit parent context (via config flag)
- Useful for passing relevant state down the hierarchy
- Default: isolated (no inheritance)

### 6. Tool Scoping

Tools are registered per-agent based on config:

```go
func (r *Registry) createAgent(cfg *AgentConfig) (*agent.Agent, error) {
    a := agent.New(r.client, cfg.System)
    
    // Register only the tools specified in config
    for _, toolName := range cfg.Tools {
        tool := r.getToolByName(toolName)
        if tool != nil {
            a.AddTool(tool)
        }
    }
    
    return a, nil
}
```

Tool registry maps names → tool constructors:
- `shell` → `tools.Shell()`
- `read_file` → `tools.ReadFile()`
- `spawn_agent` → `tools.SpawnAgent(registry)`
- etc.

### 7. Message Passing & Communication

Agents communicate through:

1. **Spawn → Result** — Parent spawns child with task, gets back result string
2. **Context Inheritance** (optional) — Child can read parent's context chunks
3. **Session Logging** — All interactions logged with agent names

No direct agent-to-agent messaging in v1. Keep it simple: hierarchical spawn with return values.

## Implementation Plan

### Phase 1: Core Registry (this PR)

- [ ] Create `agent/registry/` package
- [ ] Define `AgentConfig` struct
- [ ] Implement YAML config loading
- [ ] Build agent registry with lazy initialization
- [ ] Add per-agent context store mapping
- [ ] Add per-agent session management

### Phase 2: Tool Scoping

- [ ] Create tool name → constructor registry
- [ ] Update agent creation to filter tools by config
- [ ] Test tool isolation (some agents can't use shell)

### Phase 3: Sub-Agent Spawning

- [ ] Implement `spawn_agent` tool
- [ ] Handle session parent-child relationships
- [ ] Log spawn/completion events
- [ ] Return sub-agent results to parent

### Phase 4: Context Integration

- [ ] Wire context stores into agent message building
- [ ] Implement context inheritance (optional)
- [ ] Test tag-based context isolation

### Phase 5: CLI Integration

- [ ] Update `cmd/inber/` to use registry
- [ ] Add `--agent <name>` flag to select agent
- [ ] Default to `orchestrator` agent
- [ ] Show active agent in prompt

## Directory Structure

```
inber/
  agent/
    agent.go
    models.go
    registry/              # NEW
      registry.go          # Registry, AgentConfig
      config.go            # YAML loading
      loader.go            # Agent creation logic
      spawn.go             # Sub-agent spawning
  agents/                  # NEW - agent configs
    orchestrator.yaml
    coder.yaml
    researcher.yaml
  tools/
    tools.go
    spawn.go               # NEW - spawn_agent tool
  context/
    store.go               # (no changes needed)
    builder.go
  session/
    session.go             # Enhanced with agent hierarchy
```

## Example Usage

```bash
# Start with orchestrator agent
./inber --agent orchestrator

> Write a web server in Go with tests

# Orchestrator delegates:
# 1. Spawns "coder" agent with task "implement Go web server"
# 2. Spawns "coder" agent with task "write tests for web server"
# 3. Aggregates results and responds

# Each sub-agent has:
# - Its own session log
# - Its own context store
# - Its own tool access (coder has shell, orchestrator doesn't)

# User sees:
[coder] implementing web server...
[coder] running tests...
[orchestrator] Task complete. Created server.go and server_test.go with 3 passing tests.
```

## Benefits

1. **Separation of concerns** — Each agent has a specific role
2. **Safety** — Tool scoping limits what each agent can do
3. **Observability** — Session logs show full agent hierarchy
4. **Scalability** — Easy to add new specialist agents
5. **Context efficiency** — Each agent only loads relevant chunks
6. **Natural fit** — Builds on existing agent/context/session structure

## Future Extensions

- **Agent-to-agent messaging** — Direct communication beyond spawn/result
- **Shared context pools** — Explicit context sharing between agents
- **Agent lifecycle management** — Long-running agents beyond spawned tasks
- **Multi-turn sub-agent conversations** — Interactive delegation
- **Agent discovery** — Agents query available specialists
- **Resource limits** — Per-agent token budgets, timeouts
- **Streaming** — Stream sub-agent progress to parent

## Open Questions

1. Should sub-agents be completely ephemeral, or can they persist?
   - **Decision**: Ephemeral by default. Persistent agents are phase 2.

2. How deep can agent hierarchies go?
   - **Decision**: No hard limit, but warn after depth 3.

3. Should agents share tool state (e.g., same shell session)?
   - **Decision**: No. Each agent gets fresh tool instances.

4. How to handle context inheritance with large contexts?
   - **Decision**: Optional inheritance, tag-filtered. Parent can mark chunks as "inheritable".

5. What happens if a sub-agent spawns a sub-agent?
   - **Decision**: Allow it. Track full ancestry in session logs.

---

This design provides the foundation for multi-agent orchestration while fitting naturally into inber's existing architecture. Start with Phase 1 and iterate.
