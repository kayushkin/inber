package agent

import (
	"context"
	"strings"
	"testing"
)

// The sideband fields — done, note, split — are instructions the model writes
// into a tool call's arguments, and extractSideband deletes them from that
// input whatever becomes of them. So the same rule as the "then" chain applies:
// a field that was taken out and not carried out has to be reported on the tool
// result, because to the model a completed task and a silently discarded one
// look identical.

func sidebandSummary(t *testing.T, callbacks *SidebandCallbacks, input string) string {
	t.Helper()
	_, sb := extractSideband(input)
	return processSideband(context.Background(), sb, callbacks)
}

func TestASidebandFieldThatCannotBeReadSaysSo(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"done is not a list", `{"done":"the first one"}`, "[done ignored: it is not a list of task indices]"},
		{"done names nothing", `{"done":[]}`, "[done ignored: it names no task]"},
		{"note is not an object", `{"note":"remember this"}`, "[note ignored: it is not an object with a key and a value]"},
		{"note has no key", `{"note":{"value":"remember this"}}`, "[note ignored: it has no key to save under]"},
		{"split is not an object", `{"split":3}`, "[split ignored: it is not an object with an index and subtasks]"},
		{"split has no subtasks", `{"split":{"index":1,"into":[]}}`, "[split ignored: it names no subtasks to split into]"},
	}

	everyCallback := &SidebandCallbacks{
		CompleteTasks: func(context.Context, []int) error { return nil },
		SaveNote:      func(string, string) error { return nil },
		SplitTask:     func(int, []string) error { return nil },
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			summary := sidebandSummary(t, everyCallback, testCase.input)
			if !strings.Contains(summary, testCase.want) {
				t.Errorf("summary does not report the ignored field\n got: %q\nwant: %q", summary, testCase.want)
			}
		})
	}
}

// prepareTools injects the sideband schema into every tool of every agent, so
// any model can write "done" — including to an agent that has no task list
// behind it. That call used to disappear.
func TestASidebandFieldWithNothingBehindItSaysSo(t *testing.T) {
	cases := []struct {
		name      string
		callbacks *SidebandCallbacks
		input     string
		want      string
	}{
		{"no callbacks at all", nil, `{"done":[0]}`, "[done ignored: this agent has nothing to apply it to]"},
		{"no task list", &SidebandCallbacks{}, `{"done":[0,1]}`, "[done ignored: this agent has nothing to apply it to]"},
		{"no scratchpad", &SidebandCallbacks{}, `{"note":{"key":"k","value":"v"}}`, "[note ignored: this agent has nothing to apply it to]"},
		{"no plan to split", &SidebandCallbacks{}, `{"split":{"index":0,"into":["a","b"]}}`, "[split ignored: this agent has nothing to apply it to]"},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			summary := sidebandSummary(t, testCase.callbacks, testCase.input)
			if !strings.Contains(summary, testCase.want) {
				t.Errorf("summary does not report the unhandled field\n got: %q\nwant: %q", summary, testCase.want)
			}
		})
	}
}

// The reporting is additional, not a replacement: a field that works still
// reports what it did, and one bad field does not take a good one with it.
func TestAWorkingSidebandFieldStillReportsWhatItDid(t *testing.T) {
	var completed []int
	callbacks := &SidebandCallbacks{
		CompleteTasks: func(_ context.Context, indices []int) error {
			completed = indices
			return nil
		},
		SaveNote: func(string, string) error { return nil },
	}

	summary := sidebandSummary(t, callbacks, `{"done":[2],"note":"not an object"}`)

	if len(completed) != 1 || completed[0] != 2 {
		t.Fatalf("the good field did not run: completed=%v", completed)
	}
	if !strings.Contains(summary, "[✓ completed 1 task(s)]") {
		t.Errorf("summary lost the completed task: %q", summary)
	}
	if !strings.Contains(summary, "[note ignored:") {
		t.Errorf("summary lost the ignored note: %q", summary)
	}
}

// A tool call with no sideband fields says nothing, which is what keeps the
// reporting free: the fields are advertised on every tool of every request and
// used on almost none of them.
func TestAToolCallWithNoSidebandFieldsSaysNothing(t *testing.T) {
	if summary := sidebandSummary(t, &SidebandCallbacks{}, `{"path":"/a.go"}`); summary != "" {
		t.Errorf("a plain tool call reported %q, want nothing", summary)
	}
}

// The sideband callbacks fire before the primary tool is even dispatched, so by
// the time a call is refused or fails, the task really has been completed and
// the note really has been saved. That report used to be thrown away with the
// primary tool's output, which is the same silence one level up: work that
// happened, and a model that was never told.
func TestTheSidebandReportSurvivesACallThatDoesNotRun(t *testing.T) {
	completed := 0
	a := &Agent{
		sidebandCallbacks: &SidebandCallbacks{
			CompleteTasks: func(context.Context, []int) error {
				completed++
				return nil
			},
		},
		ToolRefusal: func(string, string) string { return "not in this mode" },
	}

	outputs := runBlocks(t, a, []Tool{(&recordingTool{}).tool("primary")},
		toolUseBlock("t1", "primary", `{"done":[0]}`))

	if completed != 1 {
		t.Fatalf("the sideband callback fired %d times, want 1", completed)
	}
	if len(outputs) != 1 {
		t.Fatalf("got %d tool results, want 1", len(outputs))
	}
	if !strings.Contains(outputs[0], "[✓ completed 1 task(s)]") {
		t.Errorf("a refused call swallowed the sideband report: %q", outputs[0])
	}
	if !strings.Contains(outputs[0], "refused: primary was not run") {
		t.Errorf("result lost the refusal: %q", outputs[0])
	}
}
