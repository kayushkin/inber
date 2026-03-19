package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/kayushkin/inber/memory"
)

func TestMemorySaveSearchForget(t *testing.T) {
	dir := setupTestRepo(t)
	orig, _ := os.Getwd()
	defer os.Chdir(orig)
	os.Chdir(dir)

	// Save a memory directly via the store
	store, err := memory.OpenOrCreate(dir)
	if err != nil {
		t.Fatalf("failed to open memory store: %v", err)
	}

	m := memory.Memory{
		ID:         "test-mem-001",
		Content:    "The quick brown fox jumps over the lazy dog",
		Tags:       []string{"test", "animals"},
		Importance: 0.8,
		Source:     "user",
	}
	if err := store.Save(m); err != nil {
		t.Fatalf("failed to save memory: %v", err)
	}
	store.Close()

	// Test search
	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	memorySearchLimit = 10
	runMemorySearch(nil, []string{"fox", "dog"})

	w.Close()
	os.Stdout = old
	buf.ReadFrom(r)

	output := buf.String()
	if !strings.Contains(output, "quick brown fox") {
		t.Errorf("expected memory content in search results, got: %s", output)
	}

	// Test list
	r, w, _ = os.Pipe()
	os.Stdout = w
	buf.Reset()

	memoryListLimit = 10
	memoryListMin = 0.0
	runMemoryList(nil, nil)

	w.Close()
	os.Stdout = old
	buf.ReadFrom(r)

	output = buf.String()
	if !strings.Contains(output, "quick brown fox") {
		t.Errorf("expected memory in list, got: %s", output)
	}

	// Test stats
	r, w, _ = os.Pipe()
	os.Stdout = w
	buf.Reset()

	runMemoryStats(nil, nil)

	w.Close()
	os.Stdout = old
	buf.ReadFrom(r)

	output = buf.String()
	if !strings.Contains(output, "Total memories: 1") {
		t.Errorf("expected 1 memory in stats, got: %s", output)
	}
}