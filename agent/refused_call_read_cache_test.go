package agent

import (
	"errors"
	"testing"
)

// The defect these tests pin: executeTools invalidated the read cache for a
// write, and flushed it entirely for a shell, BEFORE executeWithChain asked the
// gate whether the call was allowed to happen at all. A refused call never
// reaches tool.Run — nothing was written — so the eviction paid for a re-read
// of a file that had not changed.
//
// It is a cost defect, not a correctness one: over-invalidating is always safe.
// But it fires hardest exactly where refusals are the norm. Observe mode
// refuses every write by construction (guard.CheckTool's fall-through), so an
// Observe session re-read every file it ever attempted to write, once per
// attempt.
//
// The chained call already got this right — outcome.chainTool is assigned only
// after chainTool.Run returns, so a refused chain never named itself to the
// invalidation block. The fix makes the primary call say the same thing about
// itself, through outcome.primaryRan.
//
// These drive the real executeTools loop through runBlocks. A test that asked
// executeWithChain directly would not see the invalidation at all: it lives in
// the caller.

// refusing returns a gate that refuses exactly the named tools, in the shape
// engine/build_hooks.go's buildToolRefusal produces — a non-empty string is the
// reason, an empty one is consent.
func refusing(tools ...string) func(tool, input string) string {
	refused := make(map[string]bool, len(tools))
	for _, t := range tools {
		refused[t] = true
	}
	return func(tool, _ string) string {
		if refused[tool] {
			return "Observe mode: read-only tools only"
		}
		return ""
	}
}

// TestARefusedWriteLeavesTheReadItWouldHaveInvalidated is the defect itself.
func TestARefusedWriteLeavesTheReadItWouldHaveInvalidated(t *testing.T) {
	a := &Agent{readCache: NewReadCache(), ToolRefusal: refusing("write_files")}
	a.readCache.RecordFullRead("/a.go", 12)

	writer := &recordingTool{output: "written"}

	runBlocks(t, a,
		[]Tool{writer.tool("write_files")},
		toolUseBlock("t1", "write_files", `{"path":"/a.go","content":"x"}`),
	)

	if len(writer.inputs) != 0 {
		t.Fatalf("the gate refused this call but the tool ran %d times — the test is not "+
			"measuring what it claims", len(writer.inputs))
	}
	if _, cached := a.readCache.Check("/a.go"); !cached {
		t.Error("a refused write evicted /a.go from the read cache: the file was never " +
			"written, so the next read of it is paid for twice")
	}
}

// TestARefusedShellLeavesTheWholeCacheAlone is the same defect through the
// other invalidation path, the one that takes every entry rather than the ones
// the input names.
func TestARefusedShellLeavesTheWholeCacheAlone(t *testing.T) {
	a := &Agent{readCache: NewReadCache(), ToolRefusal: refusing("shell_commands")}
	a.readCache.RecordFullRead("/a.go", 12)
	a.readCache.RecordFullRead("/b.go", 34)

	shell := &recordingTool{output: "ran"}

	runBlocks(t, a,
		[]Tool{shell.tool("shell_commands")},
		toolUseBlock("t1", "shell_commands", `{"command":"sed -i s/x/y/ /a.go"}`),
	)

	if len(shell.inputs) != 0 {
		t.Fatalf("the gate refused this call but the tool ran %d times", len(shell.inputs))
	}
	for _, path := range []string{"/a.go", "/b.go"} {
		if _, cached := a.readCache.Check(path); !cached {
			t.Errorf("a refused shell flushed %s out of the read cache: nothing ran, so "+
				"nothing can have gone stale", path)
		}
	}
}

// TestAWriteThatRanAndFAILEDStillEvicts is the control that stops the fix from
// becoming "only evict on success". A tool that returns an error was dispatched
// and may have written part of what it was asked to — read_cache.go's own
// comment makes that argument for the shell case — so its entry has to go.
// Refused and failed are different things, and only the first one means nothing
// happened.
func TestAWriteThatRanAndFAILEDStillEvicts(t *testing.T) {
	a := &Agent{readCache: NewReadCache()} // no gate: every call is allowed
	a.readCache.RecordFullRead("/a.go", 12)

	writer := &recordingTool{err: errors.New("disk full")}

	runBlocks(t, a,
		[]Tool{writer.tool("write_files")},
		toolUseBlock("t1", "write_files", `{"path":"/a.go","content":"x"}`),
	)

	if len(writer.inputs) != 1 {
		t.Fatalf("the write ran %d times, want 1", len(writer.inputs))
	}
	if stub, cached := a.readCache.Check("/a.go"); cached {
		t.Errorf("a write that ran and failed left /a.go in the cache (%q) — it may have "+
			"written part of the file before it failed", stub)
	}
}

// TestAShellThatRanAndFAILEDStillFlushes is the same control on the flush path.
func TestAShellThatRanAndFAILEDStillFlushes(t *testing.T) {
	a := &Agent{readCache: NewReadCache()}
	a.readCache.RecordFullRead("/a.go", 12)

	shell := &recordingTool{err: errors.New("exit status 1")}

	runBlocks(t, a,
		[]Tool{shell.tool("shell_commands")},
		toolUseBlock("t1", "shell_commands", `{"command":"sed -i s/x/y/ /a.go"}`),
	)

	if len(shell.inputs) != 1 {
		t.Fatalf("the shell ran %d times, want 1", len(shell.inputs))
	}
	if _, cached := a.readCache.Check("/a.go"); cached {
		t.Error("a shell that ran and failed left the cache intact — it may have rewritten " +
			"the file before it failed")
	}
}

// TestAnAllowedWriteStillEvicts is the control that stops the fix from becoming
// "never evict". Without it, gating the invalidation on a flag that is never
// set would pass every test above.
func TestAnAllowedWriteStillEvicts(t *testing.T) {
	a := &Agent{readCache: NewReadCache(), ToolRefusal: refusing("shell_commands")}
	a.readCache.RecordFullRead("/a.go", 12)

	writer := &recordingTool{output: "written"}

	runBlocks(t, a,
		[]Tool{writer.tool("write_files")},
		toolUseBlock("t1", "write_files", `{"path":"/a.go","content":"x"}`),
	)

	if len(writer.inputs) != 1 {
		t.Fatalf("the write ran %d times, want 1", len(writer.inputs))
	}
	if stub, cached := a.readCache.Check("/a.go"); cached {
		t.Errorf("an allowed write left /a.go in the cache (%q) — the next read is answered "+
			"from content the write replaced", stub)
	}
}

// TestAWriteCarryingSidebandAndChainStillEvictsItsOwnPath guards the one way
// the fix could fail silently in the dangerous direction.
//
// The invalidation now reads outcome.primaryInput — the block's input with the
// sideband and chain fields removed — where it used to read the raw block
// input. Those two are the same JSON object minus some sibling keys, and
// isFileWrite only ever looks at "path"/"files"/"edits", so they name the same
// paths. That is an argument, not a measurement, and if extractSideband or
// extractChain ever returned something isFileWrite could not parse, the failure
// would be a write that stops evicting: a stale entry, answered to the model as
// current. That is the failure this whole file's cache exists to prevent, and
// it would be invisible without this case.
func TestAWriteCarryingSidebandAndChainStillEvictsItsOwnPath(t *testing.T) {
	a := &Agent{readCache: NewReadCache()}
	a.readCache.RecordFullRead("/a.go", 12)

	writer := &recordingTool{output: "written"}
	reader := &recordingTool{output: "contents\n[complete file — 3 lines]"}

	runBlocks(t, a,
		[]Tool{writer.tool("write_files"), reader.tool("read_files")},
		toolUseBlock("t1", "write_files",
			`{"path":"/a.go","content":"x","note":"a note","then":{"tool":"read_files","input":{"path":"/b.go"}}}`),
	)

	if len(writer.inputs) != 1 {
		t.Fatalf("the write ran %d times, want 1", len(writer.inputs))
	}
	if stub, cached := a.readCache.Check("/a.go"); cached {
		t.Errorf("a write whose input also carried sideband and chain fields did not evict "+
			"its own path (%q) — the fields are stripped before isFileWrite sees the input, "+
			"and the path did not survive the stripping", stub)
	}
}

// TestARefusedChainLeavesTheCacheAlone pins the half that was already right, so
// a later refactor of the two into one shape cannot quietly lose it.
func TestARefusedChainLeavesTheCacheAlone(t *testing.T) {
	a := &Agent{readCache: NewReadCache(), ToolRefusal: refusing("shell_commands")}

	reader := &recordingTool{output: "contents\n[complete file — 3 lines]"}
	shell := &recordingTool{output: "ran"}

	runBlocks(t, a,
		[]Tool{reader.tool("read_files"), shell.tool("shell_commands")},
		toolUseBlock("t1", "read_files",
			`{"path":"/a.go","then":{"tool":"shell_commands","input":{"command":"sed -i s/x/y/ /a.go"}}}`),
	)

	if len(shell.inputs) != 0 {
		t.Fatalf("the gate refused the chained shell but it ran %d times", len(shell.inputs))
	}
	if _, cached := a.readCache.Check("/a.go"); !cached {
		t.Error("a refused chained shell flushed the read its own primary call had just " +
			"recorded")
	}
}
