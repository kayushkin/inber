# Migration Plan: Bus-Centric Architecture

> **Status, measured 2026-08-21: phases 1 and 2 shipped, phase 3 is partly done,
> phase 4 was never started. Read this as a record, not as work to pick up.**
>
> - **Phase 1 — done.** The bus client landed in `server/`, then moved to its own
>   package in `127e030`: what step 1 below calls `BusClient` is `bus.Client` today,
>   and `server.BusManager` (`server/bus.go`) holds the routing this plan gave the
>   server. ⚠️ The transport is **NATS**, not the WebSocket-subscribe / HTTP-publish
>   pair step 1 specifies — that swap is not recorded anywhere in this plan.
> - **Phase 2 — done.** `bus/cmd/bus-agent/` no longer exists.
> - **Phase 3 — partly done.** `si/feed/inber_direct.go` is retired (it sits as
>   `inber_direct_deprecated.go.bak`) and `si/feed/nats.go` is the live subscriber.
>   `si/feed/echo.go` is still there.
> - **Phase 4 — not started**, and the plan already marks it optional.

Everything flows through bus. SI is the only user interface. Inber subscribes to bus directly (no bus-agent middleman).

## Target Architecture

```
Slava ←→ SI (web) ←→ Bus ←→ Inber Server ←→ Agents
                       ↑
         Adapters (Discord, Signal, etc.)
```

## Migration Steps (ordered — remove old, then add new)

### Phase 1: Inber subscribes to bus directly

**Goal**: Server subscribes to bus `inbound`, publishes to `outbound`. Replaces bus-agent.

1. Add `BusClient` to inber `server/` package
   - WebSocket subscribe to `inbound`
   - HTTP publish to `outbound` and `events`
   - Reconnect loop with backoff
2. Add routing table to server (port from bus-agent's `registry.go`)
   - Channel → agent resolution
   - Agent registry (list, find, routes)
   - SQLite-backed, same schema as bus-agent's `agents.db`
3. Add per-agent message queues to server
   - Sequential per agent, concurrent across agents
   - Port queue logic from bus-agent's `main.go`
4. Wire bus messages into existing `Server.Run()` flow
   - Bus message → resolve agent → queue → run turn → publish response
5. Publish events to bus `events` topic
   - spawn.started, spawn.done, spawn.merged, deploy.*

**Test**: Send a message to bus `inbound` → inber picks it up → response appears on `outbound`.

### Phase 2: Remove bus-agent

**Goal**: bus-agent is no longer needed.

1. Stop running bus-agent process
2. Deprecate `bus/cmd/bus-agent/` (keep code for reference, mark deprecated)
3. Remove bus-agent from forge deploy configs
4. Remove bus-agent from staging envs

**Test**: Full message flow works without bus-agent running.

### Phase 3: SI removes direct inber connection

**Goal**: SI only talks to bus.

1. Remove `feed/inber_direct.go`
2. Remove `feed/echo.go` if unused
3. Update `feed/bus.go` to subscribe to `outbound` + `events` topics
4. Add chat UI with per-agent tabs
5. SI publishes to `inbound` with agent metadata from selected tab

**Test**: Type in SI chat tab → message flows through bus → inber responds → SI displays response.

### Phase 4: Adapters talk to bus directly (optional, future)

**Goal**: Discord/Signal adapters publish to bus without going through SI.

1. Each adapter gets its own bus client
2. Adapters publish to `inbound` with channel metadata
3. Adapters subscribe to `outbound` filtered by their channel
4. SI still sees everything (subscribes to all outbound)

Currently adapters go through SI's router which publishes to bus. This works fine — phase 4 is optional and only matters if SI being down should not block adapter messages.

## What Each Project Owns

| Project | Owns | Talks To |
|---------|------|----------|
| SI | User interface, chat UI, adapter bridge | Bus only |
| Bus | Pub/sub, persistence, queueing | Everyone (passive) |
| Inber | Agent runtime, routing, spawns, workspaces | Bus, Forge, Model APIs |
| Forge | Git workspaces, deploys | Called by Inber (library) |

## Key Principles

- **Bus is dumb** — pub/sub + persistence, no routing logic
- **Inber owns routing** — channel→agent resolution lives in inber server
- **SI sees everything** — subscribes to all outbound + events for full visibility
- **Agents are equal** — orchestrators just have more tools, not different architecture
- **Events are publish-only** — inber announces what it's doing, doesn't wait for bus approval
