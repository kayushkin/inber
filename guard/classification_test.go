package guard

import (
	"testing"

	"github.com/kayushkin/inber/memory"
	"github.com/kayushkin/inber/tools"
	toolstoretools "github.com/kayushkin/tool-store/tools"
)

// knownToolNames is every name a tool in this process actually answers to,
// derived from the tools themselves rather than restated here.
func knownToolNames(t *testing.T) map[string]bool {
	t.Helper()

	names := make(map[string]bool)

	// tool-store auto-registers its zero-argument tools.
	for _, impl := range toolstoretools.All() {
		names[impl.Name] = true
	}

	// The rest are constructed on demand: tool-store's context-taking tools
	// (repo_map, recent_files) and inber's own memory/deploy tools. Only Name is
	// read, so the throwaway arguments are never used and the nil store is never
	// dereferenced. Naming them via their constructors rather than restating the
	// strings is the point — a renamed tool must move this set, not slip past it.
	for _, name := range []string{
		tools.RepoMap("", nil).Name,
		tools.RecentFiles("").Name,
		tools.Deploy().Name,
		memory.SearchTool(nil).Name,
		memory.SaveTool(nil).Name,
		memory.ExpandTool(nil).Name,
		memory.ForgetTool(nil).Name,
	} {
		names[name] = true
	}
	return names
}

// TestClassifiedToolsExist asserts that every tool name the guard classifies is
// a name some tool actually has.
//
// This is the check that was missing. isReadOnly and isDangerous were written
// against the pre-tool-store names, and when tool-store renamed the tools
// (read_file -> read_files, write_file -> write_files, edit_file -> edit_files)
// the classifiers kept matching on names nothing answered to. A name that
// matches no tool fails open in isDangerous: Assist mode routes dangerous calls
// through ApprovalFunc, so write_files and edit_files would have been treated as
// safe and executed with no approval gate at all.
//
// The classifiers are unreachable today — CheckTool has no caller and the engine
// hardcodes Autonomous mode, which allows everything — so this was latent rather
// than live. It stops being latent the moment somebody wires up Assist.
func TestClassifiedToolsExist(t *testing.T) {
	known := knownToolNames(t)

	classified := map[string][]string{
		"isReadOnly": {
			"read_files", "list_files", "ripgrep", "memory_expand", "memory_search",
			"repo_map", "recent_files", "web_search",
		},
		"isDangerous": {
			"shell_commands", "write_files", "edit_files", "deploy",
		},
	}

	for fn, toolNames := range classified {
		for _, name := range toolNames {
			if !known[name] {
				t.Errorf("%s classifies %q, but no tool has that name — the case can never match, so the tool it was meant to cover is unclassified", fn, name)
			}
		}
	}

	// The classifier lists above must stay in step with the switches themselves.
	for _, name := range classified["isReadOnly"] {
		if !isReadOnly(name) {
			t.Errorf("isReadOnly(%q) = false, but this test lists it as read-only", name)
		}
	}
	for _, name := range classified["isDangerous"] {
		if !isDangerous(name) {
			t.Errorf("isDangerous(%q) = false, but this test lists it as dangerous", name)
		}
	}
}

// TestWriteToolsNeedApprovalInAssistMode drives the actual verdict, not the
// classifier, because that is the property that matters: a write must not reach
// the model unapproved. With the pre-rename names this passed Allowed straight
// back for every write tool.
func TestWriteToolsNeedApprovalInAssistMode(t *testing.T) {
	g := New(Config{Mode: Assist}) // no ApprovalFunc — nothing can auto-approve

	for _, tool := range []string{"shell_commands", "write_files", "edit_files", "deploy"} {
		if verdict := g.CheckTool(tool, "{}"); verdict != NeedsApproval {
			t.Errorf("CheckTool(%q) in Assist = %v, want NeedsApproval — a write tool that is not classified dangerous executes with no approval gate", tool, verdict)
		}
	}

	// Reads still pass straight through; the gate is for writes.
	for _, tool := range []string{"read_files", "list_files"} {
		if verdict := g.CheckTool(tool, "{}"); verdict != Allowed {
			t.Errorf("CheckTool(%q) in Assist = %v, want Allowed", tool, verdict)
		}
	}
}

// TestObserveModeAllowsTheRealReadTools pins the other side: in Observe mode a
// read tool whose name the classifier no longer recognises is Denied, which
// would have made Observe mode useless rather than unsafe.
func TestObserveModeAllowsTheRealReadTools(t *testing.T) {
	g := New(Config{Mode: Observe})

	for _, tool := range []string{"read_files", "list_files", "memory_search"} {
		if verdict := g.CheckTool(tool, "{}"); verdict != Allowed {
			t.Errorf("CheckTool(%q) in Observe = %v, want Allowed", tool, verdict)
		}
	}
	for _, tool := range []string{"write_files", "shell_commands"} {
		if verdict := g.CheckTool(tool, "{}"); verdict != Denied {
			t.Errorf("CheckTool(%q) in Observe = %v, want Denied", tool, verdict)
		}
	}
}
