package agent

import (
	"strings"
	"testing"
)

// A tool_use block whose arguments do not parse as JSON is the one input that
// both riders on it — the "then" chain and the done/note/split sideband — are
// never even looked for. extractChain and extractSideband each give up on the
// first unmarshal, and that branch is the only one in either that returns
// without a reason: every other way a chain is taken out reports one, and every
// other way a rider is dropped records an ignore.
//
// This is a real path, not a hypothetical. A max_tokens cut returns as a clean
// turn and carries the half-written tool_use into the conversation, so a model
// that wrote {"path":"x.go","then":{"tool":"shell_commands", and was cut
// mid-argument gets one error naming the primary tool and no word that its
// follow-up was never considered.

// wantUnparsedInputNote is the whole line the model must get back. It is
// written out rather than assembled from the constants under test: a fixture
// built from chainNote and the reason string would still pass if both moved
// together, which is the drift it exists to catch.
const wantUnparsedInputNote = `--- then(then) not run: the tool input did not parse as JSON, ` +
	`so a "then" chain or a done/note/split rider in it — if there was one — was never read ---`

func TestAnUnparseableToolInputSaysItsRidersWereNeverRead(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{
			// The truncated case the finding is about: cut mid-chain, so a
			// "then" really is in there and really was never read.
			name:  "cut mid-chain",
			input: `{"path":"/a.go","then":{"tool":"shell","input":{"command":"go bu`,
		},
		{
			// Cut before any rider was written. Nobody can tell the two apart
			// from the bytes, which is why the note says "if there was one".
			name:  "cut before any rider",
			input: `{"path":"/a.g`,
		},
		{
			name:  "not JSON at all",
			input: `path=/a.go then=shell`,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			primary := &recordingTool{output: "read ok"}
			result := chainResult(t, &Agent{}, []Tool{primary.tool("primary")}, testCase.input)

			// The primary call still runs and still errors in its own words.
			// This note is an addition to what the model already gets, not a
			// replacement for it, and nothing here changes which tools run.
			if len(primary.inputs) != 1 {
				t.Fatalf("primary tool ran %d times, want 1", len(primary.inputs))
			}
			if primary.inputs[0] != testCase.input {
				t.Errorf("the primary tool was handed something other than the raw input\n got: %q\nwant: %q",
					primary.inputs[0], testCase.input)
			}
			if !strings.Contains(result, "read ok") {
				t.Errorf("result lost the primary output: %q", result)
			}
			if !strings.Contains(result, wantUnparsedInputNote) {
				t.Errorf("an unparseable tool input was dropped without a word\n got: %q\nwant it to contain: %q",
					result, wantUnparsedInputNote)
			}
		})
	}
}

// The note is for the input nobody could read. An input that parses has its own
// reasons for every rider it drops, and must not collect this one on top of
// them — a control, because a check that fires on every input would pass the
// test above without reporting anything the model did not already know.
func TestAParseableToolInputDoesNotCollectTheUnparsedNote(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{name: "no riders at all", input: `{"path":"/a.go"}`},
		{name: "a chain that runs", input: `{"path":"/a.go","then":{"tool":"shell","input":{}}}`},
		{name: "a chain that cannot run", input: `{"path":"/a.go","then":42}`},
		{name: "a rider that is ignored", input: `{"path":"/a.go","done":"all of them"}`},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			result := chainResult(t, &Agent{},
				[]Tool{(&recordingTool{output: "read ok"}).tool("primary"), (&recordingTool{output: "build ok"}).tool("shell")},
				testCase.input)

			if strings.Contains(result, "did not parse as JSON") {
				t.Errorf("a parseable input was reported as unparseable: %q", result)
			}
		})
	}
}
