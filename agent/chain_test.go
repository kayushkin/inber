package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// recordingTool is a tool that remembers the inputs it was run with and answers
// with a fixed output.
type recordingTool struct {
	inputs []string
	output string
	err    error
}

func (rt *recordingTool) tool(name string) Tool {
	return Tool{
		Name: name,
		Run: func(_ context.Context, input string) (string, error) {
			rt.inputs = append(rt.inputs, input)
			return rt.output, rt.err
		},
	}
}

func toolUseBlock(id, name, input string) anthropic.ContentBlockUnion {
	return anthropic.ContentBlockUnion{
		Type:  "tool_use",
		ID:    id,
		Name:  name,
		Input: json.RawMessage(input),
	}
}

// runBlocks runs one turn's worth of tool_use blocks through executeTools and
// returns the final text of each tool result, in order.
func runBlocks(t *testing.T, a *Agent, tools []Tool, blocks ...anthropic.ContentBlockUnion) []string {
	t.Helper()
	a.tools = tools
	toolMap := make(map[string]Tool, len(tools))
	for _, tl := range tools {
		toolMap[tl.Name] = tl
	}

	var outputs []string
	hooks := a.hooks
	if hooks == nil {
		hooks = &Hooks{}
	}
	// ModifyToolResult sees the exact text that goes into the conversation.
	previous := hooks.ModifyToolResult
	hooks.ModifyToolResult = func(toolID, name, output string, isError bool) string {
		outputs = append(outputs, output)
		if previous != nil {
			return previous(toolID, name, output, isError)
		}
		return ""
	}
	a.hooks = hooks

	resp := &anthropic.Message{Content: blocks}
	result := &TurnResult{}
	if _, _, err := a.executeTools(context.Background(), resp, &toolInfo{toolMap: toolMap}, result); err != nil {
		t.Fatalf("executeTools: %v", err)
	}
	return outputs
}

// A cached read answers with a stub instead of the file. The chained call that
// rode in on the same tool_use block is a separate instruction and must still
// run — it used to be dropped with no log, no error and no marker, so the model
// believed it had run.
func TestCachedReadStillRunsThenChain(t *testing.T) {
	a := &Agent{readCache: NewReadCache()}
	a.readCache.RecordFullRead("/a.go", 1, 12)

	chained := &recordingTool{output: "build ok"}
	reader := &recordingTool{output: "should not be read again"}

	outputs := runBlocks(t, a,
		[]Tool{reader.tool("read_files"), chained.tool("shell")},
		toolUseBlock("t1", "read_files",
			`{"path":"/a.go","then":{"tool":"shell","input":{"command":"go build"}}}`),
	)

	if len(chained.inputs) != 1 {
		t.Fatalf("chained tool ran %d times, want 1 — the chain was dropped", len(chained.inputs))
	}
	if len(reader.inputs) != 0 {
		t.Fatalf("cached file was re-read: %v", reader.inputs)
	}
	if len(outputs) != 1 {
		t.Fatalf("got %d tool results, want 1", len(outputs))
	}
	if !strings.Contains(outputs[0], "already in context") {
		t.Errorf("result lost the cache stub: %q", outputs[0])
	}
	if !strings.Contains(outputs[0], "--- then(shell) ---") {
		t.Errorf("result does not report the chained call: %q", outputs[0])
	}
	if !strings.Contains(outputs[0], "build ok") {
		t.Errorf("result lost the chained output: %q", outputs[0])
	}
}

// The sideband fields (done, note, split) ride in on the same input as the
// chain and were dropped by the same short-circuit.
func TestCachedReadStillProcessesSideband(t *testing.T) {
	a := &Agent{readCache: NewReadCache()}
	a.readCache.RecordFullRead("/a.go", 1, 12)

	var completed []int
	var noteKey, noteValue string
	a.sidebandCallbacks = &SidebandCallbacks{
		CompleteTasks: func(indices []int) error { completed = append(completed, indices...); return nil },
		SaveNote:      func(key, value string) error { noteKey, noteValue = key, value; return nil },
	}

	reader := &recordingTool{output: "should not be read again"}
	runBlocks(t, a,
		[]Tool{reader.tool("read_files")},
		toolUseBlock("t1", "read_files",
			`{"path":"/a.go","done":[2],"note":{"key":"layout","value":"agent/ holds the loop"}}`),
	)

	if len(completed) != 1 || completed[0] != 2 {
		t.Errorf("done was dropped on a cached read: %v", completed)
	}
	if noteKey != "layout" || noteValue != "agent/ holds the loop" {
		t.Errorf("note was dropped on a cached read: %q=%q", noteKey, noteValue)
	}
}

// Each call in a block is recorded from its own output. Reading the combined
// text credited the primary read with the line count the chained read printed.
func TestChainedReadIsRecordedUnderItsOwnPath(t *testing.T) {
	a := &Agent{readCache: NewReadCache()}

	reads := map[string]string{
		"/a.go": "package a\n[complete file — 12 lines]",
		"/b.go": "package b\n[complete file — 99 lines]",
	}
	reader := Tool{
		Name: "read_files",
		Run: func(_ context.Context, input string) (string, error) {
			var in struct {
				Path string `json:"path"`
			}
			if err := json.Unmarshal([]byte(input), &in); err != nil {
				return "", err
			}
			out, ok := reads[in.Path]
			if !ok {
				return "", fmt.Errorf("no such file %q", in.Path)
			}
			return out, nil
		},
	}

	runBlocks(t, a, []Tool{reader},
		toolUseBlock("t1", "read_files",
			`{"path":"/a.go","then":{"tool":"read_files","input":{"path":"/b.go"}}}`),
	)

	stubA, cachedA := a.readCache.Check("/a.go")
	if !cachedA {
		t.Fatal("primary read was not cached")
	}
	if !strings.Contains(stubA, "12 lines") {
		t.Errorf("/a.go recorded with the chained read's line count: %q", stubA)
	}
	stubB, cachedB := a.readCache.Check("/b.go")
	if !cachedB {
		t.Fatal("chained read was not cached at all")
	}
	if !strings.Contains(stubB, "99 lines") {
		t.Errorf("/b.go recorded with the wrong line count: %q", stubB)
	}
}

// A write reached through a chain invalidates the cache exactly as a primary
// write does. Without this the next read of that file is answered from a stub
// describing content that no longer exists.
func TestChainedWriteInvalidatesTheCache(t *testing.T) {
	a := &Agent{readCache: NewReadCache()}
	a.readCache.RecordFullRead("/a.go", 1, 12)

	reader := &recordingTool{output: "stub path, never reached"}
	writer := &recordingTool{output: "written"}

	runBlocks(t, a,
		[]Tool{reader.tool("read_files"), writer.tool("write_file")},
		toolUseBlock("t1", "read_files",
			`{"path":"/a.go","then":{"tool":"write_file","input":{"path":"/a.go","content":"package a\n"}}}`),
	)

	if len(writer.inputs) != 1 {
		t.Fatalf("chained write ran %d times, want 1", len(writer.inputs))
	}
	if stub, cached := a.readCache.Check("/a.go"); cached {
		t.Errorf("cache still answers for a file the chain rewrote: %q", stub)
	}
}

// An uncached read is unaffected: the tool runs, its output is returned, and
// the chain still runs after it.
func TestUncachedReadRunsToolThenChain(t *testing.T) {
	a := &Agent{readCache: NewReadCache()}

	reader := &recordingTool{output: "package a\n[complete file — 3 lines]"}
	chained := &recordingTool{output: "build ok"}

	outputs := runBlocks(t, a,
		[]Tool{reader.tool("read_files"), chained.tool("shell")},
		toolUseBlock("t1", "read_files",
			`{"path":"/a.go","then":{"tool":"shell","input":{"command":"go build"}}}`),
	)

	if len(reader.inputs) != 1 {
		t.Fatalf("primary tool ran %d times, want 1", len(reader.inputs))
	}
	if strings.Contains(reader.inputs[0], chainField) {
		t.Errorf("chain field leaked into the primary tool's input: %q", reader.inputs[0])
	}
	if len(chained.inputs) != 1 {
		t.Fatalf("chained tool ran %d times, want 1", len(chained.inputs))
	}
	if !strings.Contains(outputs[0], "package a") || !strings.Contains(outputs[0], "build ok") {
		t.Errorf("combined output is missing a half: %q", outputs[0])
	}
}
