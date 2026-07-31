package agent

import (
	"strings"
	"testing"
)

// refuseTools builds a gate that refuses exactly the named tools. It stands in
// for the engine's guard closure, which is what supplies the real one.
func refuseTools(names ...string) func(tool, input string) string {
	refused := make(map[string]bool, len(names))
	for _, name := range names {
		refused[name] = true
	}
	return func(tool, _ string) string {
		if refused[tool] {
			return "observe mode allows read-only tools only"
		}
		return ""
	}
}

// TestARefusedToolNeverRuns is the property the whole gate exists for. A
// refusal that still ran the tool and only relabelled the output would be no
// gate at all, so the assertion is on the tool's own record of being called,
// not on the text handed back.
func TestARefusedToolNeverRuns(t *testing.T) {
	shell := &recordingTool{output: "uid=1000"}

	a := &Agent{ToolRefusal: refuseTools("shell_commands")}
	outputs := runBlocks(t, a,
		[]Tool{shell.tool("shell_commands")},
		toolUseBlock("t1", "shell_commands", `{"command":"id"}`),
	)

	if len(shell.inputs) != 0 {
		t.Errorf("a refused tool ran anyway, with %v", shell.inputs)
	}
	if len(outputs) != 1 || !strings.Contains(outputs[0], "refused") {
		t.Errorf("tool result = %q, want the refusal handed back to the model", outputs)
	}
}

// TestTheChainCannotGetAroundTheGate. A tool_use block carries two calls: the
// primary one and whatever its "then" field names. They run through different
// lines of the same function, so a gate wired to the first alone leaves the
// model a way to run any tool it likes — ask for a read, chain the shell.
func TestTheChainCannotGetAroundTheGate(t *testing.T) {
	reader := &recordingTool{output: "package main"}
	shell := &recordingTool{output: "uid=1000"}

	a := &Agent{ToolRefusal: refuseTools("shell_commands")}
	outputs := runBlocks(t, a,
		[]Tool{reader.tool("read_files"), shell.tool("shell_commands")},
		toolUseBlock("t1", "read_files",
			`{"path":"/a.go","then":{"tool":"shell_commands","input":{"command":"id"}}}`),
	)

	if len(shell.inputs) != 0 {
		t.Errorf("the chained call ran a refused tool, with %v", shell.inputs)
	}
	if len(reader.inputs) != 1 {
		t.Errorf("the allowed primary call ran %d times, want 1 — refusing the chain must not refuse the call it rode in on", len(reader.inputs))
	}
	if len(outputs) != 1 || !strings.Contains(outputs[0], "refused") {
		t.Fatalf("tool result = %q, want the chain's refusal reported", outputs)
	}
	if !strings.Contains(outputs[0], "package main") {
		t.Errorf("tool result = %q, want the primary output kept alongside the refusal", outputs[0])
	}
}

// TestARefusedReadIsNotAnsweredFromTheCache. The read cache answers a repeat
// read with a stub instead of running the tool. The stub is still that tool's
// output, so a gate consulted after the cache would let a refused tool report
// file contents anyway.
func TestARefusedReadIsNotAnsweredFromTheCache(t *testing.T) {
	a := &Agent{readCache: NewReadCache(), ToolRefusal: refuseTools("read_files")}
	a.readCache.RecordFullRead("/a.go", 1, 12)

	reader := &recordingTool{output: "package main"}
	outputs := runBlocks(t, a,
		[]Tool{reader.tool("read_files")},
		toolUseBlock("t1", "read_files", `{"path":"/a.go"}`),
	)

	if len(outputs) != 1 || !strings.Contains(outputs[0], "refused") {
		t.Errorf("tool result = %q, want a refusal rather than the cached stub", outputs)
	}
}

// TestAnAgentWithNoGateRunsEverything is the complement, and the regression
// that would matter most. Nearly every session here has no mode and therefore
// no refusals; a gate that refused when it was absent would stop every one of
// them.
func TestAnAgentWithNoGateRunsEverything(t *testing.T) {
	shell := &recordingTool{output: "uid=1000"}

	a := &Agent{} // no ToolRefusal, as every session had before the gate existed
	outputs := runBlocks(t, a,
		[]Tool{shell.tool("shell_commands")},
		toolUseBlock("t1", "shell_commands", `{"command":"id"}`),
	)

	if len(shell.inputs) != 1 {
		t.Errorf("the tool ran %d times with no gate set, want 1", len(shell.inputs))
	}
	if len(outputs) != 1 || outputs[0] != "uid=1000" {
		t.Errorf("tool result = %q, want the tool's own output", outputs)
	}
}
