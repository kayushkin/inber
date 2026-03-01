package memory

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLazyLoading(t *testing.T) {
	// Create temp database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	
	// Create test files
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "This is test file content from disk"
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	
	identityFile := filepath.Join(tmpDir, "identity.md")
	identityContent := "# Test Identity\n\nI am a test agent"
	if err := os.WriteFile(identityFile, []byte(identityContent), 0644); err != nil {
		t.Fatalf("WriteFile identity: %v", err)
	}
	
	tests := []struct {
		name        string
		memory      Memory
		wantContent string
		wantError   bool
	}{
		{
			name: "LazyLoadFile",
			memory: Memory{
				ID:        "lazy-file-1",
				Content:   "", // Empty in DB
				Summary:   "Test file reference",
				RefType:   "file",
				RefTarget: testFile,
				IsLazy:    true,
				Importance: 0.7,
			},
			wantContent: testContent,
			wantError:   false,
		},
		{
			name: "LazyLoadIdentity",
			memory: Memory{
				ID:        "lazy-identity-1",
				Content:   "", // Empty in DB
				Summary:   "Test identity",
				RefType:   "identity",
				RefTarget: identityFile,
				IsLazy:    true,
				Importance: 1.0,
			},
			wantContent: identityContent,
			wantError:   false,
		},
		{
			name: "NonLazyMemory",
			memory: Memory{
				ID:        "normal-memory-1",
				Content:   "This content is stored in DB",
				Summary:   "Normal memory",
				RefType:   "memory",
				IsLazy:    false,
				Importance: 0.5,
			},
			wantContent: "This content is stored in DB",
			wantError:   false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save memory
			if err := store.Save(tt.memory); err != nil {
				t.Fatalf("Save: %v", err)
			}
			
			// Retrieve memory (should trigger lazy loading)
			retrieved, err := store.Get(tt.memory.ID)
			if (err != nil) != tt.wantError {
				t.Fatalf("Get error = %v, wantError %v", err, tt.wantError)
			}
			
			if err == nil {
				if retrieved.Content != tt.wantContent {
					t.Errorf("Content = %q, want %q", retrieved.Content, tt.wantContent)
				}
				
				if retrieved.Tokens == 0 {
					t.Error("Tokens not computed after lazy load")
				}
			}
		})
	}
}

func TestLazyLoadingStaleData(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	
	// Create file with initial content
	testFile := filepath.Join(tmpDir, "changing.txt")
	initialContent := "Version 1"
	if err := os.WriteFile(testFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	
	// Save lazy-loaded reference
	mem := Memory{
		ID:         "changing-file",
		Content:    "", // Not stored in DB
		Summary:    "File that will change",
		RefType:    "file",
		RefTarget:  testFile,
		IsLazy:     true,
		Importance: 0.7,
	}
	if err := store.Save(mem); err != nil {
		t.Fatalf("Save: %v", err)
	}
	
	// First retrieval
	retrieved1, err := store.Get("changing-file")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if retrieved1.Content != initialContent {
		t.Errorf("First get: content = %q, want %q", retrieved1.Content, initialContent)
	}
	
	// Change file content on disk
	time.Sleep(10 * time.Millisecond) // Ensure different mtime
	newContent := "Version 2 - updated!"
	if err := os.WriteFile(testFile, []byte(newContent), 0644); err != nil {
		t.Fatalf("WriteFile update: %v", err)
	}
	
	// Second retrieval should get NEW content (not stale DB version)
	retrieved2, err := store.Get("changing-file")
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if retrieved2.Content != newContent {
		t.Errorf("After file change: content = %q, want %q", retrieved2.Content, newContent)
	}
}

func TestLazyLoadMissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()
	
	// Save reference to non-existent file
	mem := Memory{
		ID:         "missing-file",
		Summary:    "File that doesn't exist",
		RefType:    "file",
		RefTarget:  "/nonexistent/path.txt",
		IsLazy:     true,
		Importance: 0.5,
	}
	if err := store.Save(mem); err != nil {
		t.Fatalf("Save: %v", err)
	}
	
	// Retrieval should fail with clear error
	_, err = store.Get("missing-file")
	if err == nil {
		t.Fatal("Expected error for missing file, got nil")
	}
	t.Logf("Got expected error: %v", err)
}
