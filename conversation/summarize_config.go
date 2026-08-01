package conversation

// SummarizeConfig configures conversation summarization behavior
type SummarizeConfig struct {
	// Trigger: summarize when message count exceeds this
	TriggerMessages int
	// How many recent turns to keep in full (never summarize)
	KeepRecentTurns int
	// Model to use for summarization (empty = same as agent)
	Model string
	// Max tokens for the summary output
	MaxSummaryTokens int
	// Save full conversation to memory before summarizing
	SaveToMemory bool
	// ArchiveIsRecallable says whether the model reading the summary can call
	// memory_expand. It decides one thing: whether the injected summary block
	// names the archive's id.
	//
	// SaveToMemory alone cannot decide it. The archive is tagged out of the
	// automatic context on purpose, so the only way back to it is a tool call by
	// id, and the id lives nowhere the model can see. An agent whose configured
	// tool list omits memory_expand has the archive written and no way to reach
	// it; naming it there would be a pointer at a tool the model does not hold.
	//
	// The caller answers from the tools it actually put on the wire, not from a
	// guess: engine.summarizeIfNeeded reads EnabledToolNames.
	ArchiveIsRecallable bool
}

// DefaultSummarizeConfig returns defaults based on agent role
func DefaultSummarizeConfig(role AgentRole) SummarizeConfig {
	switch role {
	case RoleOrchestrator:
		return SummarizeConfig{
			TriggerMessages:  80, // ~40 turns
			KeepRecentTurns:  15,
			MaxSummaryTokens: 1024,
			SaveToMemory:     true,
		}
	case RoleCoder:
		return SummarizeConfig{
			TriggerMessages:  40, // ~20 turns
			KeepRecentTurns:  8,
			MaxSummaryTokens: 800,
			SaveToMemory:     true,
		}
	default:
		return SummarizeConfig{
			TriggerMessages:  60,
			KeepRecentTurns:  12,
			MaxSummaryTokens: 1024,
			SaveToMemory:     true,
		}
	}
}

// SummarizeResult contains the results of conversation summarization
type SummarizeResult struct {
	Summarized      bool   // whether summarization occurred
	SummarizedTurns int    // number of turns that were summarized
	KeptMessages    int    // number of messages in final result
	SummaryTokens   int    // estimated tokens in the summary
	MemorySaved     bool   // whether full conversation was saved to memory
	MemoryID        string // memory ID if saved
}