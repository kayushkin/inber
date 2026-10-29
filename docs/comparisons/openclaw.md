# OpenClaw Comparison

**Last reviewed**: 2026-10-29 against openclaw/openclaw@2f47962 (2026-10-29).

**Framework**: Node.js-based personal AI assistant with multi-channel support  
**GitHub**: https://github.com/openclaw/openclaw  
**Architecture**: Gateway + WebSocket control plane with channel plugins  
**Studied**: March 2026 (the sections below are that study; see the update that follows for what has changed)

## Update, 2026-10-29

OpenClaw is alive and very active, now run by the OpenClaw Foundation. Changes since March that bear on this doc:

- **Heartbeat is retired.** `docs/gateway/heartbeat.md` is now a migration page: `openclaw doctor` turns each heartbeat monitor and `HEARTBEAT.md` checklist into an ordinary automation (cron) job with its own schedule, prompt, scratch space, model, session and delivery settings. Sections 2 and 5 below describe the old design.
- **Agent runtimes are plugins.** OpenClaw splits provider, model and agent runtime (`docs/concepts/agent-runtimes.md`). For Claude it can run the official `claude` binary as a CLI backend: `claude -p` with stream-json output, OpenClaw's prompt passed by `--append-system-prompt-file`, and OpenClaw's own tools offered to Claude through a loopback MCP bridge filtered by its tool policy (`docs/gateway/cli-backends.md`). This reuses an existing Claude Code login and its subscription limits. For OpenAI, the usual setup runs turns through a Codex app-server harness on a ChatGPT/Codex subscription. API-key routes remain for both.
- **Channels**: Discord, iMessage, Slack, Teams, Telegram, WhatsApp "and 20+ more", plus native apps for macOS, iOS, Android, Windows and Linux.

Inber changed more. It is no longer a single-user CLI: it is a long-lived multi-session server (`server/`), with the CLI split into the separate `inber-cli` repo. The present-tense claims about inber below have been corrected to match its code on 2026-10-29.

## What OpenClaw Does Well

### 1. **Session Management Architecture**
- **Clean session isolation**: Direct chats vs group chats get separate session keys
- **Flexible DM scoping**: `main` (continuity), `per-peer`, `per-channel-peer`, `per-account-channel-peer`
- **Identity linking**: Map same person across channels to single session
- **Auto-reset policies**: Daily at 4AM, idle timeouts, per-channel/per-type overrides
- **Session store as source of truth**: Gateway owns all state, UIs query via WebSocket

**Inber insight**: inber now runs many sessions at once, keyed by `agent:<name>:<scope>` in a `sync.Map` (`server/server.go`), with lane-based concurrency (`server/queue.go`) and SQLite persistence (`server/store.go`). It still has no channel-aware isolation or identity linking; channel routing happens outside inber, on the message bus.

### 2. **Heartbeat System (Proactive Agent)**
- **Batched periodic checks**: Single heartbeat replaces multiple cron jobs
- **Context-aware decisions**: Full session history for smart prioritization  
- **Smart suppression**: Returns `HEARTBEAT_OK` when nothing needs attention
- **Configurable schedules**: 30min default, active hours, delivery targets
- **HEARTBEAT.md checklist**: Agent reads markdown file with tasks to check

**Inber insight**: inber still has no proactive loop of its own; scheduled work comes from an outside scheduler calling inber's HTTP API. OpenClaw has since dropped heartbeat for plain cron jobs too (see the update above).

### 3. **Sophisticated Tool Policy System**
- **Layered security**: Sandbox mode with allowlist/denylist per tool
- **Session-aware policies**: Different tool access for main vs group sessions
- **Agent-to-agent controls**: `sessions_spawn`, `sessions_send` with visibility scoping
- **Per-session overrides**: Thinking level, model, tool access per session
- **Send policies**: Block delivery by channel type, session type, key prefix

**Inber insight**: No longer all-or-nothing. `guard/` gives each session a mode (observe: read-only tools; assist: writes with approval routing; autonomous: everything), checked on every tool call in `engine/build_hooks.go`. A request can also switch tools off per session (`no_tools`, and the disabled-tools list in `session/disabled_tools.go`), override the model and thinking budget, and set turn, token, cost and time limits (`RunRequest` in `server/server.go`). Spawn depth is capped by `max_spawn_depth` (`server/config.go`). There is still no per-channel or per-session-type policy and no send policy.

### 4. **Channel Abstraction Layer**
- **15+ channel plugins**: WhatsApp, Telegram, Discord, Slack, Signal, etc.
- **Unified message routing**: All channels → Gateway → Agent → Response delivery
- **Per-channel configuration**: Different models, tool policies, group settings
- **Media pipeline**: Images/audio/video with transcription hooks
- **Group management**: Mention gating, reply tags, activation modes

**Inber insight**: inber still has no channel plugins, but it is no longer CLI-bound. It takes messages from a NATS message bus (`bus/`, `server/bus.go`), where separate adapter services bridge chat platforms, and it serves HTTP routes including an llm-bridge-compatible set (`server/api_bridge.go`). `server/openclaw.go` can even forward a bus message to an OpenClaw gateway.

### 5. **Cron vs Heartbeat Design**
- **Cron for precision**: Exact timestamps, isolated sessions, model overrides
- **Heartbeat for awareness**: Batched monitoring, context-aware, cheaper
- **Smart scheduling**: Automatic load spreading for top-of-hour jobs
- **Session targeting**: `main` (system event), `isolated` (clean slate), custom

**Inber insight**: inber has neither inside itself; it leaves timing to an outside scheduler.

## What Inber Does Better

### 1. **Simplicity & Focus**
- **No channel layer**: No multi-channel or group management code; chat platforms stay in separate services on the bus
- **Direct tool execution**: Tools are Go functions called in-process; OpenClaw's Claude CLI backend reaches its tools over an MCP bridge
- **Short call chain**: Server → Engine → Agent → Tools
- **Go's advantages**: Faster startup, single binary, better resource efficiency

### 2. **Development Experience**
- **Type safety**: Go's compile-time guarantees vs Node.js runtime errors
- **Memory use**: Smaller footprint than a Node.js gateway (Go does have a garbage collector, with short pauses)
- **Deployment**: Single static binary vs Node.js + npm ecosystem

### 3. **Context/Memory Integration**
- **Built-in memory management**: Context compaction, memory extraction, conversation pruning
- **Egress redaction**: every provider client strips live secrets from outgoing requests (`redact/`, `agent/redaction.go`)
- **State**: one `messages.json` per session workspace with small sidecar files (`session/`), plus a SQLite store of sessions and requests

## What Inber Should Adopt

### 1. **Proactive Agent Pattern (HIGH PRIORITY in March; OpenClaw itself has since replaced heartbeat with cron jobs)**
```go
// engine/heartbeat.go - new file
type HeartbeatConfig struct {
    Interval     time.Duration
    ActiveHours  *ActiveHours  
    ChecklistPath string // HEARTBEAT.md
}

func (e *Engine) StartHeartbeat() {
    // Read HEARTBEAT.md, run checks, suppress if nothing urgent
}
```

**Why**: Major engagement gap. Users want assistants that check in proactively.

### 2. **Tool Permission Interface (MEDIUM PRIORITY; partly done: `guard.CheckTool` and execution modes, and `agent/provider.go` exists but is typed on Anthropic's SDK)**
```go
// agent/provider.go - extract interface
type Provider interface {
    Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)
}

type ToolPolicy interface {
    AllowTool(toolName string, context ToolContext) bool
}
```

**Why**: Enable safe tool execution in different contexts (automated vs manual).

### 3. **Session Store Interface (MEDIUM PRIORITY)**
```go
// engine/session.go - make swappable
type SessionStore interface {
    Get(key string) (*Session, error)
    Put(key string, session *Session) error
    List(filter SessionFilter) ([]*Session, error)
}
```

**Why**: Enable different backends (SQLite, filesystem, Redis) without coupling.

### 4. **Multi-Session Support (LOW PRIORITY; done in `server/`, not in `engine/`: one engine per session)**
```go
// engine/engine.go - support multiple concurrent sessions
type Engine struct {
    sessions map[string]*Session
    store    SessionStore
}
```

**Why**: Could enable automation workflows, background tasks, multi-project support.

## What's Different (Architectural Choices)

| Aspect | OpenClaw | Inber |
|--------|----------|-------|
| **Runtime** | Node.js + WebSocket gateway | Go HTTP server (`inber-server`) |
| **Sessions** | Multi-session with channel isolation | Multi-session, keyed by agent and scope, lane-limited |
| **Channels** | 25+ messaging platforms | None built in; NATS bus and HTTP |
| **Model loop** | Built-in loop, or Claude CLI / Codex app-server runtimes | Own loops: Anthropic SDK, plus an OpenAI loop (`engine/turn_openai.go`) that can use the ChatGPT subscription (`agent/codex_subscription.go`) |
| **Security** | Sandbox + tool policies | Execution modes, limits, egress redaction |
| **State** | Distributed (sessions.json + JSONL) | Per-session workspace files + SQLite |
| **Automation** | Cron jobs + webhooks (heartbeat retired) | None inside inber |
| **Tools** | In-process, or MCP bridge for CLI runtimes | Direct Go function calls |

## Implementation Priority

1. **Heartbeat system** - Extract interface, implement basic version (not done; OpenClaw has moved away from it)
2. **Provider interface** - Decouple Anthropic SDK from agent logic (started: `agent/provider.go`, still Anthropic-typed; OpenAI uses a separate loop)
3. **Tool policies** - Add permission checking to tool execution (done in basic form: `guard/`)
4. **Session store** - Make memory/conversation storage swappable (not done)

## Key Takeaway

OpenClaw is a **multi-user, multi-channel communication platform** with sophisticated session management and security.

Inber was a **single-user development tool** in March. It is now a **multi-session agent server** that other services drive over HTTP and a message bus, still without channel plugins of its own.

The March insight was that **OpenClaw's heartbeat system and tool policies are universally valuable**. Tool policies held up, and inber now has a basic version. Heartbeat did not: OpenClaw folded it into cron jobs.