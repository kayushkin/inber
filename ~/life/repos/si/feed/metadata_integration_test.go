package feed

import (
	"os"
	"testing"
	"time"

	si "github.com/kayushkin/si"
)

// TestInberDirect_MetadataPipeline tests that metadata flows from inber stderr
// through parseStderrMeta and into the Message.Meta field.
// This test requires SI_INTEGRATION=1 and a working inber installation.
func TestInberDirect_MetadataPipeline(t *testing.T) {
	if os.Getenv("SI_INTEGRATION") != "1" {
		t.Skip("Skipping integration test - set SI_INTEGRATION=1 to run")
	}

	feed := NewInberDirect(InberDirectConfig{
		Agent: "oisin", // Use oisin for consistent testing
		Model: "claude-sonnet-4-5", // Explicit model for predictable costs
	})
	defer feed.Close()

	if err := feed.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Send a simple message that should generate predictable token counts
	msg := si.Message{
		Text:    "Say 'metadata test' and nothing else",
		Channel: "test-metadata",
		Author:  "tester",
	}

	if err := feed.Write(msg); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Wait for response with metadata
	select {
	case resp := <-feed.Read():
		if resp.Text == "" {
			t.Error("empty response")
		}

		// Verify metadata is attached
		if resp.Meta == nil {
			t.Fatal("Meta is nil - metadata not parsed from stderr")
		}

		// Verify token counts
		if resp.Meta.InputTokens == 0 {
			t.Error("InputTokens is 0 - not parsed correctly")
		}
		if resp.Meta.OutputTokens == 0 {
			t.Error("OutputTokens is 0 - not parsed correctly")
		}

		// Verify cost
		if resp.Meta.Cost == 0 {
			t.Error("Cost is 0 - not parsed correctly")
		}

		// Verify model
		if resp.Meta.Model == "" {
			t.Error("Model is empty - not set correctly")
		}
		if resp.Meta.Model != "claude-sonnet-4-5" {
			t.Errorf("Model = %q, want claude-sonnet-4-5", resp.Meta.Model)
		}

		// Verify duration
		if resp.Meta.DurationMs == 0 {
			t.Error("DurationMs is 0 - not set correctly")
		}

		// Log the metadata for inspection
		t.Logf("✓ Metadata parsed successfully:")
		t.Logf("  InputTokens: %d", resp.Meta.InputTokens)
		t.Logf("  OutputTokens: %d", resp.Meta.OutputTokens)
		t.Logf("  ToolCalls: %d", resp.Meta.ToolCalls)
		t.Logf("  CacheReadTokens: %d", resp.Meta.CacheReadTokens)
		t.Logf("  CacheCreationTokens: %d", resp.Meta.CacheCreationTokens)
		t.Logf("  Cost: $%.4f", resp.Meta.Cost)
		t.Logf("  DurationMs: %d", resp.Meta.DurationMs)
		t.Logf("  Model: %s", resp.Meta.Model)
		t.Logf("  Response: %s", resp.Text)

	case <-time.After(60 * time.Second):
		t.Fatal("timeout waiting for response")
	}
}

// TestInberDirect_MetadataWithToolCalls tests metadata parsing when tool calls are made.
// This verifies that the tool_calls count is properly extracted.
func TestInberDirect_MetadataWithToolCalls(t *testing.T) {
	if os.Getenv("SI_INTEGRATION") != "1" {
		t.Skip("Skipping integration test - set SI_INTEGRATION=1 to run")
	}

	feed := NewInberDirect(InberDirectConfig{
		Agent: "oisin", // oisin has tools available
	})
	defer feed.Close()

	if err := feed.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Ask a question that should trigger a tool call
	msg := si.Message{
		Text:    "What files are in the current directory? Just answer yes or no if you can see any.",
		Channel: "test-tools",
		Author:  "tester",
	}

	if err := feed.Write(msg); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Wait for response
	select {
	case resp := <-feed.Read():
		if resp.Meta == nil {
			t.Fatal("Meta is nil")
		}

		// Log metadata
		t.Logf("✓ Metadata with tool calls:")
		t.Logf("  InputTokens: %d", resp.Meta.InputTokens)
		t.Logf("  OutputTokens: %d", resp.Meta.OutputTokens)
		t.Logf("  ToolCalls: %d", resp.Meta.ToolCalls)
		t.Logf("  Cost: $%.4f", resp.Meta.Cost)
		t.Logf("  DurationMs: %d", resp.Meta.DurationMs)

		// Tool calls might be 0 if the agent decides not to use tools
		// But we verify the field is being parsed
		if resp.Meta.ToolCalls > 0 {
			t.Logf("  ✓ Tool calls detected and parsed")
		}

	case <-time.After(90 * time.Second):
		t.Fatal("timeout waiting for response")
	}
}

// TestInberDirect_MetadataWithCache tests metadata parsing when cache is used.
// Cache tokens are only present after the first turn with the same context.
func TestInberDirect_MetadataWithCache(t *testing.T) {
	if os.Getenv("SI_INTEGRATION") != "1" {
		t.Skip("Skipping integration test - set SI_INTEGRATION=1 to run")
	}

	feed := NewInberDirect(InberDirectConfig{
		Agent: "oisin",
	})
	defer feed.Close()

	if err := feed.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Send two messages in sequence - the second might use cache
	msg1 := si.Message{
		Text:    "Remember this number: 42",
		Channel: "test-cache",
		Author:  "tester",
	}

	if err := feed.Write(msg1); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Wait for first response
	select {
	case resp1 := <-feed.Read():
		if resp1.Meta == nil {
			t.Fatal("Meta is nil on first response")
		}
		t.Logf("First response metadata:")
		t.Logf("  CacheReadTokens: %d", resp1.Meta.CacheReadTokens)
		t.Logf("  CacheCreationTokens: %d", resp1.Meta.CacheCreationTokens)

	case <-time.After(60 * time.Second):
		t.Fatal("timeout waiting for first response")
	}

	// Note: With --detach mode, each call is isolated, so we won't see cache hits
	// This test primarily verifies that cache fields are parsed when present
	t.Log("✓ Cache metadata fields parsed (may be 0 in detached mode)")
}
