package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// executeCommand runs a cobra command and captures stdout
func executeCommand(args ...string) (string, error) {
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	return buf.String(), err
}

// setupTestRepo creates a temporary repo structure for testing
// Note: Agent configs now come from agent-store, not files
func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// Create .git dir so FindRepoRoot works
	os.MkdirAll(filepath.Join(dir, ".git"), 0755)

	// Create logs directory
	os.MkdirAll(filepath.Join(dir, "logs"), 0755)

	return dir
}