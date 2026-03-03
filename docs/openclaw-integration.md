# OpenClaw Integration

Inber can delegate tasks to OpenClaw agents running on the gateway, treating them as specialist sub-agents.

## Configuration

Add an `openclaw` section to your `agents.json`:

```json
{
    "default": "task-manager",
    "agents": {
        "task-manager": {
            "name": "task-manager",
            "tools": ["spawn_agent", "check_spawns"]
        }
    },
    "openclaw": {
        "url": "ws://localhost:18789/ws",
        "token": "your-gateway-auth-token",
        "agents": ["kayushkin", "downloadstack", "claxon-android"]
    }
}
```

### Fields

- `url`: WebSocket URL of the OpenClaw gateway (typically `ws://localhost:18789/ws`)
- `token`: Authentication token for the gateway (found in `~/.openclaw/auth.json`)
- `agents`: List of agent names that should route to OpenClaw instead of spawning locally

## Usage

When an orchestrator agent uses `spawn_agent` with an agent name listed in the OpenClaw config, inber will automatically delegate to the OpenClaw gateway instead of spawning a local instance.

Example:

```javascript
// In your orchestrator agent's conversation:
{
  "tool": "spawn_agent",
  "input": {
    "agent": "kayushkin",  // This agent is in the openclaw.agents list
    "task": "Analyze this code pattern",
    "wait": true
  }
}
```

The task will be sent to the OpenClaw gateway, which will route it to the `kayushkin` agent. The response is returned as if it were a local sub-agent spawn.

## Protocol

The integration uses the OpenClaw gateway WebSocket protocol:

1. **Connect**: Establish WebSocket connection to gateway
2. **Handshake**: Respond to `connect.challenge` with authentication
3. **Agent Request**: Send agent invocation request
4. **Stream Response**: Buffer assistant deltas until lifecycle end
5. **Return Result**: Convert to inber's `TurnResult` format

## Testing

Run the OpenClaw integration tests:

```bash
cd cmd/inber
go test -v -run TestOpenClawSubagent
```

Run the sí integration tests (which test sub-agent spawning end-to-end):

```bash
cd test
go test -v -run TestSiPipeline
```

## Implementation

- `cmd/inber/openclaw_feed.go`: OpenClaw gateway client implementation
- `agent/registry/spawn_tool.go`: Routing logic (checks if agent is in OpenClaw config)
- `agent/registry/config.go`: Configuration loading
- `test/si_integration_test.go`: Integration tests for sub-agent spawning through sí

## Token Tracking

OpenClaw agents report token usage through the `usage` stream event. This is automatically tracked and included in the spawn result's `InputTokens` and `OutputTokens` fields.
