package context

import (
	"sort"
	"strings"
)

// Builder assembles context from a store based on tags and token budget
type Builder struct {
	store       *Store
	tokenBudget int
}

// NewBuilder creates a new context builder
func NewBuilder(store *Store, tokenBudget int) *Builder {
	return &Builder{
		store:       store,
		tokenBudget: tokenBudget,
	}
}

// Build assembles an ordered list of chunks that fit within the token budget
// Priority:
// 1. Always-include chunks ("identity", "always" tags)
// 2. Tag-matched chunks (size-aware, with minimum relevance threshold)
// 3. Recent conversation chunks
func (b *Builder) Build(messageTags []string) []Chunk {
	allChunks := b.store.ListAll()
	
	// Deduplicate chunks by content hash
	allChunks = deduplicateChunks(allChunks)
	
	var alwaysInclude []Chunk
	var tagMatched []Chunk
	var conversation []Chunk
	
	messageTagSet := make(map[string]bool)
	for _, tag := range messageTags {
		messageTagSet[tag] = true
	}
	
	// Check if tests are requested
	includeTests := messageTagSet["test"]
	
	// Categorize chunks
	for _, chunk := range allChunks {
		// Exclude test files by default unless test tag is in message
		if hasTag(chunk.Tags, "test") && !includeTests {
			continue
		}
		
		if hasTag(chunk.Tags, "identity") || hasTag(chunk.Tags, "always") {
			alwaysInclude = append(alwaysInclude, chunk)
		} else if matchCount := countMatchingTags(chunk.Tags, messageTagSet); matchCount > 0 {
			// Score-based filtering based on size and tag match strength
			if shouldInclude(chunk, matchCount) {
				tagMatched = append(tagMatched, chunk)
			}
		} else if chunk.Source == "user" || chunk.Source == "assistant" {
			conversation = append(conversation, chunk)
		}
	}
	
	// Sort tag-matched by relevance (more matching tags = higher priority)
	sort.Slice(tagMatched, func(i, j int) bool {
		scoreI := countMatchingTags(tagMatched[i].Tags, messageTagSet)
		scoreJ := countMatchingTags(tagMatched[j].Tags, messageTagSet)
		if scoreI != scoreJ {
			return scoreI > scoreJ
		}
		// Tie-breaker: smaller chunks first (more likely to fit)
		return tagMatched[i].Tokens < tagMatched[j].Tokens
	})
	
	// Sort conversation by recency (most recent first)
	sort.Slice(conversation, func(i, j int) bool {
		return conversation[i].CreatedAt.After(conversation[j].CreatedAt)
	})
	
	// Build final list within budget
	var result []Chunk
	tokensUsed := 0
	
	// 1. Always-include chunks (must fit)
	for _, chunk := range alwaysInclude {
		if tokensUsed+chunk.Tokens <= b.tokenBudget {
			result = append(result, chunk)
			tokensUsed += chunk.Tokens
		}
	}
	
	// 2. Tag-matched chunks
	for _, chunk := range tagMatched {
		if tokensUsed+chunk.Tokens <= b.tokenBudget {
			result = append(result, chunk)
			tokensUsed += chunk.Tokens
		}
	}
	
	// 3. Recent conversation chunks
	for _, chunk := range conversation {
		if tokensUsed+chunk.Tokens <= b.tokenBudget {
			result = append(result, chunk)
			tokensUsed += chunk.Tokens
		} else {
			break // Stop if we run out of budget
		}
	}
	
	return result
}

// shouldInclude determines if a chunk should be included based on size and tag match count
func shouldInclude(chunk Chunk, matchCount int) bool {
	// Chunks < 500 tokens: include if any tag matches
	if chunk.Tokens < 500 {
		return matchCount > 0
	}
	
	// Chunks 500-5000 tokens: need strong match (multiple shared tags)
	if chunk.Tokens <= 5000 {
		return matchCount >= 2
	}
	
	// Chunks > 5000 tokens: need very strong match
	return matchCount >= 3
}

// hasTag checks if a tag list contains a specific tag
func hasTag(tags []string, target string) bool {
	for _, tag := range tags {
		if tag == target {
			return true
		}
	}
	return false
}

// countMatchingTags counts how many tags from the chunk are in the message tag set
func countMatchingTags(chunkTags []string, messageTagSet map[string]bool) int {
	count := 0
	for _, tag := range chunkTags {
		if messageTagSet[tag] {
			count++
		}
	}
	return count
}

// deduplicateChunks removes duplicate chunks based on ID + content similarity
func deduplicateChunks(chunks []Chunk) []Chunk {
	if len(chunks) == 0 {
		return chunks
	}
	
	seenIDs := make(map[string]bool)
	seenContent := make(map[string]bool)
	var result []Chunk
	
	for _, chunk := range chunks {
		// Skip if we've seen this exact ID before (strict duplicate)
		if seenIDs[chunk.ID] {
			continue
		}
		
		// For content deduplication, only check long chunks (> 100 chars)
		// Short chunks like "package main" are too common to deduplicate
		if len(chunk.Text) > 100 {
			// Check for near-duplicates (chunks that are very similar)
			isDuplicate := false
			for existing := range seenContent {
				if areSimilar(chunk.Text, existing) {
					isDuplicate = true
					break
				}
			}
			
			if isDuplicate {
				continue
			}
			
			seenContent[chunk.Text] = true
		}
		
		seenIDs[chunk.ID] = true
		result = append(result, chunk)
	}
	
	return result
}

// areSimilar checks if two texts are similar enough to be considered duplicates
func areSimilar(text1, text2 string) bool {
	// Exact match
	if text1 == text2 {
		return true
	}
	
	// Length-based quick filter
	len1, len2 := len(text1), len(text2)
	if len1 == 0 || len2 == 0 {
		return false
	}
	
	// If lengths are very different, not similar
	ratio := float64(len1) / float64(len2)
	if ratio < 0.8 || ratio > 1.2 {
		return false
	}
	
	// Check if one is a substring of the other (with some tolerance)
	if len1 < len2 {
		return strings.Contains(text2, text1)
	} else {
		return strings.Contains(text1, text2)
	}
}
