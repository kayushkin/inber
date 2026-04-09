// Package engine orchestrates agent conversations: it builds system prompts,
// manages context budgets, runs conversation turns, and coordinates memory,
// tools, and workflow automation.
//
// Start here to understand the main path:
//
//	NewEngine   → initializes all subsystems (see engine_new.go for helpers)
//	RunTurn     → the 4-phase turn pipeline
//	Close       → cleanup and session summary
//
// Supporting files (this package):
//
//	turn_prepare.go      Phase 1: stash large messages, repair alternation, summarize/prune
//	turn_prompt.go       Phase 2: build system prompt with memory context and cache control
//	turn_execute.go      Phase 3: select model, call API (Anthropic or OpenAI path)
//	turn_postprocess.go  Phase 4: extract memories, stash responses, track tokens
//	build.go             Agent construction: system blocks, tools, hooks, limits
//	build_tools.go       Tool resolution from agent config or defaults
//	build_hooks.go       Injection, limit checks, display callbacks
//	lifecycle.go         Summarization, pruning, checkpointing, session save
//	failover.go          Model health tracking and failover selection
//	workflow_hooks.go    Auto-commit, auto-format, build/test after tool calls
//
// Companion packages:
//
//	guard/       Safety controls: execution modes, cost/token limits, repetition detection
//	trace/       Structured execution logging for analysis and optimization
//	codeindex/   AST-based codebase analysis for intelligent context building
//	checkpoint/  Workspace state snapshots with diff and restore
package engine

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/forge"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/agent/registry"
	"github.com/kayushkin/inber/checkpoint"
	"github.com/kayushkin/inber/codeindex"
	"github.com/kayushkin/inber/conversation"
	"github.com/kayushkin/inber/guard"
	"github.com/kayushkin/inber/memory"
	sessionMod "github.com/kayushkin/inber/session"
	"github.com/kayushkin/inber/trace"
	modelstore "github.com/kayushkin/model-store"
)

// ---------------------------------------------------------------------------
// Engine — the central runtime
// ---------------------------------------------------------------------------

// Engine holds all state for a single agent session. Created by NewEngine,
// driven by RunTurn, cleaned up by Close.
type Engine struct {
	// --- Public state (read by server, session management) ---
	Client      *anthropic.Client
	Agent       *agent.Agent
	Model       string
	AgentName   string
	AgentConfig *registry.AgentConfig
	Messages    []anthropic.MessageParam
	Turn        TurnState
	Cache       CacheState
	Limits      LimitConfig
	Tokens      TokenTotals

	// --- Subsystems ---
	MemStore         memory.MemoryStore      // semantic memory (agent-store SQLite)
	Session          *sessionMod.Session     // JSONL turn logger
	SessionDB        *sessionMod.DB          // session metadata DB
	Guard            *guard.Guard            // execution modes, limits, repetition detection
	Trace            *trace.Recorder         // structured execution logging
	CodeIndex        *codeindex.Index        // AST-based codebase symbol index
	Checkpoint       *checkpoint.Manager     // workspace state snapshots
	IdentityOverride string                  // system prompt for raw/override modes

	// --- Internal state ---
	repoRoot           string
	agentTools         []agent.Tool
	display            *DisplayHooks
	displayMu          sync.Mutex
	workspace          *sessionMod.Workspace
	thinkingBud        int64
	stashCfg           conversation.StashConfig
	extractCfg         conversation.ExtractionConfig
	staged             *conversation.StagedConversation
	toolInputsCache    map[string]string
	contextInjectors   []ContextInjector
	workflowHooks      *WorkflowHooks
	forgeHook          *forge.Hook
	forgeDB            *forge.Forge
	modelStore         *modelstore.Store
	ownsModelStore     bool
	modelClient        *agent.ModelClient
	agentRegistry      *registry.Registry
	modelExplicitlySet bool
	noHooks            bool
	injections         <-chan string
	memoryProfiler     *MemoryProfiler
}

// ---------------------------------------------------------------------------
// NewEngine — initialize all subsystems
// ---------------------------------------------------------------------------

// NewEngine creates a fully initialized Engine. Each step delegates to a
// helper in engine_new.go — look there for implementation details.
func NewEngine(cfg EngineConfig) (*Engine, error) {
	repoRoot, err := setupRepoRoot(cfg.RepoRoot)
	if err != nil {
		return nil, fmt.Errorf("repo root: %w", err)
	}

	stashCfg, extractCfg := initializeConfigs(cfg)

	e := &Engine{
		Model:              cfg.Model,
		repoRoot:           repoRoot,
		display:            cfg.Display,
		thinkingBud:        cfg.Thinking,
		stashCfg:           stashCfg,
		extractCfg:         extractCfg,
		modelExplicitlySet: cfg.ModelExplicitlySet,
		toolInputsCache:    make(map[string]string),
		contextInjectors:   cfg.ContextInjectors,
		noHooks:            cfg.NoHooks,
		injections:         cfg.Injections,
	}

	// 1. Agent identity — load from registry, resolve model
	if err := e.initAgent(cfg); err != nil {
		return nil, err
	}

	// 2. Memory — SQLite-backed semantic store (skipped in raw mode)
	if err := e.initMemory(cfg); err != nil {
		return nil, err
	}

	// 3. Session — JSONL logging, workspace, message persistence
	if err := e.initSession(cfg); err != nil {
		return nil, err
	}

	// 4. Model client — Anthropic/OpenAI, auth, failover
	if err := e.initModelClient(cfg); err != nil {
		return nil, err
	}

	// 5. Workflow — auto-commit, auto-format, forge integration
	e.initWorkflow(cfg)

	// 6. Tools — shell, files, memory, spawn, server-injected extras
	if !cfg.NoTools {
		e.initTools(cfg)
	}

	// 7. Limits, profiling, cache config
	e.initLimitsAndProfiling(cfg)

	// 8. Guard — execution modes, cost limits, repetition detection
	e.Guard = guard.New(guard.Config{
		Mode:           guard.Autonomous, // TODO: configurable via EngineConfig
		MaxTurns:       e.Limits.MaxTurns,
		MaxInputTokens: e.Limits.MaxInputTokens,
	})

	// 9. Trace — structured execution logging (nil = disabled)
	e.Trace = trace.NewRecorder("", "", e.AgentName) // TODO: enable via config

	// 10. Code index — AST-based codebase analysis (nil-safe, no-op if empty)
	e.CodeIndex, _ = codeindex.Open(e.repoRoot)

	// 11. Checkpoint — workspace state snapshots (nil-safe)
	if e.Session != nil {
		e.Checkpoint, _ = checkpoint.New(e.repoRoot, e.Session.SessionID())
	}

	return e, nil
}

// ---------------------------------------------------------------------------
// RunTurn — the main execution pipeline
// ---------------------------------------------------------------------------

// RunTurn sends a user message through the turn pipeline:
//
//  1. Guard    — check limits, verify mode allows this turn
//  2. Prepare  — stash large messages, repair alternation, summarize/prune
//  3. Context  — build system prompt with memory, code index, cache control
//  4. Execute  — select model, call API, loop on tool calls
//  5. Process  — extract memories, stash response, track tokens/cost
//  6. Record   — update guard counters, write trace, take checkpoint
func (e *Engine) RunTurn(input string) (*agent.TurnResult, error) {
	e.Turn.Counter++
	e.Turn.StartTime = time.Now()
	fmt.Fprintf(os.Stderr, "\n%s━━━ Turn %d ━━━%s\n", cyan+bold, e.Turn.Counter, reset)

	if e.memoryProfiler != nil {
		e.memoryProfiler.TakeSnapshot(e.Turn.Counter)
	}

	// 1. Guard — check if we've exceeded limits before doing work
	if e.Guard != nil {
		if exceeded, reason := e.Guard.CheckLimits(); exceeded {
			return nil, fmt.Errorf("guard: %s", reason)
		}
	}

	sessionID := ""
	if e.Session != nil {
		sessionID = e.Session.SessionID()
		e.Session.LogUser(input)
	}

	// 2-3. Prepare input and build context
	processedInput := e.prepareInput(input, sessionID)                   // turn_prepare.go
	systemBlocks := e.buildTurnContext(processedInput)                    // turn_prompt.go

	// 4. Execute agent
	result, err := e.executeAgent(context.Background(), systemBlocks)    // turn_execute.go
	if err != nil {
		return nil, err
	}

	// 5. Post-process
	if err := e.postProcessResult(result, input, sessionID); err != nil { // turn_postprocess.go
		Log.Warn("post-processing failed: %v", err)
	}

	// 6. Record — update guard, trace, checkpoint
	if e.Guard != nil {
		e.Guard.RecordTurn(result.InputTokens)
	}
	e.Trace.RecordTurn(trace.Turn{
		Number:       e.Turn.Counter,
		Input:        input,
		Output:       result.Text,
		InputTokens:  result.InputTokens,
		OutputTokens: result.OutputTokens,
	})
	if e.Checkpoint != nil {
		e.Checkpoint.Take(fmt.Sprintf("turn %d", e.Turn.Counter), e.Turn.Counter)
	}

	if e.memoryProfiler != nil {
		e.memoryProfiler.TakeSnapshot(e.Turn.Counter)
	}

	return result, nil
}

// ---------------------------------------------------------------------------
// Close — cleanup
// ---------------------------------------------------------------------------

// Close finalizes the session: writes trace summary, saves memory, closes all stores.
func (e *Engine) Close() {
	if e.workflowHooks != nil {
		if summary := e.workflowHooks.FinishSession(); summary != "" {
			fmt.Fprintln(os.Stderr, "\n"+summary)
		}
	}
	e.Trace.WriteSummary()
	if e.CodeIndex != nil {
		e.CodeIndex.Close()
	}
	if e.MemStore != nil && len(e.Messages) > 0 {
		SaveSessionSummary(e.MemStore, e.Messages, e.AgentName)
	}
	if e.MemStore != nil {
		e.MemStore.Close()
	}
	if e.Session != nil {
		e.Session.Close()
	}
	if e.SessionDB != nil {
		e.SessionDB.Close()
	}
	if e.modelStore != nil && e.ownsModelStore {
		e.modelStore.Close()
	}
	if e.forgeDB != nil {
		e.forgeDB.Close()
	}
	if e.memoryProfiler != nil {
		e.memoryProfiler.Close()
	}
}
