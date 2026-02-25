package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAutoLoad(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create a sample Go file
	goFile := `package test

func Hello() string {
	return "hello"
}
`
	err := os.WriteFile(filepath.Join(tmpDir, "test.go"), []byte(goFile), 0644)
	if err != nil {
		t.Fatal(err)
	}
	
	// Configure auto-load
	cfg := AutoLoadConfig{
		RootDir:        tmpDir,
		AgentName:      "test-agent",
		IdentityText:   "You are a test agent.",
		RepoMapEnabled: true,
		RecencyWindow:  1 * time.Hour,
	}
	
	// Load context
	store, err := AutoLoad(cfg)
	if err != nil {
		t.Fatalf("AutoLoad failed: %v", err)
	}
	
	// Verify identity chunk exists
	identity, ok := store.Get("identity")
	if !ok {
		t.Error("identity chunk not found")
	} else {
		if !strings.Contains(identity.Text, "test agent") {
			t.Errorf("identity text incorrect: %s", identity.Text)
		}
		if !hasTag(identity.Tags, "identity") {
			t.Error("identity chunk missing 'identity' tag")
		}
		if !hasTag(identity.Tags, "always") {
			t.Error("identity chunk missing 'always' tag")
		}
	}
	
	// Verify repo map chunk exists
	repoMap, ok := store.Get("repo-map")
	if !ok {
		t.Error("repo-map chunk not found")
	} else {
		if !strings.Contains(repoMap.Text, "func Hello()") {
			t.Errorf("repo map missing function signature: %s", repoMap.Text)
		}
		if !hasTag(repoMap.Tags, "repo-map") {
			t.Error("repo map chunk missing 'repo-map' tag")
		}
	}
	
	// Recent files chunk should exist if files were just created
	recentFiles, ok := store.Get("recent-files")
	if ok {
		if !hasTag(recentFiles.Tags, "recent") {
			t.Error("recent files chunk missing 'recent' tag")
		}
	}
}

func TestLoadIdentityFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	identityFile := filepath.Join(tmpDir, "identity.txt")
	
	identityContent := "You are a specialized testing agent with deep knowledge of Go."
	err := os.WriteFile(identityFile, []byte(identityContent), 0644)
	if err != nil {
		t.Fatal(err)
	}
	
	cfg := AutoLoadConfig{
		RootDir:        tmpDir,
		AgentName:      "test-agent",
		IdentityFile:   identityFile,
		RepoMapEnabled: false,
	}
	
	store, err := AutoLoad(cfg)
	if err != nil {
		t.Fatalf("AutoLoad failed: %v", err)
	}
	
	identity, ok := store.Get("identity")
	if !ok {
		t.Fatal("identity chunk not found")
	}
	
	if identity.Text != identityContent {
		t.Errorf("identity text mismatch.\nExpected: %s\nGot: %s", identityContent, identity.Text)
	}
}

func TestLoadProjectContext(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Create .openclaw directory
	openclawDir := filepath.Join(tmpDir, ".openclaw")
	err := os.Mkdir(openclawDir, 0755)
	if err != nil {
		t.Fatal(err)
	}
	
	// Create AGENTS.md
	agentsContent := "# AGENTS.md\n\nYou are the dev agent for testing."
	err = os.WriteFile(filepath.Join(openclawDir, "AGENTS.md"), []byte(agentsContent), 0644)
	if err != nil {
		t.Fatal(err)
	}
	
	// Create README.md
	readmeContent := "# Test Project\n\nThis is a test project."
	err = os.WriteFile(filepath.Join(tmpDir, "README.md"), []byte(readmeContent), 0644)
	if err != nil {
		t.Fatal(err)
	}
	
	store := NewStore()
	err = LoadProjectContext(store, tmpDir)
	if err != nil {
		t.Fatalf("LoadProjectContext failed: %v", err)
	}
	
	// Check AGENTS.md was loaded
	agentsChunk, ok := store.Get("project-AGENTS.md")
	if !ok {
		t.Error("AGENTS.md chunk not found")
	} else {
		if !strings.Contains(agentsChunk.Text, "dev agent") {
			t.Error("AGENTS.md content incorrect")
		}
		if !hasTag(agentsChunk.Tags, "agents") {
			t.Error("AGENTS.md missing 'agents' tag")
		}
	}
	
	// Check README.md was loaded
	readmeChunk, ok := store.Get("project-README.md")
	if !ok {
		t.Error("README.md chunk not found")
	} else {
		if !strings.Contains(readmeChunk.Text, "Test Project") {
			t.Error("README.md content incorrect")
		}
		if !hasTag(readmeChunk.Tags, "readme") {
			t.Error("README.md missing 'readme' tag")
		}
	}
}
