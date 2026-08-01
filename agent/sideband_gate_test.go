package agent

import (
	"context"
	"strings"
	"testing"
)

// The sideband fields are the third thing a tool_use block can ask for, after
// the primary call and the "then" chain, and they are the one the gate never
// saw. Each of them writes a file, and a `done` that empties the task plan runs
// the project's build command through `bash -c`. So a gate on the primary call
// alone is the same hole the chain used to be, one field over: ask for a read,
// ride a `done` on it.
//
// These tests drive the gate at the same level TestARefusedToolNeverRuns does —
// the assertion is on whether the callback ran, not on the text handed back.

// observeGate stands in for the engine's guard closure under Observe: the read
// tools are allowed, everything else — including anything it has never heard
// of, which is what a sideband field is — is refused. That fall-through is
// guard.CheckTool's own shape, not an invention of this test.
func observeGate() func(tool, input string) string {
	readOnly := map[string]bool{
		"read_files": true, "list_files": true, "ripgrep": true,
		"memory_expand": true, "memory_search": true,
		"repo_map": true, "recent_files": true, "web_search": true,
	}
	return func(tool, _ string) string {
		if readOnly[tool] {
			return ""
		}
		return "observe mode allows read-only tools only"
	}
}

// sidebandRecorder counts each callback so a test can assert on what actually
// ran rather than on what was reported.
type sidebandRecorder struct {
	completed, noted, split int
}

func (r *sidebandRecorder) callbacks() *SidebandCallbacks {
	return &SidebandCallbacks{
		CompleteTasks: func(context.Context, []int) error { r.completed++; return nil },
		SaveNote:      func(string, string) error { r.noted++; return nil },
		SplitTask:     func(int, []string) error { r.split++; return nil },
	}
}

// TestObserveModeRefusesTheSidebandFields is the property this gate exists for.
// Observe is documented as read-only — no file writes, no shell execution — and
// that has to be true of the whole block, not of its primary call.
func TestObserveModeRefusesTheSidebandFields(t *testing.T) {
	var recorder sidebandRecorder

	summary := sidebandSummaryGated(t, recorder.callbacks(),
		`{"done":[0],"note":{"key":"k","value":"v"},"split":{"index":0,"into":["a","b"]}}`,
		observeGate())

	if recorder.completed+recorder.noted+recorder.split != 0 {
		t.Errorf("observe mode applied sideband fields: %d completed, %d noted, %d split",
			recorder.completed, recorder.noted, recorder.split)
	}
	for _, field := range []string{"done", "note", "split"} {
		want := "refused: " + field + " was not run"
		if !strings.Contains(summary, want) {
			t.Errorf("summary does not report %s as refused: %q", field, summary)
		}
	}
	if !strings.Contains(summary, "observe mode") {
		t.Errorf("summary does not say which mode refused the fields: %q", summary)
	}
}

// TestAnAllowedToolCannotCarryARefusedSidebandField is the case a gate on the
// primary call cannot see, and the reason the fields needed their own answer
// rather than being refused alongside a refused primary. read_files is allowed
// in observe mode; the `done` riding on it reaches saveTaskPlan and, when it
// empties the plan, `bash -c`.
func TestAnAllowedToolCannotCarryARefusedSidebandField(t *testing.T) {
	var recorder sidebandRecorder
	reader := &recordingTool{output: "package main"}

	a := &Agent{sidebandCallbacks: recorder.callbacks(), ToolRefusal: observeGate()}
	outputs := runBlocks(t, a, []Tool{reader.tool("read_files")},
		toolUseBlock("t1", "read_files", `{"path":"/a.go","done":[0]}`))

	if recorder.completed != 0 {
		t.Errorf("a sideband field rode into observe mode on an allowed tool and ran %d time(s)", recorder.completed)
	}
	if len(reader.inputs) != 1 {
		t.Errorf("the allowed primary call ran %d time(s), want 1 — refusing the rider must not refuse the call", len(reader.inputs))
	}
	if len(outputs) != 1 || !strings.Contains(outputs[0], "refused: done was not run") {
		t.Errorf("tool result = %q, want the refused rider reported to the model", outputs)
	}
}

// TestTheSidebandFieldsGoToTheGateUnderANameNoToolCanHave. The three field
// names are strings the model writes into a tool's arguments, so gating them
// under those bare names would put a rider and any tool that ever answers to
// the same name on one key — the collision would be silent and would land
// whichever way the gate happened to classify it.
func TestTheSidebandFieldsGoToTheGateUnderANameNoToolCanHave(t *testing.T) {
	var asked []string
	record := func(tool, _ string) string {
		asked = append(asked, tool)
		return ""
	}

	sidebandSummaryGated(t, &SidebandCallbacks{},
		`{"done":[0],"note":{"key":"k","value":"v"},"split":{"index":0,"into":["a"]}}`, record)

	want := []string{"sideband:done", "sideband:note", "sideband:split"}
	if len(asked) != len(want) {
		t.Fatalf("the gate was asked about %v, want %v", asked, want)
	}
	for i, name := range want {
		if asked[i] != name {
			t.Errorf("the gate was asked about %q, want %q", asked[i], name)
		}
	}
}

// TestTheGateSeesWhatTheFieldActuallyAsksFor. The gate is handed the field's
// own value, not the tool's arguments it was stripped out of. An approver that
// is shown "{}" for every rider cannot tell a note from a `done` that is about
// to run the build.
func TestTheGateSeesWhatTheFieldActuallyAsksFor(t *testing.T) {
	seen := map[string]string{}
	record := func(tool, input string) string {
		seen[tool] = input
		return ""
	}

	sidebandSummaryGated(t, &SidebandCallbacks{},
		`{"done":[2,3],"note":{"key":"layout","value":"cmd/ is the entrypoint"}}`, record)

	if got := seen["sideband:done"]; got != "[2,3]" {
		t.Errorf("the gate saw %q for done, want the task indices it would complete", got)
	}
	if got := seen["sideband:note"]; !strings.Contains(got, "layout") {
		t.Errorf("the gate saw %q for note, want the note it would write", got)
	}
}

// TestASessionWithNoGateAppliesEverySidebandField. Almost every session on this
// host runs with no mode and therefore no gate, and the fields have to keep
// working there exactly as before — a fail-closed default would break every one
// of them, which is worse than the hole it shuts.
func TestASessionWithNoGateAppliesEverySidebandField(t *testing.T) {
	var recorder sidebandRecorder

	summary := sidebandSummaryGated(t, recorder.callbacks(),
		`{"done":[0],"note":{"key":"k","value":"v"},"split":{"index":0,"into":["a","b"]}}`, nil)

	if recorder.completed != 1 || recorder.noted != 1 || recorder.split != 1 {
		t.Errorf("an ungated session applied %d completed, %d noted, %d split — want 1 of each",
			recorder.completed, recorder.noted, recorder.split)
	}
	if strings.Contains(summary, "refused") {
		t.Errorf("an ungated session refused a sideband field: %q", summary)
	}
}
