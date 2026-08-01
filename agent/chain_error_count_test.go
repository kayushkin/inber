package agent

import (
	"context"
	"errors"
	"testing"
)

// One tool_use block is one thing the model asked for, so it may report at most
// one failure — whatever else rode along inside its arguments.
//
// The counter these tests are really about is engine.Turn.ConsecutiveErrors,
// which is incremented in exactly one place: the OnToolResult hook, whenever it
// is handed isError. It drives the error-recovery context ladder in
// engine/contextBudget — 1 error widens memory recall from 6,000 tokens to
// 20,000, 3 errors to 35,000, 5 to 50,000, each of which also rewrites the
// cached system-prompt prefix and pays for the whole prompt again.
//
// A block whose primary call is refused and that also carried a "then" reported
// itself twice: once from the deferred chain note inside executeWithChain, and
// once from the dispatcher that reports the errors executeWithChain hands back.
// Three ordinary policy refusals therefore climbed the ladder as if there had
// been six failures.
//
// errorReports counts the error-flagged tool results a single block produces.
func errorReports(t *testing.T, a *Agent, tools []Tool, input string) int {
	t.Helper()

	errors := 0
	hooks := a.hooks
	if hooks == nil {
		hooks = &Hooks{}
	}
	previous := hooks.OnToolResult
	hooks.OnToolResult = func(toolID, name, output string, isError bool) {
		if isError {
			errors++
		}
		if previous != nil {
			previous(toolID, name, output, isError)
		}
	}
	a.hooks = hooks

	runBlocks(t, a, tools, toolUseBlock("t1", "primary", input))
	return errors
}

func refuseNamed(refused string) func(tool, input string) string {
	return func(tool, input string) string {
		if tool == refused {
			return "policy says no"
		}
		return ""
	}
}

// The four ways a block can fail once and be counted twice. In every one of
// them the primary call is the failure and the chain note is a consequence of
// it, not a second independent thing going wrong.
func TestABlockThatFailsOnceIsReportedAsOneError(t *testing.T) {
	failing := Tool{
		Name: "primary",
		Run:  func(context.Context, string) (string, error) { return "", errors.New("disk full") },
	}
	ok := &recordingTool{output: "read ok"}
	chained := &recordingTool{output: "build ok"}

	cases := []struct {
		name  string
		agent *Agent
		tools []Tool
		input string
	}{
		{
			name:  "the primary call is refused and the block carried a chain",
			agent: &Agent{ToolRefusal: refuseNamed("primary")},
			tools: []Tool{ok.tool("primary"), chained.tool("shell")},
			input: `{"path":"/a.go","then":{"tool":"shell","input":{"command":"go build"}}}`,
		},
		{
			name:  "the primary call names no tool and the block carried a chain",
			agent: &Agent{},
			tools: []Tool{chained.tool("shell")},
			input: `{"path":"/a.go","then":{"tool":"shell","input":{"command":"go build"}}}`,
		},
		{
			name:  "the primary tool fails and the block carried a chain",
			agent: &Agent{},
			tools: []Tool{failing, chained.tool("shell")},
			input: `{"path":"/a.go","then":{"tool":"shell","input":{"command":"go build"}}}`,
		},
		{
			name:  "the primary call is refused and the chain was malformed",
			agent: &Agent{ToolRefusal: refuseNamed("primary")},
			tools: []Tool{ok.tool("primary")},
			input: `{"path":"/a.go","then":42}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := errorReports(t, tc.agent, tc.tools, tc.input); got != 1 {
				t.Errorf("the block reported %d errors, want 1 — "+
					"ConsecutiveErrors climbs the recovery ladder %dx too fast", got, got)
			}
		})
	}
}

// The mirror of the above: when the primary call succeeds, nobody else reports
// a failure for the block, so the chain note is the only record that part of
// what the model asked for did not happen. It must still count, or a session
// that fails only its chains never widens its recall at all.
func TestAChainThatIsTheOnlyFailureStillReportsOne(t *testing.T) {
	primary := &recordingTool{output: "read ok"}

	cases := []struct {
		name  string
		agent *Agent
		tools []Tool
		input string
	}{
		{
			name:  "the chain was malformed",
			agent: &Agent{},
			tools: []Tool{primary.tool("primary")},
			input: `{"path":"/a.go","then":42}`,
		},
		{
			name:  "the chain names a tool that does not exist",
			agent: &Agent{},
			tools: []Tool{primary.tool("primary")},
			input: `{"path":"/a.go","then":{"tool":"deploy","input":{}}}`,
		},
		{
			name:  "the chain is refused",
			agent: &Agent{ToolRefusal: refuseNamed("shell")},
			tools: []Tool{primary.tool("primary"), (&recordingTool{output: "build ok"}).tool("shell")},
			input: `{"path":"/a.go","then":{"tool":"shell","input":{"command":"go build"}}}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := errorReports(t, tc.agent, tc.tools, tc.input); got != 1 {
				t.Errorf("the block reported %d errors, want 1 — "+
					"a chain that did not run is the block's one failure", got)
			}
		})
	}
}

// The counts that were already right, kept so the fix cannot buy the cases
// above by breaking these.
func TestBlocksThatAlreadyCountedCorrectlyStillDo(t *testing.T) {
	primary := &recordingTool{output: "read ok"}
	chained := &recordingTool{output: "build ok"}

	t.Run("a refused call with no chain reports one", func(t *testing.T) {
		got := errorReports(t, &Agent{ToolRefusal: refuseNamed("primary")},
			[]Tool{primary.tool("primary")}, `{"path":"/a.go"}`)
		if got != 1 {
			t.Errorf("got %d errors, want 1", got)
		}
	})

	t.Run("a chained tool that runs and fails reports one", func(t *testing.T) {
		failing := Tool{
			Name: "shell",
			Run:  func(context.Context, string) (string, error) { return "", errors.New("exit 1") },
		}
		got := errorReports(t, &Agent{},
			[]Tool{primary.tool("primary"), failing},
			`{"path":"/a.go","then":{"tool":"shell","input":{"command":"go build"}}}`)
		if got != 1 {
			t.Errorf("got %d errors, want 1", got)
		}
	})

	t.Run("a block where everything works reports none", func(t *testing.T) {
		got := errorReports(t, &Agent{},
			[]Tool{primary.tool("primary"), chained.tool("shell")},
			`{"path":"/a.go","then":{"tool":"shell","input":{"command":"go build"}}}`)
		if got != 0 {
			t.Errorf("got %d errors, want 0", got)
		}
	})
}
