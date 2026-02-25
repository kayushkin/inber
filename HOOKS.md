# Lifecycle Hooks & Event System

The inber agent framework includes a comprehensive lifecycle hooks system that allows agents to observe and control operations at specific points in the system.

## Overview

Hooks enable:
- **Observation** — log, audit, or monitor agent operations
- **Interception** — approve, deny, or modify operations before they execute
- **Gatekeeper mode** — one agent can control whether another agent's operations proceed
- **Scheduled triggers** — agents can activate on a timer or cron schedule

## Event Types

### `on_session_start`
Fires when a new session begins (before any messages are processed).

**Event data:**
```json
{
  "type": "on_session_start",
  "timestamp": "2026-02-24T23:00:00Z",
  "agent_name": "coder",
  "session_id": "session-123",
  "data": {}
}
```

### `on_before_request`
Fires before each API call to Claude. Allows inspection/modification of the request.

**Event data:**
```json
{
  "type": "on_before_request",
  "agent_name": "coder",
  "session_id": "session-123",
  "data": {
    "model": "claude-sonnet-4-5",
    "params_json": "..."
  }
}
```

### `on_after_response`
Fires after each API response from Claude.

**Event data:**
```json
{
  "type": "on_after_response",
  "agent_name": "coder",
  "session_id": "session-123",
  "data": {
    "stop_reason": "tool_use",
    "input_tokens": 1500,
    "output_tokens": 800
  }
}
```

### `on_tool_call`
Fires when a tool is about to be executed. Gatekeepers can intercept and deny.

**Event data:**
```json
{
  "type": "on_tool_call",
  "agent_name": "coder",
  "session_id": "session-123",
  "data": {
    "tool_id": "tool-abc123",
    "tool_name": "shell",
    "input": "{\"command\": \"rm -rf /\"}"
  }
}
```

### `on_tool_result`
Fires after a tool returns a result.

**Event data:**
```json
{
  "type": "on_tool_result",
  "agent_name": "coder",
  "session_id": "session-123",
  "data": {
    "tool_id": "tool-abc123",
    "tool_name": "shell",
    "output": "...",
    "is_error": false
  }
}
```

### `on_before_spawn`
Fires before a sub-agent is spawned.

**Event data:**
```json
{
  "type": "on_before_spawn",
  "agent_name": "orchestrator",
  "session_id": "session-123",
  "data": {
    "child_agent": "coder"
  }
}
```

### `on_after_spawn`
Fires after a sub-agent completes.

**Event data:**
```json
{
  "type": "on_after_spawn",
  "agent_name": "orchestrator",
  "session_id": "session-123",
  "data": {
    "child_agent": "coder",
    "success": true,
    "result": "..."
  }
}
```

### `on_session_end`
Fires when a session ends.

**Event data:**
```json
{
  "type": "on_session_end",
  "agent_name": "coder",
  "session_id": "session-123",
  "data": {
    "total_cost": 0.05,
    "total_tokens": 10000
  }
}
```

## Hook Actions

Hook handlers return a `HookResult` with one of three actions:

- **`proceed`** — Continue normally
- **`abort`** — Stop the operation (only gatekeepers can abort)
- **`modify`** — Modify the event data and proceed

Example:
```go
handler := func(ctx context.Context, event *Event) (*HookResult, error) {
    // Inspect the event
    if event.Type == EventToolCall {
        toolName := event.Data["tool_name"].(string)
        
        // Dangerous operation?
        if toolName == "shell" {
            input := event.Data["input"].(string)
            if strings.Contains(input, "rm -rf") {
                return &HookResult{
                    Action:  ActionAbort,
                    Message: "Dangerous command blocked",
                }, nil
            }
        }
    }
    
    return &HookResult{Action: ActionProceed}, nil
}
```

## Configuration

Hooks are configured per-agent in `agents.json`:

```json
{
  "agents": {
    "security": {
      "name": "security",
      "role": "Security gatekeeper",
      "model": "claude-sonnet-4-5",
      "tools": [],
      "hooks": {
        "hooks": ["on_tool_call"],
        "gatekeeper_for": ["coder", "researcher"]
      }
    },
    "logger": {
      "name": "logger",
      "role": "Audit logger",
      "model": "claude-haiku-4",
      "tools": ["write_file"],
      "hooks": {
        "hooks": [
          "on_session_start",
          "on_tool_call",
          "on_tool_result",
          "on_session_end"
        ]
      }
    },
    "cost-monitor": {
      "name": "cost-monitor",
      "role": "Cost tracking",
      "model": "claude-haiku-4",
      "tools": [],
      "hooks": {
        "hooks": ["on_after_response"],
        "schedule": {
          "interval": "5m"
        }
      }
    }
  }
}
```

### Hook Options

- **`hooks`** — Array of event types to subscribe to
- **`gatekeeper_for`** — Array of agent names to gate for
- **`schedule`** — Optional scheduled trigger
  - `interval`: e.g., "30s", "5m", "1h"
  - `cron`: cron expression (not yet implemented)

## Gatekeeper Mode

Gatekeepers have special powers:
- They run **before** regular subscribers
- They can **abort** operations (regular subscribers can only observe)
- They can **modify** event data that other handlers will see

Use cases:
- Security checks (block dangerous commands)
- Cost limits (abort expensive operations)
- Content filtering (sanitize inputs/outputs)
- Rate limiting (throttle API calls)

Example: Security gatekeeper that blocks dangerous shell commands:
```json
{
  "security": {
    "name": "security",
    "role": "Security gatekeeper - blocks dangerous operations",
    "hooks": {
      "hooks": ["on_tool_call"],
      "gatekeeper_for": ["coder"]
    }
  }
}
```

## Scheduled Triggers

Agents can be configured to activate on a schedule:

```json
{
  "cost-monitor": {
    "name": "cost-monitor",
    "role": "Periodic cost reporting",
    "hooks": {
      "hooks": ["on_after_response"],
      "schedule": {
        "interval": "5m"
      }
    }
  }
}
```

**Note:** Scheduled triggers are configured but not yet fully implemented in the runtime.

## Usage Example

### Registering Hooks

```go
import (
    "github.com/kayushkin/inber/agent"
)

// Create hook registry
registry := agent.NewHookRegistry()

// Define a handler
handler := func(ctx context.Context, event *agent.Event) (*agent.HookResult, error) {
    log.Printf("Event: %s for agent %s", event.Type, event.AgentName)
    return &agent.HookResult{Action: agent.ActionProceed}, nil
}

// Register agent with hooks
config := &agent.HookConfig{
    Events: []agent.EventType{
        agent.EventSessionStart,
        agent.EventToolCall,
    },
}

err := registry.Register("logger", config, handler)
if err != nil {
    log.Fatal(err)
}

// Attach registry to agent
agentInstance.SetHookRegistry(registry)
agentInstance.SetIdentity("coder", "session-123")
```

### Dispatching Events

```go
// Create an event
event := agent.ToolCallEvent("coder", "session-123", "tool-1", "shell", []byte(`{"command": "ls"}`))

// Dispatch to all subscribers
result, err := registry.Dispatch(ctx, event)
if err != nil {
    log.Printf("Hook error: %v", err)
}

if result.Action == agent.ActionAbort {
    log.Printf("Operation aborted: %s", result.Message)
    // Don't proceed with the operation
}
```

## Design Philosophy

The hooks system follows an **event bus pattern**:
- Decoupled: Agents don't need to know about each other
- Extensible: New event types can be added easily
- Composable: Multiple handlers can process the same event
- Observable: All operations can be logged/audited

Hooks are **synchronous** — they block the operation until all handlers complete. This ensures proper sequencing and allows gatekeepers to effectively control operations.

## Future Enhancements

- **Async hooks** — for non-blocking observation
- **Hook priorities** — control execution order
- **Cron support** — full cron expression parsing
- **Event replay** — for debugging and testing
- **Hook middleware** — composable hook transformations
