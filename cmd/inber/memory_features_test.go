package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kayushkin/inber/memory"
)

func TestDetectContentType(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected ContentType
	}{
		{
			name:     "error dump",
			content:  "Error: something went wrong\nTraceback:\n  at file.go:123",
			expected: ContentTypeErrorDump,
		},
		{
			name:     "code blocks",
			content:  "```go\nfunc main() {}\n```\n\n```python\nprint('hello')\n```",
			expected: ContentTypeCodeBlock,
		},
		{
			name:     "log output",
			content:  "2024-01-15 12:34:56 [INFO] Server started\n2024-01-15 12:35:00 [DEBUG] Request received",
			expected: ContentTypeLogOutput,
		},
		{
			name:     "file contents",
			content:  "/path/to/file.go:123: syntax error\n/another/file.go:45: undefined",
			expected: ContentTypeFileContent,
		},
		{
			name:     "generic text",
			content:  "This is just some normal text without any special markers",
			expected: ContentTypeLargeText,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectContentType(tt.content)
			if result != tt.expected {
				t.Errorf("DetectContentType() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestStashLargeContent(t *testing.T) {
	// Create temporary directory for test DB
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_memory.db")

	// Create memory store
	memStore, err := memory.NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create memory store: %v", err)
	}
	defer memStore.Close()

	// Create large content (>1000 tokens)
	largeContent := strings.Repeat("This is a line of text with some meaningful content. ", 100)

	cfg := DefaultStashConfig()
	result, err := StashLargeContent(largeContent, "test-session", memStore, cfg)
	if err != nil {
		t.Fatalf("StashLargeContent failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result for large content")
	}

	// Verify memory was saved
	if result.MemoryID == "" {
		t.Error("Expected non-empty memory ID")
	}

	if result.Tokens < cfg.MinBlockSize {
		t.Errorf("Expected tokens >= %d, got %d", cfg.MinBlockSize, result.Tokens)
	}

	// Verify we can retrieve it
	mem, err := memStore.Get(result.MemoryID)
	if err != nil {
		t.Fatalf("failed to retrieve stashed memory: %v", err)
	}

	if mem.Content != largeContent {
		t.Error("Retrieved content doesn't match original")
	}

	// Verify tags
	expectedTags := []string{"large-input", "stashed", "test-session", string(result.ContentType)}
	for _, expectedTag := range expectedTags {
		found := false
		for _, tag := range mem.Tags {
			if tag == expectedTag {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected tag %q not found in %v", expectedTag, mem.Tags)
		}
	}
}

func TestDetectAndStashLargeBlocks(t *testing.T) {
	// Create temporary directory for test DB
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_memory.db")

	// Create memory store
	memStore, err := memory.NewStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create memory store: %v", err)
	}
	defer memStore.Close()

	// Create text with a large code block
	largeCode := strings.Repeat("func example() {\n    // do something\n}\n", 50)
	input := "Here's the code:\n```go\n" + largeCode + "```\nWhat do you think?"

	cfg := DefaultStashConfig()
	cfg.MinBlockSize = 100 // Lower threshold for testing

	modified, stashed, err := DetectAndStashLargeBlocks(input, "test-session", memStore, cfg)
	if err != nil {
		t.Fatalf("DetectAndStashLargeBlocks failed: %v", err)
	}

	if len(stashed) == 0 {
		t.Fatal("Expected at least one stashed block")
	}

	// Verify the modified text contains the summary
	if !strings.Contains(modified, "Large content stashed") {
		t.Error("Modified text should contain stash summary")
	}

	// Verify the original code block was replaced
	if strings.Contains(modified, largeCode) {
		t.Error("Modified text should not contain the original large code block")
	}

	// Verify we can retrieve the stashed content
	mem, err := memStore.Get(stashed[0].MemoryID)
	if err != nil {
		t.Fatalf("failed to retrieve stashed content: %v", err)
	}

	if !strings.Contains(mem.Content, "func example()") {
		t.Error("Stashed content should contain the code")
	}
}

func TestCalculateSimilarity(t *testing.T) {
	tests := []struct {
		name      string
		a         string
		b         string
		minScore  float64
		maxScore  float64
	}{
		{
			name:     "identical",
			a:        "the quick brown fox",
			b:        "the quick brown fox",
			minScore: 0.9,
			maxScore: 1.0,
		},
		{
			name:     "similar",
			a:        "user prefers golang for backend",
			b:        "user prefers using golang for backend services",
			minScore: 0.5,
			maxScore: 0.9,
		},
		{
			name:     "different",
			a:        "completely different content here",
			b:        "totally unrelated text over there",
			minScore: 0.0,
			maxScore: 0.3,
		},
		{
			name:     "empty",
			a:        "",
			b:        "something",
			minScore: 0.0,
			maxScore: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := calculateSimilarity(tt.a, tt.b)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("calculateSimilarity() = %v, want between %v and %v", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestExtractionConfig(t *testing.T) {
	cfg := DefaultExtractionConfig()

	if !cfg.Enabled {
		t.Error("Expected extraction to be enabled by default")
	}

	if cfg.MinExchangeTokens <= 0 {
		t.Error("Expected positive MinExchangeTokens")
	}

	if cfg.DuplicateThreshold <= 0 || cfg.DuplicateThreshold > 1 {
		t.Error("Expected DuplicateThreshold to be between 0 and 1")
	}
}

func TestStashConfig(t *testing.T) {
	cfg := DefaultStashConfig()

	if !cfg.Enabled {
		t.Error("Expected stashing to be enabled by default")
	}

	if cfg.UserMessageThreshold <= 0 {
		t.Error("Expected positive UserMessageThreshold")
	}

	if cfg.AssistantThreshold <= 0 {
		t.Error("Expected positive AssistantThreshold")
	}

	if cfg.DefaultImportance <= 0 || cfg.DefaultImportance > 1 {
		t.Error("Expected DefaultImportance to be between 0 and 1")
	}
}

// TestBackgroundExtractMemories_Integration tests the full extraction flow
// This test requires an API key and makes real API calls, so it's skipped by default
func TestBackgroundExtractMemories_Integration(t *testing.T) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("Skipping integration test - no ANTHROPIC_API_KEY")
	}

	// This is a placeholder for integration testing
	// In real testing, you'd:
	// 1. Create a test anthropic.Client
	// 2. Create a test memory store
	// 3. Call BackgroundExtractMemories with test data
	// 4. Verify memories were extracted and saved
	
	// For now, just verify the function exists and has the right signature
	t.Log("BackgroundExtractMemories integration test placeholder")
}
