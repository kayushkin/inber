package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// TestAgentsList tests that the agents list command works with agent-store
// It uses the real agent-store database, so we check for "claxon" which is the default
func TestAgentsList(t *testing.T) {
	dir := setupTestRepo(t)
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	// Call the command function directly
	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	runAgentsList(nil, nil)

	w.Close()
	os.Stdout = old
	buf.ReadFrom(r)

	output := buf.String()
	// Check for claxon which exists in the real agent-store
	if !strings.Contains(output, "claxon") {
		t.Errorf("expected output to contain 'claxon', got: %s", output)
	}
	if !strings.Contains(output, "Configured agents") {
		t.Errorf("expected output to contain 'Configured agents', got: %s", output)
	}
}

// TestAgentsShow tests that the agents show command works with agent-store
func TestAgentsShow(t *testing.T) {
	dir := setupTestRepo(t)
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Use claxon which exists in the real agent-store
	runAgentsShow(nil, []string{"claxon"})

	w.Close()
	os.Stdout = old
	buf.ReadFrom(r)

	output := buf.String()
	if !strings.Contains(output, "claxon") {
		t.Errorf("expected agent name in output, got: %s", output)
	}
}