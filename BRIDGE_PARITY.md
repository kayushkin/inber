# llm-bridge Feature Parity Tracker

Tracks what inber needs to implement to support every llm-bridge capability.
Current declared capabilities in llm-bridge-server: `["model"]`
Target: `["compact", "fork", "model", "effort", "tools", "budget", "system_prompt"]`

## Status Legend

- **DONE** — implemented and wired end-to-end (inber backend + api_bridge.go + llm-bridge-inber harness)
- **BACKEND** — inber has the machinery but the bridge layer doesn't expose it
- **MISSING** — not implemented anywhere in the chain

---

## 1. Harness Protocol Operations

These are methods the harness binary (llm-bridge-inber) receives on stdin from llm-bridge-server.

| Method | Status | Notes |
|--------|--------|-------|
| `start` | DONE | Creates session via `POST /sessions` |
| `message` | DONE | Sends message via `POST /sessions/{id}/send` with SSE streaming |
| `resume` | MISSING | Harness has `Resume` field in startParams but ignores it. Inber can reload sessions from DB but no bridge endpoint exists. |
| `fork` | BACKEND | `server.forkSession()` exists, `handleForkSpawn` endpoint exists, but no bridge-compatible `POST /sessions/{id}/fork` endpoint and harness doesn't handle the method. |
| `config` | BACKEND | Agent config is mutable (`PATCH /api/agents/config` exists) but no bridge-compatible `POST /sessions/{id}/config` endpoint. Harness doesn't handle the method. |
| `compact` | BACKEND | `conversation.ManageConversation()` and `SummarizeConversation()` exist. `handleMemoryCompact` exists for memory. Harness stubs it as "not supported". No bridge endpoint for context compaction. |
| `interrupt` | DONE | SIGINT handler emits idle state. Bridge `POST /sessions/{id}/interrupt` calls `StopSession()`. |
| `stop` | DONE | Bridge `POST /sessions/{id}/stop` calls `StopSession()`. |

### TODO: Harness (llm-bridge-inber/main.go)

1. **`resume`** — When `start` has `resume: true`, call a resume endpoint instead of creating a new session. Restore conversation history.
2. **`fork`** — Add handler that calls `POST /sessions/{id}/fork` on inber.
3. **`config`** — Add handler that calls `POST /sessions/{id}/config` on inber.
4. **`compact`** — Replace stub with actual call to `POST /sessions/{id}/compact` on inber.

---

## 2. Inber Bridge API Endpoints (api_bridge.go)

Endpoints llm-bridge-server expects when driving inber directly or via the harness.

| Endpoint | Status | Notes |
|----------|--------|-------|
| `GET /health` | DONE | |
| `GET /harnesses` | DONE | |
| `GET /sessions` | DONE | |
| `POST /sessions` | DONE | |
| `GET /sessions/{id}` | DONE | |
| `POST /sessions/{id}/send` | DONE | Sync + SSE streaming |
| `GET /sessions/{id}/events` | DONE | SSE subscription |
| `POST /sessions/{id}/stop` | DONE | |
| `POST /sessions/{id}/interrupt` | DONE | Currently identical to stop — should pause without killing |
| `GET /sessions/{id}/history` | DONE | |
| `POST /sessions/{id}/resume` | MISSING | Need endpoint to resume an idle session with its conversation intact |
| `POST /sessions/{id}/fork` | MISSING | Need bridge-compatible fork (not spawn-based). Should clone conversation up to a point and return new session ID. |
| `POST /sessions/{id}/config` | MISSING | Need endpoint to update model, effort/thinking budget, disabled tools, max token budget mid-session |
| `POST /sessions/{id}/compact` | MISSING | Need endpoint to trigger conversation summarization/pruning with optional user-provided summary |
| `GET /sessions/{id}/discover` | MISSING | Discover on-disk sessions. Inber has DB sessions — expose stored sessions for this session's agent. |
| `GET /sessions/{id}/messages` | MISSING | Materialized message history (distinct from raw event history). Could proxy to existing `/api/sessions/{id}/context`. |

### TODO: api_bridge.go additions

1. **`POST /sessions/{id}/resume`** — Load session from DB, restore Engine messages, set state to idle, return session info.
2. **`POST /sessions/{id}/fork`** — Clone session conversation, create new session key, copy messages up to current point, return new session.
3. **`POST /sessions/{id}/config`** — Accept `{model, effort, disabled_tools, max_budget}`. Update the session's Engine/Agent config in-place.
4. **`POST /sessions/{id}/compact`** — Accept optional `{summary}`. Run `ManageConversation` or `SummarizeConversation` on the session's messages.
5. **`GET /sessions/{id}/discover`** — Query DB for stored sessions belonging to the same agent.
6. **`GET /sessions/{id}/messages`** — Return materialized message list from session's Engine.

---

## 3. Capability Registration

File: `llm-bridge-server/internal/server/health.go:59`

Current:
```go
msg.HarnessInber: {"model"},
```

Target (after all features are implemented):
```go
msg.HarnessInber: {"compact", "fork", "model", "effort", "tools", "budget", "system_prompt"},
```

---

## 4. Event Parity

Events that llm-bridge supports but inber doesn't emit through the bridge.

| Event Type | Status | Notes |
|------------|--------|-------|
| `EventResult` | DONE | Emitted on "done" stream event |
| `EventStream` | DONE | Text deltas |
| `EventToolCall` | DONE | |
| `EventToolResult` | DONE | |
| `EventThinking` | DONE | |
| `EventSystem` | DONE | Status messages |
| `EventError` | DONE | |
| `EventSessionState` | DONE | State transitions |
| `EventApproval` | MISSING | Inber's guard system has `NeedsApproval` verdicts but these aren't surfaced as bridge events. The guard currently blocks in-process — needs to emit an approval event and wait for response. |
| `EventPlan` | MISSING | Inber doesn't emit plan events. Could surface the scratchpad/task tool output as plan events. |

### TODO: Event additions

1. **Approval events** — When guard returns `NeedsApproval`, emit `EventApproval` with tool name, command, file path. Add a mechanism for the bridge to send approval/denial back.
2. **Plan events** — When the agent uses the task/scratchpad tool, emit `EventPlan` with the plan text.

---

## 5. Interrupt vs Stop Distinction

Currently `handleBridgeInterrupt` and `handleBridgeStop` both call `StopSession()` which cancels the context. They should behave differently:

- **Interrupt** (`POST /sessions/{id}/interrupt`): Pause the current turn. Session goes to `idle` state. Conversation preserved. Can send new messages.
- **Stop** (`POST /sessions/{id}/stop`): Terminate the session. Session goes to `aborted` state. No further messages.

### TODO

1. Add `InterruptSession(key)` to inber server that cancels only the current API call context (not the session context), preserves partial results, and sets state to idle.
2. Keep `StopSession(key)` as full termination.
3. Update `handleBridgeInterrupt` to call `InterruptSession`.

---

## 6. Session Resume Flow

Inber stores sessions in SQLite with full turn history. The resume flow needs:

1. **Harness**: When `start` params include `resume: true` and `session_id`, call resume endpoint instead of create.
2. **Bridge endpoint**: `POST /sessions/{id}/resume` — load session from DB, reconstruct Engine with message history, register in active sessions map.
3. **Conversation repair**: Run `conversation.Repair()` on loaded messages (already exists).

---

## 7. Dynamic Config Update

llm-bridge's `ConfigSessionRequest` supports:
- `model` — switch model mid-session
- `effort` — change thinking budget (maps to inber's `Thinking` field)
- `disabled_tools` — disable specific tools
- `max_budget` — set token budget cap

### TODO

1. **Bridge endpoint** `POST /sessions/{id}/config`:
   - `model` → update `session.Engine.Model` and recreate API client
   - `effort` → map to thinking budget: `"high"` = current budget, `"low"` = 0, or accept raw token count
   - `disabled_tools` → filter tool list on Engine
   - `max_budget` → update guard's `MaxInputTokens` limit
2. **Harness**: Add `config` method handler that forwards to the endpoint.

---

## Implementation Status

### Done (2026-04-14)

1. **Interrupt vs Stop** — `InterruptSession` added to server, bridge endpoint updated
2. **Config** — `POST /sessions/{id}/config` endpoint + harness `config` method
3. **Resume** — `POST /sessions/{id}/resume` endpoint + harness `start` with `resume:true`
4. **Compact** — `POST /sessions/{id}/compact` endpoint + harness `compact` method
5. **Fork** — `POST /sessions/{id}/fork` endpoint + harness `fork` method
6. **Discover** — `GET /sessions/{id}/discover` endpoint
7. **Messages** — `GET /sessions/{id}/messages` endpoint
8. **Capability registration** — Updated to `["compact", "fork", "model", "effort", "tools", "budget"]`
9. **Harness methods** — `resume`, `fork`, `config`, `compact`, `interrupt`, `stop` all wired

### Remaining (future work)

1. **Approval events** — Guard's `CheckTool` exists but is never called from the agent execution loop. Wiring this requires changes to `agent.Run()` to check guard verdicts before executing tools and emit `EventApproval` events when `NeedsApproval` is returned.
2. **Plan events** — No plan/task tool exists yet. Needs a new tool that emits `EventPlan` via display hooks.
3. **system_prompt capability** — Not yet declared. The system prompt can be overridden at session creation (`RunRequest.System`) but not updated mid-session via the bridge config endpoint.
