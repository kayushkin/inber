package engine

import (
	"time"

	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/conversation"
	"github.com/kayushkin/inber/guard"
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
	RepoRoot string
	// WorkspaceRoots is every repository the session has checked out, when it
	// runs inside a forge workspace. Empty everywhere else — a session with one
	// repository is fully described by RepoRoot. The root marked primary must
	// be RepoRoot; NewEngine refuses the pair if it is not.
	WorkspaceRoots []WorkspaceRoot
	CommandName    string // "chat", "run", or "serve"
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
	Mode           string  // execution mode: observe, assist, autonomous ("" = autonomous)
	MaxTurns       int     // max API round-trips per RunTurn (0 = unlimited)
	MaxInputTokens int     // max cumulative input tokens (0 = unlimited)
	MaxCost        float64 // max cumulative dollar cost (0 = unlimited)
	MaxDuration    int     // max seconds the session may go on running (0 = unlimited)

	// Diagnostics
	MemoryProfiling bool
	MemoryLogPath   string
	Blueprint       bool // emit prompt blueprint diffs per turn

	// The three below are read by nothing. Each has exactly one occurrence in
	// the tree — its own declaration here — so setting any of them turns on
	// nothing at all, and the subsystems they name are TODO stubs rather than
	// features waiting on a flag. They are documented rather than deleted
	// because whether inber grows these three subsystems is a product question;
	// what is not a question is that a caller must not read them as switches.
	EnableTrace      bool // no reader; trace.NewRecorder("", …) returns nil and its methods are no-ops
	EnableCodeIndex  bool // no reader; codeindex.Open returns an empty index and Search/RepoMap are TODO
	EnableCheckpoint bool // no reader; every checkpoint package method returns checkpoint.ErrNotImplemented
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
	// PendingVolatileNotes holds notes queued during context preparation, which
	// runs before the prompt build that assigns VolatileContext. See
	// volatile_context.go.
	PendingVolatileNotes []string
	LastManageTurn       int
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
	MaxCost         float64 // dollars — 0 = unlimited
	MaxDuration     int     // seconds — 0 = unlimited
}

// GuardConfig renders the engine's limits as the guard's configuration.
//
// The guard enforces the limits; this is the one place that says which of the
// engine's limits it is given. It used to be written inline at the guard's
// construction and listed MaxTurns and MaxInputTokens only, so MaxCost reached
// the guard as its zero value — and guard.Config documents 0 as unlimited, so
// the omission read as "no cap wanted" rather than as a missing field.
//
// MaxResponseTime is deliberately absent: it bounds a single turn's wall clock
// and is checked by the build hooks in build_hooks.go, not by the guard.
// MaxDuration is the other wall clock and does belong here — it bounds the
// session rather than the turn, and the guard is the only thing that outlives a
// turn to measure it. Both are seconds, which is exactly why they are easy to
// confuse; the names are the only thing keeping them apart.
func (l LimitConfig) GuardConfig(mode guard.Mode) guard.Config {
	return guard.Config{
		Mode:           mode,
		MaxTurns:       l.MaxTurns,
		MaxInputTokens: l.MaxInputTokens,
		MaxCost:        l.MaxCost,
		MaxDuration:    l.MaxDuration,
	}
}

// TokenTotals holds session-level token usage.
type TokenTotals struct {
	Input  int
	Output int
	Cost   float64
}
