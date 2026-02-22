package context

import (
	"testing"
)

func TestBuilder_BasicBuild(t *testing.T) {
	store := NewStore()
	
	chunks := []Chunk{
		{ID: "1", Text: "Identity chunk", Tokens: 100, Tags: []string{"identity"}},
		{ID: "2", Text: "User message", Tokens: 50, Tags: []string{"user", "question"}},
		{ID: "3", Text: "Unrelated", Tokens: 50, Tags: []string{"other"}},
	}
	
	for _, chunk := range chunks {
		store.Add(chunk)
	}
	
	builder := NewBuilder(store, 1000)
	result := builder.Build([]string{"question"})
	
	// Should include identity and tag-matched chunk
	if len(result) < 2 {
		t.Errorf("Expected at least 2 chunks, got %d", len(result))
	}
	
	// Identity chunk should be included
	hasIdentity := false
	for _, chunk := range result {
		if chunk.ID == "1" {
			hasIdentity = true
		}
	}
	if !hasIdentity {
		t.Error("Identity chunk should always be included")
	}
}

func TestBuilder_BudgetRespected(t *testing.T) {
	store := NewStore()
	
	chunks := []Chunk{
		{ID: "1", Text: "A", Tokens: 100, Tags: []string{"match"}},
		{ID: "2", Text: "B", Tokens: 100, Tags: []string{"match"}},
		{ID: "3", Text: "C", Tokens: 100, Tags: []string{"match"}},
		{ID: "4", Text: "D", Tokens: 100, Tags: []string{"match"}},
	}
	
	for _, chunk := range chunks {
		store.Add(chunk)
	}
	
	// Budget only allows 2.5 chunks
	builder := NewBuilder(store, 250)
	result := builder.Build([]string{"match"})
	
	totalTokens := 0
	for _, chunk := range result {
		totalTokens += chunk.Tokens
	}
	
	if totalTokens > 250 {
		t.Errorf("Budget exceeded: used %d tokens, budget was 250", totalTokens)
	}
}

func TestBuilder_AlwaysInclude(t *testing.T) {
	store := NewStore()
	
	chunks := []Chunk{
		{ID: "identity", Text: "I am a bot", Tokens: 100, Tags: []string{"identity"}},
		{ID: "always", Text: "Important", Tokens: 100, Tags: []string{"always"}},
		{ID: "regular", Text: "Regular", Tokens: 100, Tags: []string{"other"}},
	}
	
	for _, chunk := range chunks {
		store.Add(chunk)
	}
	
	builder := NewBuilder(store, 1000)
	result := builder.Build([]string{"unrelated"})
	
	// Should include identity and always tags even with no match
	hasIdentity := false
	hasAlways := false
	
	for _, chunk := range result {
		if chunk.ID == "identity" {
			hasIdentity = true
		}
		if chunk.ID == "always" {
			hasAlways = true
		}
	}
	
	if !hasIdentity {
		t.Error("Identity chunk should always be included")
	}
	if !hasAlways {
		t.Error("Always chunk should always be included")
	}
}

func TestBuilder_SizeAwareFiltering(t *testing.T) {
	store := NewStore()
	
	chunks := []Chunk{
		// Small chunk, single tag match - should be included
		{ID: "small", Text: "S", Tokens: 200, Tags: []string{"test-tag"}},
		
		// Medium chunk, single tag match - should NOT be included (needs 2+ matches)
		{ID: "medium-single", Text: "M1", Tokens: 1000, Tags: []string{"test-tag"}},
		
		// Medium chunk, multiple tag matches - should be included
		{ID: "medium-multi", Text: "M2", Tokens: 1000, Tags: []string{"test-tag", "another-tag"}},
		
		// Large chunk, two tag matches - should NOT be included (needs 3+ matches)
		{ID: "large-two", Text: "L1", Tokens: 6000, Tags: []string{"test-tag", "another-tag"}},
		
		// Large chunk, three tag matches - should be included
		{ID: "large-three", Text: "L2", Tokens: 6000, Tags: []string{"test-tag", "another-tag", "third-tag"}},
	}
	
	for _, chunk := range chunks {
		store.Add(chunk)
	}
	
	builder := NewBuilder(store, 50000) // Large budget
	result := builder.Build([]string{"test-tag", "another-tag", "third-tag"})
	
	included := make(map[string]bool)
	for _, chunk := range result {
		included[chunk.ID] = true
	}
	
	if !included["small"] {
		t.Error("Small chunk with single tag match should be included")
	}
	
	if included["medium-single"] {
		t.Error("Medium chunk with single tag match should NOT be included")
	}
	
	if !included["medium-multi"] {
		t.Error("Medium chunk with multiple tag matches should be included")
	}
	
	if included["large-two"] {
		t.Error("Large chunk with only two tag matches should NOT be included")
	}
	
	if !included["large-three"] {
		t.Error("Large chunk with three tag matches should be included")
	}
}

func TestBuilder_TestFileExclusion(t *testing.T) {
	store := NewStore()
	
	chunks := []Chunk{
		{ID: "main.go", Text: "package main", Tokens: 100, Tags: []string{"file", "code", "go"}},
		{ID: "main_test.go", Text: "package main", Tokens: 100, Tags: []string{"file", "code", "go", "test"}},
		{ID: "helper_test.go", Text: "package main", Tokens: 100, Tags: []string{"file", "code", "go", "test"}},
	}
	
	for _, chunk := range chunks {
		store.Add(chunk)
	}
	
	builder := NewBuilder(store, 1000)
	
	// Without "test" tag - test files should be excluded
	result := builder.Build([]string{"code", "go"})
	
	hasTestFile := false
	for _, chunk := range result {
		if chunk.ID == "main_test.go" || chunk.ID == "helper_test.go" {
			hasTestFile = true
		}
	}
	
	if hasTestFile {
		t.Error("Test files should be excluded when 'test' tag not in message")
	}
	
	// With "test" tag - test files should be included
	result = builder.Build([]string{"test", "go"})
	
	hasTestFile = false
	for _, chunk := range result {
		if chunk.ID == "main_test.go" || chunk.ID == "helper_test.go" {
			hasTestFile = true
		}
	}
	
	if !hasTestFile {
		t.Error("Test files should be included when 'test' tag in message")
	}
}

func TestBuilder_ConversationRecency(t *testing.T) {
	store := NewStore()
	
	// Create conversation chunks with different timestamps
	old := Chunk{
		ID:     "old",
		Text:   "Old message",
		Tokens: 50,
		Tags:   []string{},
		Source: "user",
	}
	old.CreatedAt = old.CreatedAt.Add(-2 * 60 * 60) // 2 hours ago
	
	recent := Chunk{
		ID:     "recent",
		Text:   "Recent message",
		Tokens: 50,
		Tags:   []string{},
		Source: "user",
	}
	recent.CreatedAt = recent.CreatedAt.Add(-5 * 60) // 5 minutes ago
	
	store.Add(old)
	store.Add(recent)
	
	builder := NewBuilder(store, 100) // Budget for 2 chunks max
	result := builder.Build([]string{})
	
	// Recent message should be prioritized
	hasRecent := false
	for _, chunk := range result {
		if chunk.ID == "recent" {
			hasRecent = true
		}
	}
	
	if !hasRecent {
		t.Error("Recent conversation should be prioritized")
	}
}

func TestBuilder_EmptyStore(t *testing.T) {
	store := NewStore()
	builder := NewBuilder(store, 1000)
	
	result := builder.Build([]string{"anything"})
	
	if len(result) != 0 {
		t.Errorf("Expected empty result from empty store, got %d chunks", len(result))
	}
}
