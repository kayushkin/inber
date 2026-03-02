package memory

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestReferenceExpansionFlow tests the end-to-end flow of reference expansion
// via the memory_expand tool with different reference types.
func TestReferenceExpansionFlow(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	// Create the memory_expand tool
	expandTool := ExpandTool(store)

	tests := []struct {
		name           string
		setupMemory    func() Memory
		setupFile      func() string // Returns file path if needed
		wantContentLen int           // Minimum expected content length
		wantError      bool
	}{
		{
			name: "ExpandFileReference",
			setupFile: func() string {
				path := filepath.Join(tmpDir, "code.go")
				content := `package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
}
`
				os.WriteFile(path, []byte(content), 0644)
				return path
			},
			setupMemory: func() Memory {
				return Memory{
					ID:         "file-ref-1",
					Content:    "", // Empty - lazy loaded
					Summary:    "Code file reference",
					Tags:       []string{"file", "code", ".go"},
					RefType:    "file",
					RefTarget:  "", // Will be set by setupFile
					IsLazy:     true,
					Importance: 0.7,
				}
			},
			wantContentLen: 50,
			wantError:      false,
		},
		{
			name: "ExpandConversationReference",
			setupMemory: func() Memory {
				fullConversation := `[User]
What is 2 + 2?

[Assistant]
2 + 2 equals 4.

[User]
Thanks!

[Assistant]
You're welcome!
`
				return Memory{
					ID:         "conversation-abc123",
					Content:    fullConversation,
					Summary:    "Full conversation (3 turns)",
					Tags:       []string{"conversation", "history"},
					RefType:    "memory", // Default type
					IsLazy:     true,
					Importance: 0.4,
					Source:     "summarization",
				}
			},
			wantContentLen: 50,
			wantError:      false,
		},
		{
			name: "ExpandIdentityReference",
			setupFile: func() string {
				path := filepath.Join(tmpDir, "identity.md")
				content := `# Agent Identity

I am Claxon, an AI coding assistant.

## Core Values
- Write clean, tested code
- Explain complex concepts clearly
- Help developers learn and grow
`
				os.WriteFile(path, []byte(content), 0644)
				return path
			},
			setupMemory: func() Memory {
				return Memory{
					ID:         "identity-config",
					Content:    "", // Empty - lazy loaded
					Summary:    "Agent identity configuration",
					Tags:       []string{"identity", "config", "always-load"},
					RefType:    "identity",
					RefTarget:  "", // Will be set by setupFile
					IsLazy:     true,
					AlwaysLoad: true,
					Importance: 1.0,
				}
			},
			wantContentLen: 100,
			wantError:      false,
		},
		{
			name: "ExpandNonLazyMemory",
			setupMemory: func() Memory {
				return Memory{
					ID:         "normal-memory",
					Content:    "This is a normal memory stored directly in the database",
					Summary:    "Normal memory (not lazy)",
					Tags:       []string{"test", "normal"},
					RefType:    "memory",
					IsLazy:     false,
					Importance: 0.5,
				}
			},
			wantContentLen: 20,
			wantError:      false,
		},
		{
			name: "ExpandMissingFile",
			setupMemory: func() Memory {
				return Memory{
					ID:         "missing-file-ref",
					Content:    "",
					Summary:    "Reference to non-existent file",
					Tags:       []string{"file", "missing"},
					RefType:    "file",
					RefTarget:  "/nonexistent/path/to/file.txt",
					IsLazy:     true,
					Importance: 0.5,
				}
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup file if needed
			var filePath string
			if tt.setupFile != nil {
				filePath = tt.setupFile()
			}

			// Setup memory
			mem := tt.setupMemory()
			if filePath != "" {
				mem.RefTarget = filePath
			}

			// Save to database
			if err := store.Save(mem); err != nil {
				t.Fatalf("Save: %v", err)
			}

			// Expand via tool
			input := map[string]string{"id": mem.ID}
			inputJSON, _ := json.Marshal(input)

			result, err := expandTool.Run(context.Background(), string(inputJSON))

			// Check error expectation
			if tt.wantError {
				if err == nil && !contains(result, "error:") {
					t.Fatalf("Expected error, got success: %s", result)
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// Verify content length
			if len(result) < tt.wantContentLen {
				t.Errorf("Content too short: got %d chars, want >= %d", len(result), tt.wantContentLen)
			}

			// Verify the result contains the memory ID
			if !contains(result, mem.ID) {
				t.Errorf("Result doesn't contain memory ID %s", mem.ID)
			}

			// Verify it contains actual content (not just metadata)
			if !contains(result, "Content:") {
				t.Error("Result missing 'Content:' section")
			}
		})
	}
}

// TestReferenceTypeCoherence verifies that all reference types are handled consistently
func TestReferenceTypeCoherence(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	// Test that all documented reference types can be saved and retrieved
	refTypes := []struct {
		refType    string
		needsFile  bool
		shouldLoad bool // Should loadLazyContent succeed?
	}{
		{"memory", false, true},      // Default type, content in DB
		{"file", true, true},          // File reference, loads from disk
		{"identity", true, true},      // Identity file, loads from disk
		{"repo-map", false, false},    // Generated on-demand, doesn't load
		{"tools", false, false},       // Generated on-demand, doesn't load
		{"web", false, false},         // Not yet implemented
		{"conversation", false, true}, // Alias for memory type
	}

	for _, tt := range refTypes {
		t.Run(tt.refType, func(t *testing.T) {
			var refTarget string
			if tt.needsFile {
				path := filepath.Join(tmpDir, tt.refType+".txt")
				os.WriteFile(path, []byte("test content for "+tt.refType), 0644)
				refTarget = path
			}

			mem := Memory{
				ID:         "ref-" + tt.refType,
				Content:    "stored content",
				Summary:    "Test " + tt.refType + " reference",
				Tags:       []string{"test", tt.refType},
				RefType:    tt.refType,
				RefTarget:  refTarget,
				IsLazy:     tt.needsFile, // Only lazy if we need to load from file
				Importance: 0.5,
			}

			// Should be able to save any reference type
			if err := store.Save(mem); err != nil {
				t.Fatalf("Failed to save %s reference: %v", tt.refType, err)
			}

			// Should be able to retrieve it
			retrieved, err := store.Get(mem.ID)
			if err != nil {
				t.Fatalf("Failed to retrieve %s reference: %v", tt.refType, err)
			}

			if retrieved.RefType != tt.refType {
				t.Errorf("RefType mismatch: got %s, want %s", retrieved.RefType, tt.refType)
			}

			if retrieved.RefTarget != refTarget {
				t.Errorf("RefTarget mismatch: got %s, want %s", retrieved.RefTarget, refTarget)
			}

			// Verify lazy loading behavior
			if tt.needsFile && retrieved.IsLazy {
				// Content should be loaded from file
				if len(retrieved.Content) == 0 {
					t.Error("Lazy file content not loaded")
				}
			}
		})
	}
}

// TestStashedContentReferences tests references to stashed content (e.g., from clipboard, selections)
func TestStashedContentReferences(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	// Stashed content is stored directly in the database, like conversations
	stashedCode := `func calculateTotal(items []Item) float64 {
	var total float64
	for _, item := range items {
		total += item.Price * float64(item.Quantity)
	}
	return total
}
`

	stash := Memory{
		ID:         "stash-clipboard-abc",
		Content:    stashedCode,
		Summary:    "Stashed code snippet from clipboard",
		Tags:       []string{"stash", "clipboard", "code", "go"},
		RefType:    "stash",
		IsLazy:     false, // Content stored in DB
		Importance: 0.6,
		Source:     "user",
	}

	if err := store.Save(stash); err != nil {
		t.Fatalf("Save stashed content: %v", err)
	}

	// Retrieve and verify
	retrieved, err := store.Get(stash.ID)
	if err != nil {
		t.Fatalf("Get stashed content: %v", err)
	}

	if retrieved.Content != stashedCode {
		t.Error("Stashed content not preserved correctly")
	}

	// Verify it can be expanded via the tool
	expandTool := ExpandTool(store)
	input := map[string]string{"id": stash.ID}
	inputJSON, _ := json.Marshal(input)

	result, err := expandTool.Run(context.Background(), string(inputJSON))
	if err != nil {
		t.Fatalf("Expand stashed content: %v", err)
	}

	if !contains(result, "calculateTotal") {
		t.Error("Expanded content missing expected function name")
	}
}

// TestLazyLoadingPreservesStaleDataProtection ensures that lazy-loaded
// references always read fresh content from disk, not stale cached versions.
func TestLazyLoadingPreservesStaleDataProtection(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer store.Close()

	// Create a file
	filePath := filepath.Join(tmpDir, "changing.txt")
	os.WriteFile(filePath, []byte("Version 1"), 0644)

	// Create lazy reference
	ref := Memory{
		ID:         "changing-file",
		Content:    "", // Not stored
		Summary:    "File that will change",
		Tags:       []string{"file"},
		RefType:    "file",
		RefTarget:  filePath,
		IsLazy:     true,
		Importance: 0.5,
	}

	if err := store.Save(ref); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// First retrieval
	m1, err := store.Get(ref.ID)
	if err != nil {
		t.Fatalf("First get: %v", err)
	}
	if m1.Content != "Version 1" {
		t.Errorf("First get: got %q, want %q", m1.Content, "Version 1")
	}

	// Modify file
	os.WriteFile(filePath, []byte("Version 2 - updated!"), 0644)

	// Second retrieval should get fresh content
	m2, err := store.Get(ref.ID)
	if err != nil {
		t.Fatalf("Second get: %v", err)
	}
	if m2.Content != "Version 2 - updated!" {
		t.Errorf("Second get: got %q, want fresh content", m2.Content)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && 
		(s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
