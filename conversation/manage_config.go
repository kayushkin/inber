package conversation

import (
	"strings"
)

// AgentRole represents different agent roles with different management needs
type AgentRole string

const (
	RoleOrchestrator AgentRole = "orchestrator"
	RoleCoder        AgentRole = "coder"
	RoleTester       AgentRole = "tester"
	RoleDefault      AgentRole = "default"
)

// ManagementConfig configures conversation management behavior (pruning + stashing)
type ManagementConfig struct {
	Role                    AgentRole // Agent role determines strategy
	KeepRecentTurns         int       // Keep last N conversation turns in full
	AssistantTruncateAfter  int       // Truncate assistant messages older than N turns
	ToolResultKeepFull      int       // Keep tool results in full for last N turns
	ToolResultSummary       int       // Summarize tool results N to ToolResultKeepFull turns ago
	ToolResultDrop          int       // Drop tool results older than N turns
	ToolCallKeepFull        int       // Keep tool call inputs in full for last N turns
	AutoSaveThreshold       int       // Token count threshold for auto-saving to memory
	AggressiveTruncation    bool      // Legacy field for backwards compatibility
	MemorySaveThreshold     int       // Auto-save to memory if pruning would remove this many turns
	TokenBudget             int       // Target token budget for managed conversation
	MinimumImportance       float64   // Minimum importance score for auto-saving memories
	
	// Batched management — only prune/dedup every N turns for cache stability
	ManageInterval          int       // Only manage every N turns (0 = every turn, default 5)

	// Stashing configuration
	Stash                   StashConfig // Configuration for stashing large content
}

// PruneConfig is an alias for backward compatibility
type PruneConfig = ManagementConfig

// OrchestratorManagementConfig returns config optimized for orchestrator agents
func OrchestratorManagementConfig() ManagementConfig {
	return ManagementConfig{
		Role:                   RoleOrchestrator,
		KeepRecentTurns:        40,
		AssistantTruncateAfter: 8,
		ToolResultKeepFull:     3,
		ToolResultSummary:      8,
		ToolResultDrop:         8,
		ToolCallKeepFull:       5,
		AutoSaveThreshold:      500,
		AggressiveTruncation:   true,
		MemorySaveThreshold:    10,
		TokenBudget:            50000,
		ManageInterval:         3,
		MinimumImportance:      0.3,
		Stash: StashConfig{
			Enabled:               true,
			UserMessageThreshold:  1000,
			AssistantThreshold:    1500,
			MinBlockSize:         1000,
			DefaultImportance:    0.6,
		},
	}
}

// CoderManagementConfig returns config optimized for coder/implementer agents
func CoderManagementConfig() ManagementConfig {
	return ManagementConfig{
		Role:                   RoleCoder,
		KeepRecentTurns:        20,
		AssistantTruncateAfter: 15,
		ToolResultKeepFull:     10,
		ToolResultSummary:      20,
		ToolResultDrop:         20,
		ToolCallKeepFull:       5,
		AutoSaveThreshold:      1000,
		AggressiveTruncation:   true,
		MemorySaveThreshold:    10,
		TokenBudget:            50000,
		ManageInterval:         5,
		MinimumImportance:      0.3,
		Stash: StashConfig{
			Enabled:               true,
			UserMessageThreshold:  800,
			AssistantThreshold:    1200,
			MinBlockSize:         800,
			DefaultImportance:    0.6,
		},
	}
}

// TesterManagementConfig returns config optimized for tester/validator agents
func TesterManagementConfig() ManagementConfig {
	return ManagementConfig{
		Role:                   RoleTester,
		KeepRecentTurns:        20,
		AssistantTruncateAfter: 10,
		ToolResultKeepFull:     15, // Testers need test output
		ToolResultSummary:      25,
		ToolResultDrop:         25,
		ToolCallKeepFull:       5,
		AutoSaveThreshold:      1000,
		AggressiveTruncation:   true,
		MemorySaveThreshold:    10,
		TokenBudget:            50000,
		ManageInterval:         5,
		MinimumImportance:      0.3,
		Stash: StashConfig{
			Enabled:               true,
			UserMessageThreshold:  1000,
			AssistantThreshold:    1500,
			MinBlockSize:         1000,
			DefaultImportance:    0.6,
		},
	}
}

// DefaultManagementConfig returns sensible defaults for conversation management
func DefaultManagementConfig() ManagementConfig {
	return ManagementConfig{
		Role:                   RoleDefault,
		KeepRecentTurns:        35,
		AssistantTruncateAfter: 10,
		ToolResultKeepFull:     3,
		ToolResultSummary:      10,
		ToolResultDrop:         10,
		ToolCallKeepFull:       5,
		AutoSaveThreshold:      500,
		AggressiveTruncation:   true,
		MemorySaveThreshold:    10,
		TokenBudget:            50000,
		ManageInterval:         5,
		MinimumImportance:      0.3,
		Stash: StashConfig{
			Enabled:               true,
			UserMessageThreshold:  1000,
			AssistantThreshold:    1500,
			MinBlockSize:         1000,
			DefaultImportance:    0.6,
		},
	}
}

// Backward compatibility functions
func OrchestratorPruneConfig() PruneConfig { return OrchestratorManagementConfig() }
func CoderPruneConfig() PruneConfig { return CoderManagementConfig() }
func TesterPruneConfig() PruneConfig { return TesterManagementConfig() }
func DefaultPruneConfig() PruneConfig { return DefaultManagementConfig() }
// Removed: Use DefaultStashConfig() from stash.go instead

// ManagementConfigForRole returns the appropriate config for the given role string
func ManagementConfigForRole(roleStr string) ManagementConfig {
	lower := strings.ToLower(roleStr)
	
	// Check for orchestrator patterns
	if strings.Contains(lower, "orchestrat") || strings.Contains(lower, "coordinat") || 
	   strings.Contains(lower, "dispatch") || strings.Contains(lower, "delegat") {
		return OrchestratorManagementConfig()
	}
	
	// Check for coder patterns
	if strings.Contains(lower, "code") || strings.Contains(lower, "implement") || 
	   strings.Contains(lower, "scholar") || strings.Contains(lower, "develop") {
		return CoderManagementConfig()
	}
	
	// Check for tester patterns
	if strings.Contains(lower, "test") || strings.Contains(lower, "validat") || 
	   strings.Contains(lower, "sentinel") {
		return TesterManagementConfig()
	}
	
	return DefaultManagementConfig()
}

// Backward compatibility function
func PruneConfigForRole(roleStr string) PruneConfig {
	return ManagementConfigForRole(roleStr)
}

// PruningStrategy describes how messages were managed (backward compatibility)
type PruningStrategy struct {
	Name        string
	Description string
}

var (
	StrategyKeepRecent = PruningStrategy{
		Name:        "keep-recent",
		Description: "Keep last N turns in full, remove older turns",
	}
	StrategyAggressiveTruncate = PruningStrategy{
		Name:        "aggressive-truncate",
		Description: "Keep recent turns, truncate old tool results",
	}
	StrategySummarize = PruningStrategy{
		Name:        "summarize",
		Description: "Keep recent turns, summarize old conversation with LLM",
	}
	StrategyRoleBased = PruningStrategy{
		Name:        "role-based",
		Description: "Apply role-specific pruning rules based on message type and age",
	}
	StrategyUnified = PruningStrategy{
		Name:        "unified",
		Description: "Apply unified management including stashing and pruning",
	}
)