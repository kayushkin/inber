package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/google/uuid"
	"github.com/kayushkin/aiauth"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/agent/registry"
	inbercontext "github.com/kayushkin/inber/context"
	"github.com/kayushkin/inber/memory"
	sessionMod "github.com/kayushkin/inber/session"
	"github.com/kayushkin/inber/tools"
)

// DisplayHooks configures how engine events are shown to the user.
type DisplayHooks struct {
	OnThinking   func(text string)
	OnToolCall   func(name string, input string)
	OnToolResult func(name string, output string, isError bool)
}

// EngineConfig configures the Engine.
type EngineConfig struct {
	Model          string
	Thinking       int64
	AgentName      string // load from registry
	Raw            bool   // skip context/memory
	NoTools        bool
	SystemOverride string
	RepoRoot       string
	CommandName    string // "chat" or "run" for session registration
	NewSession     bool   // start fresh instead of continuing default session
	Detach         bool   // one-off session, don't save to workspace
	Display        *DisplayHooks
}

// Engine encapsulates the shared setup and execution logic for chat and run.
type Engine struct {
	AuthStore    *aiauth.Store
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

	repoRoot       string
	agentTools     []agent.Tool
	display        *DisplayHooks
	workspace      *sessionMod.Workspace
	thinkingBud    int64
	lastNamedBlocks []sessionMod.NamedBlock
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

	e := &Engine{
		Model:       cfg.Model,
		repoRoot:    repoRoot,
		display:     cfg.Display,
		thinkingBud: cfg.Thinking,
	}

	// Load agent config
	var identityText string
	if cfg.AgentName != "" {
		registryCfg, err := registry.LoadConfig(
			filepath.Join(repoRoot, "agents.json"),
			filepath.Join(repoRoot, "agents"),
		)
		if err != nil {
			return nil, fmt.Errorf("error loading agents: %w", err)
		}

		ac, ok := registryCfg.Agents[cfg.AgentName]
		if !ok {
			return nil, fmt.Errorf("agent not found: %s", cfg.AgentName)
		}
		e.AgentConfig = ac
		if ac.Model != "" {
			e.Model = ac.Model
		}
		identityText = ac.System
		e.AgentName = cfg.AgentName
	} else {
		e.AgentName = cfg.CommandName
		if e.AgentName == "" {
			e.AgentName = "default"
		}
	}

	// Context & memory
	if cfg.SystemOverride == "" && !cfg.Raw {
		Log.Infof("loading context...")
		contextCfg := inbercontext.DefaultAutoLoadConfig(repoRoot)
		if identityText == "" {
			identityText = "You are a helpful coding assistant. You have access to shell, file reading/writing/editing, and directory listing tools. Use them to help the user."
		}
		contextCfg.IdentityText = identityText

		cs, err := inbercontext.AutoLoad(contextCfg)
		if err != nil {
			Log.Warn("context auto-load failed: %v", err)
			cs = inbercontext.NewStore()
		}
		if err := inbercontext.LoadProjectContext(cs, repoRoot); err != nil {
			Log.Warn("failed to load project context: %v", err)
		}
		e.ContextStore = cs
		// Append to the "loading context..." line (no newline from Infof)
		fmt.Fprintf(os.Stderr, " done (%d chunks)\n", cs.Count())

		// Memory
		ms, err := memory.OpenOrCreate(repoRoot)
		if err != nil {
			Log.Warn("memory disabled: %v", err)
		} else {
			e.MemStore = ms
			if err := memory.LoadIntoContext(ms, cs, 10, 0.6); err != nil {
				Log.Warn("failed to load memories: %v", err)
			}
		}
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
				e.Messages = msgs
				Log.Info("resuming session (%d messages)", len(msgs))
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
	}

	// API client (via aiauth with auto-refresh)
	e.AuthStore = aiauth.DefaultStore()
	client, err := e.AuthStore.AnthropicClient()
	if err != nil {
		return nil, fmt.Errorf("failed to get Anthropic client: %w", err)
	}
	e.Client = client

	// Tools
	if !cfg.NoTools {
		e.agentTools = e.buildTools()
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

	return e, nil
}

// buildTools resolves tools from agent config or defaults.
func (e *Engine) buildTools() []agent.Tool {
	var result []agent.Tool

	if e.AgentConfig != nil && len(e.AgentConfig.Tools) > 0 {
		for _, toolName := range e.AgentConfig.Tools {
			// Handle repo_map tool specially
			if toolName == "repo_map" {
				ignorePatterns := []string{
					"*.log", "*.tmp", ".git/*", "vendor/*",
					"node_modules/*", ".openclaw/*", "logs/*",
				}
				result = append(result, tools.RepoMap(e.repoRoot, ignorePatterns))
				continue
			}
			
			for _, t := range tools.All() {
				if t.Name == toolName {
					result = append(result, t)
					break
				}
			}
		}
		if e.MemStore != nil {
			for _, toolName := range e.AgentConfig.Tools {
				if strings.HasPrefix(toolName, "memory_") {
					for _, t := range memory.AllMemoryTools(e.MemStore) {
						if t.Name == toolName {
							result = append(result, t)
							break
						}
					}
				}
			}
		}
	} else {
		result = tools.All()
		if e.MemStore != nil {
			result = append(result, memory.AllMemoryTools(e.MemStore)...)
		}
		// Add repo_map tool by default
		ignorePatterns := []string{
			"*.log", "*.tmp", ".git/*", "vendor/*",
			"node_modules/*", ".openclaw/*", "logs/*",
		}
		result = append(result, tools.RepoMap(e.repoRoot, ignorePatterns))
	}

	return result
}

// BuildSystemPrompt builds a context-aware system prompt as individual named blocks.
func (e *Engine) BuildSystemPrompt(userMessage string) []sessionMod.NamedBlock {
	if e.ContextStore == nil {
		return nil
	}
	messageTags := inbercontext.AutoTag(userMessage, "user")
	builder := inbercontext.NewBuilder(e.ContextStore, 50000)
	chunks := builder.Build(messageTags)

	blocks := make([]sessionMod.NamedBlock, len(chunks))
	for i, chunk := range chunks {
		blocks[i] = sessionMod.NamedBlock{ID: chunk.ID, Text: chunk.Text}
	}

	if e.workspace != nil {
		// If workspace has edits, use those instead
		if wsBlocks, err := e.workspace.ReadSystem(); err == nil && wsBlocks != nil {
			Log.Info("using edited prompt from %s (%d blocks)", e.workspace.Dir, len(wsBlocks))
			blocks = wsBlocks
		}

		// Write current prompt to workspace for editing before next turn
		e.workspace.WriteSystem(blocks)
	}

	return blocks
}

// buildAgent creates a fresh Agent with current system prompt, tools, and hooks.
func (e *Engine) buildAgent(blocks []sessionMod.NamedBlock) *agent.Agent {
	systemBlocks := make([]anthropic.TextBlockParam, len(blocks))
	for i, b := range blocks {
		systemBlocks[i] = anthropic.TextBlockParam{Text: b.Text}
	}
	a := agent.NewWithSystemBlocks(e.Client, systemBlocks)
	for _, t := range e.agentTools {
		a.AddTool(t)
	}
	if e.thinkingBud > 0 {
		a.SetThinking(e.thinkingBud)
	}
	a.SetHooks(e.buildHooks())
	e.Agent = a
	return a
}

// buildHooks creates hooks that combine logging and display.
func (e *Engine) buildHooks() *agent.Hooks {
	hooks := &agent.Hooks{}

	if e.display != nil && e.display.OnThinking != nil {
		hooks.OnThinking = e.display.OnThinking
	}
	if e.display != nil && e.display.OnToolCall != nil {
		hooks.OnToolCall = func(toolID, name string, input []byte) {
			e.display.OnToolCall(name, string(input))
		}
	}
	if e.display != nil && e.display.OnToolResult != nil {
		hooks.OnToolResult = func(toolID, name, output string, isError bool) {
			e.display.OnToolResult(name, output, isError)
		}
	}

	if e.Session != nil {
		logHooks := e.Session.Hooks()
		origThinking := hooks.OnThinking
		origToolCall := hooks.OnToolCall
		origToolResult := hooks.OnToolResult

		hooks.OnRequest = func(params *anthropic.MessageNewParams) {
			if logHooks.OnRequest != nil {
				logHooks.OnRequest(params)
			}
			e.TurnCounter++
			sessionMod.WritePromptBreakdown(e.Session.FilePath(), e.Session.SessionID(), e.TurnCounter, params, e.lastNamedBlocks)
		}
		hooks.OnResponse = func(resp *anthropic.Message) {
			stopReason := string(resp.StopReason)
			toolCalls := 0
			for _, block := range resp.Content {
				if block.Type == "tool_use" {
					toolCalls++
				}
			}
			e.Session.EndTurn(
				int(resp.Usage.InputTokens),
				int(resp.Usage.OutputTokens),
				toolCalls,
				stopReason,
				"",
			)
		}
		hooks.OnThinking = func(text string) {
			if logHooks.OnThinking != nil {
				logHooks.OnThinking(text)
			}
			if origThinking != nil {
				origThinking(text)
			}
		}
		hooks.OnToolCall = func(toolID, name string, input []byte) {
			if logHooks.OnToolCall != nil {
				logHooks.OnToolCall(toolID, name, input)
			}
			if origToolCall != nil {
				origToolCall(toolID, name, input)
			}
		}
		hooks.OnToolResult = func(toolID, name, output string, isError bool) {
			if logHooks.OnToolResult != nil {
				logHooks.OnToolResult(toolID, name, output, isError)
			}
			if origToolResult != nil {
				origToolResult(toolID, name, output, isError)
			}
		}
	}

	return hooks
}

// RunTurn sends a user message, rebuilds the system prompt, runs the agent, and returns the result.
func (e *Engine) RunTurn(input string) (*agent.TurnResult, error) {
	if e.Session != nil {
		e.Session.LogUser(input)
	}

	e.Messages = append(e.Messages, anthropic.NewUserMessage(anthropic.NewTextBlock(input)))

	// Prune conversation if needed (keep last 35 turns)
	e.pruneIfNeeded()

	systemBlocks := e.BuildSystemPrompt(input)
	e.lastNamedBlocks = systemBlocks
	e.buildAgent(systemBlocks)

	result, err := e.Agent.Run(context.Background(), e.Model, &e.Messages)
	if err != nil {
		return nil, err
	}

	if e.Session != nil {
		e.Session.LogAssistant(result.Text, result.InputTokens, result.OutputTokens, result.ToolCalls)
	}

	// Save messages snapshot for session resume
	e.saveMessages()
	
	// Checkpoint if needed (every 20 turns)
	e.checkpointIfNeeded()

	return result, nil
}

// pruneIfNeeded checks if conversation should be pruned and does so if necessary.
func (e *Engine) pruneIfNeeded() {
	cfg := DefaultPruneConfig()
	
	if !ShouldPrune(e.Messages, cfg) {
		return
	}

	sessionID := ""
	if e.Session != nil {
		sessionID = e.Session.SessionID()
	}

	pruned, result, err := PruneConversation(
		context.Background(),
		e.Messages,
		e.MemStore,
		sessionID,
		cfg,
	)

	if err != nil {
		Log.Warn("pruning failed: %v", err)
		return
	}

	if result.PrunedMessages > 0 {
		e.Messages = pruned
		Log.Info("pruned %d messages (%d tokens freed, %d memories saved)",
			result.PrunedMessages, result.TokensFreed, result.MemoriesSaved)
		
		if e.Session != nil {
			e.Session.LogPrune(result.PrunedMessages, result.TokensFreed, result.Strategy)
		}
	}
}

// checkpointIfNeeded creates a checkpoint if we've reached the checkpoint interval.
func (e *Engine) checkpointIfNeeded() {
	if e.Session == nil {
		return
	}

	cfg := sessionMod.DefaultCheckpointConfig()
	if !sessionMod.ShouldCheckpoint(e.TurnCounter, cfg) {
		return
	}

	// Generate summary and extract key facts
	summary := sessionMod.GenerateConversationSummary(e.Messages)
	keyFacts := sessionMod.ExtractKeyFacts(e.Messages, 10)

	err := e.Session.SaveCheckpoint(e.Messages, summary, keyFacts)
	if err != nil {
		Log.Warn("checkpoint failed: %v", err)
	} else {
		Log.Info("checkpoint saved (turn %d)", e.TurnCounter)
	}
}

// saveMessages writes the current messages to the workspace and session log dir.
func (e *Engine) saveMessages() {
	data, err := json.Marshal(e.Messages)
	if err != nil {
		return
	}
	// Save to workspace (persistent default session)
	if e.workspace != nil {
		e.workspace.SaveMessages(data)
	}
	// Also save to session log dir
	if e.Session != nil {
		sessDir := filepath.Dir(e.Session.FilePath())
		os.WriteFile(filepath.Join(sessDir, "messages.json"), data, 0644)
	}
}

// LogUser logs a user message to the session (for external callers that need pre-logging).
func (e *Engine) LogUser(input string) {
	if e.Session != nil {
		e.Session.LogUser(input)
	}
}

// LogAssistant logs an assistant response to the session.
func (e *Engine) LogAssistant(result *agent.TurnResult) {
	if e.Session != nil {
		e.Session.LogAssistant(result.Text, result.InputTokens, result.OutputTokens, result.ToolCalls)
	}
}

// Close saves session summary, closes memory store, and unregisters the active session.
func (e *Engine) Close() {
	// Auto-save session summary to memory
	if e.MemStore != nil && len(e.Messages) > 0 {
		saveSessionSummary(e.MemStore, e.Messages, e.AgentName)
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
}

// saveSessionSummary generates a brief session summary and saves it to memory.
func saveSessionSummary(store *memory.Store, messages []anthropic.MessageParam, agentName string) {
	var parts []string
	for _, msg := range messages {
		role := string(msg.Role)
		for _, block := range msg.Content {
			if block.OfText != nil {
				text := block.OfText.Text
				if len(text) > 200 {
					text = text[:200] + "..."
				}
				parts = append(parts, fmt.Sprintf("%s: %s", role, text))
			}
		}
	}

	if len(parts) == 0 {
		return
	}

	summary := fmt.Sprintf("Session summary (%s):\n%s", agentName, strings.Join(parts, "\n"))
	if len(summary) > 2000 {
		summary = summary[:2000]
	}

	m := memory.Memory{
		ID:         uuid.New().String(),
		Content:    summary,
		Tags:       []string{"session-summary", agentName},
		Importance: 0.4,
		Source:     "system",
	}

	if err := store.Save(m); err != nil {
		Log.Warn("failed to save session summary: %v", err)
	}
}

// FindRepoRoot finds the repository root by looking for .git directory.
func FindRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		gitDir := filepath.Join(dir, ".git")
		if _, err := os.Stat(gitDir); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("not in a git repository")
		}
		dir = parent
	}
}
