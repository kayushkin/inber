# OpenClaw Integration

Inber and OpenClaw coexist as two orchestrators connected via the bus.

## Architecture

```
Dashboard (SI) → Bus (inbound) → { inber-server, openclaw-bus } → agents
                  Bus (outbound) ← { inber-server, openclaw-bus } ← responses
```

Each message on the bus has an `orchestrator` field ("inber" or "openclaw"). Each runtime filters for its own messages.

## openclaw-bus Adapter

Standalone Go service (`github.com/kayushkin/openclaw-bus`) that bridges OpenClaw to the bus:

1. **Bus → OpenClaw**: Subscribes to `inbound` (orchestrator:"openclaw"), forwards to OpenClaw chat completions API
2. **OpenClaw → Bus**: Streams text deltas to `outbound` topic
3. **JSONL tailing**: Watches OpenClaw session files for tool_call/thinking events (not exposed by SSE API)
4. **Logpush**: Polls session files, pushes conversation history to logstack

Session key format: `agent:<agentId>:main`

## Agent Registry

Both orchestrators register their agents with the bus. The dashboard (`/api/agents`) merges them:
- Inber agents: from agent-store SQLite
- OpenClaw agents: from OpenClaw gateway config

## Shared Resources

**model-store** (port 8150) — shared model/credential management. Both orchestrators use it for API keys and health tracking.

**agent-store** — shared agent registry SQLite. Contains agent names, roles, models, system prompts.

## Systemd Services

- `inber-server.service` — inber on port 8200
- `openclaw-bus.service` — openclaw adapter (BUS_URL, OPENCLAW_URL, OPENCLAW_TOKEN, LOGSTACK_URL)
