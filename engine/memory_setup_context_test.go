package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// newWorkspaceForSetup builds a directory holding one file, which is all the
// recency scan needs to have something to find.
func newWorkspaceForSetup(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	return root
}

// Preparing a session scans the workspace, and that scan is the longest thing
// engine construction does. A caller that has withdrawn must be told so rather
// than handed an engine it never waited for.
func TestSetupMemoryStoreFailsForACancelledCaller(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	store, err := setupMemoryStore(cancelled, newWorkspaceForSetup(t), "identity", "test-agent")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("setupMemoryStore returned err %v, want context.Canceled", err)
	}
	if store != nil {
		store.Close()
		t.Error("setupMemoryStore returned a store for a caller that had already given up")
	}
}

// The complement: a live caller still gets a prepared store, so the test above
// cannot pass by construction failing for everyone.
func TestSetupMemoryStorePreparesForALiveCaller(t *testing.T) {
	store, err := setupMemoryStore(context.Background(), newWorkspaceForSetup(t), "identity", "test-agent")
	if err != nil {
		t.Fatalf("setupMemoryStore: %v", err)
	}
	defer store.Close()

	identity, err := store.Get("identity")
	if err != nil || identity == nil {
		t.Fatalf("identity not loaded: %v", err)
	}
}
