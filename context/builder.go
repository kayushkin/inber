package context

import (
	"sort"
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
// 2. Tag-matched chunks (size-aware)
// 3. Recent conversation chunks
func (b *Builder) Build(messageTags []string) []Chunk {
	allChunks := b.store.ListAll()
	
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
