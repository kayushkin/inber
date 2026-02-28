# Async Sub-Agent Spawning

## Overview

Inber's orchestrator agents can now spawn sub-agents in **fire-and-forget mode** (async by default), matching the pattern used by OpenClaw's `sessions_spawn`.

## The Problem

Previously, `spawn_agent` was **synchronous/blocking**:
- Orchestrator calls `spawn_agent(agent="fionn", task="implement feature X")`
- The tool **blocks** until Fionn completes the task
- Only then does the orchestrator continue

For complex multi-subtask requests like "fix mobile layout, add swipe nav, add continuous scroll", the orchestrator would spawn each agent **sequentially**, wasting time.

## The Solution

Async spawning pattern:
1. **Orchestrator decomposes** the task quickly
2. **Spawns multiple sub-agents in parallel** (returns immediately with task IDs)
3. **Returns to user** immediately with "here's what I kicked off"
4. **Sub-agents run in background** goroutines
5. **Results are written to disk** when ready
6. **Orchestrator can check status** later with `check_spawns`

## Usage

### Async Mode (Default)

```json
{
  "agent": "fionn",
  "task": "Implement mobile-responsive layout for the reader view"
}
```

Returns immediately:
```
🚀 Spawned fionn (task a3f2b8c1)

Task: Implement mobile-responsive layout for the reader view

Status: Running in background

Use check_spawns to see results when ready.
```

### Sync Mode (wait:true)

For tasks that **must** complete before continuing:

```json
{
  "agent": "scathach",
  "task": "Run all tests",
  "wait": true
}
```

Blocks until completion:
```
✅ Agent: scathach (task b7e9a3d2)

All tests passed: 47/47

[Tokens: in=1234 out=567 | Tools: 3]
```

## Checking Status

### Check Specific Task

```json
{
  "task_id": "a3f2b8c1"
}
```

Returns:
```
✅ Task a3f2b8c1 — fionn — completed (23.4s)
Task: Implement mobile-responsive layout for the reader view

Result:
✅ Mobile layout implemented! Added:
- CSS media queries for <768px screens
- Touch-friendly tap targets (min 44px)
- Responsive font scaling
- Swipe navigation handlers

[Tokens: in=8923 out=2341 | Tools: 12]
```

### List All Running Tasks

```json
{}
```

Returns:
```
⏳ Task c8d2e1a9 — oisin — running (3.2s)
Add deployment verification script

---

⏳ Task f4a7b3c6 — scathach — running (1.8s)
Run integration tests for new scroll behavior
```

### List All Tasks (Including Completed)

```json
{
  "all": true
}
```

## Architecture

### SpawnManager

Tracks running and completed sub-agents:
- **In-memory registry** for active spawns
- **Disk persistence** in `.inber/logs/_spawns/*.json`
- **Goroutine pool** for background execution
- **Completion detection** via polling

### Result Files

Each spawned task gets a JSON file:
```json
{
  "id": "a3f2b8c1",
  "agent": "fionn",
  "task": "Implement mobile layout",
  "started_at": "2026-02-27T23:45:12Z",
  "completed_at": "2026-02-27T23:45:35Z",
  "status": "completed",
  "result": "✅ Mobile layout implemented!...",
  "input_tokens": 8923,
  "output_tokens": 2341,
  "tool_calls": 12
}
```

### Agent Configuration

Orchestrator agents have both tools:
```json
{
  "name": "task-manager",
  "tools": [
    "spawn_agent",
    "check_spawns",
    "..."
  ]
}
```

Specialist agents (fionn, scathach, oisin) do **not** have `spawn_agent` — prevents recursive spawning chains.

## Best Practices

### Orchestrator Pattern

1. **Decompose quickly** — analyze the request, break into subtasks
2. **Spawn in parallel** — launch all independent tasks at once
3. **Return immediately** — tell user what's running
4. **Check later** — use `check_spawns` to gather results
5. **Report back** — aggregate and present final results

Example orchestrator flow:
```
User: "Fix mobile layout, add swipe nav, add continuous scroll"

Orchestrator:
1. Analyze → 3 independent tasks
2. Spawn fionn (mobile layout)
3. Spawn fionn (swipe nav)  
4. Spawn fionn (continuous scroll)
5. Return → "Kicked off 3 tasks: a3f2b8c1, b7e9a3d2, c8d2e1a9"

[Later, when user asks for update]
6. check_spawns → all completed
7. Aggregate results → present summary
```

### When to Use wait:true

- **Sequential dependencies** — Task B needs Task A's output
- **Critical verification** — must confirm before proceeding
- **Small/fast tasks** — overhead of async not worth it

### Cleanup

Old completed tasks stay in `_spawns/` directory for auditing. The in-memory registry auto-cleans completed tasks older than 1 hour (future: configurable).

## Comparison to OpenClaw

OpenClaw's `sessions_spawn`:
- Returns session ID immediately
- Sub-agent runs in separate process
- Results announced via push notification

Inber's `spawn_agent`:
- Returns task ID immediately
- Sub-agent runs in goroutine (same process)
- Results written to disk, polled via `check_spawns`

Both patterns achieve the same goal: **async delegation with fast orchestrator response**.

## Future Enhancements

- **Push notifications** — announce completion back to parent session
- **Result streaming** — stream partial results as sub-agent progresses
- **Dependency graphs** — declare task dependencies, auto-sequence spawns
- **Resource limits** — cap concurrent spawns to prevent overload
- **Cross-session spawns** — spawn agents that outlive parent session
