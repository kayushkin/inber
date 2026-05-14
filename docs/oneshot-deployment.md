# `/api/oneshot` — Deployment Spec

Status: endpoint code is committed (`a148065`), inber-server cannot currently start on the host so the endpoint is unreachable. This doc tracks the remaining changes another agent needs to make in this repo (and one host-side change) to get oneshot live.

## Goal

`POST /api/oneshot` is a stateless single-turn LLM call used by lightweight automation (kanban classifier, future renamer/scoper/curator services) that need cheap, schema-forced structured output without spinning up a real agent session.

Wire contract is the canonical `msg.OneShotRequest` / `msg.OneShotResponse` defined in `~/repos/llm-bridge/msg/oneshot.go`. The full call chain looks like:

```
agent-job-store (future) ─▶ llm-bridge-server POST /instances/{id}/oneshot
                              └▶ exec llm-bridge-inber -oneshot (stdin: OneShotRequest JSON)
                                   └▶ HTTP POST localhost:8200/api/oneshot
                                        └▶ inber → Anthropic SDK (hard-forced tool_choice)
```

All three callers (bridge-server proxy, harness binary, inber endpoint) are committed; the only thing missing is making inber-server actually run on this host.

## Already done (commit a148065)

- `server/api_oneshot.go` — handler. Builds `anthropic.MessageNewParams` directly, sets `ToolChoice = OfTool{Name: "output"}` when a schema is provided. Bypasses sessions, queue, memory, agent loop entirely. Uses `ANTHROPIC_API_KEY` from process env via `anthropic.NewClient()`.
- `server/api.go` — route registered at line 16.
- `memory/memory.go` — dropped stale `Session = asmemory.Session` re-export (memory-store Phase II.B retired it).
- `agent/registry/registry_test_mocks.go` — dropped dead `SaveSession` mock that referenced the same retired type.
- `server/api_agent_config.go:73` — `a.Orchestrator` → `a.Harness` (agent-store rename fallout).

Without those last three the `server` package didn't build.

## Required changes (deploy blockers)

### 1. Make inber-server startable when agent-store is broken

**Where:** `cmd/inber-server/main.go` (the selftest path — find the `selftest` call near the bottom of `main()` / `run()`) and `server/server.go` (where `agentStore` is opened around line 142-149).

**Problem:** The systemd unit fails on startup with:
```
{"level":"error","message":"FAIL: agent-store not available","component":"selftest"}
error: selftest: 1 critical check(s) failed
```
…because of the in-flight `orchestrator → harness` rename in `~/repos/agent-store` — its migration has a stale `orchestrator_id` reference that's failing against the on-host DB. **Don't fix agent-store from here** — that's its own work-in-progress.

**Fix in inber:** add a flag that demotes agent-store from "critical" to "optional" in the selftest. Two acceptable shapes:

- `--require=nats,workspace` (allowlist of critical checks; everything else is best-effort)
- `--allow-missing-agent-store` (single-purpose flag)

I'd lean toward the allowlist — it generalizes to other future flaky dependencies. The selftest already separates "FAIL" vs "WARN"; this flag just demotes specific checks from FAIL→WARN.

Acceptance: with the flag set in the systemd unit, inber-server starts and serves `/api/health` even when agent-store can't be opened. Agent-store-dependent endpoints (`/api/agents`, `/api/agents/config`) can 503 — that's fine, the oneshot path doesn't touch them.

### 2. Resolve `ANTHROPIC_API_KEY` from auth-store at startup

**Where:** `cmd/inber-server/main.go` — add a flag, resolve early in `run()`, set the env var before any agent code reads it.

**Problem:** The systemd unit doesn't set `ANTHROPIC_API_KEY`. Even with (1) fixed, oneshot calls will fail with `401 Unauthorized` from Anthropic.

**Fix:** new flag `--api-key-from-auth-store <app-name>` that:
1. At startup, calls `GET http://localhost:8303/api/resolve/anthropic` with headers:
   - `X-Auth-App: <app-name>` (defaults to `inber-server`)
   - `X-Auth-Reason: inber-server startup`
   - `Authorization: Bearer ${AUTH_STORE_TOKEN}` (env)
2. Parses the response (`api_key` field for `auth_type=api_key`, `access_token` for `oauth`/`token`).
3. Sets `os.Setenv("ANTHROPIC_API_KEY", secret)` before agent code initializes.
4. Fails loud on resolve error (`return fmt.Errorf("resolve anthropic credential: %w", err)`) — don't fall back to ambient env, that violates "single source of truth" in CLAUDE.md.

Reference implementation already exists at `~/repos/scheduler/cmd/kanban-classifier/main.go:756-782` (the auth-store HTTP call). The auth-store client wrapper in `~/repos/llm-bridge-server/internal/authstoreclient/client.go` is a fuller Go client if you want to lift its `Resolve` / `ResolveByProvider` methods into inber rather than duplicating the HTTP plumbing.

Pin the credential routing first by setting `intended_app='inber-server'` on the credential row in auth-store (`sqlite3 ~/.config/auth-store/auth.db`). Without that, the resolver falls back to the dry generic key (`cred_aiauth_anthropic_api`) and oneshot calls will hit "credit balance too low" — same outage that started this whole thread.

### 3. Update the systemd unit

**Where:** `~/.config/systemd/user/inber-server.service` (host-side, NOT in the repo).

Per CLAUDE.md feedback memory `oss-prep-systemd`: keep `User=` / `Environment=PATH,HOME` lines intact; the inber repo should ship a *template* that the deploy script writes a host-local drop-in for.

Current unit's `ExecStart=/home/kayushkincom/bin/inber serve --addr :8200` becomes:

```ini
ExecStart=/home/kayushkincom/bin/inber --addr :8200 --require=nats,workspace --api-key-from-auth-store=inber-server
Environment=AUTH_STORE_URL=http://127.0.0.1:8303
Environment=AUTH_STORE_TOKEN=changeme
Environment=NATS_URL=nats://localhost:4222
Environment=LOGSTACK_URL=http://localhost:8088
Environment=HOME=/home/kayushkincom
```

(Note: the current `serve` positional arg is silently swallowed by flag parsing — the binary is built from `cmd/inber-server/main.go` which only has `-addr` and `-config` flags. Removing `serve` from the ExecStart is a cosmetic cleanup.)

If inber has a `deploy.sh` analog like the rest of the ecosystem, the systemd unit template + host drop-in should land there. If not, leave a `docs/systemd/inber-server.service.template` and document the install steps.

## Optional follow-ups (not blockers)

### 4. Per-request API key override on `/api/oneshot`

Right now inber-server reads `ANTHROPIC_API_KEY` once at startup. For multi-credential routing (e.g. classifier uses one key, harness-watch uses another), one option is to accept `X-Anthropic-Key` (or a more abstracted `X-Auth-App`) header on the oneshot request and pass it to `anthropic.NewClient(option.WithAPIKey(...))`.

Defer this until we actually need it — for v1 the systemd unit + auth-store routing is enough.

### 5. Caching headers

`anthropic.NewClient()` (in `server/api_oneshot.go`) creates the client without the `anthropic-beta: prompt-caching-2024-07-31` header. Stateless oneshot calls don't benefit from cross-call caching, but for very long prompts (>1024 tokens) caching within a single call can still help. Easy add if metrics show it's worth it.

### 6. Cost tracking

`OneShotResponse.Usage` already carries token counts. If we want per-app cost tracking later (which jobs cost what), inber-server should log each oneshot to a usage table. Out of scope for v1.

## Verifying the deploy

Once 1–3 are done, an external smoke test confirms the full chain:

```bash
# direct against inber
curl -sfS -X POST http://localhost:8200/api/oneshot \
  -H 'Content-Type: application/json' \
  -d '{"prompt":"Classify the sentiment.","schema":{"type":"object","properties":{"sentiment":{"type":"string","enum":["positive","negative","neutral"]}},"required":["sentiment"]}}' \
  | jq

# through llm-bridge-server (after creating an inber instance in harness-store)
curl -sfS -X POST http://localhost:8160/instances/<inber-instance-id>/oneshot \
  -H 'Content-Type: application/json' \
  -d '{...same body...}' | jq
```

A successful response has a `parsed` field containing the schema-conformant JSON object — not stringified, not in `text`. If `parsed` is empty and `stop_reason != "tool_use"`, hard-forcing failed and needs debugging.

## Cross-repo references

| Repo | Commit | What |
|---|---|---|
| inber | `a148065` | `/api/oneshot` endpoint + consumer-side rename fixups (this repo) |
| llm-bridge | `9fcee4a` | `msg.OneShotRequest`/`Response` + `OneShotCapable` marker |
| llm-bridge-inber | `e60a7d7` | `-oneshot` flag mode |
| llm-bridge-server | `e413e13` | `POST /instances/{id}/oneshot` proxy |
