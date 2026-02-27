# Task-Manager Agent System

As of Feb 2026, inber now uses a **task-manager** agent as the default entry point. This document explains the new architecture and how it differs from the previous system.

## What Changed

### Before
- Three independent agents: `coder`, `researcher`, `orchestrator`
- No clear entry point - user had to choose which agent to use
- Orchestrator was designed for multi-agent coordination but wasn't the default

### After
- **task-manager** is the default agent (specified in `agents.json`)
- Task-manager acts as intelligent dispatcher to specialist agents
- Can dynamically create new specialized agents on-demand
- Agent templates available in `agents/templates/` for reuse

---

## Architecture

```
User Request
     │
     ▼
┌──────────────┐
│ task-manager │  ◄── Default entry point
└──────────────┘
     │
     ├─► Handle directly (simple queries)
     │
     ├─► Delegate to specialist
     │   ├─► coder (implementation, debugging)
     │   ├─► researcher (analysis, docs)
     │   └─► orchestrator (complex coordination)
     │
     └─► Create new agent (on-demand specialization)
         ├─► security-auditor
         ├─► test-engineer
         ├─► performance-optimizer
         └─► [any other specialist needed]
```

---

## Key Features

### 1. Intelligent Task Routing

Task-manager analyzes each request and determines the best approach:

- **Simple questions** → Handle directly
- **Code changes** → Delegate to `coder`
- **Analysis/research** → Delegate to `researcher`
- **Complex multi-step** → Break down and delegate in sequence
- **Specialized need** → Create custom agent

### 2. Dynamic Agent Creation

Task-manager can create new agents by:
1. Detecting need for specialized capability
2. Selecting appropriate template from `agents/templates/`
3. Writing customized `agents/{name}.md` identity file
4. Updating `agents.json` with configuration
5. Immediately delegating to new agent

**Example:**
```
User: "Audit the codebase for security vulnerabilities"

Task-manager:
1. Recognizes need for security specialist
2. Uses researcher template (read-only)
3. Creates security-auditor.md with security focus
4. Adds security-auditor to agents.json
5. Delegates audit to new agent
```

### 3. Task State Management

Task-manager maintains state in workspace:

**`.inber/workspace/task-manager/`**
- `task-queue.json` - Active and pending tasks
- `delegation-log.md` - History of delegations
- `decisions.md` - Key decisions and rationales

### 4. Memory-Driven Learning

Task-manager saves important patterns to memory:
- Which delegations worked well
- User preferences for agent selection
- Common task patterns
- Specialist agents that were created

This allows it to improve over time.

---

## Configuration

### agents.json

```json
{
  "default": "task-manager",  // ← New field specifies entry point
  "agents": {
    "task-manager": { ... },
    "coder": { ... },
    "researcher": { ... },
    "orchestrator": { ... }
  }
}
```

**Benefits:**
- Explicit declaration of default agent
- Easy to change default if needed
- CLI tools show default status

### Agent Identity

**agents/task-manager.md** contains:
- Role description
- Decision framework (when to handle vs delegate)
- Specialist agent catalog
- Task management workflow
- Communication style

See the full file for details.

---

## Usage

### Default Behavior (No Agent Specified)

```bash
./inber "Implement authentication"
```

Task-manager receives this, recognizes it's a coding task, and delegates to `coder`.

### Explicit Agent Selection (Still Works)

```bash
./inber --agent coder "Implement authentication"
```

Bypasses task-manager, goes directly to `coder`.

**When to use explicit agent:**
- You know exactly which specialist you need
- Working in a focused context (deep debugging session)
- Testing/debugging a specific agent

### Agent List Shows Default

```bash
./inber agents list
```

Output:
```
Configured agents (4):

  Default: task-manager

  task-manager *  model: claude-sonnet-4-5  8 tools
  coder            model: claude-sonnet-4-5  9 tools
  researcher       model: claude-sonnet-4-5  4 tools
  orchestrator     model: claude-sonnet-4-5  5 tools
```

---

## Templates System

### Location
`agents/templates/` contains reusable agent identities:
- `coder.md`
- `researcher.md`
- `orchestrator.md`

### Purpose
- Starting points for new specialized agents
- Maintained for common agent patterns
- Can be copied and customized

### Usage

**Manual:**
```bash
cp agents/templates/coder.md agents/my-specialist.md
# Edit my-specialist.md
# Add entry to agents.json
```

**Automated (future):**
Task-manager will use templates automatically when creating agents.

---

## System Prompt Documentation

New comprehensive docs in `docs/system-prompts/`:

- **overview.md** - What inber is, architecture, key features
- **tools.md** - All available tools, usage patterns, best practices
- **context-system.md** - Tag-based context, budgets, auto-loading
- **memory-system.md** - Persistent memory, search, importance scoring
- **workspace.md** - Workspace files, agent-specific state
- **sessions.md** - Session continuity, default/new/detached modes

**Purpose:**
- Agent system prompts can reference these docs
- Agents can load them as context on-demand
- Single source of truth for framework behavior
- Easier to maintain than inline documentation

---

## Migration Guide

### For Users

No action required - system is backward compatible:
- All existing agents still work
- Can still use `--agent` flag to specify agent
- New default behavior (task-manager) is optional

**Recommended:**
- Try the new default (no `--agent` flag)
- Let task-manager handle routing
- Provide feedback on delegation decisions

### For Developers

**If adding new tools:**
1. Register in `agent/registry/tools.go`
2. Update `docs/system-prompts/tools.md`
3. Consider which agents should have access

**If creating custom agents:**
1. Start with template from `agents/templates/`
2. Customize identity in `agents/{name}.md`
3. Add config to `agents.json`
4. Test with `./inber agents show {name}`

**If modifying task-manager:**
- Identity is in `agents/task-manager.md`
- Config is in `agents.json` under `task-manager`
- Update decision framework in markdown file

---

## Registry API Changes

### New Return Type

`LoadConfig()` now returns `*RegistryConfig` instead of `map[string]*AgentConfig`:

```go
// Before
configs, err := LoadConfig(path, dir)
cfg := configs["agent-name"]

// After
registryCfg, err := LoadConfig(path, dir)
cfg := registryCfg.Agents["agent-name"]
defaultAgent := registryCfg.Default
```

### New Registry Method

```go
// Get default agent name
reg.Default() string
```

### Memory Tools Registration

Memory tools now registered separately:

```go
reg := registry.New(client, configDir, logsDir)
reg.SetMemoryStore(memStore)  // Registers memory tools
```

---

## Benefits

### For Users
- **Simpler UX** - Don't need to know which agent to use
- **Better routing** - Intelligent task analysis
- **Fewer errors** - Task-manager prevents misrouting
- **Learning system** - Improves over time via memory

### For Developers
- **Clearer architecture** - Single entry point
- **Extensibility** - Easy to add new specialists
- **Templates** - Reusable agent patterns
- **Documentation** - Comprehensive system prompt docs

### For AI Agents
- **Context clarity** - Know their role in the system
- **Better decisions** - Clear delegation framework
- **Knowledge base** - System prompt docs provide deep understanding
- **State management** - Workspace for tracking tasks

---

## Future Enhancements

1. **Actual spawn_agent tool** - Currently task-manager explains delegation; future version will have real sub-agent spawning
2. **Agent lifecycle management** - Start, stop, monitor sub-agents
3. **Result aggregation** - Combine outputs from multiple agents
4. **Parallel delegation** - Run multiple specialists concurrently
5. **Agent discovery** - Auto-detect available specialists
6. **Performance metrics** - Track delegation success rates
7. **Cost optimization** - Choose agent based on budget constraints

---

## Troubleshooting

### Task-manager makes wrong delegation decision

**Solution:** Use `--agent` flag to bypass task-manager:
```bash
./inber --agent coder "your request"
```

**Long-term:** Task-manager learns from memory - save corrections:
```
You: "That should have gone to the researcher, not coder"
Task-manager: [saves preference to memory]
```

### Want to change default agent

Edit `agents.json`:
```json
{
  "default": "coder",  // ← Change this
  ...
}
```

### Task-manager not found

Ensure:
1. `agents/task-manager.md` exists
2. `agents.json` has `task-manager` entry
3. Rebuild: `go build ./cmd/inber/`

---

## Related Documentation

- [Multi-Agent Design](multi-agent-design.md) - Original design doc
- [Agent Registry README](../agent/registry/README.md) - Registry API
- [System Prompts](system-prompts/README.md) - Agent knowledge base
- [AGENTS.md](../AGENTS.md) - Developer guide
