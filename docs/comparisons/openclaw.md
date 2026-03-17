# OpenClaw Comparison

**Framework**: Node.js-based personal AI assistant with multi-channel support  
**GitHub**: https://github.com/openclaw/openclaw  
**Architecture**: Gateway + WebSocket control plane with channel plugins  
**Studied**: March 2026

## What OpenClaw Does Well

### 1. **Session Management Architecture**
- **Clean session isolation**: Direct chats vs group chats get separate session keys
- **Flexible DM scoping**: `main` (continuity), `per-peer`, `per-channel-peer`, `per-account-channel-peer`
- **Identity linking**: Map same person across channels to single session
- **Auto-reset policies**: Daily at 4AM, idle timeouts, per-channel/per-type overrides
- **Session store as source of truth**: Gateway owns all state, UIs query via WebSocket

**Inber insight**: Our conversation package handles single sessions well, but lacks OpenClaw's multi-session orchestration and channel-aware isolation.

### 2. **Heartbeat System (Proactive Agent)**
- **Batched periodic checks**: Single heartbeat replaces multiple cron jobs
- **Context-aware decisions**: Full session history for smart prioritization  
- **Smart suppression**: Returns `HEARTBEAT_OK` when nothing needs attention
- **Configurable schedules**: 30min default, active hours, delivery targets
- **HEARTBEAT.md checklist**: Agent reads markdown file with tasks to check

**Inber insight**: We have no equivalent proactive system. This is a major gap for user engagement.

### 3. **Sophisticated Tool Policy System**
- **Layered security**: Sandbox mode with allowlist/denylist per tool
- **Session-aware policies**: Different tool access for main vs group sessions
- **Agent-to-agent controls**: `sessions_spawn`, `sessions_send` with visibility scoping
- **Per-session overrides**: Thinking level, model, tool access per session
- **Send policies**: Block delivery by channel type, session type, key prefix

**Inber insight**: Our tools are all-or-nothing. Need interface for granular tool permissions.

### 4. **Channel Abstraction Layer**
- **15+ channel plugins**: WhatsApp, Telegram, Discord, Slack, Signal, etc.
- **Unified message routing**: All channels → Gateway → Agent → Response delivery
- **Per-channel configuration**: Different models, tool policies, group settings
- **Media pipeline**: Images/audio/video with transcription hooks
- **Group management**: Mention gating, reply tags, activation modes

**Inber insight**: We have no channel abstraction. Everything is CLI-bound.

### 5. **Cron vs Heartbeat Design**
- **Cron for precision**: Exact timestamps, isolated sessions, model overrides
- **Heartbeat for awareness**: Batched monitoring, context-aware, cheaper
- **Smart scheduling**: Automatic load spreading for top-of-hour jobs
- **Session targeting**: `main` (system event), `isolated` (clean slate), custom

**Inber insight**: This is elegant separation of concerns we lack entirely.

## What Inber Does Better

### 1. **Simplicity & Focus**
- **Single-user CLI**: No multi-channel complexity, no group management
- **Direct tool execution**: No WebSocket overhead, no session routing complexity
- **Minimal architecture**: Engine → Agent → Tools, straightforward call chain
- **Go's advantages**: Faster startup, single binary, better resource efficiency

### 2. **Development Experience**
- **Type safety**: Go's compile-time guarantees vs Node.js runtime errors
- **Memory management**: No garbage collection pauses, predictable performance
- **Deployment**: Single static binary vs Node.js + npm ecosystem
- **Local-first**: No network dependencies for basic operation

### 3. **Context/Memory Integration**
- **Built-in memory management**: Context compaction, memory extraction, conversation pruning
- **Unified conversation model**: Single conversation abstraction vs complex session trees
- **Simpler state**: One conversation file vs distributed session store + transcripts

## What Inber Should Adopt

### 1. **Proactive Agent Pattern (HIGH PRIORITY)**
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

### 2. **Tool Permission Interface (MEDIUM PRIORITY)**
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

### 4. **Multi-Session Support (LOW PRIORITY)**
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
| **Runtime** | Node.js + WebSocket gateway | Go CLI |
| **Sessions** | Multi-session with channel isolation | Single conversation |
| **Channels** | 15+ messaging platforms | CLI only |
| **Security** | Sandbox + tool policies | Filesystem permissions |
| **State** | Distributed (sessions.json + JSONL) | Single conversation file |
| **Automation** | Heartbeat + Cron + Webhooks | None |
| **Tools** | WebSocket RPC with policies | Direct Go function calls |

## Implementation Priority

1. **Heartbeat system** - Extract interface, implement basic version
2. **Provider interface** - Decouple Anthropic SDK from agent logic  
3. **Tool policies** - Add permission checking to tool execution
4. **Session store** - Make memory/conversation storage swappable

## Key Takeaway

OpenClaw is a **multi-user, multi-channel communication platform** with sophisticated session management and security.

Inber is a **single-user development tool** focused on simplicity and directness.

The core insight: **OpenClaw's heartbeat system and tool policies are universally valuable**, even for single-user CLI tools. These should be Inber's top architectural priorities.