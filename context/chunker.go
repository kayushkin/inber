package context

import (
	"fmt"
	"strings"
	"time"
)

const (
	// MaxChunkSize is the threshold for splitting chunks
	MaxChunkSize = 5000
	
	// TargetChunkSize is the target size for sub-chunks
	TargetChunkSize = 3000
)

// Chunker splits large inputs into smaller chunks
type Chunker struct {
	tagger Tagger
}

// NewChunker creates a new chunker
func NewChunker(tagger Tagger) *Chunker {
	return &Chunker{
		tagger: tagger,
	}
}

// ChunkInput processes input and returns one or more chunks
// If input is small, returns a single chunk
// If input is large, splits it into multiple chunks
func (c *Chunker) ChunkInput(id, text, source string, baseTags []string) []Chunk {
	tokens := EstimateTokens(text)
	
	// If small enough, return as single chunk
	if tokens <= MaxChunkSize {
		tags := append([]string{}, baseTags...)
		if c.tagger != nil {
			tags = append(tags, c.tagger.Tag(text, source)...)
		}
		
		return []Chunk{{
			ID:        id,
			Text:      text,
			Tokens:    tokens,
			Tags:      deduplicateTags(tags),
			Source:    source,
			CreatedAt: time.Now(),
		}}
	}
	
	// Large input - split it
	return c.splitLarge(id, text, source, baseTags)
}

// splitLarge splits a large text into multiple chunks
func (c *Chunker) splitLarge(baseID, text, source string, baseTags []string) []Chunk {
	var chunks []Chunk
	
	// Try to split by natural boundaries
	parts := c.splitByBoundaries(text)
	
	chunkIndex := 0
	for _, part := range parts {
		partTokens := EstimateTokens(part)
		
		// If part is still too large, split by size
		if partTokens > MaxChunkSize {
			subParts := c.splitBySize(part, TargetChunkSize)
			for _, subPart := range subParts {
				chunk := c.makeChunk(baseID, chunkIndex, subPart, source, baseTags)
				chunks = append(chunks, chunk)
				chunkIndex++
			}
		} else {
			chunk := c.makeChunk(baseID, chunkIndex, part, source, baseTags)
			chunks = append(chunks, chunk)
			chunkIndex++
		}
	}
	
	return chunks
}

// makeChunk creates a chunk with auto-generated tags
func (c *Chunker) makeChunk(baseID string, index int, text, source string, baseTags []string) Chunk {
	id := fmt.Sprintf("%s-%d", baseID, index)
	
	tags := append([]string{}, baseTags...)
	if c.tagger != nil {
		tags = append(tags, c.tagger.Tag(text, source)...)
	}
	
	return Chunk{
		ID:        id,
		Text:      text,
		Tokens:    EstimateTokens(text),
		Tags:      deduplicateTags(tags),
		Source:    source,
		CreatedAt: time.Now(),
	}
}

// splitByBoundaries splits text by natural boundaries (paragraphs, code blocks, etc.)
func (c *Chunker) splitByBoundaries(text string) []string {
	// First try to split by double newlines (paragraphs)
	paragraphs := strings.Split(text, "\n\n")
	
	var parts []string
	var current strings.Builder
	currentTokens := 0
	
	for _, para := range paragraphs {
		paraTokens := EstimateTokens(para)
		
		// If adding this paragraph exceeds target, flush current
		if currentTokens > 0 && currentTokens+paraTokens > TargetChunkSize {
			parts = append(parts, current.String())
			current.Reset()
			currentTokens = 0
		}
		
		if current.Len() > 0 {
			current.WriteString("\n\n")
		}
		current.WriteString(para)
		currentTokens += paraTokens
	}
	
	// Add remaining
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	
	// If we got no split (single paragraph), return as-is
	if len(parts) == 0 {
		parts = []string{text}
	}
	
	return parts
}

// splitBySize splits text into chunks of approximately targetSize tokens
func (c *Chunker) splitBySize(text string, targetSize int) []string {
	// Split by lines first
	lines := strings.Split(text, "\n")
	
	var parts []string
	var current strings.Builder
	currentTokens := 0
	
	for _, line := range lines {
		lineTokens := EstimateTokens(line)
		
		// If adding this line exceeds target, flush current
		if currentTokens > 0 && currentTokens+lineTokens > targetSize {
			parts = append(parts, current.String())
			current.Reset()
			currentTokens = 0
		}
		
		if current.Len() > 0 {
			current.WriteString("\n")
		}
		current.WriteString(line)
		currentTokens += lineTokens
	}
	
	// Add remaining
	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	
	// If no split happened, force split by characters
	if len(parts) == 0 || (len(parts) == 1 && EstimateTokens(parts[0]) > MaxChunkSize) {
		return c.splitByCharacters(text, targetSize)
	}
	
	return parts
}

// splitByCharacters forcibly splits text by character count (last resort)
func (c *Chunker) splitByCharacters(text string, targetSize int) []string {
	targetChars := targetSize * 4 // ~4 chars per token
	
	var parts []string
	for i := 0; i < len(text); i += targetChars {
		end := i + targetChars
		if end > len(text) {
			end = len(text)
		}
		parts = append(parts, text[i:end])
	}
	
	return parts
}

// deduplicateTags removes duplicate tags while preserving order
func deduplicateTags(tags []string) []string {
	seen := make(map[string]bool)
	var result []string
	
	for _, tag := range tags {
		if !seen[tag] {
			seen[tag] = true
			result = append(result, tag)
		}
	}
	
	return result
}
