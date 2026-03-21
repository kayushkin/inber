package conversation

import "strings"

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