# Agent Registry

The registry package provides multi-agent support for inber, allowing you to define, manage, and orchestrate multiple specialized agents with isolated sessions, contexts, and tool access.

## Features

- **YAML-based agent configuration** - Define agents declaratively
- **Session isolation** - Each agent has its own conversation log
- **Context isolation** - Each agent has its own tagged context store
- **Tool scoping** - Control which tools each agent can access
- **Lazy initialization** - Agents created on-demand
- **Thread-safe** - Safe for concurrent access

## Quick Start

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

// Get an agent
coder, err := reg.Get("coder")
if err != nil {
    log.Fatal(err)
}

// Run a task
result, err := coder.Run(ctx, "claude-sonnet-4-5", &messages)
```

## Agent Configuration

Create YAML files in the `agents/` directory:

```yaml
name: coder
role: "software engineer"
system: |
  You are a coding specialist.
model: claude-sonnet-4-5
thinking: 0
tools:
  - shell
  - read_file
  - write_file
context:
  tags:
    - code
  budget: 50000
```

## API

### Registry

```go
// Create a new registry
func New(client *anthropic.Client, configDir, logsDir string) (*Registry, error)

// List all agent names
func (r *Registry) List() []string

// Get agent configuration
func (r *Registry) GetConfig(name string) (*AgentConfig, error)

// Get agent instance (creates if needed)
func (r *Registry) Get(name string) (*agent.Agent, error)

// Get agent's context store
func (r *Registry) GetContext(name string) (*context.Store, error)

// Get agent's session
func (r *Registry) GetSession(name string) (*session.Session, error)

// Close specific agent session
func (r *Registry) CloseSession(name string)

// Close all sessions
func (r *Registry) CloseAll()
```

### Configuration

```go
type AgentConfig struct {
    Name     string        // Unique agent identifier
    Role     string        // Brief role description
    System   string        // System prompt
    Model    string        // Claude model (default: claude-sonnet-4-5)
    Thinking int64         // Extended thinking budget (0 = disabled)
    Tools    []string      // Tool names agent can access
    Context  ContextConfig // Context configuration
}

type ContextConfig struct {
    Tags         []string // Context tags
    Budget       int      // Token budget
    InheritParent bool    // Inherit parent context
}
```

## Built-in Agents

The `agents/` directory includes three pre-configured agents:

### orchestrator
- Coordinates complex tasks
- Delegates to specialist agents
- Tools: spawn_agent (coming soon), read_file, list_files

### coder
- Software engineering specialist
- Full shell and file access
- Tools: shell, read_file, write_file, edit_file, list_files

### researcher
- Research and analysis specialist
- Read-only access
- Tools: read_file, list_files

## Examples

See `examples/multi-agent/main.go` for a complete working example.

## Testing

```bash
go test ./agent/registry/ -v
```

## Roadmap

- [x] Phase 1: Core registry and config loading
- [ ] Phase 2: Tool scoping implementation
- [ ] Phase 3: Sub-agent spawning (`spawn_agent` tool)
- [ ] Phase 4: Context inheritance
- [ ] Phase 5: CLI integration

See `docs/multi-agent-design.md` for the full design document.
