package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Test smart tagger with import detection
func TestSmartTagger_ImportDetection(t *testing.T) {
	tagger := NewSmartTagger()
	
	code := `package main

import (
	"context"
	"fmt"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
)

func main() {
	client := anthropic.NewClient()
	a := agent.NewAgent("test")
}`
	
	tags := tagger.Tag(code, "file")
	
	// Should detect imports
	hasImportTag := false
	hasPkgTag := false
	for _, tag := range tags {
		if strings.HasPrefix(tag, "import:") {
			hasImportTag = true
		}
		if tag == "pkg:anthropic-sdk-go" || tag == "pkg:agent" {
			hasPkgTag = true
		}
	}
	
	if !hasImportTag {
		t.Error("Expected import tags to be detected")
	}
	
	if !hasPkgTag {
		t.Error("Expected package tags to be extracted")
	}
	
	t.Logf("Tags: %v", tags)
}

// Test smart tagger with function call detection
func TestSmartTagger_FunctionCallDetection(t *testing.T) {
	tagger := NewSmartTagger()
	
	code := `
	client := anthropic.NewClient()
	result := agent.Run(ctx, model)
	fmt.Println("done")
`
	
	tags := tagger.Tag(code, "code")
	
	// Should detect function calls (non-stdlib)
	hasCallTag := false
	for _, tag := range tags {
		if strings.HasPrefix(tag, "call:") && !strings.HasPrefix(tag, "call:fmt") {
			hasCallTag = true
			break
		}
	}
	
	if !hasCallTag {
		t.Error("Expected function call tags to be detected")
	}
	
	t.Logf("Tags: %v", tags)
}

// Test file importance scoring
func TestImportanceScorer_BasicScoring(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create test files with different characteristics
	
	// 1. Small, recently modified file
	small := filepath.Join(tmpDir, "small.go")
	os.WriteFile(small, []byte("package test\nfunc Small() {}"), 0644)
	
	// 2. Large file
	large := filepath.Join(tmpDir, "large.go")
	largeContent := strings.Repeat("// comment\n", 1000)
	os.WriteFile(large, []byte(largeContent), 0644)
	
	scorer := NewImportanceScorer(tmpDir, 24*time.Hour)
	
	// Score small file
	smallScore, err := scorer.ScoreFile(small)
	if err != nil {
		t.Fatal(err)
	}
	
	// Score large file
	largeScore, err := scorer.ScoreFile(large)
	if err != nil {
		t.Fatal(err)
	}
	
	// Small file should have better size score
	if smallScore.SizeScore <= largeScore.SizeScore {
		t.Errorf("Expected small file to have better size score: %.2f vs %.2f",
			smallScore.SizeScore, largeScore.SizeScore)
	}
	
	// Both should have similar recency (just created)
	if smallScore.RecencyScore < 0.9 || largeScore.RecencyScore < 0.9 {
		t.Errorf("Expected high recency scores for just-created files")
	}
	
	t.Logf("Small file score: %.2f (recency: %.2f, size: %.2f)",
		smallScore.Score, smallScore.RecencyScore, smallScore.SizeScore)
	t.Logf("Large file score: %.2f (recency: %.2f, size: %.2f)",
		largeScore.Score, largeScore.RecencyScore, largeScore.SizeScore)
}

// Test duplicate detection
func TestDeduplicateChunks(t *testing.T) {
	// Short duplicates (< 100 chars) are NOT deduplicated by content
	shortChunks := []Chunk{
		{ID: "1", Text: "Hello world", Tags: []string{"test"}},
		{ID: "2", Text: "Hello world", Tags: []string{"test"}}, // Same content, different ID
		{ID: "3", Text: "Different content", Tags: []string{"test"}},
	}
	
	dedupShort := deduplicateChunks(shortChunks)
	
	// Short chunks should all be kept (IDs are different)
	if len(dedupShort) != len(shortChunks) {
		t.Errorf("Short chunks should not be deduplicated: %d -> %d",
			len(shortChunks), len(dedupShort))
	}
	
	// Long duplicates (> 100 chars) ARE deduplicated by content
	longText1 := strings.Repeat("This is a long piece of text that will be deduplicated because it's over 100 characters. ", 3)
	longText2 := "Different long text " + strings.Repeat("x", 100)
	
	longChunks := []Chunk{
		{ID: "1", Text: longText1, Tags: []string{"test"}},
		{ID: "2", Text: longText1, Tags: []string{"test"}}, // Duplicate content
		{ID: "3", Text: longText2, Tags: []string{"test"}},
	}
	
	dedupLong := deduplicateChunks(longChunks)
	
	// Should remove content duplicate
	if len(dedupLong) >= len(longChunks) {
		t.Errorf("Expected long duplicates to be removed: %d -> %d",
			len(longChunks), len(dedupLong))
	}
	
	// ID duplicates are always removed
	idDuplicates := []Chunk{
		{ID: "same", Text: "First", Tags: []string{"test"}},
		{ID: "same", Text: "Second", Tags: []string{"test"}}, // Same ID, different content
		{ID: "different", Text: "Third", Tags: []string{"test"}},
	}
	
	dedupID := deduplicateChunks(idDuplicates)
	
	// Should remove ID duplicate
	if len(dedupID) != 2 {
		t.Errorf("Expected ID duplicates to be removed: got %d chunks", len(dedupID))
	}
	
	t.Logf("Deduplication: short=%d->%d, long=%d->%d, ID=%d->%d",
		len(shortChunks), len(dedupShort),
		len(longChunks), len(dedupLong),
		len(idDuplicates), len(dedupID))
}

// Test smart filtering based on size and tag match
func TestBuilder_SmartFiltering(t *testing.T) {
	store := NewStore()
	
	// Add chunks with different sizes and tag matches
	
	// Small chunk with 1 matching tag - should be included
	store.Add(Chunk{
		ID:     "small-1tag",
		Text:   "Small content",
		Tokens: 100,
		Tags:   []string{"agent"},
	})
	
	// Large chunk with 1 matching tag - should be excluded
	store.Add(Chunk{
		ID:     "large-1tag",
		Text:   strings.Repeat("Large content ", 500),
		Tokens: 6000,
		Tags:   []string{"agent"},
	})
	
	// Large chunk with 3 matching tags - should be included
	store.Add(Chunk{
		ID:     "large-3tags",
		Text:   strings.Repeat("Important content ", 500),
		Tokens: 6000,
		Tags:   []string{"agent", "important", "core"},
	})
	
	builder := NewBuilder(store, 10000)
	result := builder.Build([]string{"agent", "important", "core"})
	
	// Check which chunks were included
	hasSmall := false
	hasLarge1Tag := false
	hasLarge3Tags := false
	
	for _, chunk := range result {
		switch chunk.ID {
		case "small-1tag":
			hasSmall = true
		case "large-1tag":
			hasLarge1Tag = true
		case "large-3tags":
			hasLarge3Tags = true
		}
	}
	
	if !hasSmall {
		t.Error("Expected small chunk with 1 tag to be included")
	}
	
	if hasLarge1Tag {
		t.Error("Expected large chunk with only 1 tag to be excluded")
	}
	
	if !hasLarge3Tags {
		t.Error("Expected large chunk with 3 tags to be included")
	}
	
	t.Logf("Smart filtering test passed: small=%v, large-1tag=%v, large-3tags=%v",
		hasSmall, hasLarge1Tag, hasLarge3Tags)
}

// Test that importance scoring integrates with stub loading
func TestLoadRecentWithImportance(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewStore()
	
	// Create some test files
	for i := 1; i <= 3; i++ {
		filename := filepath.Join(tmpDir, "file"+string(rune('0'+i))+".go")
		content := "package test\n"
		if i == 1 {
			// File 1: small and simple
			content += "func Test1() {}"
		} else if i == 2 {
			// File 2: medium
			content += strings.Repeat("func Test() {}\n", 10)
		} else {
			// File 3: large
			content += strings.Repeat("func Test() {}\n", 100)
		}
		os.WriteFile(filename, []byte(content), 0644)
	}
	
	// Load with importance scoring
	err := LoadRecentlyModifiedAsStubs(store, tmpDir, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	
	chunks := store.ListAll()
	if len(chunks) == 0 {
		t.Fatal("Expected some chunks to be loaded")
	}
	
	// Verify stubs contain importance indicators (⭐)
	hasImportanceMarker := false
	for _, chunk := range chunks {
		if strings.Contains(chunk.Text, "⭐") {
			hasImportanceMarker = true
		}
		t.Logf("Loaded stub: %s", chunk.Text)
	}
	
	if !hasImportanceMarker {
		t.Error("Expected importance markers in stub text")
	}
}

// Benchmark smart tagger vs basic tagger
func BenchmarkSmartTagger(b *testing.B) {
	code := `package main

import (
	"context"
	"fmt"
	"github.com/anthropics/anthropic-sdk-go"
)

func main() {
	client := anthropic.NewClient()
	ctx := context.Background()
	fmt.Println(ctx)
}
`
	
	b.Run("SmartTagger", func(b *testing.B) {
		tagger := NewSmartTagger()
		for i := 0; i < b.N; i++ {
			tagger.Tag(code, "file")
		}
	})
	
	b.Run("BasicTagger", func(b *testing.B) {
		tagger := NewPatternTagger()
		for i := 0; i < b.N; i++ {
			tagger.Tag(code, "file")
		}
	})
}
