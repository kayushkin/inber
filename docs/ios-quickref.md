# iOS Integration Quick Reference

## TL;DR

**iOS Shortcuts → OpenClaw Agents:**
```bash
curl -X POST http://localhost:18789/hooks/agent \
  -H "Authorization: Bearer inber-ios-hooks-2026" \
  -H "Content-Type: application/json" \
  -d '{"message":"Your task","agentId":"inber","wakeMode":"now"}'
```

**Response:** `{"ok":true,"runId":"uuid"}`

## Working Configuration

**OpenClaw Gateway:** `localhost:18789` (or `192.168.x.x:18789` from LAN)
**Hooks Token:** `inber-ios-hooks-2026`
**Hooks Enabled:** Yes (all agents allowed via `["*"]`)

## OpenClaw Config (Current)

```json
// ~/.openclaw/openclaw.json
{
  "hooks": {
    "enabled": true,
    "token": "inber-ios-hooks-2026",
    "allowedAgentIds": ["*"]
  }
}
```

## Available OpenClaw Agents

From `~/.openclaw/openclaw.json`:
- **main** - Default Claxon agent (🦀)
- **inber** - Inber agent workspace
- **kayushkin** - kayushkin.com workspace
- **downloadstack** - DownloadStack project
- **claxon-android** - Android integration
- **inber-party** - Inber Party UI
- **agent-dashboard** - Agent monitoring dashboard
- **agent-watchdog** - Agent health watchdog
- **si** - SI project
- **agent-bench** - Agent benchmarking
- **argraphments** - Argraphments project
- **session-stream** - Session Stream project

## Legacy Inber Agents (Internal)

From `/home/slava/life/repos/inber/agents.json`:
- **task-manager** - Orchestrator, primary dispatcher
- **fionn** - Coder, code implementation
- **scathach** - Tester, validation
- **oisin** - Courier, deployment/git
- **bran** - Pipeline coordinator
- **researcher** - Research/analysis
- **orchestrator** - Task delegation
- **party** - Fullstack dev for inber-party
- **worker** - General-purpose, simple tasks

## OpenClaw iOS App

The OpenClaw iOS app (super-alpha) can connect directly to the gateway:

**Client ID:** `openclaw-ios` (pre-defined in OpenClaw)
**Connection:** WebSocket to `ws://SERVER:18789/ws`
**Authentication:** Gateway token: `8a8b770d8433b3cd93b8c2cc9263a79a9eac17800ab5c92c`

**Connection Flow:**
1. iOS app connects via WebSocket
2. Gateway sends `connect.challenge`
3. iOS sends connect request with:
   - `client.id: "openclaw-ios"`
   - `role: "operator"` (for chat) or `"node"` (for remote control)
   - `auth.token: <gateway-token>`
4. Gateway validates and sends `hello-ok`
5. iOS can send/receive messages to agents

## Network Options

| Option | URL Format | Notes |
|--------|-----------|-------|
| Local | `http://192.168.x.x:18789` | Same WiFi network |
| Tailscale | `http://100.x.y.z:18789` | Recommended for remote |
| Public | `https://your.domain.com` | Requires reverse proxy + TLS |

## Test Commands

```bash
# Check gateway status
openclaw gateway status

# Test hooks endpoint
./test-ios-hooks.sh localhost:18789 YOUR_TOKEN

# Test inber directly
./inber run -a task-manager "Say hello"
```

## iOS Shortcut Steps

1. **Ask for Input** → Get user's message
2. **Get Contents of URL** → POST to `/hooks/agent`
3. **Show Result** → Display response

## Files

| File | Purpose |
|------|---------|
| `docs/ios-integration.md` | Full design doc |
| `docs/ios-shortcuts-guide.md` | Shortcut setup guide |
| `docs/shortcut-template.json` | Shortcut template |
| `test-ios-hooks.sh` | Test script |
