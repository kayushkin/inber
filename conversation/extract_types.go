package conversation

// ExtractionConfig configures background memory extraction
type ExtractionConfig struct {
	Enabled             bool
	MinExchangeTokens   int     // Only extract from exchanges larger than this
	Model               string  // Model to use for extraction (defaults to current model)
	DuplicateThreshold  float64 // Search score threshold for duplicate detection
	MaxSearchResults    int     // How many similar memories to check for duplicates
	MinImportance       float64 // Don't save memories below this importance
}

// DefaultExtractionConfig returns sensible defaults
func DefaultExtractionConfig() ExtractionConfig {
	return ExtractionConfig{
		Enabled:            true,
		MinExchangeTokens:  200,
		Model:              "", // Use same model as agent
		DuplicateThreshold: 0.7,
		MaxSearchResults:   5,
		MinImportance:      0.3,
	}
}

// ExtractedMemory represents a memory extracted by the LLM
type ExtractedMemory struct {
	Content    string   `json:"content"`
	Importance float64  `json:"importance"`
	Tags       []string `json:"tags"`
}

// ExtractionResult describes what was extracted
type ExtractionResult struct {
	ExtractedCount int
	SavedCount     int
	DuplicateCount int
	Error          error
}

// ToolCallSummary represents a simplified tool call for extraction context
type ToolCallSummary struct {
	Name string
}