package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/forge"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/agent/registry"
	inbercontext "github.com/kayushkin/inber/context"
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
	CommandName    string // "chat" or "run" for session registration
	NewSession     bool   // start fresh instead of continuing default session
	Detach         bool   // one-off session, don't save to workspace
	Display        *DisplayHooks
	StashConfig    *conversation.StashConfig      // Large message stashing config (nil = use defaults)
	ExtractConfig  *conversation.ExtractionConfig // Background extraction config (nil = use defaults)
	AutoWorkflow   AutoWorkflowConfig // Auto-branch, auto-commit, auto-format (Phase 1)
	MaxTurns       int            // max API round-trips per RunTurn (0 = unlimited)
	MaxInputTokens int            // max cumulative input tokens per RunTurn (0 = unlimited)
	Injections     <-chan string  // channel for mid-run message injection (optional, from stdin)
}

// Engine encapsulates the shared setup and execution logic for chat and run.
type Engine struct {
	Client       *anthropic.Client
	Agent        *agent.Agent
	ContextStore *inbercontext.Store
	MemStore     *memory.Store
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
	workspace       *sessionMod.Workspace
	thinkingBud     int64
	lastNamedBlocks []sessionMod.NamedBlock
	stashCfg        conversation.StashConfig
	extractCfg      conversation.ExtractionConfig
	consecutiveErrors  int  // track consecutive tool errors for context escalation
	lastTurnHadError   bool
	autoRefMgr      *memory.AutoReferenceManager // auto-creates references after tool calls
	toolInputsCache map[string]string             // toolID -> input JSON for auto-reference creation
	workflowHooks   *WorkflowHooks                // auto-commit, auto-format, build/test
	forgeHook       *forge.Hook                   // workspace/preview automation
	forgeDB         *forge.Forge                  // forge database handle
	modelStore      *modelstore.Store             // model usage tracking (opened once, closed in Close())
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
}

// NewEngine creates and fully initializes an Engine: context, memory, tools, session, hooks.
func NewEngine(cfg EngineConfig) (*Engine, error) {
	repoRoot := cfg.RepoRoot
	if repoRoot == "" {
		var err error
		repoRoot, err = FindRepoRoot()
		if err != nil {
			repoRoot, _ = os.Getwd()
		}
	}

	// Initialize configs with defaults if not provided
	stashCfg := conversation.DefaultStashConfig()
	if cfg.StashConfig != nil {
		stashCfg = *cfg.StashConfig
	}

	extractCfg := conversation.DefaultExtractionConfig()
	if cfg.ExtractConfig != nil {
		extractCfg = *cfg.ExtractConfig
	}

	e := &Engine{
		Model:              cfg.Model,
		repoRoot:           repoRoot,
		display:            cfg.Display,
		thinkingBud:        cfg.Thinking,
		stashCfg:           stashCfg,
		extractCfg:         extractCfg,
		modelExplicitlySet: cfg.ModelExplicitlySet,
	}

	// Load agent config — resolve agent name from flag, config default, or fallback
	agentName := cfg.AgentName
	var identityText string

	// Load from agent-store (the only source of truth)
	registryCfg, fromStore := registry.LoadConfigWithFallback("", "")
	if !fromStore || registryCfg == nil {
		return nil, fmt.Errorf("failed to load agent config from agent-store")
	}
	Log.Info("loaded config from agent-store")

	// If no agent specified, use the default from config
	if agentName == "" && registryCfg.Default != "" {
		agentName = registryCfg.Default
	}

	if agentName != "" && registryCfg != nil {
		ac, ok := registryCfg.Agents[agentName]
		if !ok {
			return nil, fmt.Errorf("agent not found: %s", agentName)
		}
		e.AgentConfig = ac
		// Only use agent config model if user didn't explicitly set --model flag
		if ac.Model != "" && !cfg.ModelExplicitlySet {
			e.Model = ac.Model
		}
		identityText = ac.System
		e.AgentName = agentName
	} else if agentName == "" {
		e.AgentName = cfg.CommandName
		if e.AgentName == "" {
			e.AgentName = "default"
		}
	}

	// Context & memory
	if cfg.SystemOverride == "" && !cfg.Raw {
		Log.Infof("loading context...")
		
		// Memory store
		ms, err := memory.OpenOrCreate(repoRoot)
		if err != nil {
			return nil, fmt.Errorf("failed to open memory store: %w", err)
		}
		e.MemStore = ms

		// Prepare session: load identity + recent files into memory
		if identityText == "" {
			identityText = "You are Claxon 🦀, the main orchestrator agent. Casual, direct, not flowery. You have shell access, file tools, memory, and can spawn project agents. Get to the point."
		}
		
		prepCfg := memory.PrepareSessionConfig{
			RootDir:        repoRoot,
			IdentityText:   identityText,
			AgentName:      e.AgentName,
			RecencyWindow:  24 * time.Hour,
			RecentFilesTTL: 10 * time.Minute,
		}
		
		if err := ms.PrepareSession(prepCfg); err != nil {
			Log.Warn("failed to prepare session: %v", err)
		}
		// Count memories for logging
		recentMems, _ := ms.ListRecent(100, 0.0)
		fmt.Fprintf(os.Stderr, " done (%d memories)\n", len(recentMems))

		// Initialize auto-reference manager for creating references after tool calls
		autoRefConfig := memory.DefaultAutoReferenceConfig()
		e.autoRefMgr = memory.NewAutoReferenceManager(ms, repoRoot, autoRefConfig)
		e.toolInputsCache = make(map[string]string)

		// Keep a minimal context store for backward compatibility
		// (some tools might still use it)
		e.ContextStore = inbercontext.NewStore()
	} else if cfg.Raw && identityText == "" {
		identityText = "You are a helpful assistant."
	}

	// Session continuity: resume by default, --new to start fresh, --detach for one-off
	ws := sessionMod.NewWorkspace(repoRoot, e.AgentName)
	if cfg.Detach {
		// Detached: don't load or save workspace messages
		e.workspace = nil
	} else {
		e.workspace = ws
		if !cfg.NewSession {
			if msgs, err := ws.LoadMessages(); err == nil && len(msgs) > 0 {
				repaired := conversation.RepairEmptyContent(msgs)
				repaired = conversation.RepairDanglingToolUse(repaired)
				repaired = conversation.RepairAlternation(repaired)
				repaired = agent.SanitizeMessageToolIDs(repaired)
				e.Messages = repaired
				// Save repaired messages back so we don't re-repair every time
				if conversation.LastRepairCount > 0 || len(repaired) != len(msgs) {
					if data, err := json.Marshal(repaired); err == nil {
						ws.SaveMessages(data)
						Log.Info("repaired session messages (%d tool calls, %d→%d messages)",
							conversation.LastRepairCount, len(msgs), len(repaired))
					}
				}
				Log.Info("resuming session (%d messages)", len(e.Messages))
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
		e.SessionDB = sdb
		if n, _ := sdb.DetectInterrupted(); n > 0 {
			Log.Warn("detected %d interrupted session(s) from previous runs", n)
		}
	}

	sess, err := sessionMod.New("logs", e.Model, e.AgentName, "")
	if err != nil {
		Log.Warn("logging disabled: %v", err)
	} else {
		e.Session = sess
		Log.Info("logging to %s", sess.FilePath())
		if sdb != nil {
			sess.AttachDB(sdb, cfg.CommandName)
		}
		
		// Configure automatic truncation of large tool results
		truncCfg := sessionMod.TruncateConfigForRole(e.AgentName)
		sess.SetTruncateConfig(truncCfg)
		
		// Initialize workflow automation (auto-commit, auto-format)
		workflowCfg := cfg.AutoWorkflow
		if workflowCfg == (AutoWorkflowConfig{}) {
			workflowCfg = DefaultAutoWorkflowConfig()
		}
		e.workflowHooks = NewWorkflowHooks(repoRoot, sess.SessionID(), e.AgentName, workflowCfg)
	}

	// Initialize forge hook (project detection for build/preview tracking)
	if f, err := forge.Open(""); err == nil {
		e.forgeDB = f
		if proj := f.FindProjectByPath(repoRoot); proj != nil {
			e.forgeHook = f.NewHook(forge.HookConfig{
				Project:     proj.ID,
				AutoBuild:   false,
				AutoPreview: false,
			})
			Log.Info("forge: project %q detected", proj.ID)
		}
	}

	// Open model-store once for the lifetime of the engine
	store, err := modelstore.Open("")
	if err == nil {
		e.modelStore = store
		// Register OAuth providers for token refresh
		store.RegisterDefaultOAuthProviders()
		// Enable auto-sync to OpenClaw's auth-profiles.json
		store.EnableAuthProfileSync("")
		// Seed if empty (one-time init)
		providers, _ := store.Providers()
		if len(providers) == 0 {
			if err := store.Seed(); err != nil {
				Log.Warn("failed to seed model-store: %v", err)
			}
		}
		// Initial sync to ensure OpenClaw has latest credentials
		if syncErr := store.SyncToAuthProfiles(""); syncErr != nil {
			Log.Warn("initial auth-profiles sync: %v", syncErr)
		}
	} else {
		Log.Warn("model-store unavailable: %v", err)
	}
	
	modelClient, err := agent.NewModelClient(e.Model, e.modelStore)
	if err != nil {
		return nil, fmt.Errorf("failed to create model client: %w", err)
	}
	Log.Info("model: %s (provider=%s, openai=%v)", e.Model, modelClient.Provider, modelClient.IsOpenAI())
	
	// Update e.Model with the resolved model ID (in case it was an alias)
	if modelClient.Model != nil {
		e.Model = modelClient.Model.ID
	}
	
	// Store the model client for later use in RunTurn
	e.modelClient = modelClient
	
	// For Anthropic, set the client field (for backward compatibility)
	if modelClient.AnthropicClient != nil {
		e.Client = modelClient.AnthropicClient
	}

	// Create agent registry if spawn tools are needed
	if e.AgentConfig != nil && e.needsSpawnTools(e.AgentConfig.Tools) {
		reg, _, err := registry.NewWithFallback(e.Client, "", filepath.Join(repoRoot, "logs"))
		if err != nil {
			Log.Warn("failed to create agent registry: %v", err)
		} else {
			e.agentRegistry = reg
			// Set model client for OpenAI-compatible providers
			reg.SetModelClient(modelClient)
			// Set model store for creating per-agent model clients
			if e.modelStore != nil {
				reg.SetModelStore(e.modelStore)
			}
			// Set memory store for memory tools in spawned agents
			if e.MemStore != nil {
				reg.SetMemoryStore(e.MemStore)
			}
			Log.Info("agent registry enabled for spawn tools (from agent-store)")
		}
	}

	// Tools
	if !cfg.NoTools {
		e.agentTools = e.buildTools()

		// Append gateway-injected tools (replace same-named tools).
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
		
		// Load tool registry into memory so agent knows what tools are available
		if e.MemStore != nil {
			toolMetas := make([]memory.ToolMetadata, 0, len(e.agentTools))
			for _, t := range e.agentTools {
				category := "general"
				if strings.HasPrefix(t.Name, "read_") || strings.HasPrefix(t.Name, "write_") || 
				   strings.HasPrefix(t.Name, "edit_") || strings.HasPrefix(t.Name, "list_") {
					category = "filesystem"
				} else if t.Name == "repo_map" || t.Name == "recent_files" {
					category = "code-introspection"
				} else if strings.HasPrefix(t.Name, "memory_") {
					category = "memory"
				} else if t.Name == "shell" {
					category = "execution"
				}
				
				toolMetas = append(toolMetas, memory.ToolMetadata{
					Name:        t.Name,
					Description: t.Description,
					Category:    category,
				})
			}
			
			if err := e.MemStore.LoadToolRegistry(toolMetas); err != nil {
				Log.Warn("failed to load tool registry: %v", err)
			}
		}
	}

	// Store system override for raw/override modes
	if cfg.SystemOverride != "" {
		// Create a minimal context store with just the override
		e.ContextStore = inbercontext.NewStore()
		e.ContextStore.Add(inbercontext.Chunk{
			ID:     "system-override",
			Text:   cfg.SystemOverride,
			Tags:   []string{"identity"},
			Source: "override",
		})
	} else if cfg.Raw {
		e.ContextStore = inbercontext.NewStore()
		e.ContextStore.Add(inbercontext.Chunk{
			ID:     "identity",
			Text:   identityText,
			Tags:   []string{"identity"},
			Source: "identity",
		})
	}

	// Write tools list to workspace for reference
	if e.workspace != nil && len(e.agentTools) > 0 {
		toolInfos := make([]sessionMod.ToolInfo, len(e.agentTools))
		for i, t := range e.agentTools {
			toolInfos[i] = sessionMod.ToolInfo{Name: t.Name, Description: t.Description}
		}
		e.workspace.WriteToolsList(toolInfos)
	}

	// Hooks
	e.noHooks = cfg.NoHooks

	// Initialize turn/token limits
	e.maxTurns = cfg.MaxTurns
	e.maxInputTokens = cfg.MaxInputTokens
	if e.AgentConfig != nil && e.AgentConfig.Limits != nil {
		if e.maxTurns == 0 {
			e.maxTurns = e.AgentConfig.Limits.MaxTurns
		}
		if e.maxInputTokens == 0 {
			e.maxInputTokens = e.AgentConfig.Limits.MaxInputTokens
		}
		if e.maxResponseTime == 0 {
			e.maxResponseTime = e.AgentConfig.Limits.MaxResponseTime
		}
	}
	if cfg.Detach {
		if e.maxTurns == 0 {
			e.maxTurns = 25
		}
		if e.maxInputTokens == 0 {
			e.maxInputTokens = 500000
		}
	}
	if e.maxTurns > 0 || e.maxInputTokens > 0 || e.maxResponseTime > 0 {
		Log.Info("limits: maxTurns=%d, maxInputTokens=%d, maxResponseTime=%ds", e.maxTurns, e.maxInputTokens, e.maxResponseTime)
	}

	// Mid-run injection channel
	e.injections = cfg.Injections

	return e, nil
}

