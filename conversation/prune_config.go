package conversation

// AgentRole represents different agent roles with different pruning needs
type AgentRole string

const (
	RoleOrchestrator AgentRole = "orchestrator"
	RoleCoder        AgentRole = "coder"
	RoleTester       AgentRole = "tester"
	RoleDefault      AgentRole = "default"
)

// PruneConfig configures conversation pruning behavior
type PruneConfig struct {
	Role                    AgentRole // Agent role determines pruning strategy
	KeepRecentTurns         int       // Keep last N conversation turns in full
	AssistantTruncateAfter  int       // Truncate assistant messages older than N turns
	ToolResultKeepFull      int       // Keep tool results in full for last N turns
	ToolResultSummary       int       // Summarize tool results N to ToolResultKeepFull turns ago
	ToolResultDrop          int       // Drop tool results older than N turns
	ToolCallKeepFull        int       // Keep tool call inputs in full for last N turns
	AutoSaveThreshold       int       // Token count threshold for auto-saving to memory
	AggressiveTruncation    bool      // Legacy field for backwards compatibility
	MemorySaveThreshold     int       // Auto-save to memory if pruning would remove this many turns
	TokenBudget             int       // Target token budget for pruned conversation
	MinimumImportance       float64   // Minimum importance score for auto-saving memories
}

// OrchestratorPruneConfig returns pruning config optimized for orchestrator agents
func OrchestratorPruneConfig() PruneConfig {
	return PruneConfig{
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
		MinimumImportance:      0.3,
	}
}

// CoderPruneConfig returns pruning config optimized for coder/implementer agents
func CoderPruneConfig() PruneConfig {
	return PruneConfig{
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
		MinimumImportance:      0.3,
	}
}

// TesterPruneConfig returns pruning config optimized for tester/validator agents
func TesterPruneConfig() PruneConfig {
	return PruneConfig{
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
		MinimumImportance:      0.3,
	}
}

// DefaultPruneConfig returns sensible defaults for conversation pruning
func DefaultPruneConfig() PruneConfig {
	return PruneConfig{
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
		MinimumImportance:      0.3,
	}
}

// PruneConfigForRole returns the appropriate pruning config for a given role string
func PruneConfigForRole(roleStr string) PruneConfig {
	switch AgentRole(roleStr) {
	case RoleOrchestrator:
		return OrchestratorPruneConfig()
	case RoleCoder:
		return CoderPruneConfig()
	case RoleTester:
		return TesterPruneConfig()
	case RoleDefault:
		return DefaultPruneConfig()
	default:
		// Unknown role, return default config
		config := DefaultPruneConfig()
		config.Role = AgentRole(roleStr)
		return config
	}
}