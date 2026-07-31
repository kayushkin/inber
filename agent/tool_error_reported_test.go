package agent

import (
	"errors"
	"strings"
	"testing"
)

// A tool call has three ways to end without the tool ever answering: the gate
// refuses it, the name matches no tool, or the tool runs and returns an error.
// All three hand the model an error tool_result, and all three used to reach
// OnToolResult never — so the display showed the call going out and nothing
// coming back, the session log recorded a tool_use with no tool_result, and
// ConsecutiveErrors (incremented only in that hook) stayed at zero, which left
// the whole error-recovery context ladder unreachable.
//
// These tests pin that every ending of a tool call is reported once, with the
// text the model was given.

// toolResultReport is one OnToolResult call.
type toolResultReport struct {
	toolID  string
	name    string
	output  string
	isError bool
}

// reportingAgent returns an agent whose OnToolResult calls land in the returned
// slice, in order.
func reportingAgent() (*Agent, *[]toolResultReport) {
	var reports []toolResultReport
	a := &Agent{hooks: &Hooks{
		OnToolResult: func(toolID, name, output string, isError bool) {
			reports = append(reports, toolResultReport{toolID, name, output, isError})
		},
	}}
	return a, &reports
}

func TestEveryFailedToolCallIsReported(t *testing.T) {
	cases := []struct {
		name string
		// what the block asks for
		tools   []Tool
		call    string
		refusal func(tool, input string) string
		// what the report has to say
		wantIn string
	}{
		{
			name:   "the tool ran and returned an error",
			tools:  []Tool{(&recordingTool{err: errors.New("no such file")}).tool("primary")},
			call:   `{"path":"/gone.go"}`,
			wantIn: "no such file",
		},
		{
			name:   "the call names no tool",
			tools:  []Tool{(&recordingTool{output: "ok"}).tool("something_else")},
			call:   `{"path":"/a.go"}`,
			wantIn: `unknown tool "primary"`,
		},
		{
			name:    "the gate refused the call",
			tools:   []Tool{(&recordingTool{output: "ok"}).tool("primary")},
			call:    `{"path":"/a.go"}`,
			refusal: func(string, string) string { return "outside the workspace" },
			wantIn:  "outside the workspace",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a, reports := reportingAgent()
			a.ToolRefusal = tc.refusal

			outputs := runBlocks(t, a, tc.tools, toolUseBlock("t1", "primary", tc.call))
			if len(outputs) != 1 {
				t.Fatalf("got %d tool results, want 1", len(outputs))
			}

			if len(*reports) != 1 {
				t.Fatalf("OnToolResult called %d times, want 1 — "+
					"the failure reaches neither the display nor the session log", len(*reports))
			}
			got := (*reports)[0]
			if !got.isError {
				t.Errorf("reported isError=false; ConsecutiveErrors is incremented only when it is true")
			}
			if got.toolID != "t1" {
				t.Errorf("reported under tool id %q, want %q — the log pairs the result with its tool_use by id", got.toolID, "t1")
			}
			if got.name != "primary" {
				t.Errorf("reported under tool name %q, want %q", got.name, "primary")
			}
			if !strings.Contains(got.output, tc.wantIn) {
				t.Errorf("reported output %q does not say why the call failed (want it to contain %q)", got.output, tc.wantIn)
			}
			// The model and the log have to be told the same thing.
			if got.output != outputs[0] {
				t.Errorf("reported %q but the model was given %q", got.output, outputs[0])
			}
		})
	}
}

// A call that succeeds is reported exactly once, by the dispatcher. Reporting
// it again in the caller would double-count it into ConsecutiveErrors' sibling
// bookkeeping and print it twice.
func TestASucceedingToolCallIsReportedOnce(t *testing.T) {
	a, reports := reportingAgent()
	primary := &recordingTool{output: "read ok"}

	runBlocks(t, a, []Tool{primary.tool("primary")}, toolUseBlock("t1", "primary", `{"path":"/a.go"}`))

	if len(*reports) != 1 {
		t.Fatalf("OnToolResult called %d times for one succeeding call, want 1", len(*reports))
	}
	if (*reports)[0].isError {
		t.Errorf("a succeeding call was reported as an error")
	}
	if (*reports)[0].output != "read ok" {
		t.Errorf("reported %q, want %q", (*reports)[0].output, "read ok")
	}
}

// Each block in a turn is reported under its own id, so a turn that mixes
// working and failing calls does not lose the failures among the successes.
// This is the shape the defect was measured with: three blocks, three tool
// results, zero reports.
func TestAMixedTurnReportsEveryBlock(t *testing.T) {
	a, reports := reportingAgent()
	a.ToolRefusal = func(tool, _ string) string {
		if tool == "refused" {
			return "not allowed here"
		}
		return ""
	}
	tools := []Tool{
		(&recordingTool{output: "read ok"}).tool("works"),
		(&recordingTool{err: errors.New("boom")}).tool("fails"),
		(&recordingTool{output: "ok"}).tool("refused"),
	}

	outputs := runBlocks(t, a, tools,
		toolUseBlock("t1", "works", `{}`),
		toolUseBlock("t2", "fails", `{}`),
		toolUseBlock("t3", "missing", `{}`),
		toolUseBlock("t4", "refused", `{}`),
	)
	if len(outputs) != 4 {
		t.Fatalf("got %d tool results, want 4", len(outputs))
	}

	if len(*reports) != 4 {
		t.Fatalf("OnToolResult called %d times for 4 calls, want 4", len(*reports))
	}
	wantError := map[string]bool{"t1": false, "t2": true, "t3": true, "t4": true}
	for _, got := range *reports {
		want, known := wantError[got.toolID]
		if !known {
			t.Errorf("report under unexpected tool id %q", got.toolID)
			continue
		}
		if got.isError != want {
			t.Errorf("%s reported isError=%v, want %v", got.toolID, got.isError, want)
		}
		delete(wantError, got.toolID)
	}
	for id := range wantError {
		t.Errorf("%s was never reported", id)
	}
}
