package context

import (
	"sync"
	"testing"
	"time"
)

func TestStore_AddAndGet(t *testing.T) {
	store := NewStore()
	
	chunk := Chunk{
		ID:     "test-1",
		Text:   "Hello, world!",
		Tags:   []string{"greeting"},
		Source: "user",
	}
	
	err := store.Add(chunk)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	
	retrieved, ok := store.Get("test-1")
	if !ok {
		t.Fatal("Get failed: chunk not found")
	}
	
	if retrieved.Text != chunk.Text {
		t.Errorf("Text mismatch: got %q, want %q", retrieved.Text, chunk.Text)
	}
	
	if len(retrieved.Tags) != 1 || retrieved.Tags[0] != "greeting" {
		t.Errorf("Tags mismatch: got %v, want %v", retrieved.Tags, chunk.Tags)
	}
}

func TestStore_AddWithoutID(t *testing.T) {
	store := NewStore()
	
	chunk := Chunk{
		Text: "No ID",
	}
	
	err := store.Add(chunk)
	if err == nil {
		t.Fatal("Expected error when adding chunk without ID")
	}
}

func TestStore_AutoTokenEstimate(t *testing.T) {
	store := NewStore()
	
	chunk := Chunk{
		ID:   "test-tokens",
		Text: "This is a test with approximately twenty tokens in it",
	}
	
	store.Add(chunk)
	retrieved, _ := store.Get("test-tokens")
	
	if retrieved.Tokens == 0 {
		t.Error("Tokens should be estimated automatically")
	}
	
	// Rough check: should be around 13 tokens (52 chars / 4)
	if retrieved.Tokens < 10 || retrieved.Tokens > 20 {
		t.Errorf("Token estimate seems off: got %d", retrieved.Tokens)
	}
}

func TestStore_Delete(t *testing.T) {
	store := NewStore()
	
	chunk := Chunk{
		ID:   "delete-me",
		Text: "Test",
	}
	
	store.Add(chunk)
	
	deleted := store.Delete("delete-me")
	if !deleted {
		t.Error("Delete should return true for existing chunk")
	}
	
	_, ok := store.Get("delete-me")
	if ok {
		t.Error("Chunk should not exist after delete")
	}
	
	deleted = store.Delete("nonexistent")
	if deleted {
		t.Error("Delete should return false for nonexistent chunk")
	}
}

func TestStore_ListByTags(t *testing.T) {
	store := NewStore()
	
	chunks := []Chunk{
		{ID: "1", Text: "A", Tags: []string{"alpha", "beta"}},
		{ID: "2", Text: "B", Tags: []string{"beta", "gamma"}},
		{ID: "3", Text: "C", Tags: []string{"gamma", "delta"}},
		{ID: "4", Text: "D", Tags: []string{"epsilon"}},
	}
	
	for _, chunk := range chunks {
		store.Add(chunk)
	}
	
	// Test single tag match
	results := store.ListByTags([]string{"beta"})
	if len(results) != 2 {
		t.Errorf("Expected 2 chunks with 'beta' tag, got %d", len(results))
	}
	
	// Test multiple tags (OR match)
	results = store.ListByTags([]string{"alpha", "epsilon"})
	if len(results) != 2 {
		t.Errorf("Expected 2 chunks with 'alpha' or 'epsilon', got %d", len(results))
	}
	
	// Test no match
	results = store.ListByTags([]string{"nonexistent"})
	if len(results) != 0 {
		t.Errorf("Expected 0 chunks, got %d", len(results))
	}
}

func TestStore_Expiration(t *testing.T) {
	store := NewStore()
	
	past := time.Now().Add(-1 * time.Hour)
	future := time.Now().Add(1 * time.Hour)
	
	chunks := []Chunk{
		{ID: "expired", Text: "Old", ExpiresAt: &past},
		{ID: "valid", Text: "Current", ExpiresAt: &future},
		{ID: "permanent", Text: "Forever"},
	}
	
	for _, chunk := range chunks {
		store.Add(chunk)
	}
	
	// Expired chunk should not be retrievable
	_, ok := store.Get("expired")
	if ok {
		t.Error("Expired chunk should not be retrievable")
	}
	
	// Valid chunk should be retrievable
	_, ok = store.Get("valid")
	if !ok {
		t.Error("Valid chunk should be retrievable")
	}
	
	// Permanent chunk should be retrievable
	_, ok = store.Get("permanent")
	if !ok {
		t.Error("Permanent chunk should be retrievable")
	}
	
	// Count should exclude expired
	count := store.Count()
	if count != 2 {
		t.Errorf("Expected count of 2 (excluding expired), got %d", count)
	}
}

func TestStore_ThreadSafety(t *testing.T) {
	store := NewStore()
	var wg sync.WaitGroup
	
	// Concurrent writes
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			chunk := Chunk{
				ID:   string(rune('A' + id%26)),
				Text: "Concurrent",
			}
			store.Add(chunk)
		}(i)
	}
	
	// Concurrent reads
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			store.Get(string(rune('A' + id%26)))
		}(i)
	}
	
	wg.Wait()
	
	// Should not panic and should have some chunks
	count := store.Count()
	if count == 0 {
		t.Error("Expected some chunks after concurrent operations")
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		text   string
		expect int
	}{
		{"", 0},
		{"test", 2},        // 4 chars / 3 ≈ 2 tokens
		{"hello world", 4}, // 11 chars / 3 ≈ 4 tokens
		{"this is a longer sentence with more words", 14}, // 41 chars → (41+2)/3 = 14 tokens
	}
	
	for _, tt := range tests {
		got := EstimateTokens(tt.text)
		if got != tt.expect {
			t.Errorf("EstimateTokens(%q) = %d, want %d", tt.text, got, tt.expect)
		}
	}
}
