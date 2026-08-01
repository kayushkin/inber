package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	toolstoretools "github.com/kayushkin/tool-store/tools"
	"github.com/kayushkin/aiauth"
	"github.com/kayushkin/forge"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/agent/registry"
	"github.com/kayushkin/inber/conversation"
	"github.com/kayushkin/inber/internal/fsutil"
	"github.com/kayushkin/inber/memory"
	sessionMod "github.com/kayushkin/inber/session"
	modelstore "github.com/kayushkin/model-store"
)

// setupRepoRoot validates and sets up the repository root directory.
func setupRepoRoot(configRepoRoot string) (string, error) {
	repoRoot := configRepoRoot
	if repoRoot == "" {
		var err error
		repoRoot, err = fsutil.FindRepoRoot()
		if err != nil {
			repoRoot, _ = os.Getwd()
		}
	}
	// Safety: never use home directory as repo root — would walk entire home tree
	if home, _ := os.UserHomeDir(); repoRoot == home {
		Log.Warn("repo root resolved to home directory, refusing — set workspace in agent config")
		repoRoot = ""
	}
	return repoRoot, nil
}

// initializeConfigs sets up default configurations if not provided.
func initializeConfigs(cfg EngineConfig) (conversation.StashConfig, conversation.ExtractionConfig) {
	stashCfg := conversation.DefaultStashConfig()
	if cfg.StashConfig != nil {
		stashCfg = *cfg.StashConfig
	}

	extractCfg := conversation.DefaultExtractionConfig()
	if cfg.ExtractConfig != nil {
		extractCfg = *cfg.ExtractConfig
	}

	return stashCfg, extractCfg
}

// loadAgentConfig loads agent configuration from the registry.
func loadAgentConfig(agentName string, commandName string, modelExplicitlySet bool) (string, string, *registry.AgentConfig, error) {
	var identityText string
	var resolvedAgentName string
	var agentConfig *registry.AgentConfig

	// Load from agent-store (the only source of truth)
	registryCfg, err := registry.LoadConfig()
	if err != nil || registryCfg == nil {
		return "", "", nil, fmt.Errorf("failed to load agent config from agent-store: %v", err)
	}
	Log.Info("loaded config from agent-store")

	// If no agent specified, use the default from config
	if agentName == "" && registryCfg.Default != "" {
		agentName = registryCfg.Default
	}

	if agentName != "" && registryCfg != nil {
		ac, ok := registryCfg.Agents[agentName]
		if !ok {
			return "", "", nil, fmt.Errorf("agent not found: %s", agentName)
		}
		agentConfig = ac
		identityText = ac.System
		resolvedAgentName = agentName
	} else if agentName == "" {
		resolvedAgentName = commandName
		if resolvedAgentName == "" {
			resolvedAgentName = "default"
		}
	}

	return resolvedAgentName, identityText, agentConfig, nil
}

// setupMemoryStore initializes the memory store and prepares the session.
//
// ctx bounds preparation, which scans the workspace for recently modified
// files: git first, and a walk of the whole tree when there is no git history
// to read.
func setupMemoryStore(ctx context.Context, repoRoot, identityText, agentName string) (memory.MemoryStore, error) {
	Log.Infof("loading context (repoRoot=%s)...", repoRoot)

	ms, err := memory.OpenOrCreate(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to open memory store: %w", err)
	}

	// Prepare session: load identity + recent files into memory
	if identityText == "" {
		identityText = "You are Claxon 🦀, the main orchestrator agent. Casual, direct, not flowery. You have shell access, file tools, memory, and can spawn project agents. Get to the point."
	}
	
	prepCfg := memory.PrepareSessionConfig{
		RootDir:        repoRoot,
		IdentityText:   identityText,
		AgentName:      agentName,
		RecencyWindow:  24 * time.Hour,
		RecentFilesTTL: 10 * time.Minute,
	}
	
	if err := ms.PrepareSession(ctx, prepCfg); err != nil {
		// A session that could not be prepared still runs, with less in memory
		// than it should have — that is why this is a warning.
		//
		// A caller who withdrew is a different event, and continuing to build
		// an engine for it would report a working session to nobody. Ask the
		// context rather than the error: preparation's own scan deadline
		// arrives here looking exactly like a cancellation.
		if ctxErr := ctx.Err(); ctxErr != nil {
			ms.Close()
			return nil, ctxErr
		}
		Log.Warn("failed to prepare session: %v", err)
	}

	// Count memories for logging
	recentMems, _ := ms.ListRecent(100, 0.0)
	fmt.Fprintf(os.Stderr, " done (%d memories)\n", len(recentMems))

	return ms, nil
}

// setupSession initializes session management and workspace, and loads the
// resumable state: the persisted transcript and the turn count that produced it.
// The two are loaded in the same branch on purpose — a turn count restored
// without its transcript describes messages that are not there.
func setupSession(repoRoot, agentName, commandName string, newSession, detach bool) (*sessionMod.Session, *sessionMod.DB, *sessionMod.Workspace, []anthropic.MessageParam, int, error) {
	var session *sessionMod.Session
	var sessionDB *sessionMod.DB
	var workspace *sessionMod.Workspace
	var messages []anthropic.MessageParam
	var turnCounter int

	// Session continuity: resume by default, --new to start fresh, --detach for one-off
	ws := sessionMod.NewWorkspace(repoRoot, agentName)
	if detach {
		// Detached: don't load or save workspace messages
		workspace = nil
	} else {
		workspace = ws
		if !newSession {
			msgs, err := ws.LoadMessages()
			// No transcript yet is the ordinary case: every workspace is that
			// until its first turn ends. A transcript that exists and cannot be
			// read is not, and it must not be walked past — saveResumableState
			// rewrites this same file at the end of every turn, so carrying on
			// would replace the conversation we failed to read with one that
			// starts empty. Stop and say which file, so it can be moved aside.
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return nil, nil, nil, nil, 0, fmt.Errorf(
					"read workspace transcript %s: %w (move it aside or pass --new to start a fresh session)",
					filepath.Join(ws.Dir, "messages.json"), err)
			}
			if len(msgs) > 0 {
				repaired := conversation.RepairEmptyContent(msgs)
				repaired, repairCount := conversation.RepairDanglingToolUse(repaired)
				repaired = conversation.RepairAlternation(repaired)
				repaired = agent.SanitizeMessageToolIDs(repaired)
				messages = repaired
				// Save repaired messages back so we don't re-repair every time
				if repairCount > 0 || len(repaired) != len(msgs) {
					if data, err := json.Marshal(repaired); err == nil {
						ws.SaveMessages(data)
						Log.Info("repaired session messages (%d tool calls, %d→%d messages)",
							repairCount, len(msgs), len(repaired))
					}
				}
				counter, err := sessionMod.LoadTurnCounter(ws.Dir)
				if err != nil {
					Log.Warn("turn counter unreadable, resuming as if from turn 0: %v", err)
				}
				turnCounter = counter
				Log.Info("resuming session (%d messages, turn %d)", len(messages), turnCounter)
			}
		} else {
			ws.ClearMessages()
		}
	}

	// Session DB (tracks sessions/turns in SQLite)
	sdb, err := sessionMod.OpenDB(repoRoot)
	if err != nil {
		Log.Warn("session tracking disabled: %v", err)
	} else {
		sessionDB = sdb
		if n, _ := sdb.DetectInterrupted(); n > 0 {
			Log.Warn("detected %d interrupted session(s) from previous runs", n)
		}
	}

	// The session logger every engine hook writes through. sessionMod.New owns
	// the whole layout below this root — the agent segment included — and
	// creates the directories itself, so there is neither a join nor a mkdir to
	// do here. This used to append agentName before handing the path over, and
	// because sessionMod.New appends it again the logs landed in
	// logs/<agent>/<agent>/. The registry, built eighty lines below this, has
	// always passed the same root unjoined.
	logsDir := filepath.Join(repoRoot, "logs")

	// A session that cannot be created fails session setup. The fallback that
	// used to stand here — a zero-value Session, installed with a comment
	// saying it prevented nil pointer crashes — did the opposite: its json
	// encoder and log file are both nil, so LogUser, LogToolResult, FilePath
	// and Close each panic on first use. LogUser runs on every turn, so the
	// warning below the failure was one turn away from a crash it described as
	// avoided. There is nothing to degrade to: a logger with no file open logs
	// nothing, and every caller here treats the session as writable.
	session, err = sessionMod.New(logsDir, "", agentName, "", nil)
	if err != nil {
		return nil, nil, nil, nil, 0, fmt.Errorf("create session logger in %s: %w", logsDir, err)
	}

	return session, sessionDB, workspace, messages, turnCounter, nil
}

// setupModelStore initializes or uses the provided model store.
func setupModelStore(providedStore *modelstore.Store) (*modelstore.Store, error) {
	// Use shared model-store if provided, otherwise open our own
	if providedStore != nil {
		return providedStore, nil
	}

	store, err := modelstore.Open("")
	if err != nil {
		Log.Warn("model-store unavailable: %v", err)
		return nil, err
	}

	// Seed if empty (one-time init)
	providers, _ := store.Providers()
	if len(providers) == 0 {
		if err := store.Seed(); err != nil {
			Log.Warn("failed to seed model-store: %v", err)
		}
	}

	return store, nil
}

// setupAuthStore initializes the aiauth credential store.
func setupAuthStore() *aiauth.Store {
	return aiauth.DefaultStore()
}

// createModelClient creates the appropriate model client for the given model.
func createModelClient(model string, store *modelstore.Store, auth *aiauth.Store) (*agent.ModelClient, string, *anthropic.Client, error) {
	modelClient, err := agent.NewModelClient(model, store, auth)
	if err != nil {
		return nil, "", nil, fmt.Errorf("failed to create model client: %w", err)
	}
	Log.Info("model: %s (provider=%s, openai=%v)", model, modelClient.Provider, modelClient.IsOpenAI())
	
	// Update model with the resolved model ID (in case it was an alias)
	resolvedModel := model
	if modelClient.Model != nil {
		resolvedModel = modelClient.Model.ID
	}
	
	// For Anthropic, extract the client field (for backward compatibility)
	var anthropicClient *anthropic.Client
	if modelClient.AnthropicClient != nil {
		anthropicClient = modelClient.AnthropicClient
	}

	return modelClient, resolvedModel, anthropicClient, nil
}

// setupAgentRegistry creates agent registry if spawn tools are needed.
func setupAgentRegistry(agentConfig *registry.AgentConfig, client *anthropic.Client, repoRoot string, modelClient *agent.ModelClient, modelStore *modelstore.Store, memStore memory.MemoryStore) (*registry.Registry, error) {
	if agentConfig == nil || !needsSpawnTools(agentConfig.Tools) {
		return nil, nil
	}

	reg, err := registry.New(client, filepath.Join(repoRoot, "logs"))
	if err != nil {
		Log.Warn("failed to create agent registry: %v", err)
		return nil, err
	}

	// Set model client for OpenAI-compatible providers
	reg.SetModelClient(modelClient)
	// Set model store for creating per-agent model clients
	if modelStore != nil {
		reg.SetModelStore(modelStore)
	}
	// Set memory store for memory tools in spawned agents
	if memStore != nil {
		reg.SetMemoryStore(memStore)
	}
	Log.Info("agent registry enabled for spawn tools (from agent-store)")

	return reg, nil
}

// needsSpawnTools checks if the agent configuration requires spawn tools.
func needsSpawnTools(tools []string) bool {
	for _, tool := range tools {
		if tool == "spawn_agent" || tool == "spawn_*" {
			return true
		}
	}
	return false
}

// setupForgeHook initializes forge project detection and hooks.
func setupForgeHook(repoRoot, sessionID, agentName string, workflowConfig AutoWorkflowConfig) (*forge.Forge, *forge.Hook, error) {
	f, err := forge.Open("")
	if err != nil {
		return nil, nil, err
	}

	var forgeHook *forge.Hook
	if proj := f.FindProjectByPath(repoRoot); proj != nil {
		forgeHook = f.NewHook(forge.HookConfig{
			Project:     proj.ID,
			AutoBuild:   false,
			AutoPreview: false,
		})
		Log.Info("forge: project %q detected", proj.ID)
	}

	return f, forgeHook, nil
}

// loadToolsIntoMemory loads the tool registry into memory store.
func loadToolsIntoMemory(memStore memory.MemoryStore, tools []agent.Tool) error {
	if memStore == nil {
		return nil
	}

	toolMetas := make([]memory.ToolMetadata, 0, len(tools))
	for _, t := range tools {
		category := "general"
		if strings.HasPrefix(t.Name, "read_") || strings.HasPrefix(t.Name, "write_") || 
		   strings.HasPrefix(t.Name, "edit_") || strings.HasPrefix(t.Name, "list_") {
			category = "filesystem"
		} else if t.Name == "repo_map" || t.Name == "recent_files" {
			category = "code-introspection"
		} else if strings.HasPrefix(t.Name, "memory_") {
			category = "memory"
		} else if t.Name == "shell" || t.Name == "shell_commands" {
			category = "execution"
		}
		
		toolMetas = append(toolMetas, memory.ToolMetadata{
			Name:        t.Name,
			Description: t.Description,
			Category:    category,
		})
	}
	
	return memStore.LoadToolRegistry(toolMetas)
}

// setupLimits initializes turn/token/cost limits from config and agent config.
//
// It returns the whole LimitConfig rather than a list of numbers because the
// caller's job is to install every limit the engine has. The earlier signature
// returned three ints, and MaxCost — declared beside MaxTurns and
// MaxInputTokens on EngineConfig, and accepted as `max_cost` over the HTTP API
// — was simply left out of the list, so nothing ever read it. Returning the
// struct means a limit added to LimitConfig arrives at the guard by default
// instead of only if someone remembers to widen a tuple.
func setupLimits(cfg EngineConfig, agentConfig *registry.AgentConfig) LimitConfig {
	limits := LimitConfig{
		MaxTurns:       cfg.MaxTurns,
		MaxInputTokens: cfg.MaxInputTokens,
		MaxCost:        cfg.MaxCost,
		MaxDuration:    cfg.MaxDuration,
	}

	// registry.AgentLimits has no per-agent cost cap and no per-agent session
	// duration, so MaxCost and MaxDuration have no agent-config fallback to
	// read. Adding one is a change to the stored agent config schema, which this
	// wiring deliberately does not make.
	if agentConfig != nil && agentConfig.Limits != nil {
		if limits.MaxTurns == 0 {
			limits.MaxTurns = agentConfig.Limits.MaxTurns
		}
		if limits.MaxInputTokens == 0 {
			limits.MaxInputTokens = agentConfig.Limits.MaxInputTokens
		}
		if limits.MaxResponseTime == 0 {
			limits.MaxResponseTime = agentConfig.Limits.MaxResponseTime
		}
	}

	if cfg.Detach {
		if limits.MaxTurns == 0 {
			limits.MaxTurns = 25
		}
		if limits.MaxInputTokens == 0 {
			limits.MaxInputTokens = 500000
		}
		// No default cost cap for detached sessions: MaxTurns and
		// MaxInputTokens already bound them, and a dollar figure invented here
		// would be a spend policy nobody asked for.
	}

	if limits.MaxTurns > 0 || limits.MaxInputTokens > 0 || limits.MaxResponseTime > 0 || limits.MaxCost > 0 || limits.MaxDuration > 0 {
		Log.Info("limits: maxTurns=%d, maxInputTokens=%d, maxResponseTime=%ds, maxCost=$%.2f, maxDuration=%ds",
			limits.MaxTurns, limits.MaxInputTokens, limits.MaxResponseTime, limits.MaxCost, limits.MaxDuration)
	}

	return limits
}

// setupMemoryProfiling initializes memory profiling if enabled.
func setupMemoryProfiling(enabled bool, logPath string) (*MemoryProfiler, error) {
	if !enabled {
		return nil, nil
	}

	memoryProfiler, err := NewMemoryProfiler(true, logPath)
	if err != nil {
		Log.Warn("failed to initialize memory profiler: %v", err)
		return nil, err
	}

	memoryProfiler.Start()
	Log.Info("memory profiling enabled")
	return memoryProfiler, nil
}

// ---------------------------------------------------------------------------
// init* methods — called by NewEngine in engine.go
// ---------------------------------------------------------------------------

// initAgent loads agent identity from registry and resolves the model.
func (e *Engine) initAgent(cfg EngineConfig) error {
	agentName, identityText, agentConfig, err := loadAgentConfig(cfg.AgentName, cfg.CommandName, cfg.ModelExplicitlySet)
	if err != nil {
		return err
	}
	e.AgentName = agentName
	e.AgentConfig = agentConfig

	if agentConfig != nil && agentConfig.Model != "" && !cfg.ModelExplicitlySet {
		e.Model = agentConfig.Model
	}

	// Stash identity text for initMemory (via closure-free field)
	e.IdentityOverride = identityText
	return nil
}

// initMemory opens the semantic memory store and prepares the session context.
func (e *Engine) initMemory(ctx context.Context, cfg EngineConfig) error {
	identityText := e.IdentityOverride

	if cfg.SystemOverride != "" {
		e.IdentityOverride = cfg.SystemOverride
		return nil
	}
	if cfg.Raw {
		if identityText == "" {
			identityText = "You are a helpful assistant."
		}
		e.IdentityOverride = identityText
		return nil
	}

	// Normal mode: open memory store, clear identity override (memory provides it)
	memStore, err := setupMemoryStore(ctx, e.repoRoot, identityText, e.AgentName)
	if err != nil {
		return err
	}
	e.MemStore = memStore
	e.IdentityOverride = "" // memory store provides identity
	return nil
}

// initSession sets up JSONL logging, workspace, and message persistence.
func (e *Engine) initSession(cfg EngineConfig) error {
	session, sessionDB, workspace, messages, turnCounter, err := setupSession(e.repoRoot, e.AgentName, cfg.CommandName, cfg.NewSession, cfg.Detach)
	if err != nil {
		return fmt.Errorf("session: %w", err)
	}
	e.Session = session
	e.SessionDB = sessionDB
	e.workspace = workspace
	// Held, not installed: e.staged does not exist yet, and installing a
	// restored transcript means freezing it. initLimitsAndProfiling builds
	// e.staged and then hands both to RestoreSession.
	e.Messages = messages
	e.restoredTurnCounter = turnCounter
	return nil
}

// initModelClient creates the unified Anthropic/OpenAI client with auth.
func (e *Engine) initModelClient(cfg EngineConfig) error {
	modelStore, err := setupModelStore(cfg.ModelStore)
	if err != nil {
		return fmt.Errorf("model store: %w", err)
	}
	e.modelStore = modelStore
	e.ownsModelStore = (cfg.ModelStore == nil)
	e.authStore = setupAuthStore()

	modelClient, resolvedModel, anthropicClient, err := createModelClient(e.Model, e.modelStore, e.authStore)
	if err != nil {
		return err
	}
	e.Model = resolvedModel
	e.modelClient = modelClient
	e.Client = anthropicClient
	return nil
}

// initWorkflow sets up session logging, workflow automation, and forge integration.
func (e *Engine) initWorkflow(cfg EngineConfig) {
	if e.Session != nil {
		Log.Info("logging to %s", e.Session.FilePath())
		if e.SessionDB != nil {
			e.Session.AttachDB(e.SessionDB, cfg.CommandName)
		}
		truncCfg := sessionMod.TruncateConfigForRole(e.AgentName)
		e.Session.SetTruncateConfig(truncCfg)

		workflowCfg := cfg.AutoWorkflow
		if workflowCfg == (AutoWorkflowConfig{}) {
			workflowCfg = DefaultAutoWorkflowConfig()
		}
		e.workflowHooks = NewWorkflowHooks(e.repoRoot, e.Session.SessionID(), e.AgentName, workflowCfg)
	}

	forgeDB, forgeHook, err := setupForgeHook(e.repoRoot, "", e.AgentName, AutoWorkflowConfig{})
	if err == nil {
		e.forgeDB = forgeDB
		e.forgeHook = forgeHook
	}

	agentRegistry, err := setupAgentRegistry(e.AgentConfig, e.Client, e.repoRoot, e.modelClient, e.modelStore, e.MemStore)
	if err == nil {
		e.agentRegistry = agentRegistry
	}
}

// initTools builds and configures the tool set.
func (e *Engine) initTools(cfg EngineConfig) {
	e.setToolSet(mergeExtraTools(e.buildTools(), cfg.ExtraTools))

	// Register context injectors for tools that provide always-visible context
	e.registerToolContextInjectors()

	if err := loadToolsIntoMemory(e.MemStore, e.agentTools); err != nil {
		Log.Warn("failed to load tool registry: %v", err)
	}

	if e.workspace != nil && len(e.agentTools) > 0 {
		toolInfos := make([]sessionMod.ToolInfo, len(e.agentTools))
		for i, t := range e.agentTools {
			toolInfos[i] = sessionMod.ToolInfo{Name: t.Name, Description: t.Description}
		}
		if err := e.workspace.WriteToolsList(toolInfos); err != nil {
			Log.Warn("failed to write workspace tool list: %v", err)
		}
	}
}

// registerToolContextInjectors adds context injectors for tools like task_plan and scratchpad.
func (e *Engine) registerToolContextInjectors() {
	for _, t := range e.agentTools {
		switch t.Name {
		case "task_plan":
			repoRoot := e.repoRoot
			e.contextInjectors = append(e.contextInjectors, func() []sessionMod.NamedBlock {
				content := toolstoretools.LoadPlanContext(repoRoot)
				if content == "" {
					return nil
				}
				return []sessionMod.NamedBlock{{ID: "task-plan", Text: content}}
			})
		case "scratchpad":
			repoRoot, agentName := e.repoRoot, e.AgentName
			e.contextInjectors = append(e.contextInjectors, func() []sessionMod.NamedBlock {
				content := toolstoretools.LoadScratchpadContext(repoRoot, agentName)
				if content == "" {
					return nil
				}
				return []sessionMod.NamedBlock{{ID: "scratchpad", Text: content}}
			})
		}
	}
}

// initLimitsAndProfiling sets up safety limits, memory profiling, cache config, and staged conversation.
func (e *Engine) initLimitsAndProfiling(cfg EngineConfig) {
	e.Limits = setupLimits(cfg, e.AgentConfig)

	if mp, err := setupMemoryProfiling(cfg.MemoryProfiling, cfg.MemoryLogPath); err == nil {
		e.memoryProfiler = mp
	}

	e.Cache.BlueprintEnabled = cfg.Blueprint
	if !e.Cache.BlueprintEnabled {
		if v := os.Getenv("INBER_BLUEPRINT"); v == "1" || v == "true" {
			e.Cache.BlueprintEnabled = true
		}
	}

	pruneCfg := e.pruneConfig()
	e.staged = conversation.NewStagedConversation(pruneCfg.ManageInterval)
	// initSession runs earlier in NewEngine and may already have loaded a
	// workspace transcript and its turn count. Now that e.staged exists, install
	// them the same way every other resume site does — see
	// Engine.RestoreSession for why a bare assignment re-prunes the transcript
	// and restarts the turn clock.
	e.RestoreSession(e.Messages, e.restoredTurnCounter)
}