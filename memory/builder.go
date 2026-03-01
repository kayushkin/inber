package memory

import (
	"sort"
	"time"
)

// BuildContextRequest specifies how to build context from memory
type BuildContextRequest struct {
	Tags              []string // Tags to match (from message/query)
	TokenBudget       int      // Maximum tokens to include
	MinImportance     float64  // Minimum importance threshold (default: 0.0)
	ExcludeTags       []string // Tags to exclude (e.g., "test", "archived")
	IncludeAlwaysLoad bool     // Whether to include AlwaysLoad memories (default: true)
	MaxChunkSize      int      // Skip memories larger than this (default: 0 = no limit)
}

// BuildContext retrieves memories suitable for including in a prompt.
// Returns memories ordered by priority and total tokens used.
//
// Priority order:
// 1. AlwaysLoad memories (identity, instructions)
// 2. Tag-matched memories (more matches = higher priority)
// 3. High importance memories
// 4. Recently accessed memories
func (s *Store) BuildContext(req BuildContextRequest) ([]Memory, int, error) {
	// Set defaults
	if req.TokenBudget <= 0 {
		req.TokenBudget = 32000
	}
	if !req.IncludeAlwaysLoad && req.MinImportance == 0 {
		req.MinImportance = 0.4 // reasonable default if not including always-load
	}

	// Build query
	now := time.Now()
	query := `
	SELECT m.id, m.content, m.summary, m.original_id, m.importance, m.access_count, 
	       m.last_accessed, m.created_at, m.source, m.embedding, m.always_load, m.expires_at, m.tokens
	FROM memories m
	WHERE m.importance >= ?
	  AND (m.expires_at IS NULL OR m.expires_at > ?)
	`
	args := []interface{}{req.MinImportance, now.Unix()}

	// Add exclusion filter if needed
	if len(req.ExcludeTags) > 0 {
		placeholders := make([]string, len(req.ExcludeTags))
		for i := range req.ExcludeTags {
			placeholders[i] = "?"
			args = append(args, req.ExcludeTags[i])
		}
		query += ` AND m.id NOT IN (
			SELECT memory_id FROM memory_tags WHERE tag IN (` + join(placeholders, ",") + `)
		)`
	}

	// Execute query
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	// Scan all candidate memories
	var candidates []memoryWithScore
	tagSet := make(map[string]bool)
	for _, tag := range req.Tags {
		tagSet[tag] = true
	}

	for rows.Next() {
		m, err := s.scanMemory(rows)
		if err != nil {
			continue
		}

		// Skip oversized memories if limit set
		if req.MaxChunkSize > 0 && m.Tokens > req.MaxChunkSize {
			continue
		}

		// Load tags for this memory
		m.Tags, _ = s.loadTags(m.ID)

		// Calculate score
		score := calculateScore(m, tagSet)
		candidates = append(candidates, memoryWithScore{
			memory: m,
			score:  score,
		})
	}

	// Sort by priority
	sort.Slice(candidates, func(i, j int) bool {
		// AlwaysLoad memories always come first
		if candidates[i].memory.AlwaysLoad != candidates[j].memory.AlwaysLoad {
			return candidates[i].memory.AlwaysLoad
		}
		// Then by score
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		// Tie-breaker: smaller memories first (more likely to fit)
		return candidates[i].memory.Tokens < candidates[j].memory.Tokens
	})

	// Build result list within budget
	var result []Memory
	tokensUsed := 0

	for _, candidate := range candidates {
		m := candidate.memory

		// Check budget
		if tokensUsed+m.Tokens > req.TokenBudget {
			// If it's an AlwaysLoad memory, we have a problem
			if m.AlwaysLoad {
				// Still include it but warn
				result = append(result, m)
				tokensUsed += m.Tokens
			}
			continue
		}

		result = append(result, m)
		tokensUsed += m.Tokens
	}

	return result, tokensUsed, nil
}

// memoryWithScore pairs a memory with its relevance score
type memoryWithScore struct {
	memory Memory
	score  float64
}

// calculateScore computes a relevance score for a memory
func calculateScore(m Memory, tagSet map[string]bool) float64 {
	score := m.Importance

	// Tag matching bonus
	matchCount := 0
	for _, tag := range m.Tags {
		if tagSet[tag] {
			matchCount++
		}
	}
	score += float64(matchCount) * 0.3 // each matching tag adds 0.3

	// Recency bonus (recently accessed memories are more relevant)
	daysSinceAccess := time.Since(m.LastAccessed).Hours() / 24
	if daysSinceAccess < 1 {
		score += 0.2
	} else if daysSinceAccess < 7 {
		score += 0.1
	}

	return score
}

// join is a simple string join helper
func join(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}
