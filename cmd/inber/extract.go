package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/google/uuid"
	"github.com/kayushkin/inber/agent"
	inbercontext "github.com/kayushkin/inber/context"
	"github.com/kayushkin/inber/memory"
)

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

const extractionPrompt = `Extract any facts, decisions, preferences, or important context from this exchange worth remembering long-term.

For each item, provide:
- content: the fact/decision (concise, 1-2 sentences max)
- importance: 0.0-1.0 (0.3=minor, 0.5=moderate, 0.7=important, 0.9=critical)
- tags: relevant keywords (e.g., ["coding", "preference"], ["decision", "architecture"])

Guidelines:
- Only extract genuinely useful information worth remembering across sessions
- Skip trivial greetings, confirmations, or transient information
- Focus on: decisions made, preferences stated, facts learned, problems solved
- Be concise - each memory should be self-contained and clear
- If nothing worth remembering, return empty array

Response format: JSON array of {content, importance, tags[]}

Example:
[
  {
    "content": "User prefers using Go for backend services, values simplicity over abstraction",
    "importance": 0.6,
    "tags": ["preference", "golang", "architecture"]
  },
  {
    "content": "Decided to use SQLite for memory storage instead of external DB to minimize dependencies",
    "importance": 0.7,
    "tags": ["decision", "architecture", "database"]
  }
]

Exchange to analyze:
`

// ToolCallSummary represents a simplified tool call for extraction context
type ToolCallSummary struct {
	Name string
}

// BackgroundExtractMemories runs in a goroutine after a turn completes.
// It extracts facts/decisions from the last exchange and saves to memory.
func BackgroundExtractMemories(
	ctx context.Context,
	client *anthropic.Client,
	userMessage string,
	assistantResponse string,
	toolCalls []ToolCallSummary,
	sessionID string,
	memStore *memory.Store,
	cfg ExtractionConfig,
	logger *Logger,
) {
	// Defer recovery to prevent goroutine panics from crashing the app
	defer func() {
		if r := recover(); r != nil {
			if logger != nil {
				logger.Warn("memory extraction panic: %v", r)
			}
		}
	}()

	// Check if exchange is substantive enough
	combinedTokens := inbercontext.EstimateTokens(userMessage) + 
		inbercontext.EstimateTokens(assistantResponse)
	
	if combinedTokens < cfg.MinExchangeTokens {
		return // Too trivial
	}

	// Build exchange summary
	var exchangeText strings.Builder
	exchangeText.WriteString("USER: ")
	exchangeText.WriteString(userMessage)
	exchangeText.WriteString("\n\nASSISTANT: ")
	exchangeText.WriteString(assistantResponse)

	// Include tool calls if present (summarized)
	if len(toolCalls) > 0 {
		exchangeText.WriteString("\n\nTool calls made:")
		for _, tc := range toolCalls {
			exchangeText.WriteString(fmt.Sprintf("\n- %s", tc.Name))
		}
	}

	exchange := exchangeText.String()

	// Keep prompt small (<500 tokens)
	promptTokens := inbercontext.EstimateTokens(extractionPrompt + exchange)
	if promptTokens > 500 {
		// Truncate exchange to fit budget
		maxExchangeTokens := 500 - inbercontext.EstimateTokens(extractionPrompt)
		exchangeChars := (maxExchangeTokens * 4) // ~4 chars per token
		if len(exchange) > exchangeChars {
			exchange = exchange[:exchangeChars] + "..."
		}
	}

	fullPrompt := extractionPrompt + exchange

	// Call LLM for extraction
	modelToUse := cfg.Model
	if modelToUse == "" {
		modelToUse = agent.DefaultModel // Use default if not specified
	}

	resp, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(modelToUse),
		MaxTokens: 1024,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(fullPrompt)),
		},
	})

	if err != nil {
		if logger != nil {
			logger.Warn("memory extraction API call failed: %v", err)
		}
		return
	}

	// Extract text response
	var responseText string
	for _, block := range resp.Content {
		if block.Type == "text" {
			responseText += block.Text
		}
	}

	if responseText == "" {
		return
	}

	// Parse JSON response
	var extracted []ExtractedMemory
	
	// Try to extract JSON array from the response (might have markdown fences)
	responseText = strings.TrimSpace(responseText)
	responseText = strings.TrimPrefix(responseText, "```json")
	responseText = strings.TrimPrefix(responseText, "```")
	responseText = strings.TrimSuffix(responseText, "```")
	responseText = strings.TrimSpace(responseText)

	if err := json.Unmarshal([]byte(responseText), &extracted); err != nil {
		if logger != nil {
			logger.Warn("memory extraction JSON parse failed: %v", err)
		}
		return
	}

	if len(extracted) == 0 {
		return // Nothing to save
	}

	// Save extracted memories (check for duplicates first)
	saved := 0
	duplicates := 0

	for _, item := range extracted {
		// Validate
		if item.Content == "" {
			continue
		}
		if item.Importance < cfg.MinImportance || item.Importance > 1.0 {
			continue
		}

		// Check for duplicates
		isDuplicate := false
		if cfg.DuplicateThreshold > 0 {
			existing, err := memStore.Search(item.Content, cfg.MaxSearchResults)
			if err == nil && len(existing) > 0 {
				// Check if any existing memory is very similar
				for _, existingMem := range existing {
					similarity := calculateSimilarity(item.Content, existingMem.Content)
					if similarity > cfg.DuplicateThreshold {
						isDuplicate = true
						duplicates++
						break
					}
				}
			}
		}

		if isDuplicate {
			continue
		}

		// Add session tag
		tags := append([]string{}, item.Tags...)
		tags = append(tags, "auto-extracted", sessionID)

		// Save memory
		mem := memory.Memory{
			ID:         uuid.New().String(),
			Content:    item.Content,
			Tags:       tags,
			Importance: item.Importance,
			Source:     "extraction",
		}

		if err := memStore.Save(mem); err != nil {
			if logger != nil {
				logger.Warn("failed to save extracted memory: %v", err)
			}
			continue
		}

		saved++
	}

	// Log extraction results
	if logger != nil && saved > 0 {
		logger.Info("extracted %d memories from last exchange (%d duplicates skipped)", saved, duplicates)
	}
}

// calculateSimilarity computes a simple word-overlap similarity score
// This is a lightweight alternative to full embedding comparison
func calculateSimilarity(a, b string) float64 {
	wordsA := strings.Fields(strings.ToLower(a))
	wordsB := strings.Fields(strings.ToLower(b))

	if len(wordsA) == 0 || len(wordsB) == 0 {
		return 0.0
	}

	// Build word sets
	setA := make(map[string]bool)
	setB := make(map[string]bool)
	for _, w := range wordsA {
		setA[w] = true
	}
	for _, w := range wordsB {
		setB[w] = true
	}

	// Count overlaps
	overlap := 0
	for w := range setA {
		if setB[w] {
			overlap++
		}
	}

	// Jaccard similarity
	union := len(setA) + len(setB) - overlap
	if union == 0 {
		return 0.0
	}

	return float64(overlap) / float64(union)
}
