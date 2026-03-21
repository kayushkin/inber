// Package conversation manages the conversation lifecycle, including
// memory extraction, summarization, conversation pruning, and role-based
// management for different agent types (orchestrator, coder, tester).
package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/google/uuid"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/memory"
)

// BackgroundExtractMemories runs in a goroutine after a turn completes.
// It extracts facts/decisions from the last exchange and saves to memory.
func BackgroundExtractMemories(
	ctx context.Context,
	client *anthropic.Client,
	userMessage string,
	assistantResponse string,
	toolCalls []ToolCallSummary,
	sessionID string,
	memStore memory.MemoryStore,
	cfg ExtractionConfig,
) {
	// Defer recovery to prevent goroutine panics from crashing the app
	defer func() {
		if r := recover(); r != nil {
							log.Printf("[warn] " +"memory extraction panic: %v", r)
		}
	}()

	// Check if exchange is substantive enough
	combinedTokens := memory.EstimateTokens(userMessage) + 
		memory.EstimateTokens(assistantResponse)
	
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
	promptTokens := memory.EstimateTokens(extractionPrompt + exchange)
	if promptTokens > 500 {
		// Truncate exchange to fit budget
		maxExchangeTokens := 500 - memory.EstimateTokens(extractionPrompt)
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
					log.Printf("[warn] " +"memory extraction API call failed: %v", err)
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
					log.Printf("[warn] " +"memory extraction JSON parse failed: %v", err)
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
							log.Printf("[warn] " +"failed to save extracted memory: %v", err)
			continue
		}

		saved++
	}

	// Log extraction results
	if saved > 0 {
		log.Printf("[info] extracted %d memories from last exchange (%d duplicates skipped)", saved, duplicates)
	}
}