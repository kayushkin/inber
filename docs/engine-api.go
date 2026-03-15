// Package engine handles everything needed to execute one agent turn:
// prompt assembly, model selection, API calls, tool execution, and
// conversation lifecycle.
//
// Engine absorbs the old agent/ package. There is no separate agent layer.
//
// This file is a design reference — not compilable code yet.
package engine

import (
	"context"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	modelstore "github.com/kayushkin/model-store"
)

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

// Config is everything needed to create an engine.
// Gateway fills this from agent config + session state.
// CLI commands (run, chat) fill it from flags + repo detection.
type Config struct {
	// Agent identity
	AgentName  string // agent name (for identity/config lookup)
	RepoRoot   string // workspace / working directory

	// Model
	Model     string // preferred model (engine may failover to another)
	Thinking  int64  // thinking budget in tokens (0 = off)

	// Tools
	Tools     []string // tool allowlist (empty = all available tools)

	// Session behavior
	NewSession bool // start fresh (ignore existing messages)
	Detach     bool // one-off session, don't persist to workspace

	// Gateway-provided overrides (empty when running standalone via CLI)
	InitialMessages []anthropic.MessageParam // pre-loaded messages (for forked sessions)
	ExtraContext    []NamedBlock             // extra system prompt blocks (sub-agent context, fleet status)
	ModelStore      *modelstore.Store        // shared model store (nil = engine opens its own)

	// Limits
	MaxTurns       int // max API round-trips per turn (0 = unlimited)
	MaxInputTokens int // max cumulative input tokens (0 = unlimited)
	MaxResponseTime int // max seconds before forced stop (0 = unlimited)

	// Callbacks
	Display *DisplayHooks // streaming display callbacks (deltas, tool calls, thinking)
}

// NamedBlock is a labeled chunk of system prompt content.
type NamedBlock struct {
	ID   string // identifier (e.g. "identity", "spawn-context", "fleet-status")
	Text string // content
}

// DisplayHooks are callbacks for real-time streaming output.
type DisplayHooks struct {
	OnThinking   func(text string)
	OnTextDelta  func(text string)
	OnToolCall   func(name, input string)
	OnToolResult func(name, output string, isError bool)
}

// ---------------------------------------------------------------------------
// Engine
// ---------------------------------------------------------------------------

// Engine encapsulates everything needed to execute agent turns.
// Created once per session, reused across turns.
type Engine struct {
	// Public — readable by gateway for session management.
	Model       string                    // current model (may change via failover)
	AgentName   string                    // agent identity
	Messages    []anthropic.MessageParam  // conversation history (mutated by RunTurn)
	TurnCounter int                       // turns executed in this engine's lifetime

	// Token tracking (cumulative across turns in this engine).
	SessionInputTokens  int
	SessionOutputTokens int
	SessionCost         float64
}

// New creates an engine from config.
//
// What it does:
//   - Loads agent config from registry (identity, principles, tools)
//   - Opens memory store for context retrieval
//   - Opens or reuses model store for failover/health tracking
//   - Loads or initializes conversation messages
//   - Sets up workspace for message persistence
//
// Standalone (CLI): call New directly with Config from flags.
// Gateway: call New with Config built from AgentConfig + session state.
func New(cfg Config) (*Engine, error)

// Close releases resources (memory store, model store, session DB).
// Saves session summary to memory. Runs post-request hooks.
func (e *Engine) Close()

// ---------------------------------------------------------------------------
// Turn execution (the main thing)
// ---------------------------------------------------------------------------

// TurnResult is everything that happened during one turn.
type TurnResult struct {
	Text                string // final text response
	Thinking            string // thinking/reasoning text
	ToolCalls           int    // total tool calls made
	InputTokens         int
	OutputTokens        int
	CacheCreationTokens int
	CacheReadTokens     int
}

// RunTurn is the core method. Sends a user message and returns the response.
//
// What it does, in order:
//   1. Append user message to conversation
//   2. Summarize if conversation is too long
//   3. Prune if still too long
//   4. Build system prompt (identity + context + memories, budget-aware)
//   5. Resolve tools (from agent config or defaults)
//   6. Select model (failover if preferred model is unhealthy)
//   7. Call API (Anthropic or OpenAI-compatible)
//   8. Tool loop: execute tools, re-call API, repeat until end_turn
//   9. Record model health (success/failure timing)
//  10. Stash large responses to memory
//  11. Save messages to workspace
//  12. Track token usage
//  13. Return TurnResult
//
// The tool loop (steps 7-8) runs inline — no goroutines.
// Tools are executed sequentially within one API response.
// Multiple API round-trips happen when the model returns tool_use.
func (e *Engine) RunTurn(input string) (*TurnResult, error)

// ---------------------------------------------------------------------------
// System prompt assembly
// ---------------------------------------------------------------------------

// BuildSystemPrompt assembles context-aware system prompt blocks.
//
// What it does:
//   - Auto-tags the user message for relevance matching
//   - Calculates token budget based on turn state (errors → bigger budget)
//   - Retrieves relevant memories from the memory store
//   - Adds gateway-injected extra context (ExtraContext from Config)
//   - Adds fleet status (active agents) for orchestrator awareness
//   - Writes system blocks to workspace for debugging
//
// Returns named blocks that get assembled into the API request's system param.
func (e *Engine) BuildSystemPrompt(userMessage string) []NamedBlock

// buildTools resolves the tool set for this engine.
//
// Sources (in priority order):
//   - Config.Tools (if non-empty, only these tools)
//   - Agent config from registry (agent-specific tool list)
//   - Default: all tools + memory tools + repo_map + recent_files
//
// Special tools:
//   - spawn_agent: provided by gateway, calls gateway.Spawn()
//   - memory_*: added when memory store is available
//   - repo_map, recent_files: workspace-aware tools
func (e *Engine) buildTools() []Tool

// ---------------------------------------------------------------------------
// Tool definitions
// ---------------------------------------------------------------------------

// Tool is a named function the agent can call.
type Tool struct {
	Name        string
	Description string
	InputSchema anthropic.ToolInputSchemaParam
	Run         func(ctx context.Context, inputJSON string) (output string, err error)
}

// All returns all built-in tools: shell, read, write, edit, apply_patch, etc.
func All() []Tool

// ---------------------------------------------------------------------------
// Model selection and failover
// ---------------------------------------------------------------------------

// selectModel picks the best available model based on health data.
//
// Strategy:
//   1. Preferred model healthy (responded in last 30min, no recent error) → use it
//   2. Preferred model unknown (never tried) → use it (give it a chance)
//   3. Preferred model unhealthy → try fallback chain (enabled models by priority)
//   4. Nothing healthy → use preferred anyway
//
// Returns model ID and a timeout hint based on observed response times.
func (e *Engine) selectModel() (model string, timeout time.Duration)

// recordModelHealth updates the model store after an API call.
func (e *Engine) recordModelHealth(model string, durationMs int64, err error)

// ---------------------------------------------------------------------------
// Conversation lifecycle
// ---------------------------------------------------------------------------

// summarizeIfNeeded compresses old turns into a summary when conversation
// exceeds length thresholds. Uses an LLM call to generate the summary.
// Summary is saved to memory store for retrieval in future turns.
func (e *Engine) summarizeIfNeeded()

// pruneIfNeeded truncates tool results and drops old messages when
// conversation approaches the context window limit.
func (e *Engine) pruneIfNeeded()

// checkpointIfNeeded saves a snapshot every N turns for crash recovery.
func (e *Engine) checkpointIfNeeded()

// stashAssistantResponse moves large blocks from assistant responses
// into the memory store, replacing them with retrieval references.
func (e *Engine) stashAssistantResponse()

// saveMessages persists current messages to workspace and session log.
func (e *Engine) saveMessages()

// ---------------------------------------------------------------------------
// Model client (Anthropic / OpenAI)
// ---------------------------------------------------------------------------

// ModelClient wraps either the Anthropic SDK or an OpenAI-compatible API.
// Selected based on the model's provider in the model store.
type ModelClient struct {
	Provider        string           // "anthropic", "openai", "zai", etc.
	Model           *modelstore.Model
	AnthropicClient *anthropic.Client
}

// NewModelClient creates a client for the given model.
func NewModelClient(model string, store *modelstore.Store) (*ModelClient, error)

// IsOpenAI returns true if this model uses the OpenAI-compatible API path.
func (mc *ModelClient) IsOpenAI() bool

// ---------------------------------------------------------------------------
// Hooks
// ---------------------------------------------------------------------------

// Hooks are callbacks invoked during the tool loop.
type Hooks struct {
	OnRequest    func(params *anthropic.MessageNewParams)  // before each API call
	OnResponse   func(resp *anthropic.Message)             // after each API response
	OnThinking   func(text string)                         // thinking/reasoning text
	OnTextDelta  func(text string)                         // streaming text chunks
	OnToolCall   func(toolID, name string, input []byte)   // tool invocation
	OnToolResult func(toolID, name, output string, isError bool) // tool completion

	// ModifyToolResult can truncate tool output before sending to model.
	ModifyToolResult func(toolID, name, output string, isError bool) string

	// PostToolResult runs after tool completion (auto-commit, build/test).
	PostToolResult func(toolID, name, output string, isError bool) string
}
