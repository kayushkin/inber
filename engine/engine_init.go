package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/bus"
	"github.com/kayushkin/forge"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/agent/registry"
	"github.com/kayushkin/inber/conversation"
	"github.com/kayushkin/inber/memory"
	sessionMod "github.com/kayushkin/inber/session"
	modelstore "github.com/kayushkin/model-store"
)

// setupRepoRoot validates and sets up the repository root directory.
func setupRepoRoot(configRepoRoot string) (string, error) {
	repoRoot := configRepoRoot
	if repoRoot == "" {
		var err error
		repoRoot, err = FindRepoRoot()
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
func setupMemoryStore(repoRoot, identityText, agentName string) (memory.MemoryStore, error) {
	Log.Infof("loading context (repoRoot=%s)...", repoRoot)
	
	// Check if NATS URL is provided for network-based memory
	natsURL := os.Getenv("NATS_URL")
	
	var ms memory.MemoryStore
	var err error
	
	if natsURL != "" {
		// Use NATS-based memory store
		busClient, err := bus.Connect(bus.Options{
			URL:  natsURL,
			Name: "inber-memory-client",
		})
		if err != nil {
			Log.Warn("NATS connection failed, falling back to SQLite: %v", err)
		} else {
			ms = memory.NewNATSStore(busClient, 10*time.Second)
			Log.Info("using NATS memory store (url: %s)", natsURL)
		}
	}
	
	if ms == nil {
		// Fallback to SQLite-based memory store
		ms, err = memory.OpenOrCreate(repoRoot)
		if err != nil {
			return nil, fmt.Errorf("failed to open memory store: %w", err)
		}
		Log.Info("using SQLite memory store")
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
	
	if err := ms.PrepareSession(prepCfg); err != nil {
		Log.Warn("failed to prepare session: %v", err)
	}

	// Count memories for logging
	recentMems, _ := ms.ListRecent(100, 0.0)
	fmt.Fprintf(os.Stderr, " done (%d memories)\n", len(recentMems))

	return ms, nil
}

// setupSession initializes session management and workspace.
func setupSession(repoRoot, agentName, commandName string, newSession, detach bool) (*sessionMod.Session, *sessionMod.DB, *sessionMod.Workspace, []anthropic.MessageParam, error) {
	var session *sessionMod.Session
	var sessionDB *sessionMod.DB
	var workspace *sessionMod.Workspace
	var messages []anthropic.MessageParam

	// Session continuity: resume by default, --new to start fresh, --detach for one-off
	ws := sessionMod.NewWorkspace(repoRoot, agentName)
	if detach {
		// Detached: don't load or save workspace messages
		workspace = nil
	} else {
		workspace = ws
		if !newSession {
			if msgs, err := ws.LoadMessages(); err == nil && len(msgs) > 0 {
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
				Log.Info("resuming session (%d messages)", len(messages))
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

	// Create a basic session for engine hooks - we'll do full initialization later
	// For now, we just need a non-nil session that can handle CurrentTurn() calls
	logsDir := filepath.Join(repoRoot, "logs")
	if agentName != "" {
		logsDir = filepath.Join(logsDir, agentName)
	}
	if err := os.MkdirAll(logsDir, 0755); err != nil {
		Log.Warn("failed to create logs directory: %v", err)
	}
	
	// Create a minimal session that will work with the engine hooks
	session, err = sessionMod.New(logsDir, "", agentName, "", nil)
	if err != nil {
		Log.Warn("failed to create session: %v", err)
		// Create an even more minimal session to prevent nil pointer crashes
		session = &sessionMod.Session{}
	}

	return session, sessionDB, workspace, messages, nil
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
	// Bidirectional sync: pick up any tokens OpenClaw refreshed while we were down,
	// then push our state back. Order matters — From first so we get fresh tokens,
	// then To so OpenClaw has the latest from model-store.
	if n, syncErr := store.SyncFromAuthProfiles(""); syncErr != nil {
		Log.Warn("sync from auth-profiles: %v", syncErr)
	} else if n > 0 {
		Log.Info("synced %d credentials from auth-profiles.json", n)
	}
	if syncErr := store.SyncToAuthProfiles(""); syncErr != nil {
		Log.Warn("sync to auth-profiles: %v", syncErr)
	}

	return store, nil
}

// createModelClient creates the appropriate model client for the given model.
func createModelClient(model string, store *modelstore.Store) (*agent.ModelClient, string, *anthropic.Client, error) {
	modelClient, err := agent.NewModelClient(model, store)
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
		if tool == "spawn_agent" || tool == "sessions_list" || tool == "spawn_*" {
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
		} else if t.Name == "shell" {
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

// setupLimits initializes turn/token limits from config and agent config.
func setupLimits(cfg EngineConfig, agentConfig *registry.AgentConfig) (maxTurns, maxInputTokens, maxResponseTime int) {
	maxTurns = cfg.MaxTurns
	maxInputTokens = cfg.MaxInputTokens
	
	if agentConfig != nil && agentConfig.Limits != nil {
		if maxTurns == 0 {
			maxTurns = agentConfig.Limits.MaxTurns
		}
		if maxInputTokens == 0 {
			maxInputTokens = agentConfig.Limits.MaxInputTokens
		}
		if maxResponseTime == 0 {
			maxResponseTime = agentConfig.Limits.MaxResponseTime
		}
	}
	
	if cfg.Detach {
		if maxTurns == 0 {
			maxTurns = 25
		}
		if maxInputTokens == 0 {
			maxInputTokens = 500000
		}
	}
	
	if maxTurns > 0 || maxInputTokens > 0 || maxResponseTime > 0 {
		Log.Info("limits: maxTurns=%d, maxInputTokens=%d, maxResponseTime=%ds", maxTurns, maxInputTokens, maxResponseTime)
	}

	return maxTurns, maxInputTokens, maxResponseTime
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