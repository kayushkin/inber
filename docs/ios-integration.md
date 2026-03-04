# iOS Integration Design

This document outlines how iOS (Shortcuts and OpenClaw iOS app) can integrate with inber agents.

## Architecture Overview

```
┌─────────────────┐
│  iOS Shortcuts  │──────┐
└─────────────────┘      │
                         ▼
┌─────────────────┐   ┌──────────────────────┐
│ OpenClaw iOS    │──▶│  OpenClaw Gateway    │
│ App (Future)    │   │  (HTTP + WebSocket)  │
└─────────────────┘   └──────────────────────┘
                                 │
                    ┌────────────┼────────────┐
                    ▼            ▼            ▼
              ┌──────────┐ ┌──────────┐ ┌──────────┐
              │  inber   │ │ OpenClaw │ │  Other   │
              │  agents  │ │  agents  │ │  agents  │
              └──────────┘ └──────────┘ └──────────┘
```

## Part 1: iOS Shortcuts → Inber Agents

### Recommended Approach: OpenClaw Hooks

iOS Shortcuts should call OpenClaw's existing hooks API, which then spawns inber agents.

**Why this approach:**
1. **Reuses existing infrastructure** - OpenClaw already has HTTP hooks
2. **Secure authentication** - Token-based auth already implemented
3. **No new code needed** - OpenClaw can already spawn inber agents
4. **Rate limiting built-in** - OpenClaw handles this

### Shortcuts Configuration

**URL:** `http://YOUR_SERVER:18789/hooks/agent`

**Method:** `POST`

**Headers:**
- `Authorization: Bearer YOUR_HOOKS_TOKEN`
- `Content-Type: application/json`

**Body:**
```json
{
  "message": "Your task for the agent",
  "agentId": "task-manager",
  "wakeMode": "now",
  "channel": "last"
}
```

**Response:**
```json
{
  "ok": true,
  "runId": "uuid-here"
}
```

### Shortcuts Example: "Ask Inber" Action

1. **Get Input:** Ask for text "What do you need help with?"
2. **HTTP Request:**
   - URL: `http://your-server:18789/hooks/agent`
   - Method: POST
   - Headers:
     ```
     Authorization: Bearer YOUR_TOKEN
     Content-Type: application/json
     ```
   - Body: `{"message":"${Input}","agentId":"task-manager"}`
3. **Show Result:** Display the runId or success message

### Alternative: Direct HTTP API (Future Enhancement)

If you need synchronous responses, we can add a new HTTP endpoint to inber:

```go
// cmd/inber/serve.go
// New command: inber serve --port 8765

POST /api/agent/:name
{
  "message": "task description",
  "context": {} // optional
}

Response:
{
  "text": "agent response",
  "tokens": {"in": 100, "out": 50},
  "tools": 2
}
```

This would require:
1. Adding a new `serve` command to inber
2. HTTP server with authentication
3. Connection to OpenClaw or direct agent execution

## Part 2: OpenClaw iOS App → Gateway

The OpenClaw iOS app already exists (super-alpha) and connects via WebSocket.

### Current State

- **Client ID:** `openclaw-ios` (already defined in OpenClaw)
- **Connection:** WebSocket to `ws://gateway:18789/ws`
- **Authentication:** Token-based
- **Role:** Node (can receive commands from gateway)

### iOS App Features (from README)

- Pairing via setup code
- Chat + Talk surfaces through gateway
- Node commands: camera, canvas, screen record, location, etc.
- Share extension deep-linking

### Connection Flow

```
1. iOS app connects via WebSocket
2. Gateway sends connect.challenge
3. iOS sends connect request with:
   - client.id: "openclaw-ios"
   - role: "operator" (for chat) or "node" (for remote control)
   - auth.token: user's gateway token
4. Gateway validates and sends hello-ok
5. iOS can now:
   - Send messages to agents
   - Receive agent responses
   - Execute node commands (if role=node)
```

### iOS Shortcuts → OpenClaw iOS App

Shortcuts can also interact with the iOS app via deep links:

```
openclaw://chat?message=Hello%20World&agent=task-manager
```

This would require implementing URL scheme handling in the iOS app.

## Configuration

### OpenClaw Gateway Config

```yaml
# ~/.openclaw/config.yaml
hooks:
  enabled: true
  token: your-secure-token-here
  path: /hooks
  allowedAgentIds:
    - task-manager
    - fionn
    - researcher
```

### Inber Agents Config

```json
// agents.json
{
  "openclaw": {
    "url": "ws://localhost:18789/ws",
    "token": "your-gateway-token",
    "agents": ["kayushkin", "downloadstack"]
  }
}
```

## Implementation Checklist

### Phase 1: iOS Shortcuts via OpenClaw Hooks (Recommended)

- [x] OpenClaw has hooks API
- [x] OpenClaw can spawn inber agents
- [x] Document the exact Shortcuts configuration (see ios-quickref.md and ios-shortcuts-guide.md)
- [x] Create a "Ask Inber" shortcut template (see shortcut-template.json)
- [x] Test end-to-end: Shortcut → OpenClaw → inber (verified via test-ios-hooks.sh)

### Phase 2: Direct HTTP API (Optional)

- [ ] Add `inber serve` command
- [ ] Implement `/api/agent/:name` endpoint
- [ ] Add authentication middleware
- [ ] Return structured JSON response
- [ ] Update Shortcuts to use direct API

### Phase 3: iOS App Integration (Future)

- [ ] Implement URL scheme (`openclaw://`)
- [ ] Add Siri Intent for "Ask Inber"
- [ ] Widget for quick agent queries
- [ ] Background fetch for notifications

## Security Considerations

1. **Token Storage:** Shortcuts stores token in Keychain
2. **HTTPS:** Use TLS for production deployments
3. **Rate Limiting:** OpenClaw handles this
4. **Agent Allowlist:** Configure which agents Shortcuts can call

## Testing

### Manual Test via curl

```bash
# Test hooks endpoint
curl -X POST http://localhost:18789/hooks/agent \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"message":"Say hello","agentId":"task-manager"}'
```

### Test in Shortcuts

1. Create new Shortcut
2. Add "Get Contents of URL" action
3. Configure as POST with headers and body
4. Add "Show Result" action
5. Test on device

## Summary

**Recommended approach for iOS Shortcuts:**
1. Use OpenClaw's existing `/hooks/agent` endpoint
2. Configure Shortcuts to POST with proper auth
3. OpenClaw spawns the inber agent and returns runId

**For OpenClaw iOS app:**
1. Already implemented (super-alpha)
2. Connects via WebSocket with `openclaw-ios` client ID
3. Can chat with agents or receive remote commands

**Future enhancement:**
- Add direct HTTP API to inber if synchronous responses are needed
- Implement URL schemes in iOS app for deep linking
