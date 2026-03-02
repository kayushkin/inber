package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kayushkin/inber/memory"
)

// TestAutoReferenceManagerIntegration tests that the AutoReferenceManager
// works correctly when integrated via engine hooks
func TestAutoReferenceManagerIntegration(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.go")
	testContent := `package main

import "fmt"

func main() {
	fmt.Println("Hello, world!")
}
`
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create memory store
	memStore, err := memory.OpenOrCreate(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	defer memStore.Close()

	// Create auto-reference manager (same config as Engine uses)
	config := memory.DefaultAutoReferenceConfig()
	config.MinFileSize = 10 // Low threshold for test
	mgr := memory.NewAutoReferenceManager(memStore, tmpDir, config)

	// Count initial memories
	beforeMems, err := memStore.ListRecent(100, 0.0)
	if err != nil {
		t.Fatal(err)
	}
	beforeCount := len(beforeMems)
	t.Logf("Before: %d memories", beforeCount)

	// Simulate a tool call flow (as Engine does)
	toolID := "test-tool-1"
	toolName := "read_file"
	toolInput := fmt.Sprintf(`{"path":"%s"}`, testFile)

	// Simulate successful tool call + result
	if err := mgr.OnToolResult(toolID, toolName, toolInput, testContent); err != nil {
		t.Fatalf("OnToolResult failed: %v", err)
	}
	t.Logf("OnToolResult succeeded")

	// Give it a moment to save asynchronously
	time.Sleep(200 * time.Millisecond)

	// Check if reference was created
	afterMems, err := memStore.ListRecent(100, 0.0)
	if err != nil {
		t.Fatal(err)
	}
	afterCount := len(afterMems)
	t.Logf("After: %d memories", afterCount)

	if afterCount <= beforeCount {
		t.Errorf("Expected new memory to be created, but count didn't increase (before: %d, after: %d)", beforeCount, afterCount)

		// Debug: print all memories
		for i, mem := range afterMems {
			t.Logf("  Memory %d: ID=%s, RefType=%s, IsLazy=%v, Tags=%v, Content=%q",
				i, mem.ID, mem.RefType, mem.IsLazy, mem.Tags, truncate(mem.Content, 50))
		}
		return
	}

	// Find the new reference
	var fileRef *memory.Memory
	for i := range afterMems {
		if afterMems[i].RefType == "file" && afterMems[i].IsLazy {
			fileRef = &afterMems[i]
			break
		}
	}

	if fileRef == nil {
		t.Error("Expected to find file reference, but didn't")
		for i, mem := range afterMems {
			t.Logf("  Memory %d: ID=%s, RefType=%s, IsLazy=%v, Tags=%v, Content=%q",
				i, mem.ID, mem.RefType, mem.IsLazy, mem.Tags, truncate(mem.Content, 50))
		}
		return
	}

	// Verify the reference metadata
	t.Logf("Found file reference: ID=%s, RefType=%s, RefTarget=%s, IsLazy=%v",
		fileRef.ID, fileRef.RefType, fileRef.RefTarget, fileRef.IsLazy)

	if fileRef.RefTarget != testFile {
		t.Errorf("Expected RefTarget=%s, got %s", testFile, fileRef.RefTarget)
	}

	if !fileRef.IsLazy {
		t.Error("Expected IsLazy=true")
	}

	// Check that content is a summary, not full file
	if fileRef.Content == testContent {
		t.Error("Expected summary content, but got full file content (lazy loading not working)")
	}

	if len(fileRef.Content) > 200 {
		t.Errorf("Expected short summary content, got %d bytes", len(fileRef.Content))
	}

	// Test lazy loading - Get() should automatically load the full content
	expanded, err := memStore.Get(fileRef.ID)
	if err != nil {
		t.Fatalf("Failed to get reference: %v", err)
	}

	t.Logf("Expanded content length: %d bytes", len(expanded.Content))

	if expanded.Content != testContent {
		t.Errorf("Expected expanded content to match file")
		t.Logf("Got:\n%s", expanded.Content)
		t.Logf("Want:\n%s", testContent)
	}

	t.Log("✓ Auto-reference creation and lazy loading working correctly")
}

// TestAutoReferenceSkipsSmallFiles verifies that small files don't create references
func TestAutoReferenceSkipsSmallFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a tiny test file
	testFile := filepath.Join(tmpDir, "tiny.txt")
	tinyContent := "hi"
	if err := os.WriteFile(testFile, []byte(tinyContent), 0644); err != nil {
		t.Fatal(err)
	}

	memStore, err := memory.OpenOrCreate(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	defer memStore.Close()

	// Create manager with default threshold (1000 bytes)
	config := memory.DefaultAutoReferenceConfig()
	mgr := memory.NewAutoReferenceManager(memStore, tmpDir, config)

	beforeMems, _ := memStore.ListRecent(100, 0.0)
	beforeCount := len(beforeMems)

	// Try to create reference for tiny file
	toolInput := fmt.Sprintf(`{"path":"%s"}`, testFile)
	if err := mgr.OnToolResult("test-1", "read_file", toolInput, tinyContent); err != nil {
		t.Fatalf("OnToolResult failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	afterMems, _ := memStore.ListRecent(100, 0.0)
	afterCount := len(afterMems)

	if afterCount != beforeCount {
		t.Errorf("Expected no new memory for tiny file (<%d bytes), but got %d new memories",
			config.MinFileSize, afterCount-beforeCount)
	}

	t.Log("✓ Small files correctly skipped")
}

// TestAutoReferenceRepoMap tests repo_map tool creates ephemeral reference
func TestAutoReferenceRepoMap(t *testing.T) {
	tmpDir := t.TempDir()

	memStore, err := memory.OpenOrCreate(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	defer memStore.Close()

	config := memory.DefaultAutoReferenceConfig()
	mgr := memory.NewAutoReferenceManager(memStore, tmpDir, config)

	beforeMems, _ := memStore.ListRecent(100, 0.0)
	beforeCount := len(beforeMems)

	// Simulate repo_map tool result
	repoMapOutput := `package main:
  - main.go
package foo:
  - foo.go
  - bar.go
`

	toolInput := `{"path":"","format":"compact"}`
	if err := mgr.OnToolResult("test-2", "repo_map", toolInput, repoMapOutput); err != nil {
		t.Fatalf("OnToolResult failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	afterMems, _ := memStore.ListRecent(100, 0.0)
	afterCount := len(afterMems)

	if afterCount <= beforeCount {
		t.Errorf("Expected repo-map reference to be created")
		return
	}

	// Find repo-map reference
	var repoRef *memory.Memory
	for i := range afterMems {
		if afterMems[i].RefType == "repo-map" {
			repoRef = &afterMems[i]
			break
		}
	}

	if repoRef == nil {
		t.Error("Expected to find repo-map reference")
		return
	}

	// Verify it has an expiration (ephemeral)
	if repoRef.ExpiresAt == nil {
		t.Error("Expected repo-map reference to have ExpiresAt set")
	} else {
		t.Logf("Repo-map reference expires at: %v", repoRef.ExpiresAt)
	}

	// Verify it's not lazy (content stored inline for repo maps)
	if repoRef.IsLazy {
		t.Error("Expected repo-map to not be lazy (content stored inline)")
	}

	t.Log("✓ Repo-map reference created with expiration")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
