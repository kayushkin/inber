package context

import (
	"strings"
	"testing"
)

func TestChunker_SmallInput(t *testing.T) {
	tagger := NewPatternTagger()
	chunker := NewChunker(tagger)
	
	text := "This is a small input that should not be split."
	chunks := chunker.ChunkInput("test-1", text, "user", []string{"custom-tag"})
	
	if len(chunks) != 1 {
		t.Errorf("Small input should produce 1 chunk, got %d", len(chunks))
	}
	
	if chunks[0].Text != text {
		t.Error("Chunk text should match input")
	}
	
	hasCustomTag := false
	for _, tag := range chunks[0].Tags {
		if tag == "custom-tag" {
			hasCustomTag = true
		}
	}
	if !hasCustomTag {
		t.Error("Chunk should have custom tag")
	}
}

func TestChunker_LargeInputSplit(t *testing.T) {
	tagger := NewPatternTagger()
	chunker := NewChunker(tagger)
	
	// Create large input (> MaxChunkSize)
	// MaxChunkSize = 5000 tokens ~ 20000 characters
	longText := strings.Repeat("This is a test paragraph.\n\n", 1000) // ~27000 chars
	
	chunks := chunker.ChunkInput("test-large", longText, "user", []string{"test-tag"})
	
	if len(chunks) <= 1 {
		t.Errorf("Large input should be split into multiple chunks, got %d", len(chunks))
	}
	
	// Verify all chunks have base tags
	for i, chunk := range chunks {
		hasTestTag := false
		for _, tag := range chunk.Tags {
			if tag == "test-tag" {
				hasTestTag = true
			}
		}
		if !hasTestTag {
			t.Errorf("Chunk %d should inherit base tag 'test-tag'", i)
		}
		
		// Verify token count is within reasonable range
		if chunk.Tokens > MaxChunkSize {
			t.Errorf("Chunk %d exceeds MaxChunkSize: %d tokens", i, chunk.Tokens)
		}
	}
}

func TestChunker_IDGeneration(t *testing.T) {
	tagger := NewPatternTagger()
	chunker := NewChunker(tagger)
	
	longText := strings.Repeat("Test paragraph.\n\n", 500)
	chunks := chunker.ChunkInput("base-id", longText, "user", nil)
	
	if len(chunks) <= 1 {
		t.Skip("Need multiple chunks for this test")
	}
	
	// Verify IDs are sequential
	expectedIDs := make(map[string]bool)
	for i := range chunks {
		expectedIDs["base-id-"+string(rune('0'+i))] = true
	}
	
	for i, chunk := range chunks {
		expectedID := "base-id-" + string(rune('0'+i))
		if chunk.ID != expectedID {
			t.Errorf("Chunk %d has ID %q, expected %q", i, chunk.ID, expectedID)
		}
	}
}

func TestChunker_SplitByBoundaries(t *testing.T) {
	chunker := NewChunker(nil)
	
	text := "First paragraph.\n\nSecond paragraph.\n\nThird paragraph."
	parts := chunker.splitByBoundaries(text)
	
	// Should split on double newlines
	if len(parts) == 0 {
		t.Error("Should produce at least one part")
	}
	
	// Verify the split happened
	fullText := strings.Join(parts, "")
	if !strings.Contains(fullText, "First") || !strings.Contains(fullText, "Third") {
		t.Error("Split parts should preserve content")
	}
}

func TestChunker_TagPropagation(t *testing.T) {
	tagger := NewPatternTagger()
	chunker := NewChunker(tagger)
	
	// Text with code that should trigger auto-tagging
	text := strings.Repeat("```go\nfunc main() {}\n```\n\n", 500)
	chunks := chunker.ChunkInput("code-test", text, "user", []string{"manual-tag"})
	
	if len(chunks) == 0 {
		t.Fatal("Should produce at least one chunk")
	}
	
	// All chunks should have both manual and auto tags
	for i, chunk := range chunks {
		hasManualTag := false
		hasCodeTag := false
		
		for _, tag := range chunk.Tags {
			if tag == "manual-tag" {
				hasManualTag = true
			}
			if tag == "code" {
				hasCodeTag = true
			}
		}
		
		if !hasManualTag {
			t.Errorf("Chunk %d missing manual tag 'manual-tag'", i)
		}
		if !hasCodeTag {
			t.Errorf("Chunk %d missing auto tag 'code'", i)
		}
	}
}

func TestChunker_NilTagger(t *testing.T) {
	chunker := NewChunker(nil)
	
	text := "Test with no tagger"
	chunks := chunker.ChunkInput("nil-tagger", text, "user", []string{"base"})
	
	if len(chunks) != 1 {
		t.Fatalf("Expected 1 chunk, got %d", len(chunks))
	}
	
	// Should only have base tags, no auto tags
	if len(chunks[0].Tags) != 1 || chunks[0].Tags[0] != "base" {
		t.Errorf("Expected only base tag, got %v", chunks[0].Tags)
	}
}

func TestDeduplicateTags(t *testing.T) {
	tests := []struct {
		input    []string
		expected []string
	}{
		{
			input:    []string{"a", "b", "a", "c", "b"},
			expected: []string{"a", "b", "c"},
		},
		{
			input:    []string{"single"},
			expected: []string{"single"},
		},
		{
			input:    []string{},
			expected: []string{},
		},
		{
			input:    []string{"same", "same", "same"},
			expected: []string{"same"},
		},
	}
	
	for _, tt := range tests {
		result := deduplicateTags(tt.input)
		
		if len(result) != len(tt.expected) {
			t.Errorf("deduplicateTags(%v) = %v, want %v", tt.input, result, tt.expected)
			continue
		}
		
		for i := range result {
			if result[i] != tt.expected[i] {
				t.Errorf("deduplicateTags(%v) = %v, want %v", tt.input, result, tt.expected)
				break
			}
		}
	}
}

func TestChunker_SplitBySize(t *testing.T) {
	chunker := NewChunker(nil)
	
	// Create text with no paragraph breaks
	text := strings.Repeat("word ", 10000) // ~50000 chars, no double newlines
	
	parts := chunker.splitBySize(text, 1000) // Target 1000 tokens = 4000 chars
	
	if len(parts) <= 1 {
		t.Error("Large text with no boundaries should be split by size")
	}
	
	// Each part should be roughly the target size
	for i, part := range parts {
		tokens := EstimateTokens(part)
		if tokens > 2000 { // Allow some overage
			t.Errorf("Part %d has %d tokens, expected ~1000", i, tokens)
		}
	}
}
