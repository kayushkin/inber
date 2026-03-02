package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAutoReferences(t *testing.T) {
	// Create temp directory for test
	tmpDir := t.TempDir()

	// Create test file
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
	store, err := OpenOrCreate(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Create auto-reference manager
	config := DefaultAutoReferenceConfig()
	config.MinFileSize = 10 // Small threshold for test
	mgr := NewAutoReferenceManager(store, tmpDir, config)

	t.Run("CreateFileReference", func(t *testing.T) {
		// Simulate read_file tool call
		inputJSON, _ := json.Marshal(map[string]string{
			"path": testFile,
		})

		t.Logf("Calling OnToolResult with input: %s", string(inputJSON))
		err := mgr.OnToolResult("test-1", "read_file", string(inputJSON), testContent)
		if err != nil {
			t.Fatalf("OnToolResult failed: %v", err)
		}
		t.Log("OnToolResult completed successfully")

		// Verify reference was created
		memories, err := store.ListRecent(10, 0.0)
		if err != nil {
			t.Fatal(err)
		}

		if len(memories) == 0 {
			t.Fatal("expected at least one memory, got none")
		}

		t.Logf("Found %d memories:", len(memories))
		for i, mem := range memories {
			t.Logf("  [%d] RefType=%s, IsLazy=%v, Tags=%v", i, mem.RefType, mem.IsLazy, mem.Tags)
		}

		found := false
		for _, mem := range memories {
			if mem.RefType == "file" {
				found = true
				// RefTarget should be absolute path now (for lazy loading)
				if mem.RefTarget != testFile {
					t.Errorf("expected RefTarget=%s, got %s", testFile, mem.RefTarget)
				}
				if !mem.IsLazy {
					t.Error("expected IsLazy=true")
				}
				if mem.Content != "" {
					t.Error("expected Content empty for lazy file")
				}
				if mem.Summary == "" {
					t.Error("expected non-empty Summary")
				}
				if mem.Importance != 0.4 {
					t.Errorf("expected Importance=0.4, got %f", mem.Importance)
				}
				// Verify tags
				hasFileTag := false
				for _, tag := range mem.Tags {
					if tag == "file" {
						hasFileTag = true
					}
				}
				if !hasFileTag {
					t.Error("expected 'file' tag")
				}
			}
		}

		if !found {
			t.Error("file reference not found in memories")
		}
	})

	t.Run("CreateRepoMapReference", func(t *testing.T) {
		repoMapOutput := `pkg main
  main.go
    func main()
    
pkg utils
  utils.go
    func Helper()
`

		err := mgr.OnToolResult("test-2", "repo_map", "", repoMapOutput)
		if err != nil {
			t.Fatalf("OnToolResult failed: %v", err)
		}

		// Verify reference was created
		memories, err := store.ListRecent(10, 0.0)
		if err != nil {
			t.Fatal(err)
		}

		found := false
		for _, mem := range memories {
			if mem.RefType == "repo-map" {
				found = true
				if mem.IsLazy {
					t.Error("expected IsLazy=false for repo-map")
				}
				if mem.Content == "" {
					t.Error("expected non-empty Content for repo-map")
				}
				if mem.ExpiresAt == nil {
					t.Error("expected ExpiresAt to be set")
				} else {
					if mem.ExpiresAt.Before(time.Now()) {
						t.Error("expected ExpiresAt in future")
					}
				}
			}
		}

		if !found {
			t.Error("repo-map reference not found in memories")
		}
	})

	t.Run("SkipSmallFiles", func(t *testing.T) {
		// Create tiny file
		tinyFile := filepath.Join(tmpDir, "tiny.txt")
		if err := os.WriteFile(tinyFile, []byte("hi"), 0644); err != nil {
			t.Fatal(err)
		}

		// Count current memories
		beforeMems, _ := store.ListRecent(100, 0.0)
		beforeCount := len(beforeMems)

		// Try to create reference for tiny file
		inputJSON, _ := json.Marshal(map[string]string{
			"path": tinyFile,
		})

		err := mgr.OnToolResult("test-3", "read_file", string(inputJSON), "hi")
		if err != nil {
			t.Fatalf("OnToolResult failed: %v", err)
		}

		// Verify no new reference created
		afterMems, _ := store.ListRecent(100, 0.0)
		afterCount := len(afterMems)

		if afterCount != beforeCount {
			t.Errorf("expected no new memory for tiny file, got %d new memories", afterCount-beforeCount)
		}
	})
}
