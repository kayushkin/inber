package agent

import (
	"errors"
	"strings"
	"testing"
)

// A "then" chain is a second tool call the model wrote inside the first one's
// arguments, and the field is deleted from that input whether or not it can be
// run. So a chain that does not run has exactly one way to reach the model: the
// tool result it rode in on. These tests pin that every way of not running says
// so there.
//
// The measurement behind them: 95 server-session transcripts on this host hold
// 1,365 tool_use blocks, exactly one of which carried a "then" — and that one
// was thrown away in silence, an edit_files follow-up the model never learned
// had not happened.

func chainResult(t *testing.T, a *Agent, tools []Tool, input string) string {
	t.Helper()
	outputs := runBlocks(t, a, tools, toolUseBlock("t1", "primary", input))
	if len(outputs) != 1 {
		t.Fatalf("got %d tool results, want 1", len(outputs))
	}
	return outputs[0]
}

// The model writes the chain object as a JSON string rather than an object.
// This is not hypothetical: it is the only "then" chain in the transcripts on
// this host. The payload is the model's own — only its quoting differs — so the
// chained call runs.
func TestAThenChainWrittenAsTextStillRuns(t *testing.T) {
	chained := &recordingTool{output: "build ok"}
	primary := &recordingTool{output: "read ok"}

	result := chainResult(t, &Agent{},
		[]Tool{primary.tool("primary"), chained.tool("shell")},
		`{"path":"/a.go","then":"{\"tool\":\"shell\",\"input\":{\"command\":\"go build\"}}"}`)

	if len(chained.inputs) != 1 {
		t.Fatalf("chained tool ran %d times, want 1 — the chain was dropped", len(chained.inputs))
	}
	if !strings.Contains(chained.inputs[0], "go build") {
		t.Errorf("chained tool got the wrong input: %q", chained.inputs[0])
	}
	if !strings.Contains(result, "--- then(shell) ---") || !strings.Contains(result, "build ok") {
		t.Errorf("result does not report the chained call: %q", result)
	}
	if strings.Contains(primary.inputs[0], "then") {
		t.Errorf("the chain field was left in the primary tool's input: %q", primary.inputs[0])
	}
}

// Everything else that arrives in the "then" field is a follow-up the model
// asked for and is not going to get, and each one has to say which.
func TestAChainThatCannotRunSaysWhy(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "not an object",
			input: `{"then":42}`,
			want:  "--- then(then) not run: it is not an object with a tool and an input ---",
		},
		{
			name:  "text that is not a chain",
			input: `{"then":"run the build"}`,
			want:  "--- then(then) not run: it arrived as text that does not read as {tool, input} ---",
		},
		{
			name:  "no tool named",
			input: `{"then":{"input":{"command":"go build"}}}`,
			want:  "--- then(then) not run: it names no tool ---",
		},
		{
			name:  "a tool that does not exist",
			input: `{"then":{"tool":"deploy","input":{}}}`,
			want:  "--- then(deploy) not run: no tool of that name ---",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			primary := &recordingTool{output: "read ok"}
			result := chainResult(t, &Agent{}, []Tool{primary.tool("primary")}, testCase.input)

			if len(primary.inputs) != 1 {
				t.Fatalf("primary tool ran %d times, want 1", len(primary.inputs))
			}
			if !strings.Contains(result, "read ok") {
				t.Errorf("result lost the primary output: %q", result)
			}
			if !strings.Contains(result, testCase.want) {
				t.Errorf("result does not say why the chain did not run\n got: %q\nwant: %q", result, testCase.want)
			}
		})
	}
}

// A chain that parsed still does not run when the call it rode in on does not,
// and those paths return before the chain is ever reached. They were the
// silent ones: the model got an error about the primary call and no word at all
// about the follow-up.
func TestAChainSaysSoWhenThePrimaryCallDoesNotRun(t *testing.T) {
	chain := `,"then":{"tool":"shell","input":{"command":"go build"}}`

	t.Run("primary failed", func(t *testing.T) {
		primary := &recordingTool{err: errors.New("disk full")}
		chained := &recordingTool{output: "build ok"}

		result := chainResult(t, &Agent{},
			[]Tool{primary.tool("primary"), chained.tool("shell")},
			`{"path":"/a.go"`+chain+`}`)

		if len(chained.inputs) != 0 {
			t.Fatalf("the chain ran after the primary call failed: %v", chained.inputs)
		}
		if !strings.Contains(result, "--- then(shell) not run: the call it was attached to failed — disk full ---") {
			t.Errorf("result does not report the unrun chain: %q", result)
		}
	})

	t.Run("primary refused", func(t *testing.T) {
		chained := &recordingTool{output: "build ok"}
		a := &Agent{ToolRefusal: func(tool, _ string) string {
			if tool == "primary" {
				return "not in this mode"
			}
			return ""
		}}

		result := chainResult(t, a,
			[]Tool{(&recordingTool{}).tool("primary"), chained.tool("shell")},
			`{"path":"/a.go"`+chain+`}`)

		if len(chained.inputs) != 0 {
			t.Fatalf("the chain ran after the primary call was refused: %v", chained.inputs)
		}
		if !strings.Contains(result, "--- then(shell) not run: the call it was attached to was refused — not in this mode ---") {
			t.Errorf("result does not report the unrun chain: %q", result)
		}
	})

	t.Run("primary names no tool", func(t *testing.T) {
		chained := &recordingTool{output: "build ok"}

		outputs := runBlocks(t, &Agent{}, []Tool{chained.tool("shell")},
			toolUseBlock("t1", "primary", `{"path":"/a.go"`+chain+`}`))

		if len(chained.inputs) != 0 {
			t.Fatalf("the chain ran off an unknown primary tool: %v", chained.inputs)
		}
		if !strings.Contains(outputs[0], `--- then(shell) not run: the call it was attached to names no tool "primary" ---`) {
			t.Errorf("result does not report the unrun chain: %q", outputs[0])
		}
	})
}

// A refused chain keeps the wording every other refusal uses. The gate reads
// the same wherever the model runs into it — that is what RefuseToolCall is
// for — and a chained call is not an exception to it.
func TestARefusedChainReadsLikeEveryOtherRefusal(t *testing.T) {
	chained := &recordingTool{output: "build ok"}
	a := &Agent{ToolRefusal: func(tool, _ string) string {
		if tool == "shell" {
			return "not in this mode"
		}
		return ""
	}}

	result := chainResult(t, a,
		[]Tool{(&recordingTool{output: "read ok"}).tool("primary"), chained.tool("shell")},
		`{"path":"/a.go","then":{"tool":"shell","input":{"command":"go build"}}}`)

	if len(chained.inputs) != 0 {
		t.Fatalf("a refused chain ran anyway: %v", chained.inputs)
	}
	if !strings.Contains(result, "--- then(shell) refused: shell was not run — not in this mode ---") {
		t.Errorf("a refused chain does not read like a refusal: %q", result)
	}
}

// The text is what the model reads; the hooks are what the trace and the UI
// read. A nested call that ran already reports through both, and one that did
// not has to do the same or it is invisible to everything downstream.
func TestAnUnrunChainIsReportedToTheHooksAsWell(t *testing.T) {
	type toolResult struct {
		toolID  string
		name    string
		output  string
		isError bool
	}
	var results []toolResult
	var calls []string

	a := &Agent{hooks: &Hooks{
		OnToolCall: func(toolID, name string, _ []byte) {
			calls = append(calls, toolID+"/"+name)
		},
		OnToolResult: func(toolID, name, output string, isError bool) {
			results = append(results, toolResult{toolID, name, output, isError})
		},
	}}

	chainResult(t, a, []Tool{(&recordingTool{output: "read ok"}).tool("primary")},
		`{"then":{"tool":"deploy","input":{}}}`)

	var reported *toolResult
	for i := range results {
		if results[i].toolID == "t1-chain" {
			reported = &results[i]
		}
	}
	if reported == nil {
		t.Fatalf("no chain result reached the hooks: %+v", results)
	}
	if !reported.isError {
		t.Error("a chain that did not run was reported to the hooks as a success")
	}
	if reported.name != "deploy" {
		t.Errorf("the unrun chain was reported under %q, want the tool it named", reported.name)
	}
	if !strings.Contains(reported.output, "no tool of that name") {
		t.Errorf("the hook was not told why: %q", reported.output)
	}
	_ = calls
}

// A malformed chain has no tool name to report under, so it is reported under
// the field's own name — and it still reaches the hooks with the bytes the
// model actually sent, which is the only place they survive.
func TestAMalformedChainReachesTheHooksWithItsOwnBytes(t *testing.T) {
	var calls []struct {
		name  string
		input string
	}

	a := &Agent{hooks: &Hooks{
		OnToolCall: func(toolID, name string, input []byte) {
			if toolID == "t1-chain" {
				calls = append(calls, struct {
					name  string
					input string
				}{name, string(input)})
			}
		},
	}}

	chainResult(t, a, []Tool{(&recordingTool{output: "read ok"}).tool("primary")},
		`{"then":{"input":{"command":"go build"}}}`)

	if len(calls) != 1 {
		t.Fatalf("got %d chain calls at the hooks, want 1", len(calls))
	}
	if calls[0].name != chainField {
		t.Errorf("malformed chain reported as %q, want %q", calls[0].name, chainField)
	}
	if !strings.Contains(calls[0].input, "go build") {
		t.Errorf("the hook did not get what the model wrote: %q", calls[0].input)
	}
}
