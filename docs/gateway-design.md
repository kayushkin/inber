# Inber Gateway Design
**2026-03-14** — Working document

**Decision**: Gateway lives in the inber repo as `gateway/` package, invoked via `inber serve`.

## Separation of Concerns

### Two Layers

Gateway orchestrates. Engine executes. No intermediate layers.

The old `agent/` package folds into `engine/` — it was just the inner tool
loop with no standalone use case. The old `agent/registry/` (agent configs,
routing) moves to `gateway/` — that's where "which agent handles this" belongs.

```
gateway/                              engine/
├── Agent registry & routing          ├── RunTurn(input) → TurnResult
│   Agent configs (model, tools,      │   System prompt assembly
│   workspace, identity, limits)      │   Tool definitions + execution
│   agents.json / agent-store         │   API call + tool_use loop
│   Route: channel → agent            │   Failover + health tracking
│                                     │   Summarize/prune/stash
├── Session lifecycle                 │   Context budgeting
│   Create engine with agent config   │   ModelClient (Anthropic/OpenAI)
│   Keep engines alive between turns  │   Hooks (display, logging, workflow)
│   Resume/fork/kill sessions         │
│   Message persistence               │
│   Session repair                    │
│                                     │
├── Sub-agent management              │
│   Spawn/fork/announce               │
│   spawn_agent tool                  │
│   Result delivery to parent         │
│   Depth/concurrency limits          │
│                                     │
├── Queue & concurrency               │
│   Lane-based work queue             │
│   Per-session serialization         │
│                                     │
└── API surface                       │
    HTTP + WebSocket                  │
    Streaming events                  │
```

### How it flows

```
Message arrives
  → gateway routes to agent (registry lookup)
  → gateway looks up agent config (model, tools, workspace, identity)
  → gateway creates or reuses engine for this session
  → engine.RunTurn(input)
      → builds system prompt (identity + context + memories)
      → selects model (failover if needed)
      → calls API
      → tool_use loop (execute tools, continue until end_turn)
      → summarize/prune if conversation is long
      → returns TurnResult
  → gateway persists messages
  → gateway delivers response
```

Gateway owns the **what** (which agent, which config, which session).
Engine owns the **how** (build prompt, call API, run tools, return result).

## Package Layout

```
inber/
├── cmd/inber/
│   ├── serve.go           # `inber serve` — starts gateway daemon
│   ├── run.go             # `inber run` — single-shot (engine directly, no gateway)
│   └── chat.go            # `inber chat` — REPL (engine directly, no gateway)
├── gateway/
│   ├── gateway.go         # Gateway struct, config, agent registry
│   ├── session.go         # Session lifecycle, engine pooling, forking
│   ├── spawn.go           # Sub-agent spawning, result delivery
│   ├── queue.go           # Lane-based concurrent work queue
│   ├── api.go             # HTTP endpoints
│   └── stream.go          # WebSocket streaming
├── engine/                # Everything per-turn (absorbs old agent/)
│   ├── engine.go          # Engine struct, NewEngine, RunTurn, Close
│   ├── turn.go            # API call + tool_use loop (was agent.Run)
│   ├── tools.go           # Tool definitions (shell, read, write, edit, etc)
│   ├── build.go           # System prompt assembly, context budgeting
│   ├── failover.go        # Model selection, health routing
│   ├── lifecycle.go       # Summarize, prune, checkpoint, stash
│   ├── client.go          # ModelClient — Anthropic/OpenAI abstraction
│   └── hooks.go           # Display, logging, workflow hooks
├── session/               # Workspace, messages.json, session DB
├── context/               # Tag-based context system
└── memory/                # Memory store, search, references
```

---

## Core Interfaces

### Gateway

```go
package gateway

type Gateway struct {
    sessions    sync.Map                  // sessionKey → *Session
    queue       *Queue
    modelStore  *modelstore.Store         // shared, opened once
    registry    *registry.Registry        // agent configs
    config      Config
}

type Config struct {
    // Agent workspace mapping
    Agents      map[string]AgentConfig

    // Queue concurrency
    MainConcurrency     int  // default 4
    SubagentConcurrency int  // default 8
    CronConcurrency     int  // default 2

    // Sub-agent limits
    MaxSpawnDepth       int  // default 2
    MaxChildrenPerAgent int  // default 5

    // API
    ListenAddr string       // default ":8200"
    BusURL     string       // optional — if set, subscribe to bus directly
    BusToken   string
}

type AgentConfig struct {
    Workspace string   // repo root / cwd for this agent
    Model     string   // default model
    Thinking  int64
    Tools     []string // tool allowlist (empty = all)
}
```

### Session

```go
type Session struct {
    Key        string
    AgentName  string
    Engine     *engine.Engine            // kept alive between turns
    Status     SessionStatus
    SpawnDepth int
    ParentKey  string
    Children   []string                  // child session keys

    CreatedAt    time.Time
    LastActiveAt time.Time

    mu     sync.Mutex
    cancel context.CancelFunc
}

type SessionStatus int
const (
    Idle SessionStatus = iota
    Running
    Completed
    Error
)

// Turn sends a message to this session's engine and returns the result.
// Blocks until the turn completes. Only one Turn per session at a time
// (enforced by the queue, not by Session itself).
func (s *Session) Turn(ctx context.Context, input string) (*agent.TurnResult, error) {
    s.mu.Lock()
    s.Status = Running
    s.mu.Unlock()
    defer func() {
        s.mu.Lock()
        s.Status = Idle
        s.LastActiveAt = time.Now()
        s.mu.Unlock()
    }()

    return s.Engine.RunTurn(input)
}

// Fork creates a new session with a deep copy of this session's messages.
// The child gets a fresh engine but starts with the parent's conversation state.
func (s *Session) Fork(childKey string, agentName string, cfg AgentConfig) (*Session, error) {
    s.mu.Lock()
    messages := deepCopyMessages(s.Engine.Messages)
    s.mu.Unlock()

    childEngine, err := engine.NewEngine(engine.EngineConfig{
        AgentName: agentName,
        RepoRoot:  cfg.Workspace,
        Model:     cfg.Model,
        Thinking:  cfg.Thinking,
        Detach:    true,  // forked sessions don't persist to parent's workspace
    })
    if err != nil {
        return nil, err
    }

    // Inject parent's conversation history
    childEngine.Messages = messages

    return &Session{
        Key:        childKey,
        AgentName:  agentName,
        Engine:     childEngine,
        Status:     Idle,
        SpawnDepth: s.SpawnDepth + 1,
        ParentKey:  s.Key,
        CreatedAt:  time.Now(),
    }, nil
}
```

### Queue

```go
type Queue struct {
    lanes    map[string]*lane
    sessions sync.Map  // sessionKey → *sessionLock (ensures serialization)
}

type lane struct {
    name     string
    sem      chan struct{}  // buffered to concurrency limit
    pending  int64         // atomic counter for monitoring
}

// Enqueue runs work in the specified lane, serialized by session key.
// Blocks until a lane slot opens AND no other work is running on this session.
func (q *Queue) Enqueue(ctx context.Context, laneName string, sessionKey string, work func(ctx context.Context) error) error {
    // 1. Acquire session lock (per-session serialization)
    slock := q.getSessionLock(sessionKey)
    slock.Lock()
    defer slock.Unlock()

    // 2. Acquire lane slot (concurrency cap)
    l := q.getLane(laneName)
    select {
    case l.sem <- struct{}{}:
        defer func() { <-l.sem }()
    case <-ctx.Done():
        return ctx.Err()
    }

    // 3. Run
    return work(ctx)
}
```

### Spawn & Result Delivery

```go
// SpawnRequest — tool input from spawn_agent
type SpawnRequest struct {
    ParentKey    string
    Agent        string
    Task         string
    Model        string // optional override
    Fork         bool   // inherit parent's messages
}

// SpawnResult — delivered to parent when child completes
type SpawnResult struct {
    ChildKey     string
    Agent        string
    Task         string
    Status       string          // "success" | "error" | "timeout"
    Summary      string          // child's final response text
    Tokens       TokenUsage
    Duration     time.Duration
    Error        string
}

type TokenUsage struct {
    Input    int
    Output   int
    CacheRead int
    CacheWrite int
    Cost     float64
}

// Spawn creates a child session and enqueues its work.
// Returns immediately — result delivered async via callback.
func (g *Gateway) Spawn(ctx context.Context, req SpawnRequest) (*SpawnResponse, error) {
    parent := g.GetSession(req.ParentKey)
    if parent == nil {
        return nil, fmt.Errorf("parent session not found: %s", req.ParentKey)
    }

    // Check depth/child limits
    if parent.SpawnDepth >= g.config.MaxSpawnDepth {
        return nil, fmt.Errorf("max spawn depth reached (%d)", g.config.MaxSpawnDepth)
    }
    if len(parent.Children) >= g.config.MaxChildrenPerAgent {
        return nil, fmt.Errorf("max children reached (%d)", g.config.MaxChildrenPerAgent)
    }

    childKey := fmt.Sprintf("%s:sub:%s", req.ParentKey, shortUUID())
    agentCfg := g.config.Agents[req.Agent]

    var child *Session
    var err error
    if req.Fork {
        child, err = parent.Fork(childKey, req.Agent, agentCfg)
    } else {
        child, err = g.createSession(childKey, req.Agent, agentCfg)
    }
    if err != nil {
        return nil, err
    }
    child.ParentKey = req.ParentKey
    g.sessions.Store(childKey, child)
    parent.Children = append(parent.Children, childKey)

    // Enqueue the work
    go func() {
        err := g.queue.Enqueue(ctx, "subagent", childKey, func(ctx context.Context) error {
            result, err := child.Turn(ctx, req.Task)

            // Deliver result to parent
            g.deliverResult(req.ParentKey, SpawnResult{
                ChildKey: childKey,
                Agent:    req.Agent,
                Task:     req.Task,
                Status:   statusFromErr(err),
                Summary:  resultText(result),
                Tokens:   extractTokens(result),
                Duration: time.Since(child.CreatedAt),
                Error:    errString(err),
            })
            return err
        })
        if err != nil {
            log.Printf("[gateway] spawn %s failed: %v", childKey, err)
        }
    }()

    return &SpawnResponse{
        Status:   "accepted",
        ChildKey: childKey,
    }, nil
}

// deliverResult injects the child's result into the parent session.
// If the parent is currently running, it queues for the next turn.
// If the parent is idle, it triggers a new turn with the result.
func (g *Gateway) deliverResult(parentKey string, result SpawnResult) {
    parent := g.GetSession(parentKey)
    if parent == nil {
        log.Printf("[gateway] parent %s gone, dropping result from %s", parentKey, result.ChildKey)
        return
    }

    msg := fmt.Sprintf("[Sub-agent completed]\nAgent: %s (%s)\nTask: %s\nStatus: %s\nDuration: %s\nTokens: %d in / %d out ($%.3f)\n\nResult:\n%s",
        result.Agent, result.ChildKey,
        result.Task,
        result.Status,
        result.Duration.Round(time.Second),
        result.Tokens.Input, result.Tokens.Output, result.Tokens.Cost,
        result.Summary,
    )

    if result.Error != "" {
        msg += fmt.Sprintf("\n\nError: %s", result.Error)
    }

    // Inject as a system message for the parent's next turn
    g.injectMessage(parentKey, msg)
}
```

### ForkAndSpawn (batch forking)

```go
type ForkSpawnRequest struct {
    ParentKey string
    Tasks     []SpawnTask
    Model     string // optional, applies to all children
}

type SpawnTask struct {
    Agent string
    Task  string
}

// ForkAndSpawn forks the parent session N times, one per task.
// All children start with the same conversation history.
// Returns immediately — results delivered async.
func (g *Gateway) ForkAndSpawn(ctx context.Context, req ForkSpawnRequest) ([]*SpawnResponse, error) {
    var responses []*SpawnResponse

    for _, task := range req.Tasks {
        resp, err := g.Spawn(ctx, SpawnRequest{
            ParentKey: req.ParentKey,
            Agent:     task.Agent,
            Task:      task.Task,
            Model:     req.Model,
            Fork:      true,
        })
        if err != nil {
            // Log but continue — don't fail the whole batch
            log.Printf("[gateway] fork-spawn failed for %s: %v", task.Agent, err)
            continue
        }
        responses = append(responses, resp)
    }

    return responses, nil
}
```

---

## spawn_agent Tool (Updated)

The tool no longer emits to stderr. It calls the gateway directly.

```go
// In agent/registry/spawn_tool.go (or gateway/tools.go)

func (g *Gateway) SpawnAgentTool(parentSessionKey string) agent.Tool {
    return agent.Tool{
        Name:        "spawn_agent",
        Description: "Spawn a sub-agent to work on a task. Returns immediately. " +
                     "Results are delivered when the agent completes. " +
                     "Use fork=true to give the child your current conversation context.",
        InputSchema: anthropic.ToolInputSchemaParam{
            Required: []string{"agent", "task"},
            Properties: map[string]any{
                "agent": map[string]any{
                    "type":        "string",
                    "description": "Agent name to spawn",
                },
                "task": map[string]any{
                    "type":        "string",
                    "description": "Task for the agent to complete",
                },
                "model": map[string]any{
                    "type":        "string",
                    "description": "Model override (optional)",
                },
                "fork": map[string]any{
                    "type":        "boolean",
                    "description": "If true, child inherits this session's conversation history",
                    "default":     false,
                },
            },
        },
        Run: func(ctx context.Context, raw string) (string, error) {
            var in struct {
                Agent string `json:"agent"`
                Task  string `json:"task"`
                Model string `json:"model,omitempty"`
                Fork  bool   `json:"fork,omitempty"`
            }
            if err := json.Unmarshal([]byte(raw), &in); err != nil {
                return "", err
            }

            resp, err := g.Spawn(ctx, SpawnRequest{
                ParentKey: parentSessionKey,
                Agent:     in.Agent,
                Task:      in.Task,
                Model:     in.Model,
                Fork:      in.Fork,
            })
            if err != nil {
                return "", err
            }

            return fmt.Sprintf("🚀 Spawned %s (%s)\nTask: %s\nFork: %v\n\nResult will be delivered when complete.",
                in.Agent, resp.ChildKey, truncate(in.Task, 100), in.Fork), nil
        },
    }
}
```

---

## API Surface

```
POST   /api/run              — send message to agent session
POST   /api/spawn            — spawn a sub-agent
POST   /api/fork-spawn       — fork + spawn N children
GET    /api/sessions          — list sessions
GET    /api/sessions/:key     — session details
POST   /api/sessions/:key/inject  — inject message mid-run
DELETE /api/sessions/:key     — stop/kill session
GET    /api/models            — model health (replaces bus-agent API)
POST   /api/models/test       — test a model

WS     /ws/stream             — real-time events (deltas, tools, results)
```

### POST /api/run
```json
// Request
{"agent": "claxon", "message": "Hello", "session_key": "agent:claxon:main", 
 "channel": "websocket", "author": "slava"}

// Response (SSE stream)
event: delta
data: {"text": "Hey! "}

event: tool_call
data: {"tool": "shell", "input": "git status"}

event: tool_result
data: {"tool": "shell", "output": "On branch main...", "error": false}

event: done
data: {"text": "Full response", "tokens": {"in": 5000, "out": 200}, "duration_ms": 3400}
```

---

## Bus-agent After Gateway

Bus-agent becomes a thin WebSocket↔HTTP bridge:

```
Before:
  bus → bus-agent → spawn `inber run -a X` process → parse stderr → publish response

After:
  bus → bus-agent → POST gateway:8200/api/run (streaming) → publish deltas/response to bus
```

Bus-agent no longer needs:
- CLIBackend (process spawning)
- Stderr protocol (INBER_SPAWN, INBER_DELTA, INBER_TOOL)  
- Forge slot management
- Agent queue management (gateway handles this)

Bus-agent keeps:
- Bus subscription (WebSocket)
- Channel → agent routing
- Publishing to bus (responses, deltas)
- OpenClaw backend (for openclaw-routed agents)

---

## Engine Changes

Engine stays mostly unchanged. Small additions to EngineConfig:

```go
type EngineConfig struct {
    // ... existing fields ...
    
    // New: gateway can pre-load messages (for forked sessions)
    InitialMessages []anthropic.MessageParam
    
    // New: gateway can inject extra system prompt blocks
    ExtraSystemBlocks []sessionMod.NamedBlock
    
    // New: gateway can restrict tools for sub-agents
    ToolFilter []string  // if non-empty, only these tools are allowed
    
    // New: gateway passes shared model store
    ModelStore *modelstore.Store  // nil = engine opens its own
}
```

Engine's existing `NewEngine` handles these naturally:
- `InitialMessages` → set `e.Messages` instead of loading from workspace
- `ExtraSystemBlocks` → append to BuildSystemPrompt output
- `ToolFilter` → filter buildTools output
- `ModelStore` → use passed store instead of opening a new one

CLI commands (`run`, `chat`) keep working unchanged — they don't set
these fields, so engine falls back to current behavior.

---

## Migration Path

1. **Phase 1: Create gateway/ skeleton**
   Gateway struct, Session, Queue. `inber serve` starts it.
   Gateway creates engines via existing NewEngine, adds EngineConfig
   fields for InitialMessages/ExtraSystemBlocks/ToolFilter/ModelStore.
   Bus-agent gets GatewayBackend (HTTP instead of process spawning).

2. **Phase 2: Persistent sessions**
   Gateway keeps engines alive between turns instead of recreating.
   Session state in memory, persisted to disk for restart recovery.

3. **Phase 3: Spawning**
   spawn_agent calls gateway.Spawn() directly (no stderr).
   Result delivery via message injection into parent session.

4. **Phase 4: Session forking**
   Fork parent messages into N children via InitialMessages.
   ForkAndSpawn for parallel sub-agent work with shared context.
