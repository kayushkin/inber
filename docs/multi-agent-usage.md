# Multi-Agent Usage Guide

## Overview

Inber now supports multiple agents with isolated sessions, contexts, and tool access. This guide shows how to use the multi-agent system.

## Agent Configuration

Agents are defined in YAML files in the `agents/` directory:

```yaml
# agents/coder.yaml
name: coder
role: "software engineer"
system: |
  You are a coding specialist. You write, test, and debug code.
model: claude-sonnet-4-5
thinking: 0
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
  budget: 50000
```

### Configuration Fields

- **name** (required): Unique agent identifier
- **role** (required): Brief role description
- **system** (required): System prompt for the agent
- **model** (optional): Claude model to use (default: claude-sonnet-4-5)
- **thinking** (optional): Token budget for extended thinking (0 = disabled)
- **tools** (required): List of tool names this agent can access
- **context**: Context configuration
  - **tags**: Tags this agent's context should include
  - **budget**: Token budget for context
  - **inherit_parent**: Whether to inherit parent agent's context (default: false)

## Available Tools

Built-in tools that can be assigned to agents:

- `shell` - Execute shell commands
- `read_file` - Read file contents
- `write_file` - Create or overwrite files
- `edit_file` - Make surgical edits to files
- `list_files` - List directory contents
- `spawn_agent` - Spawn sub-agents (coming in Phase 3)

## Using the Registry

### Basic Usage

```go
import (
    "github.com/anthropics/anthropic-sdk-go"
    "github.com/kayushkin/inber/agent/registry"
)

// Create registry
client := anthropic.NewClient(option.WithAPIKey(apiKey))
reg, err := registry.New(&client, "agents", "logs")
if err != nil {
    log.Fatal(err)
}
defer reg.CloseAll()

// List available agents
agents := reg.List()
fmt.Println("Available agents:", agents)

// Get agent instance
coder, err := reg.Get("coder")
if err != nil {
    log.Fatal(err)
}

// Get agent's context store
ctx, err := reg.GetContext("coder")
if err != nil {
    log.Fatal(err)
}

// Get agent's session
sess, err := reg.GetSession("coder")
if err != nil {
    log.Fatal(err)
}
```

### Session Isolation

Each agent gets its own session log directory:

```
logs/
  orchestrator/
    2026-02-24_220900.jsonl
  coder/
    2026-02-24_220905.jsonl
  researcher/
    2026-02-24_221000.jsonl
```

Sessions include:
- Agent name
- Parent session ID (for sub-agents)
- Full conversation history
- Tool calls and results
- Token usage and costs

### Context Isolation

Each agent has its own context store with agent-specific tags:

```go
// Coder's context
coderCtx, _ := reg.GetContext("coder")
coderCtx.Add(context.Chunk{
    ID:   "error-1",
    Text: "error: undefined variable x",
    Tags: []string{"error", "code"},
})

// Researcher's context (different store)
researcherCtx, _ := reg.GetContext("researcher")
researcherCtx.Add(context.Chunk{
    ID:   "doc-1",
    Text: "API documentation for ...",
    Tags: []string{"research", "documentation"},
})
```

## Example: Building a Multi-Agent System

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/anthropics/anthropic-sdk-go"
    "github.com/anthropics/anthropic-sdk-go/option"
    "github.com/kayushkin/inber/agent/registry"
)

func main() {
    // Create registry
    client := anthropic.NewClient(option.WithAPIKey(os.Getenv("ANTHROPIC_API_KEY")))
    reg, err := registry.New(&client, "agents", "logs")
    if err != nil {
        log.Fatal(err)
    }
    defer reg.CloseAll()

    // Use the coder agent
    coder, _ := reg.Get("coder")
    sess, _ := reg.GetSession("coder")
    
    // Set up hooks for logging
    coder.SetHooks(sess.Hooks())

    // Run a task
    var messages []anthropic.MessageParam
    messages = append(messages, 
        anthropic.NewUserMessage(anthropic.NewTextBlock("Write a hello world program in Go")))

    cfg, _ := reg.GetConfig("coder")
    result, err := coder.Run(context.Background(), cfg.Model, &messages)
    if err != nil {
        log.Fatal(err)
    }

    fmt.Println("Result:", result.Text)
    fmt.Printf("Tokens: in=%d out=%d\n", result.InputTokens, result.OutputTokens)
}
```

## Built-in Agents

### orchestrator
- **Role**: Task coordination and delegation
- **Tools**: spawn_agent, read_file, list_files
- **Use case**: Breaking down complex tasks, delegating to specialists

### coder
- **Role**: Software engineering
- **Tools**: shell, read_file, write_file, edit_file, list_files
- **Use case**: Writing code, running tests, debugging

### researcher
- **Role**: Research and analysis
- **Tools**: read_file, list_files (read-only)
- **Use case**: Analyzing code, documentation, research tasks

## Adding Custom Agents

1. Create a YAML file in `agents/`:

```yaml
# agents/my-agent.yaml
name: my-agent
role: "your custom role"
system: |
  Your custom system prompt here.
model: claude-sonnet-4-5
tools:
  - read_file
  - write_file
context:
  tags:
    - custom
  budget: 20000
```

2. The registry automatically loads all `.yaml` files from the `agents/` directory.

3. Use your agent:

```go
myAgent, err := reg.Get("my-agent")
```

## Next Steps

- **Phase 2**: Tool scoping implementation
- **Phase 3**: Sub-agent spawning with `spawn_agent` tool
- **Phase 4**: Context inheritance and integration
- **Phase 5**: CLI integration with `--agent` flag

See [multi-agent-design.md](multi-agent-design.md) for the full design document.
