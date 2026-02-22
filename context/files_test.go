package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileLoader_LoadFiles(t *testing.T) {
	// Create temp workspace
	tmpDir := t.TempDir()
	
	// Create test files
	files := map[string]string{
		"main.go":      "package main\n\nfunc main() {}\n",
		"helper.go":    "package main\n\nfunc helper() {}\n",
		"main_test.go": "package main\n\nimport \"testing\"\n",
		"README.md":    "# Test Project\n",
		"config.toml":  "[server]\nport = 8080\n",
	}
	
	for filename, content := range files {
		path := filepath.Join(tmpDir, filename)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file %s: %v", filename, err)
		}
	}
	
	// Load files
	loader, err := NewFileLoader(tmpDir, NewPatternTagger())
	if err != nil {
		t.Fatalf("Failed to create file loader: %v", err)
	}
	
	chunks, err := loader.LoadFiles()
	if err != nil {
		t.Fatalf("LoadFiles failed: %v", err)
	}
	
	if len(chunks) != len(files) {
		t.Errorf("Expected %d chunks, got %d", len(files), len(chunks))
	}
	
	// Verify each file became a chunk
	chunkIDs := make(map[string]bool)
	for _, chunk := range chunks {
		chunkIDs[chunk.ID] = true
	}
	
	for filename := range files {
		expectedID := "file:" + filename
		if !chunkIDs[expectedID] {
			t.Errorf("Missing chunk for file %s (ID: %s)", filename, expectedID)
		}
	}
}

func TestFileLoader_TagsByExtension(t *testing.T) {
	tmpDir := t.TempDir()
	
	testCases := []struct {
		filename     string
		content      string
		expectedTags []string
	}{
		{
			filename:     "main.go",
			content:      "package main",
			expectedTags: []string{"file", "filename:main.go", "go", "code"},
		},
		{
			filename:     "app_test.go",
			content:      "package main",
			expectedTags: []string{"file", "filename:app_test.go", "go", "code", "test"},
		},
		{
			filename:     "config.toml",
			content:      "[server]",
			expectedTags: []string{"file", "filename:config.toml", "toml", "config"},
		},
		{
			filename:     "README.md",
			content:      "# Project",
			expectedTags: []string{"file", "filename:README.md", "md", "doc", "readme"},
		},
		{
			filename:     "script.sh",
			content:      "#!/bin/bash",
			expectedTags: []string{"file", "filename:script.sh", "sh", "code", "script"},
		},
	}
	
	for _, tc := range testCases {
		path := filepath.Join(tmpDir, tc.filename)
		if err := os.WriteFile(path, []byte(tc.content), 0644); err != nil {
			t.Fatalf("Failed to create %s: %v", tc.filename, err)
		}
	}
	
	loader, _ := NewFileLoader(tmpDir, NewPatternTagger())
	chunks, err := loader.LoadFiles()
	if err != nil {
		t.Fatalf("LoadFiles failed: %v", err)
	}
	
	for _, tc := range testCases {
		chunkID := "file:" + tc.filename
		var chunk *Chunk
		
		for i := range chunks {
			if chunks[i].ID == chunkID {
				chunk = &chunks[i]
				break
			}
		}
		
		if chunk == nil {
			t.Errorf("Chunk not found for %s", tc.filename)
			continue
		}
		
		for _, expectedTag := range tc.expectedTags {
			hasTag := false
			for _, tag := range chunk.Tags {
				if tag == expectedTag {
					hasTag = true
					break
				}
			}
			if !hasTag {
				t.Errorf("File %s missing expected tag %q, has: %v", tc.filename, expectedTag, chunk.Tags)
			}
		}
	}
}

func TestFileLoader_IgnoreHiddenFiles(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create regular and hidden files
	os.WriteFile(filepath.Join(tmpDir, "visible.txt"), []byte("visible"), 0644)
	os.WriteFile(filepath.Join(tmpDir, ".hidden"), []byte("hidden"), 0644)
	
	// Create hidden directory
	hiddenDir := filepath.Join(tmpDir, ".hiddendir")
	os.Mkdir(hiddenDir, 0755)
	os.WriteFile(filepath.Join(hiddenDir, "file.txt"), []byte("in hidden dir"), 0644)
	
	loader, _ := NewFileLoader(tmpDir, nil)
	chunks, err := loader.LoadFiles()
	if err != nil {
		t.Fatalf("LoadFiles failed: %v", err)
	}
	
	// Should only load visible.txt
	if len(chunks) != 1 {
		t.Errorf("Expected 1 chunk (visible.txt), got %d", len(chunks))
	}
	
	if len(chunks) > 0 && !strings.Contains(chunks[0].ID, "visible.txt") {
		t.Errorf("Expected visible.txt, got %s", chunks[0].ID)
	}
}

func TestFileLoader_GitignoreRespect(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create .gitignore
	gitignore := `*.log
build/
temp.txt
`
	os.WriteFile(filepath.Join(tmpDir, ".gitignore"), []byte(gitignore), 0644)
	
	// Create files
	os.WriteFile(filepath.Join(tmpDir, "app.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "error.log"), []byte("errors"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "temp.txt"), []byte("temp"), 0644)
	
	// Create build directory
	buildDir := filepath.Join(tmpDir, "build")
	os.Mkdir(buildDir, 0755)
	os.WriteFile(filepath.Join(buildDir, "output.bin"), []byte("binary"), 0644)
	
	loader, _ := NewFileLoader(tmpDir, nil)
	chunks, err := loader.LoadFiles()
	if err != nil {
		t.Fatalf("LoadFiles failed: %v", err)
	}
	
	// Should only load app.go (not .gitignore itself, error.log, temp.txt, or build/*)
	if len(chunks) != 1 {
		t.Errorf("Expected 1 chunk (app.go), got %d", len(chunks))
		for _, c := range chunks {
			t.Logf("  - %s", c.ID)
		}
	}
	
	if len(chunks) > 0 && !strings.Contains(chunks[0].ID, "app.go") {
		t.Errorf("Expected app.go, got %s", chunks[0].ID)
	}
}

func TestFileLoader_LoadAndUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create test file
	os.WriteFile(filepath.Join(tmpDir, "test.go"), []byte("package test"), 0644)
	
	store := NewStore()
	loader, _ := NewFileLoader(tmpDir, nil)
	
	count, err := loader.LoadAndUpdate(store)
	if err != nil {
		t.Fatalf("LoadAndUpdate failed: %v", err)
	}
	
	if count != 1 {
		t.Errorf("Expected 1 file loaded, got %d", count)
	}
	
	// Verify it's in the store
	chunk, ok := store.Get("file:test.go")
	if !ok {
		t.Error("File should be in store")
	}
	
	if !strings.Contains(chunk.Text, "package test") {
		t.Error("Chunk should contain file content")
	}
}

func TestFileLoader_BinaryFileSkipped(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create a binary-like file (contains null bytes)
	binaryData := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE}
	os.WriteFile(filepath.Join(tmpDir, "binary.bin"), binaryData, 0644)
	
	// Create a text file
	os.WriteFile(filepath.Join(tmpDir, "text.txt"), []byte("text content"), 0644)
	
	loader, _ := NewFileLoader(tmpDir, nil)
	chunks, err := loader.LoadFiles()
	if err != nil {
		t.Fatalf("LoadFiles failed: %v", err)
	}
	
	// Should only load text.txt
	if len(chunks) != 1 {
		t.Errorf("Expected 1 chunk (text file), got %d", len(chunks))
	}
	
	if len(chunks) > 0 && !strings.Contains(chunks[0].ID, "text.txt") {
		t.Errorf("Expected text.txt, got %s", chunks[0].ID)
	}
}

func TestIsBinary(t *testing.T) {
	tests := []struct {
		name     string
		content  []byte
		expected bool
	}{
		{"text", []byte("Hello, world!"), false},
		{"binary", []byte{0x00, 0x01, 0x02}, true},
		{"utf8", []byte("Hello 世界"), false},
		{"null byte", []byte("text\x00data"), true},
		{"empty", []byte{}, false},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isBinary(tt.content)
			if result != tt.expected {
				t.Errorf("isBinary(%q) = %v, want %v", tt.name, result, tt.expected)
			}
		})
	}
}

func TestMatchPattern(t *testing.T) {
	tests := []struct {
		pattern  string
		path     string
		expected bool
	}{
		{"*.log", "error.log", true},
		{"*.log", "app.go", false},
		{"build/", "build/output.bin", true},
		{"build/", "src/build.go", false},
		{"temp.txt", "temp.txt", true},
		{"temp.txt", "other.txt", false},
		{"node_modules", "node_modules/package.json", true},
		{"node_modules", "src/node_modules/pkg.json", true},
	}
	
	for _, tt := range tests {
		result := matchPattern(tt.pattern, tt.path)
		if result != tt.expected {
			t.Errorf("matchPattern(%q, %q) = %v, want %v", tt.pattern, tt.path, result, tt.expected)
		}
	}
}
