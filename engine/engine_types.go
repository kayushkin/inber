package engine

import (
	"time"

	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/conversation"
	sessionMod "github.com/kayushkin/inber/session"
	modelstore "github.com/kayushkin/model-store"
)

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

// EngineConfig configures a new Engine via NewEngine.
type EngineConfig struct {
	// Agent selection
	AgentName      string // load from registry
	Model          string
	ModelExplicitlySet bool // true if --model flag was used (overrides agent config)
	Thinking       int64  // extended thinking token budget (0 = disabled)

	// Behavior
	Raw            bool   // skip context/memory loading
	NoTools        bool   // disable all tools
	NoHooks        bool   // skip post-request verification (git/deploy checks)
	SystemOverride string // override system prompt entirely

	// Session
	RepoRoot    string
	CommandName string // "chat", "run", or "serve"
	NewSession  bool   // start fresh instead of continuing default
	Detach      bool   // one-off session, don't save to workspace

	// Server integration
	ModelStore       *modelstore.Store   // shared model store (nil = engine opens its own)
	ExtraTools       []agent.Tool        // additional tools injected by server
	ContextInjectors []ContextInjector   // extra system prompt sections from server
	Injections       <-chan string        // mid-run message injection channel

	// Display
	Display *DisplayHooks

	// Conversation management
	StashConfig   *conversation.StashConfig      // nil = use defaults
	ExtractConfig *conversation.ExtractionConfig  // nil = use defaults
	AutoWorkflow  AutoWorkflowConfig

	// Safety limits
	MaxTurns       int     // max API round-trips per RunTurn (0 = unlimited)
	MaxInputTokens int     // max cumulative input tokens (0 = unlimited)
	MaxCost        float64 // max cumulative dollar cost (0 = unlimited)

	// Diagnostics
	MemoryProfiling bool
	MemoryLogPath   string
	Blueprint       bool   // emit prompt blueprint diffs per turn
	EnableTrace     bool   // enable structured execution tracing
	EnableCodeIndex bool   // enable AST-based codebase indexing
	EnableCheckpoint bool  // enable workspace state checkpointing
}

// ContextInjector provides additional system prompt blocks at turn time.
// Used by the server to inject live session status, workspace info, etc.
type ContextInjector func() []sessionMod.NamedBlock

// ---------------------------------------------------------------------------
// Runtime state
// ---------------------------------------------------------------------------

// TurnState holds ephemeral state that changes each turn.
type TurnState struct {
	Counter           int
	StartTime         time.Time
	ConsecutiveErrors int
	LastHadError      bool
	VolatileContext   string
	LastManageTurn    int
}

// CacheState holds prompt caching state across turns.
type CacheState struct {
	LastStablePrefix *cachedPrefix
	LastBlueprint    *PromptBlueprint
	BlueprintEnabled bool
	LastNamedBlocks  []sessionMod.NamedBlock
}

// LimitConfig holds runtime limits for turn execution.
type LimitConfig struct {
	MaxTurns        int
	MaxInputTokens  int
	MaxResponseTime int
}

// TokenTotals holds session-level token usage.
type TokenTotals struct {
	Input  int
	Output int
	Cost   float64
}
