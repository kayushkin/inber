# iOS Shortcuts Guide

This guide shows you how to create iOS Shortcuts that call inber agents through OpenClaw.

## Prerequisites

1. OpenClaw gateway running and accessible from iOS
2. Hooks token configured in OpenClaw
3. Network access from iOS to OpenClaw server

## OpenClaw Configuration

First, ensure hooks are enabled in your OpenClaw config:

```yaml
# ~/.openclaw/config.yaml
hooks:
  enabled: true
  token: "your-secure-token-here"  # Generate a secure random token
  path: /hooks
  allowedAgentIds:
    - "*"  # Or list specific agents: ["task-manager", "fionn"]
  defaultSessionKey: "ios-shortcut"
  allowRequestSessionKey: true
  allowedSessionKeyPrefixes:
    - "ios:"
```

## Shortcut 1: Quick Ask

A simple shortcut that sends a message to an agent.

### Steps:

1. **Ask for Input**
   - Prompt: "What do you need?"
   - Input Type: Text

2. **Get Contents of URL**
   - URL: `http://YOUR_SERVER:18789/hooks/agent`
   - Method: POST
   - Headers:
     - Authorization: `Bearer YOUR_TOKEN`
     - Content-Type: `application/json`
   - Request Body: JSON
     ```json
     {
       "message": "[Ask for Input]",
       "agentId": "task-manager",
       "wakeMode": "now"
     }
     ```

3. **Show Result**
   - Text from JSON response `ok` and `runId`

### Export for Import:

You can create this manually or use the template below.

## Shortcut 2: Async Task with Callback

For longer tasks, use async mode and check status later.

### Steps:

1. **Ask for Input** (task description)
2. **Get Contents of URL** (submit task)
   ```json
   {
     "message": "[Input]",
     "agentId": "fionn",
     "wakeMode": "now",
     "sessionKey": "ios:task-{{timestamp}}"
   }
   ```
3. **Show Notification** with runId

## Shortcut 3: Predefined Tasks

Quick actions for common tasks.

### Examples:

**"Summarize Clipboard"**
```json
{
  "message": "Summarize this text: [[Clipboard]]",
  "agentId": "researcher",
  "wakeMode": "now"
}
```

**"Fix Code Error"**
```json
{
  "message": "Explain and fix this error: [[Clipboard]]",
  "agentId": "fionn",
  "wakeMode": "now"
}
```

**"Create Task"**
```json
{
  "message": "Create a task: [Ask for Input]",
  "agentId": "task-manager",
  "wakeMode": "now"
}
```

## Testing from Terminal

Before setting up iOS, test the endpoint:

```bash
# Test basic agent call
curl -X POST http://localhost:18789/hooks/agent \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"message":"Say hello","agentId":"task-manager"}'

# Expected response:
# {"ok":true,"runId":"uuid-here"}
```

## Network Considerations

### Local Network (Home/Office)
- Use local IP: `http://192.168.1.100:18789`
- Ensure iOS device is on same network

### Remote Access
Options:
1. **Tailscale** - Recommended, OpenClaw supports it
2. **VPN** - Connect to home network
3. **Public IP with TLS** - Use reverse proxy (nginx/caddy)

### Tailscale Setup
1. Install Tailscale on server and iOS
2. Use Tailscale IP: `http://100.x.y.z:18789`
3. Or use MagicDNS: `http://your-server.tailnet-name.ts.net:18789`

## Advanced: Siri Integration

### Option 1: Add to Siri
1. Create shortcut
2. Tap share button
3. "Add to Siri"
4. Record phrase: "Ask Inber to..."

### Option 2: Automations
1. Create Personal Automation
2. Trigger: Time of day / Location / etc.
3. Action: Run your shortcut

## Troubleshooting

### "Could not connect to server"
- Check server is running: `openclaw gateway status`
- Check firewall allows port 18789
- Try from browser: `http://server:18789/` (should show "Not Found")

### "Unauthorized"
- Check token matches config
- Check Authorization header format: `Bearer TOKEN`
- Check hooks are enabled in config

### "Agent not found"
- Check agentId is correct
- Check agent is in allowedAgentIds (if configured)
- Check inber agents.json has the agent

## Response Format

### Success (Async)
```json
{
  "ok": true,
  "runId": "550e8400-e29b-41d4-a716-446655440000"
}
```

### Error
```json
{
  "ok": false,
  "error": "error message here"
}
```

## Future: Synchronous Responses

Currently hooks are async (fire-and-forget). For synchronous responses where iOS waits for the agent's reply, we would need to:

1. Add a `/hooks/agent/sync` endpoint, or
2. Poll for results using runId, or
3. Add direct HTTP API to inber

Let me know if you need synchronous responses for your use case!
