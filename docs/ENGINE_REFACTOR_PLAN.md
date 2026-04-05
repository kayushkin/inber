# Engine Refactor Plan

_Drafted April 2026. Based on direct inspection of engine/ source._

---

## Background: What's Wrong

The `engine/` package is 30 files doing the work of 5–6 packages. The `Engine` struct has ~50 fields. `RunTurn()` does 14 distinct things. Files are named after implementation concerns, not domain concepts. The result is a codebase where you can't read anything end-to-end without jumping between 8 files.

Concrete violations found:

| Problem | Evidence |
|---|---|
| God struct | `Engine` owns: client, memory, session, model store, forge, workflow hooks, stash config, extract config, staging zone, blueprint state, profiling, injections, limits, context injectors — all in one struct |
| God function | `RunTurn()` does: increment counter, snapshot memory, log user, stash user message, repair alternation, summarize, prune, build system prompt, select model, switch provider, filter messages, run API, record health, log assistant, extract memories async, stash assistant response, save messages, checkpoint, track tokens |
| Wrong package | 5 `workflow_*.go` files — auto-commit, auto-format, build/test, deploy, git — live in `engine/` but have zero dependency on `Engine`. They depend on `WorkflowHooks` only. |
| Wrong package | `openclaw_feed.go` is a standalone WebSocket client to an external gateway. Zero dependency on `Engine`. Lives in `engine/` for historical reasons only. |
| Misnamed files | `engine_init.go` is actually a factory helpers file. `turnLifecycle.go` contains `FindRepoRoot()` and `SaveSessionSummary()` — not lifecycle functions. |
| Duplicated helpers | `isThinkingSignatureError()` in both `agent/agent.go` and `engine/turn.go`. `formatDuration()` in both `engine/fleet_status.go` and `server/session_utils.go`. |
| Mixed concerns | `build_prompts.go` handles both context budget calculation (how much memory to load) and system prompt assembly (building the actual blocks). These change for different reasons. |
| Misplaced `Close()` | `Close()` lives in `turn.go`. It has nothing to do with turns. |

---

## Design Goals

1. **Turn loop readable end-to-end in one file** — someone should be able to open `turn.go` and see exactly what a turn does, in order, with each phase as a named method call.
2. **Engine struct owns coordination, not implementation** — it holds references, not logic. Logic lives in collaborators.
3. **Each file does one thing with an obvious name** — if you have to open the file to know what it does, the name is wrong.
4. **No orphaned helpers** — `FindRepoRoot` does not live in `turnLifecycle.go`.
5. **Duplicates eliminated** — one definition, imported where needed.

---

## Proposed Package Layout

```
engine/
  engine.go             — Engine struct (trimmed), NewEngine, SetDisplayHooks, GetDisplayHooks
  engine_new.go         — constructor helpers (renamed from engine_init.go)
  turn.go               — RunTurn only: readable top-level turn loop, delegates to phases
  turn_prepare.go       — prepareInput: stash user msg, repair alternation, summarize, prune
  turn_prompt.go        — buildPrompt: context budget, system blocks assembly (split from build_prompts.go)
  turn_context.go       — context budget calculation only (extracted from build_prompts.go)
  turn_run.go           — runAgent: model selection, provider switching, API call (from turn.go)
  turn_post.go          — postProcess: stash response, extract memories, save, checkpoint, track tokens
  turn_openai.go        — OpenAI-specific turn path (already exists, keep)
  close.go              — Close(), SaveSessionSummary (moved from turn.go + turnLifecycle.go)
  lifecycle.go          — summarizeIfNeeded, pruneIfNeeded, pruneConfig (from turnLifecycle.go)
  build.go              — buildAgent, configureAgent, configureContextPruning (keep, already focused)
  build_tools.go        — tool assembly (keep)
  build_hooks.go        — hook wiring (keep)
  failover.go           — selectModel, fallbackChain, timeoutFromHealth (keep, already focused)
  fleet_status.go       — buildFleetStatus, shortModel (keep; move formatDuration out)
  display.go            — display formatting (keep)
  display_*.go          — display sub-files (keep)
  memory_profiling.go   — MemoryProfiler (keep)
  prompt_blueprint.go   — PromptBlueprint, BuildBlueprint, DiffBlueprints (keep)

engine/workflow/        — NEW PACKAGE (extracted from engine/)
  hooks.go              — WorkflowHooks struct, NewWorkflowHooks, OnToolResult, FinishSession (from workflow_hooks.go)
  git.go                — git helpers (from workflow_git.go)
  build.go              — build/test automation (from workflow_build.go)
  format.go             — formatter automation (from workflow_format.go)
  deploy.go             — deploy verification (from workflow_deploy.go)

engine/openclaw/        — NEW PACKAGE (extracted from engine/)
  client.go             — OpenClawSubagent, GatewayMessage, etc. (from openclaw_feed.go)
  client_test.go        — (from openclaw_feed_test.go)

internal/timeutil/      — NEW PACKAGE (or agent/timeutil, or just agent package)
  duration.go           — formatDuration (deduplicate engine/fleet_status.go + server/session_utils.go)

internal/apiutil/       — NEW PACKAGE (or inline in agent package)
  errors.go             — isThinkingSignatureError (deduplicate engine/turn.go + agent/agent.go)
```

**Note on `internal/` packages:** Go's `internal/` convention makes them importable only from the parent module. For a single-module repo like inber this is fine — use `internal/timeutil` and `internal/apiutil` to signal "shared infrastructure, not public API."

Alternatively: put both in `agent/` since that's the lowest common dependent. The key thing is: **one definition**.

---

## Engine Struct After Refactor

The struct is trimmed by extracting cohesive groups into sub-structs.

```go
// Engine orchestrates conversation turns. It holds references to
// collaborators but does not implement their logic directly.
type Engine struct {
    // Exported — consumed by callers (server, CLI)
    Client              *anthropic.Client   // kept for backward compat; prefer modelClient
    MemStore            memory.MemoryStore
    Session             *sessionMod.Session
    SessionDB           *sessionMod.DB
    Model               string
    AgentName           string
    AgentConfig         *registry.AgentConfig
    Messages            []anthropic.MessageParam
    TurnCounter         int
    SessionInputTokens  int
    SessionOutputTokens int
    SessionCost         float64

    // Collaborators (lowercase — internal)
    modelClient         *agent.ModelClient
    modelStore          *modelstore.Store
    memoryProfiler      *MemoryProfiler

    // Sub-structs grouping related state (see below)
    conv    convState    // conversation management state
    hooks   hooksState   // workflow + forge hooks
    limits  limitsState  // turn/token/time limits
    build   buildState   // prompt building state (cache, blueprint, staged)
    cfg     runConfig    // immutable config from EngineConfig
}

// convState holds conversation management state.
type convState struct {
    staged             *conversation.StagedConversation
    lastManageTurn     int
    consecutiveErrors  int
    lastTurnHadError   bool
    stashCfg           conversation.StashConfig
    extractCfg         conversation.ExtractionConfig
    toolInputsCache    map[string]string
}

// hooksState holds automation hooks.
type hooksState struct {
    workflow    *workflow.WorkflowHooks  // auto-commit/format/test (now from engine/workflow)
    forge       *forge.Hook
    forgeDB     *forge.Forge
    display     *DisplayHooks
    displayMu   sync.Mutex
    injections  <-chan string
    injectors   []ContextInjector
}

// limitsState holds per-turn execution limits.
type limitsState struct {
    maxTurns        int
    maxInputTokens  int
    maxResponseTime int
    turnStartTime   time.Time
}

// buildState holds prompt-building caches.
type buildState struct {
    lastNamedBlocks    []sessionMod.NamedBlock
    lastStablePrefix   *cachedPrefix
    volatileContext    string
    staged             *conversation.StagedConversation
    lastBlueprint      *PromptBlueprint
    blueprintEnabled   bool
}

// runConfig holds immutable configuration (set at NewEngine, never mutated).
type runConfig struct {
    repoRoot           string
    thinkingBud        int64
    modelExplicitlySet bool
    noHooks            bool
    agentTools         []agent.Tool
    workspace          *sessionMod.Workspace
    ownsModelStore     bool
    agentRegistry      *registry.Registry
    identityOverride   string
}
```

This doesn't change the public API — all exported fields stay exported. It just makes the struct self-documenting: you can see at a glance that there's a `conv` group and a `hooks` group.

---

## New Turn Loop

`RunTurn` in `turn.go` after the refactor — readable top to bottom, each step named:

```go
// RunTurn sends a user message and returns the assistant's response.
// It coordinates the full turn lifecycle: preparation, prompt building,
// API execution, and post-processing.
func (e *Engine) RunTurn(input string) (*agent.TurnResult, error) {
    e.TurnCounter++
    e.limits.turnStartTime = time.Now()
    e.logTurnStart(input)

    // Phase 1: Prepare input
    // Stash large blocks, repair message alternation, compress old history.
    prepared, err := e.prepareInput(input)
    if err != nil {
        return nil, err
    }

    // Phase 2: Build prompt
    // Calculate context budget, load memory blocks, assemble system prompt.
    systemBlocks := e.buildPrompt(prepared)

    // Phase 3: Run agent
    // Select model (failover), call API, handle provider differences.
    result, modelUsed, err := e.runAgent(systemBlocks)
    if err != nil {
        return nil, err
    }

    // Phase 4: Post-process
    // Extract memories, stash large response, save state, track tokens.
    e.postProcess(input, result, modelUsed)

    return result, nil
}
```

Each phase is a method. Each method lives in a file named for what it does:

| Method | File |
|---|---|
| `logTurnStart` | `turn.go` (inline, small) |
| `prepareInput` | `turn_prepare.go` |
| `buildPrompt` | `turn_prompt.go` |
| `runAgent` | `turn_run.go` |
| `postProcess` | `turn_post.go` |

---

## What Moves Where — Specific Functions

### `engine/turn.go` (after refactor: lean, top-level loop only)
- **Keep:** `RunTurn` (now a clean 20-line coordinator)
- **Keep:** `logTurnStart` (small, inline)
- **Move out:** `isThinkingSignatureError` → `internal/apiutil/errors.go`
- **Move out:** `Close` → `engine/close.go`

### `engine/turn_prepare.go` (NEW)
- `prepareInput` — orchestrates the below
- inline: stash large user message blocks (extracted from `RunTurn`)
- inline: repair message alternation (extracted from `RunTurn`)
- calls: `e.summarizeIfNeeded()`, `e.pruneIfNeeded()`

### `engine/turn_context.go` (NEW — split from `build_prompts.go`)
- `contextBudget(userMessage string) (minImportance float64, tokenBudget int)`
- `estimateConversationTokens()`
- `isVolatileMemoryID(id string) bool`
- `isVolatileBlock(id string) bool`
- `hashStrings(ss []string) [32]byte`

### `engine/turn_prompt.go` (rename/refocus `build_prompts.go`)
- `BuildSystemPrompt(userMessage string) []sessionMod.NamedBlock`
- `buildSystemBlocks(blocks) []anthropic.TextBlockParam`
- `sourceRef()` and cache prefix logic
- **Move out:** context budget functions → `turn_context.go`

### `engine/turn_run.go` (NEW — extracted from `RunTurn`)
- `runAgent(systemBlocks) (*agent.TurnResult, string, error)` — model selection + API dispatch
- calls `e.selectModel()`, `e.buildAgent(systemBlocks)`, provider switch logic

### `engine/turn_post.go` (NEW — extracted from `RunTurn`)
- `postProcess(input string, result *agent.TurnResult, modelUsed string)`
- inline: extract memories (async goroutine), stash assistant response, saveMessages, checkpointIfNeeded, track tokens

### `engine/lifecycle.go` (rename `turnLifecycle.go`)
- **Keep:** `summarizeIfNeeded`, `pruneIfNeeded`, `pruneConfig`
- **Move out:** `FindRepoRoot` → `internal/gitutil/repo.go` (or `engine/engine_new.go` as local helper)
- **Move out:** `SaveSessionSummary` → `engine/close.go`

### `engine/close.go` (NEW)
- `Close()` (moved from `turn.go`)
- `SaveSessionSummary()` (moved from `turnLifecycle.go`)

### `engine/engine_new.go` (rename `engine_init.go`)
- All constructor helpers: `setupRepoRoot`, `initializeConfigs`, `loadAgentConfig`, `setupMemoryStore`, `setupLimits`, `setupMemoryProfiling`, etc.
- Name change makes it clear: this is the constructor's helpers, not an `init()` pattern.

### `engine/workflow/` (NEW PACKAGE — extracted from `engine/`)
- `hooks.go` ← `workflow_hooks.go`
- `git.go` ← `workflow_git.go`
- `build.go` ← `workflow_build.go`
- `format.go` ← `workflow_format.go`
- `deploy.go` ← `workflow_deploy.go`
- Import path: `github.com/kayushkin/inber/engine/workflow`
- `engine/build_hooks.go` will import `workflow.WorkflowHooks` instead of the local type

### `engine/openclaw/` (NEW PACKAGE — extracted from `engine/`)
- `client.go` ← `openclaw_feed.go`
- `client_test.go` ← `openclaw_feed_test.go`
- Zero dependency on `Engine` — clean extraction

### `internal/timeutil/duration.go` (NEW — deduplication)
- `FormatDuration(d time.Duration) string` (exported)
- Replace `engine/fleet_status.go::formatDuration` and `server/session_utils.go::formatDuration`

### `internal/apiutil/errors.go` (NEW — deduplication)
- `IsThinkingSignatureError(err error) bool` (exported)
- Replace `engine/turn.go::isThinkingSignatureError` and `agent/agent.go::isThinkingSignatureError`

---

## Migration Order

Risk-ordered. Start with pure moves (no logic changes), end with the Engine struct surgery.

### Phase 0: Deduplication (lowest risk — pure additions)
1. Create `internal/timeutil/duration.go` with `FormatDuration`
2. Update `engine/fleet_status.go` to use it
3. Update `server/session_utils.go` to use it
4. Create `internal/apiutil/errors.go` with `IsThinkingSignatureError`
5. Update `engine/turn.go` to use it
6. Update `agent/agent.go` to use it
7. Build + test. No behavior change.

### Phase 1: Extract packages with no Engine dependency (zero API change risk)
1. Create `engine/openclaw/` package — move `openclaw_feed.go`, `openclaw_feed_test.go`
2. Update any callers of `OpenClawSubagent` to import new path
3. Create `engine/workflow/` package — move all `workflow_*.go` files
4. Update `engine/build_hooks.go` to import `workflow.WorkflowHooks`
5. Build + test.

### Phase 2: File renames and misplaced function moves (low risk)
1. Rename `engine_init.go` → `engine_new.go` (content unchanged)
2. Create `engine/close.go` — move `Close()` from `turn.go`, `SaveSessionSummary` from `turnLifecycle.go`
3. Move `FindRepoRoot` from `turnLifecycle.go` to `engine_new.go` (or `internal/gitutil/`)
4. Rename `turnLifecycle.go` → `lifecycle.go`
5. Build + test.

### Phase 3: Split `build_prompts.go` (medium risk — touches prompt pipeline)
1. Create `turn_context.go` — extract `contextBudget`, `isVolatileMemoryID`, `isVolatileBlock`, `hashStrings`
2. Rename `build_prompts.go` → `turn_prompt.go` (content is now just prompt assembly)
3. Build + test.

### Phase 4: Split `RunTurn` into phases (medium risk — functional change to control flow)
1. Create `turn_prepare.go` with `prepareInput` — move stash+repair+summarize+prune inline code out of RunTurn
2. Create `turn_run.go` with `runAgent` — move model selection + API dispatch
3. Create `turn_post.go` with `postProcess` — move memory extraction + stashing + save + checkpoint + token tracking
4. Rewrite `RunTurn` as the 4-phase coordinator (20 lines)
5. Build + test. This is the most delicate step — regression-test with actual API calls.

### Phase 5: Engine struct grouping (surgical — last, highest risk)
1. Define `convState`, `hooksState`, `limitsState`, `buildState`, `runConfig` sub-structs
2. Migrate field references one group at a time (start with `limitsState` — smallest, most isolated)
3. Update all internal field accesses (e.g., `e.maxTurns` → `e.limits.maxTurns`)
4. Exported fields (`Messages`, `TurnCounter`, etc.) stay flat on `Engine` — no external API change
5. Build + test after each group.

---

## What Not to Do

- **Don't merge `engine/` into `agent/`** — they have different stability contracts. `agent/` is the inner loop (API calls, tool execution); `engine/` is the outer orchestrator. Keep the boundary.
- **Don't split Engine into a pure interface** — it's used concretely everywhere. An interface adds noise with no benefit right now.
- **Don't move `display_*.go` files** — they're already well-factored and focused. Leave them alone.
- **Don't touch `build.go`** — `buildAgent`, `configureAgent` are already focused. Fine as-is.
- **Don't do Phase 4 and Phase 5 simultaneously** — splitting the turn loop and restructuring the struct at the same time is a recipe for a debugging nightmare.

---

## Expected Outcomes

After all phases:

- `turn.go` is ~30 lines — anyone can read a turn
- `engine.go` struct is ~25 fields, grouped into 5 coherent sub-structs
- `engine/workflow/` is importable independently (useful for testing and future CLI tools)
- `engine/openclaw/` is clearly a gateway client, not part of core engine
- Zero duplicate helpers
- Every file name tells you what it does before you open it
