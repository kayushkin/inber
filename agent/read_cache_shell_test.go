package agent

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	toolstoretools "github.com/kayushkin/tool-store/tools"
)

// The read cache's promise is "fully read this turn and not modified since".
// Its invalidation set was write_file/write_files/edit_file/edit_files, and the
// type comment argued that was enough because the cache only lives for one turn
// and "a shell `sed -i`, a `git checkout` or a human editing a file between
// turns all happen while the cache is empty".
//
// Two of those three are shell_commands, which is a tool. It runs inside the
// turn, with the cache full. So the second half of the promise was not kept:
// read a file, rewrite it from the shell, read it again, and the cache answered
// "already in context ... No need to re-read." over content that no longer
// existed.
//
// These tests drive the real executeTools loop through runBlocks, so they fail
// if the classification is right and the wiring is missing — which a test that
// called InvalidateAll itself would not.

// TestAShellCallEvictsAnEarlierReadInTheSameTurn is the defect, through the
// real loop: block 1 reads, block 2 shells, block 3 reads again and must
// actually re-read.
func TestAShellCallEvictsAnEarlierReadInTheSameTurn(t *testing.T) {
	a := &Agent{readCache: NewReadCache()}
	a.readCache.RecordFullRead("/a.go", 12)

	shell := &recordingTool{output: "rewritten"}
	reader := &recordingTool{output: "fresh contents\n[complete file — 3 lines]"}

	outputs := runBlocks(t, a,
		[]Tool{reader.tool("read_files"), shell.tool("shell_commands")},
		toolUseBlock("t1", "shell_commands", `{"command":"sed -i s/x/y/ /a.go"}`),
		toolUseBlock("t2", "read_files", `{"path":"/a.go"}`),
	)

	if len(shell.inputs) != 1 {
		t.Fatalf("shell ran %d times, want 1", len(shell.inputs))
	}
	if len(reader.inputs) != 1 {
		t.Fatalf("the read after the shell was answered from cache — the model kept "+
			"reasoning about content sed had already replaced (reader ran %d times, want 1)",
			len(reader.inputs))
	}
	if len(outputs) != 2 {
		t.Fatalf("got %d tool results, want 2", len(outputs))
	}
	if strings.Contains(outputs[1], "already in context") {
		t.Errorf("the post-shell read got the stub: %q", outputs[1])
	}
}

// TestAChainedShellEvictsTheReadItRodeInOn is the ordering half. The primary
// read is recorded in the cache *after* the block executes, so a chained shell
// has to be flushed after that record or the entry it was meant to remove is
// written back in behind it.
func TestAChainedShellEvictsTheReadItRodeInOn(t *testing.T) {
	a := &Agent{readCache: NewReadCache()}

	shell := &recordingTool{output: "rewritten"}
	reader := &recordingTool{output: "contents\n[complete file — 3 lines]"}

	runBlocks(t, a,
		[]Tool{reader.tool("read_files"), shell.tool("shell_commands")},
		toolUseBlock("t1", "read_files",
			`{"path":"/a.go","then":{"tool":"shell_commands","input":{"command":"sed -i s/x/y/ /a.go"}}}`),
	)

	if len(shell.inputs) != 1 {
		t.Fatalf("chained shell ran %d times, want 1", len(shell.inputs))
	}
	if stub, cached := a.readCache.Check("/a.go"); cached {
		t.Errorf("the read recorded by the primary call survived its own chained shell: %q", stub)
	}
}

// TestAnUnrelatedToolLeavesTheReadCacheAlone is the control. If it went red
// alongside the two above, the "fix" would be flushing on every call, which is
// not a fix — it is the cache being deleted.
func TestAnUnrelatedToolLeavesTheReadCacheAlone(t *testing.T) {
	a := &Agent{readCache: NewReadCache()}
	a.readCache.RecordFullRead("/a.go", 12)

	searcher := &recordingTool{output: "3 matches"}
	runBlocks(t, a,
		[]Tool{searcher.tool("web_search")},
		toolUseBlock("t1", "web_search", `{"query":"golang"}`),
	)

	if _, cached := a.readCache.Check("/a.go"); !cached {
		t.Error("web_search dropped the cache; it writes no files and must not")
	}
}

// TestAPreciseWriteStillOnlyDropsWhatItNamed pins that the blunt rule did not
// swallow the precise one.
func TestAPreciseWriteStillOnlyDropsWhatItNamed(t *testing.T) {
	a := &Agent{readCache: NewReadCache()}
	a.readCache.RecordFullRead("/kept.go", 10)
	a.readCache.RecordFullRead("/written.go", 20)

	writer := &recordingTool{output: "written"}
	runBlocks(t, a,
		[]Tool{writer.tool("write_files")},
		toolUseBlock("t1", "write_files", `{"files":[{"path":"/written.go","content":"x"}]}`),
	)

	if _, cached := a.readCache.Check("/written.go"); cached {
		t.Error("the written file should have left the cache")
	}
	if _, cached := a.readCache.Check("/kept.go"); !cached {
		t.Error("an unrelated file was dropped; the precise path is no longer precise")
	}
}

// TestTheRealShellToolActuallyRewritesTheFile keeps the tests above honest.
// They use a stub shell, so on their own they pin a rule about a tool name;
// this one runs tool-store's real shell tool and measures the file changing, so
// the premise the rule rests on is measured rather than assumed.
func TestTheRealShellToolActuallyRewritesTheFile(t *testing.T) {
	path := writeFile(t, "subject.txt", "original line one\noriginal line two\n")

	before := readViaToolStore(t, map[string]any{"path": path})
	if extractCompleteFileLines(before) == 0 {
		t.Fatalf("read tool did not report a complete read; tail was %q", before)
	}

	raw, err := json.Marshal(map[string]any{"command": "printf 'REPLACED\\n' > " + path})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := toolstoretools.Shell().Run(context.Background(), string(raw)); err != nil {
		t.Fatalf("shell failed: %v", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(after), "original line one") {
		t.Fatalf("shell_commands did not rewrite the file, so the cache would not "+
			"have been lying; it reads %q", string(after))
	}
	if ReadCacheEffect("shell_commands") != ReadCacheEverything {
		t.Errorf("shell_commands can rewrite any file but is classified %q",
			ReadCacheEffect("shell_commands"))
	}
}
