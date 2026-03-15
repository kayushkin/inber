# Inber Architecture

Inber is the **agent runtime**. It runs agents, manages sessions, spawns sub-agents, and connects to the bus for all external I/O.

## Role

- Run agent turns (engine, context, tools)
- Manage sessions (persistence, resume, repair)
- Spawn sub-agents with workspace isolation (via forge)
- Subscribe to bus for inbound messages, publish responses + events
- Route inbound messages to the correct agent (routing table, formerly in bus-agent)

## Connections

```
Inber ↔ Bus (WS subscribe inbound, HTTP publish outbound + events)
Inber → Forge (library call for workspace ops)
Inber → Model APIs (Anthropic, OpenAI, etc. via model-store)
```

Inber does NOT expose an HTTP API to SI or adapters. All I/O flows through bus.

## What Moves Here (from bus-agent)

1. **Bus subscription** — subscribe to `inbound` topic directly
2. **Agent routing** — resolve channel/metadata → agent (routing table in SQLite)
3. **Per-agent queues** — sequential processing per agent, concurrent across agents
4. **Backend execution** — run agent turns in-process (no more shelling out to `inber run`)
5. **Response publishing** — publish to bus `outbound` topic
6. **Event publishing** — publish spawn/status/deploy events to bus `events` topic
7. **Model dashboard API** — model status, credential management (move to server API)
8. **Agent registry** — agent list, routing table (merge with existing agents.json)

## What's Removed

- `cmd/bus-agent/` stays in the bus repo but is deprecated
- `feed/inber_direct.go` in SI is removed (SI no longer calls inber)
- No more INBER_SPAWN/INBER_META/INBER_DELTA stderr protocol — everything is in-process

## Message Flow

### Inbound
1. Bus delivers message on `inbound` topic via WebSocket
2. Server resolves agent from message metadata (agent field, or channel→agent route)
3. Message enters per-agent queue
4. Queue processor runs agent turn via engine
5. Response published to bus `outbound` topic

### Events
Server publishes structured events to bus `events` topic:
```json
{"kind": "spawn.started", "agent": "oisin", "task": "...", "workspace": "..."}
{"kind": "spawn.done", "agent": "oisin", "commits": 3, "branch": "spawn/oisin-..."}
{"kind": "spawn.merged", "agent": "oisin", "workspace": "..."}
{"kind": "deploy.started", "service": "si", "target": "prod"}
{"kind": "deploy.done", "service": "si", "status": "ok"}
```

## Interfaces

### BusClient (new — connects server to bus)
```go
// BusClient handles bus pub/sub for the server.
type BusClient struct {
    busURL   string
    token    string
    consumer string
}

func (c *BusClient) Subscribe(ctx context.Context, topics []string) (<-chan BusMessage, error)
func (c *BusClient) Publish(topic string, payload any) error
```

### Server (existing — gains bus integration)
```go
type Server struct {
    // existing fields...
    bus      *BusClient          // new: bus connection
    routes   *RouteTable         // new: channel→agent routing (from bus-agent registry)
    queues   map[string]*Queue   // new: per-agent message queues
}
```

## Server API (HTTP — for dashboard/management only)

The server still exposes HTTP for management, but NOT for message routing:
```
GET  /api/sessions          — list sessions
GET  /api/sessions/:key     — session detail
GET  /api/models            — model status
POST /api/models/test       — test a model
GET  /api/agents            — list agents + routes
POST /api/agents/route      — update routing
```
