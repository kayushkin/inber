package engine

import (
	"testing"

	"github.com/kayushkin/inber/memory"
	"github.com/kayushkin/inber/tools"
	toolstoretools "github.com/kayushkin/tool-store/tools"
)

// This file pins a defect as PRESENT. It is not asserting desired behaviour.
//
// OnToolResult used to gate every auto-workflow behaviour — auto-format,
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
// ONE THING HAS SINCE MOVED, and it is why these tests read the way they do
// now. `h.changedFiles` — the list of paths the session wrote — is no longer
// behind that gate. Todo 7d8bbca1 needed it: the close-time commit was doing
// `git add -A`, sweeping other agents' work into a commit claiming to be this
// session's, and the fix is to stage that list instead. Recording a path costs
// nothing, runs nothing and decides nothing, so it did not need af237d64's
// answer; the three expensive automations still do, and are still dead.
//
// So these tests still fail CLOSED on the part that is still open: they assert
// the automations are unreachable under the real tool names, and that the
// recording is not. When af237d64 lands the first assertion goes red, and its
// message says what to change it to. That is the point — a red test is how the
// next worker learns the fix took effect.

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

// TestTheAUTOMATIONSAreDeadForRealWriteTools pins the half of the defect that
// is still open: a write under the name the model actually uses runs no
// formatter, no build, no test and no commit, so it has nothing to say.
//
// It also pins the half that is fixed — the write IS recorded — because the two
// assertions are only meaningful together. Alone, "returns empty" is equally
// true of a hook that has been deleted.
func TestTheAUTOMATIONSAreDeadForRealWriteTools(t *testing.T) {
	write, edit := realWriteToolNames(t)

	for _, toolName := range []string{write, edit} {
		t.Run(toolName, func(t *testing.T) {
			h := NewWorkflowHooks(t.TempDir(), "sess-1", "tester", AutoWorkflowConfig{
				AutoCommit: true,
				AutoFormat: true,
			})

			got := h.OnToolResult(toolName, `{"path":"main.go","content":"package main"}`, "wrote 12 bytes to main.go", false)

			if got != "" {
				t.Errorf("OnToolResult(%q) = %q, want \"\" — an automation has come alive. "+
					"That is the fix af237d64 asks for; update this test to assert what "+
					"it should now do.", toolName, got)
			}
			if len(h.changedFiles) != 1 || h.changedFiles[0] != "main.go" {
				t.Errorf("changedFiles = %v after a %q, want [main.go] — the session's "+
					"own writes are going unrecorded, so the close-time commit has "+
					"nothing to attribute and commits nothing (todo 7d8bbca1).",
					h.changedFiles, toolName)
			}
		})
	}
}

// The batch shape is the one the tools' own descriptions call preferred, and
// the path scanner this replaced could only ever see the first path in it. A
// batch of five writes recorded as one file is a close-time commit that leaves
// four of them uncommitted.
func TestEveryPathInABATCHWriteIsRecorded(t *testing.T) {
	write, edit := realWriteToolNames(t)

	cases := []struct {
		toolName string
		input    string
	}{
		{write, `{"files":[{"path":"a.go","content":"a"},{"path":"b.go","content":"b"}]}`},
		{edit, `{"edits":[{"path":"a.go","old_text":"a","new_text":"A"},{"path":"b.go","old_text":"b","new_text":"B"}]}`},
		// The single-file and batch forms are additive, not exclusive: both
		// tools prepend a top-level path to the batch when one is given.
		{write, `{"path":"a.go","content":"a","files":[{"path":"b.go","content":"b"}]}`},
	}

	for _, c := range cases {
		t.Run(c.toolName+c.input[:12], func(t *testing.T) {
			h := NewWorkflowHooks(t.TempDir(), "sess-1", "tester", AutoWorkflowConfig{})

			h.OnToolResult(c.toolName, c.input, "ok", false)

			if len(h.changedFiles) != 2 {
				t.Fatalf("changedFiles = %v, want both paths — a batch write is being "+
					"recorded as one file", h.changedFiles)
			}
		})
	}
}

// A tool that does not write files must not put anything on the list. Without
// this, "record everything" would pass every assertion above while attributing
// a `read_files` call to the commit.
func TestANonWritingToolRecordsNothing(t *testing.T) {
	h := NewWorkflowHooks(t.TempDir(), "sess-1", "tester", AutoWorkflowConfig{})

	h.OnToolResult("read_files", `{"path":"main.go"}`, "package main", false)
	h.OnToolResult("shell_commands", `{"command":"ls"}`, "main.go", false)

	if len(h.changedFiles) != 0 {
		t.Fatalf("changedFiles = %v after calls that wrote nothing", h.changedFiles)
	}
}

// A failed write changed nothing on disk, so recording it would hand git a path
// that either does not exist or does not differ.
func TestAFAILEDWriteRecordsNothing(t *testing.T) {
	write, _ := realWriteToolNames(t)
	h := NewWorkflowHooks(t.TempDir(), "sess-1", "tester", AutoWorkflowConfig{})

	h.OnToolResult(write, `{"path":"main.go","content":"package main"}`, "permission denied", true)

	if len(h.changedFiles) != 0 {
		t.Fatalf("changedFiles = %v after a write that failed", h.changedFiles)
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
