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
	Summarized      bool     // whether summarization occurred
	SummarizedTurns int      // number of turns that were summarized
	KeptMessages    int      // number of messages in final result  
	SummaryTokens   int      // estimated tokens in the summary
	MemorySaved     bool     // whether full conversation was saved to memory
	MemoryID        string   // memory ID if saved
	SummaryDegraded bool     // true when the LLM summary failed and the mechanical fallback was used
	SummaryError    string   // why the LLM summary failed, when SummaryDegraded is set
}