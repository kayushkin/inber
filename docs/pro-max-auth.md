# Claude Pro Max Subscription — Auth Setup for Inber

## Overview

Slava has a Claude Pro Max subscription ($200/mo) that provides API access via OAuth tokens instead of standard API keys. This doc covers how auth flows through the system and what needs maintenance.

## Architecture

```
Claude Max Sub → OAuth tokens (access + refresh)
                     ↓
              Two consumers:
              
  1. OpenClaw (pi-ai library)     2. Inber (model-store → aiauth)
     └─ auth-profiles.json            └─ model-store SQLite DB
        ~/.openclaw/agents/main/          ~/.config/model-store/store.db
        agent/auth-profiles.json
```

### The Problem: Two Independent Refresh Cycles

Both OpenClaw and Inber can refresh expired OAuth tokens, but they don't stay in sync:

- **OpenClaw** uses `@mariozechner/pi-ai` to refresh tokens → writes to `auth-profiles.json`
- **Inber** uses `model-store` → `aiauth` library to refresh → writes to `store.db`
- **Anthropic refresh tokens are single-use** — when one system refreshes, the other's refresh token is burned

Whichever system refreshes first wins. The other is left with a dead refresh token.

### Current Flow

1. On startup, inber calls `SyncToAuthProfiles("")` — pushes model-store credentials → auth-profiles.json
2. OpenClaw reads auth-profiles.json for its credentials
3. When tokens expire, OpenClaw's pi-ai refreshes and writes new tokens back to auth-profiles.json
4. **model-store DB is never updated with the new tokens** — inber has no `SyncFromAuthProfiles` call
5. Next time inber tries to refresh, it uses the burned refresh token → fails
6. Falls back to `anthropic:api` (standard API key, pay-per-use) silently

## Credential Layout

```
model-store DB (credentials table):
  anthropic:max-oauth | oauth   | priority 10 | Max subscription
  anthropic:api       | api_key | priority 50 | API key fallback

auth-profiles.json:
  anthropic:max-oauth  → oauth (access + refresh + expires)
  anthropic:manual     → token (derived from oauth, for OpenClaw compat)
  anthropic:api        → api_key
  lastGood.anthropic   → "anthropic:manual"
```

## What Needs to Happen

### Fix 1: Bidirectional Sync (Recommended)

Add `SyncFromAuthProfiles` call in inber's startup, **after** `SyncToAuthProfiles`:

```go
// In engine/engine_init.go setupModelStore():
store.SyncToAuthProfiles("")   // existing: push model-store → auth-profiles
store.SyncFromAuthProfiles("") // NEW: pull auth-profiles → model-store (picks up OpenClaw's refreshes)
```

`SyncFromAuthProfiles` already exists in model-store (`sync.go`) — it just needs to be called.

### Fix 2: Periodic Re-sync

When inber's server is running long-term, tokens can expire mid-run. Options:
- Re-sync from auth-profiles.json before each `ResolveForModel` call (expensive)
- Add a background goroutine that re-syncs every 30 min
- Accept that for long-running server, a restart picks up fresh tokens

### Fix 3: Single Source of Truth (Ideal, More Work)

Stop OpenClaw from doing its own refresh. Make model-store the only refresher:
- Disable pi-ai's auto-refresh in OpenClaw config (if possible)
- Have model-store do all refreshing + sync to auth-profiles.json
- This eliminates the dual-refresh race entirely

## Maintenance Checklist

### When Token Expires and Both Systems Are Stale

1. Run `openclaw models auth setup-token --provider anthropic`
2. Generate a new setup token from claude.ai account settings
3. This resets both access and refresh tokens in auth-profiles.json
4. Restart inber (or trigger `SyncFromAuthProfiles`) to pick up new tokens

### When Only Inber's Token Is Stale

Run the model-store CLI to import from auth-profiles:
```bash
# If model-store has a CLI command:
model-store sync-from-openclaw

# Or manually via inber:
inber config  # shows current token status
```

Or restart inber — if Fix 1 is applied, it'll pick up OpenClaw's fresh tokens on startup.

### Checking Token Status

```bash
# Model-store DB:
sqlite3 ~/.config/model-store/store.db \
  "SELECT id, datetime(expires_at/1000, 'unixepoch', 'localtime') as expires FROM credentials WHERE auth_type='oauth';"

# Auth-profiles.json:
python3 -c "
import json, datetime
with open('$HOME/.openclaw/agents/main/agent/auth-profiles.json') as f:
    d = json.load(f)
for k,v in d['profiles'].items():
    if v.get('expires'):
        print(f\"{k}: expires {datetime.datetime.fromtimestamp(v['expires']/1000)}\")
"

# Compare tokens (should match):
python3 -c "
import json, sqlite3
conn = sqlite3.connect('$HOME/.config/model-store/store.db')
db_token = conn.execute(\"SELECT token FROM credentials WHERE id='anthropic:max-oauth'\").fetchone()[0]
with open('$HOME/.openclaw/agents/main/agent/auth-profiles.json') as f:
    ap_token = json.load(f)['profiles']['anthropic:max-oauth']['access']
print('IN SYNC' if db_token == ap_token else 'OUT OF SYNC — run SyncFromAuthProfiles')
"
```

## Current State (2026-03-25)

- ⚠️ model-store DB has a **stale** token (expired March 24)
- ✅ auth-profiles.json has a **fresh** token (expires March 25, auto-refreshed by OpenClaw)
- ⚠️ model-store refresh token is **burned** (different from auth-profiles)
- **Fix 1 is not yet applied** — inber will fail OAuth and fall back to API key
