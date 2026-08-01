package engine

import (
	"context"
	"fmt"
	"time"

	sessionMod "github.com/kayushkin/inber/session"
)

// BenchmarkTiming captures detailed timing for each initialization phase.
type BenchmarkTiming struct {
	TotalTime          time.Duration
	ConfigTime         time.Duration
	MemoryStoreTime    time.Duration
	SessionPrepTime    time.Duration
	WorkspaceTime      time.Duration
	SessionDBTime      time.Duration
	ModelStoreTime     time.Duration
	ModelClientTime    time.Duration
	AgentRegistryTime  time.Duration
	ToolsBuildingTime  time.Duration
	HooksSetupTime     time.Duration
	OverheadTime       time.Duration
}

// NewEngineBenchmark creates an Engine with detailed phase timing instrumentation.
// This is a specialized version of NewEngine that measures each initialization step.
func NewEngineBenchmark(ctx context.Context, cfg EngineConfig) (*Engine, BenchmarkTiming, error) {
	var timing BenchmarkTiming
	startTime := time.Now()
	
	// Phase 1: Setup repository root and validation
	phaseStart := time.Now()
	repoRoot, err := setupRepoRoot(cfg.RepoRoot)
	if err != nil {
		return nil, timing, fmt.Errorf("failed to setup repo root: %w", err)
	}
	if err := validateWorkspaceRoots(cfg.WorkspaceRoots, repoRoot); err != nil {
		return nil, timing, fmt.Errorf("workspace roots: %w", err)
	}

	// Initialize default configurations
	stashCfg, extractCfg := initializeConfigs(cfg)
	timing.ConfigTime = time.Since(phaseStart)

	// Create engine instance with basic configuration
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

	// Phase 2: Load agent configuration
	phaseStart = time.Now()
	agentName, identityText, agentConfig, err := loadAgentConfig(cfg.AgentName, cfg.CommandName, cfg.ModelExplicitlySet)
	if err != nil {
		return nil, timing, err
	}
	e.AgentName = agentName
	e.AgentConfig = agentConfig

	// Update model from agent config if not explicitly set
	if agentConfig != nil && agentConfig.Model != "" && !cfg.ModelExplicitlySet {
		e.Model = agentConfig.Model
	}
	timing.ConfigTime += time.Since(phaseStart)

	// Phase 3: Setup memory store and context
	phaseStart = time.Now()
	if cfg.SystemOverride == "" && !cfg.Raw {
		memStore, err := setupMemoryStore(ctx, repoRoot, identityText, e.AgentName)
		if err != nil {
			return nil, timing, err
		}
		e.MemStore = memStore
	} else if cfg.Raw && identityText == "" {
		identityText = "You are a helpful assistant."
	}
	timing.MemoryStoreTime = time.Since(phaseStart)

	// Phase 4: Setup session management and workspace
	phaseStart = time.Now()
	session, sessionDB, workspace, messages, turnCounter, err := setupSession(repoRoot, e.AgentName, cfg.CommandName, cfg.NewSession, cfg.Detach)
	if err != nil {
		return nil, timing, fmt.Errorf("failed to setup session: %w", err)
	}
	e.Session = session
	e.SessionDB = sessionDB
	e.workspace = workspace
	// Restore rather than assign: this path never reaches
	// initLimitsAndProfiling, so without it e.staged is built lazily at
	// FrozenIdx 0, the loaded transcript is treated as staging, and the turn
	// count read back by setupSession is dropped on the floor.
	e.RestoreSession(messages, turnCounter)
	timing.SessionPrepTime = time.Since(phaseStart)

	// Phase 5: Setup model store
	phaseStart = time.Now()
	modelStore, err := setupModelStore(cfg.ModelStore)
	if err != nil {
		return nil, timing, fmt.Errorf("failed to setup model store: %w", err)
	}
	e.modelStore = modelStore
	e.authStore = setupAuthStore()
	timing.ModelStoreTime = time.Since(phaseStart)

	// Phase 6: Create model client
	phaseStart = time.Now()
	modelClient, resolvedModel, anthropicClient, err := createModelClient(e.Model, e.modelStore, e.authStore)
	if err != nil {
		return nil, timing, err
	}
	e.Model = resolvedModel
	e.modelClient = modelClient
	e.Client = anthropicClient
	timing.ModelClientTime = time.Since(phaseStart)

	// Phase 7: Setup session logging and workflow hooks
	phaseStart = time.Now()
	if e.Session != nil {
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
	timing.HooksSetupTime = time.Since(phaseStart)

	// Phase 8: Setup agent registry
	phaseStart = time.Now()
	agentRegistry, err := setupAgentRegistry(e.AgentConfig, e.Client, repoRoot, modelClient, e.modelStore, e.MemStore)
	if err != nil {
		return nil, timing, err
	}
	e.agentRegistry = agentRegistry
	timing.AgentRegistryTime = time.Since(phaseStart)

	// Phase 9: Build and configure tools
	phaseStart = time.Now()
	if !cfg.NoTools {
		e.setToolSet(mergeExtraTools(e.buildTools(), cfg.ExtraTools))
		
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
	timing.ToolsBuildingTime = time.Since(phaseStart)

	// Phase 10: Final setup and limits
	phaseStart = time.Now()
	
	// Store system override for raw/override modes
	if cfg.SystemOverride != "" {
		e.IdentityOverride = cfg.SystemOverride
	} else if cfg.Raw {
		e.IdentityOverride = identityText
	}

	// Setup limits
	e.Limits = setupLimits(cfg, e.AgentConfig)

	// Setup memory profiling
	memoryProfiler, err := setupMemoryProfiling(cfg.MemoryProfiling, cfg.MemoryLogPath)
	if err != nil {
		return nil, timing, err
	}
	e.memoryProfiler = memoryProfiler
	
	timing.OverheadTime = time.Since(phaseStart)
	timing.TotalTime = time.Since(startTime)

	return e, timing, nil
}