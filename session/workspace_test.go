package session

import (
	"os"
	"path/filepath"
	"testing"
)

// The workspace's system/ directory looks like a control surface and is not one.
// These tests pin what it actually does, so the type comment on Workspace stays
// true by something other than good intentions — it claimed the opposite for a
// long time and nothing caught it (noteboard todo 9def7d6b).

func TestWriteSystemDiscardsAUserEdit(t *testing.T) {
	w := NewWorkspace(t.TempDir(), "brigid")
	blocks := []NamedBlock{{ID: "identity", Text: "generated"}}

	if err := w.WriteSystem(blocks); err != nil {
		t.Fatalf("first write: %v", err)
	}
	edited := filepath.Join(w.Dir, "system", "01-identity.md")
	if err := os.WriteFile(edited, []byte("the user's edit"), 0644); err != nil {
		t.Fatalf("simulating the user's edit: %v", err)
	}

	if err := w.WriteSystem(blocks); err != nil {
		t.Fatalf("second write: %v", err)
	}

	got, err := os.ReadFile(edited)
	if err != nil {
		t.Fatalf("reading back: %v", err)
	}
	if string(got) != "generated" {
		t.Fatalf("the user's edit survived the next turn, so system/ is now an input to the "+
			"prompt and the Workspace doc comment is wrong: got %q", got)
	}
}

func TestWriteSystemDeletesAUserAddedFileAndAStaleBlock(t *testing.T) {
	w := NewWorkspace(t.TempDir(), "brigid")

	if err := w.WriteSystem([]NamedBlock{
		{ID: "identity", Text: "a"},
		{ID: "repo map", Text: "b"},
	}); err != nil {
		t.Fatalf("first write: %v", err)
	}
	sysDir := filepath.Join(w.Dir, "system")
	added := filepath.Join(sysDir, "99-mine.md")
	if err := os.WriteFile(added, []byte("mine"), 0644); err != nil {
		t.Fatalf("simulating a user-added file: %v", err)
	}

	// The second turn produces one fewer block.
	if err := w.WriteSystem([]NamedBlock{{ID: "identity", Text: "a"}}); err != nil {
		t.Fatalf("second write: %v", err)
	}

	entries, err := os.ReadDir(sysDir)
	if err != nil {
		t.Fatalf("reading system dir: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 1 || names[0] != "01-identity.md" {
		t.Fatalf("system/ should hold exactly this turn's blocks and nothing else, got %v", names)
	}
}

func TestWriteToolsListWritesToolsMarkdownNotJSON(t *testing.T) {
	w := NewWorkspace(t.TempDir(), "brigid")
	if err := w.WriteToolsList([]ToolInfo{{Name: "read_files", Description: "reads files"}}); err != nil {
		t.Fatalf("writing tool list: %v", err)
	}
	if _, err := os.Stat(filepath.Join(w.Dir, "tools.md")); err != nil {
		t.Fatalf("expected tools.md, which is what the layout comment now documents: %v", err)
	}
	if _, err := os.Stat(filepath.Join(w.Dir, "tools.json")); err == nil {
		t.Fatal("tools.json exists again; the layout comment documents tools.md")
	}
}
