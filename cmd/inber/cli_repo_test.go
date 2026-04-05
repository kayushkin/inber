package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kayushkin/inber/internal/fsutil"
)

func TestFindRepoRoot(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)

	// Create a subdirectory
	subdir := filepath.Join(dir, "a", "b", "c")
	os.MkdirAll(subdir, 0755)

	// FindRepoRoot from subdirectory should find root
	orig, _ := os.Getwd()
	defer os.Chdir(orig)

	os.Chdir(subdir)
	root, err := fsutil.FindRepoRoot()
	if err != nil {
		t.Fatalf("FindRepoRoot failed: %v", err)
	}
	if root != dir {
		t.Errorf("expected %s, got %s", dir, root)
	}
}

func TestFindRepoRoot_NotInRepo(t *testing.T) {
	dir := t.TempDir() // no .git

	orig, _ := os.Getwd()
	defer os.Chdir(orig)

	os.Chdir(dir)
	_, err := fsutil.FindRepoRoot()
	if err == nil {
		t.Error("expected error when not in a git repository")
	}
}