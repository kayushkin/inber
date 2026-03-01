package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPostWriteHook_DetectGo(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test"), 0644)
	h := NewPostWriteHook(dir)
	if h.projectType != "go" {
		t.Fatalf("expected go, got %q", h.projectType)
	}
}

func TestPostWriteHook_IgnoresNonWriteTools(t *testing.T) {
	h := &PostWriteHook{projectType: "go", repoRoot: "."}
	if msg := h.OnToolResult("read_file"); msg != "" {
		t.Fatalf("expected empty, got %q", msg)
	}
}

func TestPostWriteHook_NoProjectType(t *testing.T) {
	h := &PostWriteHook{projectType: "", repoRoot: "."}
	if msg := h.OnToolResult("write_file"); msg != "" {
		t.Fatalf("expected empty for unknown project, got %q", msg)
	}
}

func TestPostWriteHook_Dedup(t *testing.T) {
	h := &PostWriteHook{}
	msg1 := h.dedup("error X")
	if msg1 != "error X" {
		t.Fatalf("expected error X, got %q", msg1)
	}
	msg2 := h.dedup("error X")
	if msg2 != "⚠️ same error as last build" {
		t.Fatalf("expected dedup message, got %q", msg2)
	}
	msg3 := h.dedup("error Y")
	if msg3 != "error Y" {
		t.Fatalf("expected error Y, got %q", msg3)
	}
}

func TestCompactGoError(t *testing.T) {
	output := `# example.com/foo
./main.go:10:5: undefined: Foo
./main.go:15:2: too many arguments
`
	result := compactGoError("build", output)
	if result == "" {
		t.Fatal("expected non-empty")
	}
	if !contains(result, "build failed") {
		t.Fatalf("expected 'build failed' in %q", result)
	}
	if !contains(result, "main.go:10:5") {
		t.Fatalf("expected error line in %q", result)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && findSubstring(s, sub)
}

func findSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
