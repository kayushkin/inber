// Package engine provides the core inber engine that orchestrates
// agent conversations: building system prompts, managing context budgets,
// running conversation turns, and coordinating memory and tool integration.
package engine

import (
	"fmt"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/forge"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/agent/registry"
	"github.com/kayushkin/inber/conversation"
	"github.com/kayushkin/inber/memory"
	sessionMod "github.com/kayushkin/inber/session"
	modelstore "github.com/kayushkin/model-store"
)

// DisplayHooks configures how engine events are shown to the user.
type DisplayHooks struct {
	OnThinking   func(text string)
	OnTextDelta  func(text string) // streaming text chunks from API
	OnToolCall   func(name string, input string)
	OnToolResult func(name string, output string, isError bool)
}

// EngineConfig configures the Engine.
type EngineConfig struct {
	Model              string
	ModelExplicitlySet bool // true if Model came from --model CLI flag (takes precedence over agent config)
	Thinking           int64
	AgentName      string // load from registry
	Raw            bool   // skip context/memory
	NoTools        bool
	ExtraTools     []agent.Tool // additional tools injected by gateway (spawn_agent, sessions_list, etc.)
	NoHooks        bool   // skip post-request verification (git/deploy checks)
	SystemOverride string
	RepoRoot       string
	ModelStore     *modelstore.Store // shared model store (optional - engine opens its own if nil)
	CommandName    string // "chat" or "run" for session registration
	NewSession     bool   // start fresh instead of continuing default session
	Detach         bool   // one-off session, don't save to workspace
	Display        *DisplayHooks
	StashConfig    *conversation.StashConfig      // Large message stashing config (nil = use defaults)
	ExtractConfig  *conversation.ExtractionConfig // Background extraction config (nil = use defaults)
	AutoWorkflow   AutoWorkflowConfig // Auto-branch, auto-commit, auto-format (Phase 1)
	MaxTurns         int            // max API round-trips per RunTurn (0 = unlimited)
	MaxInputTokens   int            // max cumulative input tokens per RunTurn (0 = unlimited)
	Injections       <-chan string  // channel for mid-run message injection (optional, from stdin)
	ContextInjectors []ContextInjector // extra system prompt sections injected by server
	MemoryProfiling  bool           // enable memory usage profiling
	MemoryLogPath    string         // path to memory profile log file (optional)
}

// ContextInjector provides additional system prompt blocks at turn time.
// Used by the server to inject live session status, workspace info, etc.
type ContextInjector func() []sessionMod.NamedBlock

// Engine encapsulates the shared setup and execution logic for chat and run.
type Engine struct {
	Client       *anthropic.Client
	Agent        *agent.Agent
	MemStore          memory.MemoryStore
	IdentityOverride  string // for raw/override modes (no SQLite memory)
	Session      *sessionMod.Session
	SessionDB    *sessionMod.DB
	Model        string
	AgentName    string
	AgentConfig  *registry.AgentConfig
	Messages     []anthropic.MessageParam
	TurnCounter  int

	repoRoot        string
	agentTools      []agent.Tool
	display         *DisplayHooks
	displayMu       sync.Mutex
	workspace       *sessionMod.Workspace
	thinkingBud     int64
	lastNamedBlocks []sessionMod.NamedBlock
	stashCfg        conversation.StashConfig
	extractCfg      conversation.ExtractionConfig
	consecutiveErrors  int  // track consecutive tool errors for context escalation
	lastTurnHadError   bool
	toolInputsCache   map[string]string             // toolID -> input JSON for workflow hooks
	contextInjectors  []ContextInjector              // extra system prompt sections from server
	workflowHooks   *WorkflowHooks                // auto-commit, auto-format, build/test
	forgeHook       *forge.Hook                   // workspace/preview automation
	forgeDB         *forge.Forge                  // forge database handle
	modelStore      *modelstore.Store             // model usage tracking
	ownsModelStore  bool                          // true if we opened it (false if shared from server)
	modelClient     *agent.ModelClient            // unified client (Anthropic or OpenAI)
	agentRegistry   *registry.Registry            // agent registry for spawn tools
	modelExplicitlySet bool                       // true if --model flag was used
	noHooks         bool                          // skip post-request verification
	maxTurns        int                           // max API round-trips per RunTurn (0 = unlimited)
	maxInputTokens  int                           // max cumulative input tokens per RunTurn (0 = unlimited)
	maxResponseTime int                           // max seconds for orchestrator to respond (0 = unlimited)
	turnStartTime   time.Time                     // when the current turn started (for time limit enforcement)
	injections      <-chan string                  // mid-run message injection channel (nil = disabled)

	// Session-level token tracking (exported for display)
	SessionInputTokens  int
	SessionOutputTokens int
	SessionCost         float64
	
	// Memory profiling
	memoryProfiler      *MemoryProfiler
}

// NewEngine creates and fully initializes an Engine: context, memory, tools, session, hooks.
// SetDisplayHooks updates the display hooks (thread-safe).
// Used by the gateway to point hooks at the current HTTP request's writer.
func (e *Engine) SetDisplayHooks(hooks *DisplayHooks) {
	e.displayMu.Lock()
	e.display = hooks
	e.displayMu.Unlock()
}

// GetDisplayHooks returns the current display hooks (thread-safe).
func (e *Engine) GetDisplayHooks() *DisplayHooks {
	e.displayMu.Lock()
	defer e.displayMu.Unlock()
	return e.display
}

func NewEngine(cfg EngineConfig) (*Engine, error) {
	// Setup repository root and validation
	repoRoot, err := setupRepoRoot(cfg.RepoRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to setup repo root: %w", err)
	}

	// Initialize default configurations
	stashCfg, extractCfg := initializeConfigs(cfg)

	// Create engine instance with basic configuration
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

	// Load agent configuration from registry
	agentName, identityText, agentConfig, err := loadAgentConfig(cfg.AgentName, cfg.CommandName, cfg.ModelExplicitlySet)
	if err != nil {
		return nil, err
	}
	e.AgentName = agentName
	e.AgentConfig = agentConfig

	// Update model from agent config if not explicitly set
	if agentConfig != nil && agentConfig.Model != "" && !cfg.ModelExplicitlySet {
		e.Model = agentConfig.Model
	}

	// Setup memory store and context (only if not in raw mode)
	if cfg.SystemOverride == "" && !cfg.Raw {
		memStore, err := setupMemoryStore(repoRoot, identityText, e.AgentName)
		if err != nil {
			return nil, err
		}
		e.MemStore = memStore
	} else if cfg.Raw && identityText == "" {
		identityText = "You are a helpful assistant."
	}

	// Setup session management and workspace
	session, sessionDB, workspace, messages, err := setupSession(repoRoot, e.AgentName, cfg.CommandName, cfg.NewSession, cfg.Detach)
	if err != nil {
		return nil, fmt.Errorf("failed to setup session: %w", err)
	}
	e.Session = session
	e.SessionDB = sessionDB
	e.workspace = workspace
	e.Messages = messages

	// Setup model store and client
	modelStore, err := setupModelStore(cfg.ModelStore)
	if err != nil {
		return nil, fmt.Errorf("failed to setup model store: %w", err)
	}
	e.modelStore = modelStore
	e.ownsModelStore = (cfg.ModelStore == nil) // only close if we opened it ourselves

	// Create model client
	modelClient, resolvedModel, anthropicClient, err := createModelClient(e.Model, e.modelStore)
	if err != nil {
		return nil, err
	}
	e.Model = resolvedModel
	e.modelClient = modelClient
	e.Client = anthropicClient

	// Setup session logging and workflow hooks
	if e.Session != nil {
		Log.Info("logging to %s", e.Session.FilePath())
		if e.SessionDB != nil {
			e.Session.AttachDB(e.SessionDB, cfg.CommandName)
		}
		
		// Configure automatic truncation of large tool results
		truncCfg := sessionMod.TruncateConfigForRole(e.AgentName)
		e.Session.SetTruncateConfig(truncCfg)
		
		// Initialize workflow automation (auto-commit, auto-format)
		workflowCfg := cfg.AutoWorkflow
		if workflowCfg == (AutoWorkflowConfig{}) {
			workflowCfg = DefaultAutoWorkflowConfig()
		}
		e.workflowHooks = NewWorkflowHooks(repoRoot, e.Session.SessionID(), e.AgentName, workflowCfg)
	}

	// Setup forge hook for project detection
	forgeDB, forgeHook, err := setupForgeHook(repoRoot, "", e.AgentName, AutoWorkflowConfig{})
	if err == nil {
		e.forgeDB = forgeDB
		e.forgeHook = forgeHook
	}

	// Setup agent registry for spawn tools
	agentRegistry, err := setupAgentRegistry(e.AgentConfig, e.Client, repoRoot, modelClient, e.modelStore, e.MemStore)
	if err != nil {
		return nil, err
	}
	e.agentRegistry = agentRegistry

	// Build and configure tools
	if !cfg.NoTools {
		e.agentTools = e.buildTools()

		// Append gateway-injected tools (replace same-named tools)
		for _, extra := range cfg.ExtraTools {
			replaced := false
			for i, t := range e.agentTools {
				if t.Name == extra.Name {
					e.agentTools[i] = extra
					replaced = true
					break
				}
			}
			if !replaced {
				e.agentTools = append(e.agentTools, extra)
			}
		}
		
		// Load tool registry into memory
		if err := loadToolsIntoMemory(e.MemStore, e.agentTools); err != nil {
			Log.Warn("failed to load tool registry: %v", err)
		}

		// Write tools list to workspace for reference
		if e.workspace != nil && len(e.agentTools) > 0 {
			toolInfos := make([]sessionMod.ToolInfo, len(e.agentTools))
			for i, t := range e.agentTools {
				toolInfos[i] = sessionMod.ToolInfo{Name: t.Name, Description: t.Description}
			}
			e.workspace.WriteToolsList(toolInfos)
		}
	}

	// Store system override for raw/override modes
	if cfg.SystemOverride != "" {
		e.IdentityOverride = cfg.SystemOverride
	} else if cfg.Raw {
		e.IdentityOverride = identityText
	}

	// Setup limits and profiling
	e.maxTurns, e.maxInputTokens, e.maxResponseTime = setupLimits(cfg, e.AgentConfig)

	// Setup memory profiling
	memoryProfiler, err := setupMemoryProfiling(cfg.MemoryProfiling, cfg.MemoryLogPath)
	if err != nil {
		return nil, err
	}
	e.memoryProfiler = memoryProfiler

	return e, nil
}

// GetMemoryReport returns the current memory profiling report.
func (e *Engine) GetMemoryReport() MemoryReport {
	if e.memoryProfiler == nil {
		return MemoryReport{Enabled: false}
	}
	return e.memoryProfiler.GenerateReport()
}

// IsMemoryProfilingEnabled returns whether memory profiling is active.
func (e *Engine) IsMemoryProfilingEnabled() bool {
	return e.memoryProfiler != nil
}

