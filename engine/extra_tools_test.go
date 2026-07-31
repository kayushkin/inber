package engine

import (
	"testing"

	"github.com/kayushkin/inber/agent"
)

func toolNames(tools []agent.Tool) []string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	return names
}

func TestMergeExtraToolsAppendsNewNames(t *testing.T) {
	base := []agent.Tool{{Name: "read_files"}, {Name: "shell_commands"}}
	got := mergeExtraTools(base, []agent.Tool{{Name: "spawn_agent"}})

	want := []string{"read_files", "shell_commands", "spawn_agent"}
	if names := toolNames(got); !equalStrings(names, want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
}

func TestMergeExtraToolsReplacesInPlaceWithoutGrowing(t *testing.T) {
	base := []agent.Tool{{Name: "read_files", Description: "built in"}, {Name: "shell_commands"}}
	got := mergeExtraTools(base, []agent.Tool{{Name: "read_files", Description: "injected"}})

	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 — a same-named tool must replace, not append", len(got))
	}
	if got[0].Description != "injected" {
		t.Fatalf("read_files description = %q, want the injected one", got[0].Description)
	}
	if got[0].Name != "read_files" {
		t.Fatalf("replacement landed on %q", got[0].Name)
	}
}

// A pre-existing duplicate in the base set survives the merge: the loop breaks
// at the first match. This is the behaviour that ships, and the fix for it
// needs a policy decision (noteboard todo e2d0b07b), so the test pins the
// defect as present rather than asserting the behaviour we want.
func TestMergeExtraToolsLeavesAPreExistingDuplicateInPlace(t *testing.T) {
	base := []agent.Tool{
		{Name: "read_files", Description: "first"},
		{Name: "read_files", Description: "second"},
	}
	got := mergeExtraTools(base, []agent.Tool{{Name: "read_files", Description: "injected"}})

	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Description != "injected" {
		t.Fatalf("first copy = %q, want it replaced", got[0].Description)
	}
	if got[1].Description != "second" {
		t.Fatalf("second copy = %q, want it untouched — the documented defect", got[1].Description)
	}
}

func TestMergeExtraToolsWithNoExtras(t *testing.T) {
	base := []agent.Tool{{Name: "read_files"}}
	if got := mergeExtraTools(base, nil); len(got) != 1 || got[0].Name != "read_files" {
		t.Fatalf("merging no extras changed the set: %v", toolNames(got))
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
