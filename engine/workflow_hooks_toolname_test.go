package engine

import (
	"testing"

	"github.com/kayushkin/inber/memory"
	"github.com/kayushkin/inber/tools"
	toolstoretools "github.com/kayushkin/tool-store/tools"
)

// This file pins a defect as PRESENT. It is not asserting desired behaviour.
//
// OnToolResult gates every auto-workflow behaviour — auto-format,
// build-and-test-on-write, the per-file auto-commit, and the population of
// h.changedFiles — behind
//
//	if toolName != "write_file" && toolName != "edit_file" { return "" }
//
// (workflow_hooks.go). Those are the PRE-tool-store names. The tools inber
// actually registers are plural: write_files and edit_files. OnToolResult is
// called with the model's tool name, so the guard has matched nothing since the
// rename, and everything downstream of it has been unreachable.
//
// guard/classification_test.go already has this exact check, for the exact same
// rename, and its comment says why it was written: isDangerous silently stopped
// matching the write tools and Assist mode would have waved writes through
// without approval. The check was applied to guard/ and not to engine/, and this
// is the site it missed.
//
// Re-arming the hook is NOT a rename — it re-arms a formatter run, a full
// `go build ./... && go test ./...` injected into the conversation, and a git
// commit, after every file write on a live server. Which of those the user
// wants back is todo af237d64, and it is deliberately not answered here.
//
// So these tests fail CLOSED: they assert the hook is dead. When af237d64 lands
// they will go red, and the message on each says what to change them to. That is
// the point — a red test is how the next worker learns the fix took effect.

// realWriteToolNames returns the names inber's write/edit tools actually answer
// to, read off the tools themselves rather than restated here. Same approach as
// guard/classification_test.go's knownToolNames: a future rename has to move
// this set, not slip past it.
func realWriteToolNames(t *testing.T) (write, edit string) {
	t.Helper()

	write = tools.WriteFiles().Name
	edit = tools.EditFiles().Name
	if write == "" || edit == "" {
		t.Fatal("write/edit tool has an empty name — the rig is broken, not the code")
	}
	return write, edit
}

// TestWorkflowHookNamesMatchNoRealTool pins the cause: the two names the hook
// gates on are names no tool in this process has.
func TestWorkflowHookNamesMatchNoRealTool(t *testing.T) {
	known := map[string]bool{}
	for _, impl := range toolstoretools.All() {
		known[impl.Name] = true
	}
	for _, name := range []string{
		tools.RepoMap("", nil).Name,
		tools.RecentFiles("").Name,
		tools.TaskPlan("").Name,
		tools.Scratchpad("", "").Name,
		tools.Deploy().Name,
		memory.SearchTool(nil).Name,
		memory.SaveTool(nil).Name,
		memory.ExpandTool(nil).Name,
		memory.ForgetTool(nil).Name,
	} {
		known[name] = true
	}

	for _, gated := range []string{"write_file", "edit_file"} {
		if known[gated] {
			t.Errorf("OnToolResult gates on %q and a tool now HAS that name — "+
				"the auto-workflow hook is live again. Delete this test and assert "+
				"the real behaviour instead (todo af237d64).", gated)
		}
	}

	// The control. If this fails, the helper above is not reading the real tool
	// set and the assertion it guards proves nothing.
	write, edit := realWriteToolNames(t)
	for _, name := range []string{write, edit} {
		if !known[name] {
			t.Fatalf("rig guard: %q is a registered tool but knownToolNames missed it", name)
		}
	}
}

// TestOnToolResultIsDeadForRealWriteTools pins the consequence: a write under
// the name the model actually uses produces nothing and records nothing.
//
// The input deliberately carries a top-level "path", the single-file shape
// extractFilePath can parse. That isolates the name guard as the only reason
// this returns empty — with a batch files[] input it would also return empty,
// but for a second reason (extractFilePath cannot read the batch shape, which is
// its own half of af237d64), and the test would not discriminate.
func TestOnToolResultIsDeadForRealWriteTools(t *testing.T) {
	write, edit := realWriteToolNames(t)

	for _, toolName := range []string{write, edit} {
		t.Run(toolName, func(t *testing.T) {
			h := NewWorkflowHooks(t.TempDir(), "sess-1", "tester", AutoWorkflowConfig{
				AutoCommit: true,
				AutoFormat: true,
			})

			got := h.OnToolResult(toolName, `{"path":"main.go","content":"package main"}`, "wrote 12 bytes to main.go", false)

			if got != "" {
				t.Errorf("OnToolResult(%q) = %q, want \"\" — the hook has come alive. "+
					"That is the fix af237d64 asks for; update this test to assert what "+
					"it should now do.", toolName, got)
			}
			if len(h.changedFiles) != 0 {
				t.Errorf("changedFiles = %v after a %q, want empty — the hook has come "+
					"alive (todo af237d64). FinishSession can report a file count again.",
					h.changedFiles, toolName)
			}
		})
	}
}

// TestOnToolResultStillFiresForTheDeadNames is the discriminator. Without it,
// both assertions above would stay green if someone made OnToolResult return ""
// unconditionally — which is a different change (deleting the hook, option C of
// af237d64) wearing the same test result.
func TestOnToolResultStillFiresForTheDeadNames(t *testing.T) {
	h := NewWorkflowHooks(t.TempDir(), "sess-1", "tester", AutoWorkflowConfig{
		AutoCommit: true,
		AutoFormat: true,
	})

	h.OnToolResult("write_file", `{"path":"main.go","content":"package main"}`, "ok", false)

	if len(h.changedFiles) != 1 {
		t.Fatalf("changedFiles = %v after a legacy-named write, want exactly 1 — "+
			"the body of OnToolResult is no longer reachable by ANY name, so the two "+
			"tests above would pass against a hook that had simply been deleted.",
			h.changedFiles)
	}
}
