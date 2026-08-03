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
//	checkpoint/  Workspace state snapshots — a design sketch, not built, not called
package engine

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/aiauth"
	"github.com/kayushkin/forge"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/agent/registry"
	"github.com/kayushkin/inber/codeindex"
	"github.com/kayushkin/inber/conversation"
	"github.com/kayushkin/inber/guard"
	"github.com/kayushkin/inber/memory"
	sessionMod "github.com/kayushkin/inber/session"
	"github.com/kayushkin/inber/tools"
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
	MemStore         memory.MemoryStore  // semantic memory (agent-store SQLite)
	Session          *sessionMod.Session // JSONL turn logger
	SessionDB        *sessionMod.DB      // session metadata DB
	Guard            *guard.Guard        // execution modes, limits, repetition detection
	Trace            *trace.Recorder     // structured execution logging
	CodeIndex        *codeindex.Index    // AST-based codebase symbol index
	IdentityOverride string              // system prompt for raw/override modes

	// --- Internal state ---
	repoRoot string
	// workspaceRoots is every repository this session works in, when it works
	// in more than one. The turn names them all in its volatile context; see
	// workspace_roots.go for why they are not in the system prompt.
	workspaceRoots []WorkspaceRoot
	// allTools is every tool the session was built with. agentTools is that set
	// minus disabledToolNames, and is what a turn puts on the wire. Keeping both
	// is what makes disabling reversible: filtering agentTools in place threw the
	// full set away, so each call subtracted from the previous answer and no
	// request could turn a tool back on.
	allTools          []agent.Tool
	disabledToolNames map[string]bool
	agentTools        []agent.Tool
	display           *DisplayHooks
	displayMu         sync.Mutex
	// currentMessageID is the provider's identifier for the assistant message
	// this session is producing right now. The agent reports it as soon as the
	// provider names the message, which on a streamed response is before the
	// first delta — so an event emitted while that message is arriving can be
	// stamped with the message it belongs to. It stays put between messages so
	// that a tool result, which arrives after the message that asked for the
	// tool, is still stamped with that message.
	currentMessageID    string
	messageIDMu         sync.Mutex
	workspace           *sessionMod.Workspace
	thinkingBud         int64
	stashCfg            conversation.StashConfig
	extractCfg          conversation.ExtractionConfig
	staged              *conversation.StagedConversation
	restoredTurnCounter int // turn count read back by initSession, installed by initLimitsAndProfiling
	toolInputsCache     map[string]string
	contextInjectors    []ContextInjector
	workflowHooks       *WorkflowHooks
	forgeHook           *forge.Hook
	forgeDB             *forge.Forge
	modelStore          *modelstore.Store
	ownsModelStore      bool
	authStore           *aiauth.Store
	modelClient         *agent.ModelClient
	agentRegistry       *registry.Registry
	modelExplicitlySet  bool
	noHooks             bool
	injections          <-chan string
	memoryProfiler      *MemoryProfiler
}

// ---------------------------------------------------------------------------
// NewEngine — initialize all subsystems
// ---------------------------------------------------------------------------

// NewEngine creates a fully initialized Engine. Each step delegates to a
// helper in engine_new.go — look there for implementation details.
//
// ctx bounds the setup work that reaches outside this process. Today that is
// the memory store's session preparation, which scans the workspace for
// recently modified files and can walk the entire tree to do it.
func NewEngine(ctx context.Context, cfg EngineConfig) (*Engine, error) {
	repoRoot, err := setupRepoRoot(cfg.RepoRoot)
	if err != nil {
		return nil, fmt.Errorf("repo root: %w", err)
	}
	if err := validateWorkspaceRoots(cfg.WorkspaceRoots, repoRoot); err != nil {
		return nil, fmt.Errorf("workspace roots: %w", err)
	}

	stashCfg, extractCfg := initializeConfigs(cfg)

	e := &Engine{
		Model:              cfg.Model,
		repoRoot:           repoRoot,
		workspaceRoots:     cfg.WorkspaceRoots,
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
	if err := e.initMemory(ctx, cfg); err != nil {
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
	//
	// An unreadable mode fails session creation rather than falling back to a
	// default. The mode names how much this session is trusted with, and the
	// only default available is the one that trusts it with everything.
	mode, err := guard.ParseMode(cfg.Mode)
	if err != nil {
		return nil, fmt.Errorf("execution mode: %w", err)
	}
	e.Guard = guard.New(e.Limits.GuardConfig(mode))

	// 9. Trace — structured execution logging (nil = disabled)
	e.Trace = trace.NewRecorder("", "", e.AgentName) // TODO: enable via config

	// 10. Code index — AST-based codebase analysis (nil-safe, no-op if empty)
	e.CodeIndex, _ = codeindex.Open(e.repoRoot)

	// 11. Workspace checkpoints are not wired here, because the checkpoint
	// package is a design sketch and every method returns
	// checkpoint.ErrNotImplemented. It used to be constructed at this line and
	// Take() was called on every turn, which cost nothing and bought nothing but
	// made the feature read as live to anyone auditing RunTurn. Wire it back in
	// when the package is built — the three questions that have to be answered
	// first are in its package doc.

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
//
// ctx bounds every step of the turn that can leave the process. Cancelling it
// stops the execute phase's API call and running tool, and Agent.Run refuses to
// start another round-trip; it also stops the prepare phase, whose summarizer
// is an API call in its own right and used to run to completion on a turn the
// user had already abandoned. It is the only way to stop a turn that is already
// in flight, so a caller that holds a cancel function (a session interrupt, a
// sub-agent spawn timeout) must pass its own context here rather than a fresh
// root.
func (e *Engine) RunTurn(ctx context.Context, input string) (*agent.TurnResult, error) {
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
	processedInput := e.prepareInput(ctx, input, sessionID) // turn_prepare.go
	systemBlocks, err := e.buildTurnContext(processedInput) // turn_prompt.go
	if err != nil {
		return nil, err
	}

	// 4. Execute agent
	result, err := e.executeAgent(ctx, systemBlocks) // turn_execute.go
	if err != nil {
		// A turn killed mid-stream can still carry text the user watched
		// arrive. Persist it before propagating the error, or the session log,
		// the messages snapshot and the next turn's context all lose it.
		if result != nil && result.Text != "" {
			if perr := e.postProcessResult(result, input, sessionID); perr != nil { // turn_postprocess.go
				Log.Warn("post-processing failed after turn error: %v", perr)
			}
		}
		return result, err
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
	if e.memoryProfiler != nil {
		e.memoryProfiler.TakeSnapshot(e.Turn.Counter)
	}

	return result, nil
}

// ---------------------------------------------------------------------------
// Close — cleanup
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Mid-session configuration updates
// ---------------------------------------------------------------------------

// SetModel overrides the model for subsequent turns.
func (e *Engine) SetModel(model string) {
	e.Model = model
	e.modelExplicitlySet = true
}

// SetCurrentMessageID records the provider's identifier for the assistant
// message now being produced. The agent reports it through the OnMessageID
// hook as soon as the provider names the message; nothing else should call
// this, because nothing else knows the answer.
func (e *Engine) SetCurrentMessageID(messageID string) {
	e.messageIDMu.Lock()
	e.currentMessageID = messageID
	e.messageIDMu.Unlock()
}

// CurrentMessageID returns the provider's identifier for the assistant message
// this session is producing, or the last one it produced. Empty until the
// first response of the session is named, and empty for the whole session on a
// provider that names nothing.
func (e *Engine) CurrentMessageID() string {
	e.messageIDMu.Lock()
	defer e.messageIDMu.Unlock()
	return e.currentMessageID
}

// SetThinkingBudget updates the extended thinking token budget for subsequent turns.
func (e *Engine) SetThinkingBudget(budget int64) {
	e.thinkingBud = budget
}

// ThinkingBudget returns the current thinking budget.
func (e *Engine) ThinkingBudget() int64 {
	return e.thinkingBud
}

// SetDisabledTools replaces the set of tool names excluded from subsequent
// turns. The names given are the whole answer, not an addition to it: a name
// that was disabled and is not named here is enabled again, and an empty or nil
// set restores every tool the session was built with.
//
// An unknown name is not an error. The set is a filter over the tools this
// session holds, not a registry, so a caller may name a tool this agent never
// had without emptying the wire.
func (e *Engine) SetDisabledTools(names []string) {
	disabled := make(map[string]bool, len(names))
	for _, n := range names {
		disabled[n] = true
	}
	e.disabledToolNames = disabled
	e.applyDisabledTools()
}

// memoryExpandToolName is the tool that recalls a memory by id. It is the read
// that conversation.SummarizeConversation's archive write depends on, and its
// spelling comes from the memory package, beside the tag list that names the
// same pair from the write's side, so the two halves are one string.
const memoryExpandToolName = memory.ToolNameMemoryExpand

// hasEnabledTool reports whether a turn would put the named tool on the wire.
// It answers from agentTools, the set the model is actually sent, so a tool the
// agent config never asked for and a tool SetDisabledTools took away both read
// as absent.
func (e *Engine) hasEnabledTool(name string) bool {
	for _, t := range e.agentTools {
		if t.Name == name {
			return true
		}
	}
	return false
}

// EnabledToolNames returns the names of the tools a turn would put on the wire,
// in the order the model sees them. SetDisabledTools had no exported reader, so
// a caller that changed a session's tool set had no way to find out what it had
// actually done.
func (e *Engine) EnabledToolNames() []string {
	names := make([]string, len(e.agentTools))
	for i, t := range e.agentTools {
		names[i] = t.Name
	}
	return names
}

// setToolSet installs the tools a session was built with, resolves their
// filesystem paths against the session's root, and derives the wire set from
// them. Both engine constructors go through it so neither can install tools
// without also honouring an already-disabled name.
//
// The rooting belongs here and not in buildTools because buildTools is not the
// only source of tools: the server injects its own through EngineConfig.
// ExtraTools, and an injected tool replaces a built-in one of the same name. A
// root applied before that merge would be dropped by an injected write_files
// without a word.
//
// Rooting used to happen at three points inside buildTools and only to shell,
// which is how the file tools came to resolve a relative path against the
// inber-server process's working directory while shell commands in the same
// session ran inside the agent's forge worktree.
func (e *Engine) setToolSet(toolSet []agent.Tool) {
	e.allTools = tools.ScopeToRoot(toolSet, e.repoRoot)
	e.applyDisabledTools()
}

// applyDisabledTools derives agentTools from allTools. It preserves the order
// of the surviving tools: agent/agent_run.go anchors the cache_control
// breakpoint on the last tool definition, so reordering them would move the
// breakpoint and throw away the cached prefix on every config call.
func (e *Engine) applyDisabledTools() {
	if e.allTools == nil {
		// An engine handed only its wire set — the test helpers in this package,
		// and anything that assigns agentTools directly. What it was given is the
		// full set it has.
		e.allTools = e.agentTools
	}
	if len(e.disabledToolNames) == 0 {
		e.agentTools = e.allTools
		return
	}
	filtered := make([]agent.Tool, 0, len(e.allTools))
	for _, t := range e.allTools {
		if !e.disabledToolNames[t.Name] {
			filtered = append(filtered, t)
		}
	}
	e.agentTools = filtered
}

// CompactContext triggers conversation summarization/pruning, optionally
// incorporating a user-provided summary. Returns the number of messages removed.
//
// ctx belongs to whoever asked for the compaction. This is an explicit request,
// not a step inside a turn, so it is not bound by a turn interrupt and never
// was: the caller that reaches it is an HTTP handler, and the context that
// stops it is that request's own.
func (e *Engine) CompactContext(ctx context.Context, summary string) (int, error) {
	before := len(e.Messages)

	// Run summarization first.
	summarizeErr := e.summarizeIfNeeded(ctx)

	// Then run prune. Prune is independent of the summarizer and makes no API call,
	// so it still runs and still counts even when the summary could not be produced.
	e.pruneIfNeeded(ctx)

	removed := before - len(e.Messages)

	// This compaction was asked for explicitly, so a half-done one is reported as
	// failed rather than as a smaller number of messages removed.
	if summarizeErr != nil {
		return removed, summarizeErr
	}
	return removed, nil
}

// ---------------------------------------------------------------------------
// Close
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
