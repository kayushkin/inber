package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/google/uuid"
	"github.com/kayushkin/aiauth"
	"github.com/kayushkin/aiauth/providers"
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
	StashConfig    *StashConfig      // Large message stashing config (nil = use defaults)
	ExtractConfig  *ExtractionConfig // Background extraction config (nil = use defaults)
	AutoWorkflow   AutoWorkflowConfig // Auto-branch, auto-commit, auto-format (Phase 1)
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

	repoRoot        string
	agentTools      []agent.Tool
	display         *DisplayHooks
	workspace       *sessionMod.Workspace
	thinkingBud     int64
	lastNamedBlocks []sessionMod.NamedBlock
	stashCfg        StashConfig
	extractCfg      ExtractionConfig
	consecutiveErrors  int  // track consecutive tool errors for context escalation
	lastTurnHadError   bool
	autoRefMgr      *memory.AutoReferenceManager // auto-creates references after tool calls
	toolInputsCache map[string]string             // toolID -> input JSON for auto-reference creation
	workflowHooks   *WorkflowHooks                // auto-branch, auto-commit, auto-format, build/test
	
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
	stashCfg := DefaultStashConfig()
	if cfg.StashConfig != nil {
		stashCfg = *cfg.StashConfig
	}

	extractCfg := DefaultExtractionConfig()
	if cfg.ExtractConfig != nil {
		extractCfg = *cfg.ExtractConfig
	}

	e := &Engine{
		Model:       cfg.Model,
		repoRoot:    repoRoot,
		display:     cfg.Display,
		thinkingBud: cfg.Thinking,
		stashCfg:    stashCfg,
		extractCfg:    extractCfg,
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
		
		// Memory store
		ms, err := memory.OpenOrCreate(repoRoot)
		if err != nil {
			return nil, fmt.Errorf("failed to open memory store: %w", err)
		}
		e.MemStore = ms

		// Prepare session: load identity + recent files into memory
		if identityText == "" {
			identityText = "You are Claxon, a coding assistant. Use your tools to help the user. See .inber/identity.md, .inber/soul.md, and .inber/user.md for full context."
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
				repaired := repairDanglingToolUse(msgs)
				repaired = repairAlternation(repaired)
				e.Messages = repaired
				// Save repaired messages back so we don't re-repair every time
				if lastRepairCount > 0 || len(repaired) != len(msgs) {
					if data, err := json.Marshal(repaired); err == nil {
						ws.SaveMessages(data)
						Log.Info("repaired session messages (%d tool calls, %d→%d messages)",
							lastRepairCount, len(msgs), len(repaired))
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
		
		// Initialize workflow automation (auto-branch, auto-commit, auto-format)
		workflowCfg := cfg.AutoWorkflow
		if workflowCfg == (AutoWorkflowConfig{}) {
			workflowCfg = DefaultAutoWorkflowConfig()
		}
		e.workflowHooks = NewWorkflowHooks(repoRoot, sess.SessionID(), e.AgentName, workflowCfg)
		
		// Initialize session branch
		if msg, err := e.workflowHooks.InitSession(); err != nil {
			Log.Warn("failed to init session branch: %v", err)
		} else if msg != "" {
			Log.Info(msg)
		}
	}

	// API client (via aiauth with auto-refresh)
	aiauth.RegisterProvider(&providers.Anthropic{})
	e.AuthStore = aiauth.DefaultStore()
	client, err := e.AuthStore.AnthropicClient()
	if err != nil {
		return nil, fmt.Errorf("failed to get Anthropic client: %w", err)
	}
	e.Client = client

	// Tools
	if !cfg.NoTools {
		e.agentTools = e.buildTools()
		
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

	return e, nil
}

// buildTools resolves tools from agent config or defaults.
func (e *Engine) buildTools() []agent.Tool {
	var result []agent.Tool

	if e.AgentConfig != nil && len(e.AgentConfig.Tools) > 0 {
		for _, toolName := range e.AgentConfig.Tools {
			// Handle special tools that need configuration
			if toolName == "repo_map" {
				ignorePatterns := []string{
					"*.log", "*.tmp", ".git/*", "vendor/*",
					"node_modules/*", ".openclaw/*", "logs/*",
				}
				result = append(result, tools.RepoMap(e.repoRoot, ignorePatterns))
				continue
			}
			if toolName == "recent_files" {
				result = append(result, tools.RecentFiles(e.repoRoot))
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
		// Add code introspection tools by default
		ignorePatterns := []string{
			"*.log", "*.tmp", ".git/*", "vendor/*",
			"node_modules/*", ".openclaw/*", "logs/*",
		}
		result = append(result, tools.RepoMap(e.repoRoot, ignorePatterns))
		result = append(result, tools.RecentFiles(e.repoRoot))
	}

	return result
}

// contextBudget returns the token budget for memory context loading.
// MinImportance is always 0 — ranking + budget controls what loads, not a hard cutoff.
// Budget starts minimal and escalates only when the agent is struggling.
func (e *Engine) contextBudget(userMessage string) (minImportance float64, tokenBudget int) {
	msgTokens := inbercontext.EstimateTokens(userMessage)

	// Base budget: just AlwaysLoad memories (identity/soul/user/AGENTS.md)
	// ~2-3K tokens typically
	baseBudget := 4000

	switch {
	case e.TurnCounter == 0:
		// First turn — absolute minimum, just get started
		return 0, baseBudget

	case e.consecutiveErrors >= 5:
		// Deeply stuck — throw everything relevant at it
		return 0, 50000

	case e.consecutiveErrors >= 3:
		// Stuck in a loop — significantly more context
		return 0, 35000

	case e.consecutiveErrors >= 1 || e.lastTurnHadError:
		// Hit an error — bump context to help recover
		return 0, 20000

	case msgTokens > 1000:
		// Very long/complex user message
		return 0, 15000

	case msgTokens > 300:
		// Moderate complexity
		return 0, 10000

	case e.TurnCounter > 15:
		// Long session — might need more context to stay coherent
		return 0, 8000

	default:
		// Normal turn — lean context, let ranking pick the best matches
		return 0, 6000
	}
}

// BuildSystemPrompt builds a context-aware system prompt as individual named blocks.
func (e *Engine) BuildSystemPrompt(userMessage string) []sessionMod.NamedBlock {
	// Use memory-backed context if available
	if e.MemStore != nil {
		messageTags := inbercontext.AutoTag(userMessage, "user")
		minImportance, tokenBudget := e.contextBudget(userMessage)

		req := memory.BuildContextRequest{
			Tags:              messageTags,
			TokenBudget:       tokenBudget,
			MinImportance:     minImportance,
			IncludeAlwaysLoad: true,
			ExcludeTags:       []string{"session-summary", "repo-map", "code-introspection"},
			MaxChunkSize:      5000,  // Hard skip memories above 5K tokens
			TruncateThreshold: 500,   // Preview + expand hint for memories above 500 tokens
			TruncatePreview:   300,   // ~100 token preview
		}

		memories, tokensUsed, err := e.MemStore.BuildContext(req)
		if err != nil {
			Log.Warn("failed to build context from memory: %v", err)
			return nil
		}

		Log.Info("context: %d memories, %d tokens (min_importance=%.1f, budget=%d)", len(memories), tokensUsed, minImportance, tokenBudget)

		// Convert memories to named blocks with descriptive IDs
		var blocks []sessionMod.NamedBlock
		for _, m := range memories {
			// Use content, fall back to summary
			text := m.Content
			if text == "" {
				text = m.Summary
			}
			if text == "" {
				continue // Skip empty memories entirely
			}
			desc := fmt.Sprintf("%s (%.1f", m.ID[:8], m.Importance)
			if len(m.Tags) > 0 {
				desc += fmt.Sprintf(", tags: %s", strings.Join(m.Tags, ","))
			}
			desc += ")"
			blocks = append(blocks, sessionMod.NamedBlock{ID: desc, Text: text})
		}

		if e.workspace != nil {
			// Write current prompt to workspace for inspection/editing.
			// NOTE: We no longer auto-load from workspace — it was causing stale
			// cached prompts (100+ blocks) to override the adaptive context system.
			// If manual editing is needed, add an explicit --edit-prompt flag.
			e.workspace.WriteSystem(blocks)
		}

		return blocks
	}
	
	// Fallback to old context store if memory not available
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
		// Write for inspection only — no auto-load (same as memory path above)
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

	// Set context window guard — auto-prune before API calls that would overflow
	modelInfo, ok := agent.Models[e.Model]
	if ok {
		a.SetContextWindow(modelInfo.ContextWindow)
	} else {
		a.SetContextWindow(200000) // safe default
	}
	a.SetBeforeRequest(func(messages []anthropic.MessageParam, contextWindow int) []anthropic.MessageParam {
		cfg := e.pruneConfig()
		cfg.TokenBudget = contextWindow / 2

		// First pass: truncate old tool results/assistant text
		if ShouldPrune(messages, cfg) {
			Log.Warn("context approaching limit (%d messages), pruning", len(messages))
			pruned, result, err := PruneConversation(context.Background(), messages, e.MemStore, "", cfg)
			if err == nil {
				Log.Info("pruned: %d tokens freed", result.TokensFreed)
				messages = pruned
			}
		}

		// Second pass: if still too many messages, hard-drop old ones.
		// Keep only the last N messages to fit within context window.
		// The estimator undercounts 3-4x, so be very conservative.
		maxMessages := cfg.KeepRecentTurns * 2 // e.g. 70 for default
		if len(messages) > maxMessages {
			dropTo := len(messages) - maxMessages
			// Find a clean cut point: a user message with no tool_result blocks.
			// This ensures we don't orphan tool_results from their tool_use.
			for dropTo < len(messages) {
				msg := messages[dropTo]
				if msg.Role == anthropic.MessageParamRoleUser && !hasToolResult(msg) {
					break
				}
				dropTo++
			}
			if dropTo < len(messages) && dropTo > 0 {
				Log.Warn("hard-dropping %d old messages (%d → %d)", dropTo, len(messages), len(messages)-dropTo)
				messages = messages[dropTo:]
			}
		}

		return messages
	})

	e.Agent = a
	return a
}

// hasToolResult checks if a message contains any tool_result content blocks.
func hasToolResult(msg anthropic.MessageParam) bool {
	for _, block := range msg.Content {
		if block.OfToolResult != nil {
			return true
		}
	}
	return false
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
			// TurnCounter already incremented at start of RunTurn
			sessionMod.WritePromptBreakdown(e.Session.FilePath(), e.Session.SessionID(), e.TurnCounter, params, e.lastNamedBlocks)
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
			// Cache tool input for auto-reference creation
			if e.autoRefMgr != nil && e.toolInputsCache != nil {
				e.toolInputsCache[toolID] = string(input)
			}
			
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
			// Track errors for adaptive context escalation
			if isError {
				e.consecutiveErrors++
				e.lastTurnHadError = true
			}
			
			// Auto-create references for successful tool calls
			if !isError && e.autoRefMgr != nil && e.toolInputsCache != nil {
				inputJSON := e.toolInputsCache[toolID]
				if err := e.autoRefMgr.OnToolResult(toolID, name, inputJSON, output); err != nil {
					// Log but don't fail the turn
					fmt.Fprintf(os.Stderr, "warning: failed to auto-create reference for %s: %v\n", name, err)
				}
				// NOTE: Don't delete cache here - PostToolResult needs it too
			}
		}
		// Truncate large outputs before adding to conversation
		hooks.ModifyToolResult = func(toolID, name, output string, isError bool) string {
			if e.Session != nil {
				return e.Session.TruncateToolResult(name, output, isError)
			}
			return "" // return empty string = no modification
		}
		// Workflow hooks: auto-branch, auto-commit, auto-format, build/test
		hooks.PostToolResult = func(toolID, name, output string, isError bool) string {
			if isError || e.workflowHooks == nil {
				return ""
			}
			// Get tool input from cache
			toolInput := e.toolInputsCache[toolID]
			result := e.workflowHooks.OnToolResult(name, toolInput, output, isError)
			// Clean up cache entry now that both hooks have used it
			if e.toolInputsCache != nil {
				delete(e.toolInputsCache, toolID)
			}
			return result
		}
		hooks.OnResponse = func(resp *anthropic.Message) {
			// End turn tracking
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

			// Adaptive context: reset error tracking if no errors this turn
			if !e.lastTurnHadError {
				e.consecutiveErrors = 0
			}
			e.lastTurnHadError = false

			if logHooks.OnResponse != nil {
				logHooks.OnResponse(resp)
			}
		}
	}

	return hooks
}

// RunTurn sends a user message, rebuilds the system prompt, runs the agent, and returns the result.
func (e *Engine) RunTurn(input string) (*agent.TurnResult, error) {
	// Increment and log turn number
	e.TurnCounter++
	Log.Info("━━━ Turn %d ━━━", e.TurnCounter)
	
	// Get session ID for tagging
	sessionID := ""
	if e.Session != nil {
		sessionID = e.Session.SessionID()
		e.Session.LogUser(input)
	}

	// 1. STASH LARGE USER MESSAGES (before sending to LLM)
	processedInput := input
	if e.stashCfg.Enabled && e.MemStore != nil {
		tokens := inbercontext.EstimateTokens(input)
		if tokens > e.stashCfg.UserMessageThreshold {
			modifiedInput, stashed, err := DetectAndStashLargeBlocks(input, sessionID, e.MemStore, e.stashCfg)
			if err != nil {
				Log.Warn("failed to stash large user message: %v", err)
			} else if len(stashed) > 0 {
				processedInput = modifiedInput
				totalStashed := 0
				for _, s := range stashed {
					totalStashed += s.Tokens
				}
				Log.Info("stashed %d large blocks from user message (%d tokens)", len(stashed), totalStashed)
				
				if e.Session != nil {
					e.Session.LogStash("user", len(stashed), totalStashed)
				}
			}
		}
	}

	e.Messages = append(e.Messages, anthropic.NewUserMessage(anthropic.NewTextBlock(processedInput)))

	// 1a. Summarize if conversation is very long (compress old turns into summary)
	e.summarizeIfNeeded()
	// 1b. Prune remaining conversation (truncate tool results, old messages)
	e.pruneIfNeeded()

	systemBlocks := e.BuildSystemPrompt(processedInput)
	e.lastNamedBlocks = systemBlocks
	e.buildAgent(systemBlocks)

	result, err := e.Agent.Run(context.Background(), e.Model, &e.Messages)
	if err != nil {
		return nil, err
	}

	if e.Session != nil {
		e.Session.LogAssistant(result.Text, result.InputTokens, result.OutputTokens, result.ToolCalls)
	}

	// 2. BACKGROUND MEMORY EXTRACTION (after turn completes, async)
	if e.extractCfg.Enabled && e.MemStore != nil {
		// Note: we don't have detailed tool call info in TurnResult,
		// but extraction will work without it
		var toolCalls []ToolCallSummary
		
		// Launch extraction in background goroutine
		go BackgroundExtractMemories(
			context.Background(),
			e.Client,
			input, // Original user input (not stashed version)
			result.Text,
			toolCalls,
			sessionID,
			e.MemStore,
			e.extractCfg,
			&Log,
		)
	}

	// 3. STASH LARGE ASSISTANT RESPONSES (for next turn)
	// Don't modify the response the user sees, but modify it in conversation history
	if e.stashCfg.Enabled && e.MemStore != nil {
		responseTokens := inbercontext.EstimateTokens(result.Text)
		if responseTokens > e.stashCfg.AssistantThreshold {
			// The response is already in e.Messages (added by Agent.Run)
			// We need to modify the last message (assistant message) in the history
			if len(e.Messages) > 0 && e.Messages[len(e.Messages)-1].Role == anthropic.MessageParamRoleAssistant {
				lastMsg := &e.Messages[len(e.Messages)-1]
				
				// Find text blocks in the message
				var modifiedContent []anthropic.ContentBlockParamUnion
				stashedAny := false
				
				for _, block := range lastMsg.Content {
					if block.OfText != nil {
						text := block.OfText.Text
						textTokens := inbercontext.EstimateTokens(text)
						
						if textTokens > e.stashCfg.MinBlockSize {
							modifiedText, stashed, err := DetectAndStashLargeBlocks(text, sessionID, e.MemStore, e.stashCfg)
							if err != nil {
								Log.Warn("failed to stash assistant response: %v", err)
								modifiedContent = append(modifiedContent, block)
							} else if len(stashed) > 0 {
								stashedAny = true
								modifiedContent = append(modifiedContent, anthropic.ContentBlockParamUnion{
									OfText: &anthropic.TextBlockParam{Text: modifiedText},
								})
								totalStashed := 0
								for _, s := range stashed {
									totalStashed += s.Tokens
								}
								Log.Info("stashed %d large blocks from assistant response (%d tokens)", len(stashed), totalStashed)
								
								if e.Session != nil {
									e.Session.LogStash("assistant", len(stashed), totalStashed)
								}
							} else {
								modifiedContent = append(modifiedContent, block)
							}
						} else {
							modifiedContent = append(modifiedContent, block)
						}
					} else {
						// Keep non-text blocks as-is
						modifiedContent = append(modifiedContent, block)
					}
				}
				
				if stashedAny {
					lastMsg.Content = modifiedContent
				}
			}
		}
	}

	// Save messages snapshot for session resume
	e.saveMessages()
	
	// Checkpoint if needed (every 20 turns)
	e.checkpointIfNeeded()
	
	// Track cumulative session tokens
	e.SessionInputTokens += result.InputTokens
	e.SessionOutputTokens += result.OutputTokens
	e.SessionCost += sessionMod.CalcCost(e.Model, result.InputTokens, result.OutputTokens)

	return result, nil
}

// summarizeIfNeeded checks if the conversation is long enough to warrant
// summarization. Summarization compresses old turns into a compact summary,
// saves the full conversation to memory, and replaces old messages.
// Runs before pruning — summarize first, then prune what remains.
func (e *Engine) summarizeIfNeeded() {
	role := RoleDefault
	if e.AgentConfig != nil && e.AgentConfig.Role != "" {
		role = AgentRole(strings.ToLower(e.AgentConfig.Role))
	}
	cfg := DefaultSummarizeConfig(role)

	if !ShouldSummarize(e.Messages, cfg) {
		return
	}

	sessionID := ""
	if e.Session != nil {
		sessionID = e.Session.SessionID()
	}

	model := e.Model
	if model == "" {
		model = "claude-sonnet-4-5-20250929"
	}

	summarized, result, err := SummarizeConversation(
		context.Background(),
		e.Client,
		e.Messages,
		e.MemStore,
		sessionID,
		cfg,
		model,
	)

	if err != nil {
		Log.Warn("summarization failed: %v", err)
		return
	}

	if result.Summarized {
		e.Messages = summarized
		Log.Info("summarized %d turns → %d token summary (kept %d recent messages, memory: %s)",
			result.SummarizedTurns, result.SummaryTokens, result.KeptMessages, result.MemoryID)
		
		if e.Session != nil {
			e.Session.LogSummarize(result.SummarizedTurns, result.SummaryTokens, result.KeptMessages, result.MemoryID)
		}
	}
}

// pruneIfNeeded checks if conversation should be pruned and does so if necessary.
// pruneConfig returns the appropriate PruneConfig for this engine's agent role.
func (e *Engine) pruneConfig() PruneConfig {
	if e.AgentConfig != nil && e.AgentConfig.Role != "" {
		return PruneConfigForRole(e.AgentConfig.Role)
	}
	return DefaultPruneConfig()
}

func (e *Engine) pruneIfNeeded() {
	cfg := e.pruneConfig()

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
	// Show workflow summary (auto-branch, commits made)
	if e.workflowHooks != nil {
		if summary := e.workflowHooks.FinishSession(); summary != "" {
			fmt.Fprintln(os.Stderr, "\n"+summary)
		}
	}

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
