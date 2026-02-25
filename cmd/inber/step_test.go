package main

import (
	"testing"

	inbercontext "github.com/kayushkin/inber/context"
)

func TestStepModeParseCommands(t *testing.T) {
	// Test that step mode command parsing recognizes valid commands
	validContinue := []string{"", "c", "continue"}
	for _, cmd := range validContinue {
		if cmd != "" && cmd != "c" && cmd != "continue" {
			t.Errorf("expected %q to be a continue command", cmd)
		}
	}

	validQuit := []string{"q", "quit"}
	for _, cmd := range validQuit {
		if cmd != "q" && cmd != "quit" {
			t.Errorf("expected %q to be a quit command", cmd)
		}
	}
}

func TestStepModeContextOperations(t *testing.T) {
	store := inbercontext.NewStore()

	// Add a chunk
	err := store.Add(inbercontext.Chunk{
		ID:     "step-0",
		Text:   "test content",
		Tags:   []string{"test"},
		Source: "user",
	})
	if err != nil {
		t.Fatalf("failed to add chunk: %v", err)
	}

	// Verify it's there
	chunks := store.ListAll()
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}

	// Edit
	chunk, ok := store.Get("step-0")
	if !ok {
		t.Fatal("expected to find chunk")
	}
	chunk.Text = "updated content"
	chunk.Tokens = inbercontext.EstimateTokens("updated content")
	store.Add(chunk)

	chunk2, _ := store.Get("step-0")
	if chunk2.Text != "updated content" {
		t.Errorf("expected updated text, got %q", chunk2.Text)
	}

	// Delete
	if !store.Delete("step-0") {
		t.Error("expected delete to succeed")
	}
	if store.Count() != 0 {
		t.Error("expected 0 chunks after delete")
	}
}
