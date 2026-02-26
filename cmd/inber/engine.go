package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/google/uuid"
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
	Display        *DisplayHooks
}

// Engine encapsulates the shared setup and execution logic for chat and run.
type Engine struct {
	Client       *anthropic.Client
	Agent        *agent.Agent
	ContextStore *inbercontext.Store
	MemStore     *memory.Store
	Session      *sessionMod.Session
	Model        string
	AgentName    string
	AgentConfig  *registry.AgentConfig
	Messages     []anthropic.MessageParam
	TurnCounter  int

	repoRoot    string
	agentTools  []agent.Tool
	display     *DisplayHooks
	thinkingBud int64
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
		configs, err := registry.LoadConfig(
			filepath.Join(repoRoot, "agents.json"),
			filepath.Join(repoRoot, "agents"),
		)
		if err != nil {
			return nil, fmt.Errorf("error loading agents: %w", err)
		}

		ac, ok := configs[cfg.AgentName]
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
		fmt.Fprintf(os.Stderr, "%sloading context...%s", dim, reset)
		contextCfg := inbercontext.DefaultAutoLoadConfig(repoRoot)
		if identityText == "" {
			identityText = "You are a helpful coding assistant. You have access to shell, file reading/writing/editing, and directory listing tools. Use them to help the user."
		}
		contextCfg.IdentityText = identityText

		cs, err := inbercontext.AutoLoad(contextCfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "\nwarning: context auto-load failed: %v\n", err)
			cs = inbercontext.NewStore()
		}
		if err := inbercontext.LoadProjectContext(cs, repoRoot); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to load project context: %v\n", err)
		}
		e.ContextStore = cs
		fmt.Fprintf(os.Stderr, " done (%d chunks)\n", cs.Count())

		// Memory
		ms, err := memory.OpenOrCreate(repoRoot)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: memory disabled: %v\n", err)
		} else {
			e.MemStore = ms
			if err := memory.LoadIntoContext(ms, cs, 10, 0.6); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to load memories: %v\n", err)
			}
		}
	} else if cfg.Raw && identityText == "" {
		identityText = "You are a helpful assistant."
	}

	// Session
	sess, err := sessionMod.New("logs", e.Model, e.AgentName, "")
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: logging disabled: %v\n", err)
	} else {
		e.Session = sess
		fmt.Fprintf(os.Stderr, "%slogging to %s%s\n", dim, sess.FilePath(), reset)
		if _, err := sessionMod.RegisterActive(repoRoot, sess, cfg.CommandName); err == nil {
			// deferred cleanup in Close()
		}
	}

	// API client
	key := agent.APIKey()
	if key == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY not set")
	}
	client := anthropic.NewClient(option.WithAPIKey(key))
	e.Client = &client

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

	return e, nil
}

// buildTools resolves tools from agent config or defaults.
func (e *Engine) buildTools() []agent.Tool {
	var result []agent.Tool

	if e.AgentConfig != nil && len(e.AgentConfig.Tools) > 0 {
		for _, toolName := range e.AgentConfig.Tools {
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
	}

	return result
}

// BuildSystemPrompt builds a context-aware system prompt.
func (e *Engine) BuildSystemPrompt(userMessage string) string {
	if e.ContextStore == nil {
		return ""
	}
	messageTags := inbercontext.AutoTag(userMessage, "user")
	builder := inbercontext.NewBuilder(e.ContextStore, 50000)
	chunks := builder.Build(messageTags)

	var parts []string
	for _, chunk := range chunks {
		parts = append(parts, chunk.Text)
	}

	return strings.Join(parts, "\n\n---\n\n")
}

// buildAgent creates a fresh Agent with current system prompt, tools, and hooks.
func (e *Engine) buildAgent(systemPrompt string) *agent.Agent {
	a := agent.New(e.Client, systemPrompt)
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
			sessionMod.WritePromptBreakdown(e.Session.FilePath(), e.Session.SessionID(), e.TurnCounter, params)
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

	systemPrompt := e.BuildSystemPrompt(input)
	e.buildAgent(systemPrompt)

	result, err := e.Agent.Run(context.Background(), e.Model, &e.Messages)
	if err != nil {
		return nil, err
	}

	if e.Session != nil {
		e.Session.LogAssistant(result.Text, result.InputTokens, result.OutputTokens, result.ToolCalls)
	}

	return result, nil
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
		sessionMod.UnregisterActive(e.repoRoot, e.Session.SessionID())
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
		fmt.Fprintf(os.Stderr, "warning: failed to save session summary: %v\n", err)
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
