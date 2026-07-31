package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	toolstoretools "github.com/kayushkin/tool-store/tools"
)

// The deduplicator decides a read was whole from the request — the call named
// no offset and no limit — and then lets it supersede an earlier ranged read
// of the same path. But tool-store truncates a whole-file read on its own: a
// file past the line window comes back as its first 500 lines and its last 50,
// with a hole in the middle. So the "whole" read can be missing exactly the
// lines the ranged read went and got, and stubbing the ranged read deletes the
// only copy of them from the conversation.
//
// These tests run the real read tool rather than writing its output by hand,
// so the hole in the second result is the hole the tool actually leaves.

func readViaToolStore(t *testing.T, in map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := toolstoretools.ReadFile().Run(context.Background(), string(raw))
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	return out
}

// numberedFile writes a file whose every line names itself, so a test can ask
// whether one particular line survived deduplication.
func numberedFile(t *testing.T, lines int) string {
	t.Helper()
	var b strings.Builder
	for i := 1; i <= lines; i++ {
		fmt.Fprintf(&b, "content of line %d\n", i)
	}
	path := filepath.Join(t.TempDir(), "big.go")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func conversationText(messages []anthropic.MessageParam) string {
	var b strings.Builder
	for _, msg := range messages {
		for _, block := range msg.Content {
			if block.OfToolResult != nil {
				b.WriteString(extractToolResultContent(block.OfToolResult.Content))
				b.WriteString("\n")
			}
		}
	}
	return b.String()
}

func TestRangedReadSurvivesATruncatedWholeFileRead(t *testing.T) {
	path := numberedFile(t, 3000)

	ranged := readViaToolStore(t, map[string]any{"path": path, "offset": 2000, "limit": 100})
	whole := readViaToolStore(t, map[string]any{"path": path})

	// The premise, checked rather than assumed: the ranged read holds line
	// 2050 and the whole-file read does not.
	const wanted = "content of line 2050\n"
	if !strings.Contains(ranged, wanted) {
		t.Fatalf("ranged read does not hold line 2050; the test file or the read tool changed")
	}
	if strings.Contains(whole, wanted) {
		t.Fatalf("whole-file read holds line 2050, so it was not truncated; the line window changed")
	}

	messages := conversationOf(
		readCall("toolu_ranged", "read_files", fmt.Sprintf(`{"path":%q,"offset":2000,"limit":100}`, path), ranged),
		readCall("toolu_whole", "read_files", fmt.Sprintf(`{"path":%q}`, path), whole),
	)
	DeduplicateFileRefs(messages)

	if !strings.Contains(conversationText(messages), wanted) {
		t.Errorf("line 2050 was deleted from the conversation: the ranged read was stubbed against a read that does not hold it\nranged result now: %q",
			resultFor(t, messages, "toolu_ranged"))
	}
}

// A write makes every earlier read of that path stale whatever range it
// covered, so it must still supersede a ranged read. Without this the fix
// above could be had by never stubbing a ranged read at all.
func TestWriteStillSupersedesARangedRead(t *testing.T) {
	messages := conversationOf(
		readCall("toolu_1", "read_files", `{"path":"engine/engine.go","offset":10,"limit":5}`, "five old lines"),
		readCall("toolu_2", "write_files", `{"files":[{"path":"engine/engine.go","content":"new"}]}`, "wrote engine/engine.go"),
	)

	if n := DeduplicateFileRefs(messages); n != 1 {
		t.Fatalf("deduplicated %d results, want 1", n)
	}
	if got := resultFor(t, messages, "toolu_1"); !strings.Contains(got, "superseded") {
		t.Errorf("ranged read kept after a write: %q", got)
	}
}

func TestEditStillSupersedesARangedRead(t *testing.T) {
	messages := conversationOf(
		readCall("toolu_1", "read_files", `{"path":"engine/engine.go","offset":10,"limit":5}`, "five old lines"),
		readCall("toolu_2", "edit_files", `{"edits":[{"path":"engine/engine.go","old":"a","new":"b"}]}`, "edited engine/engine.go"),
	)

	if n := DeduplicateFileRefs(messages); n != 1 {
		t.Fatalf("deduplicated %d results, want 1", n)
	}
	if got := resultFor(t, messages, "toolu_1"); !strings.Contains(got, "superseded") {
		t.Errorf("ranged read kept after an edit: %q", got)
	}
}

// Two whole-file reads of an unchanged file are cut the same way by the same
// limits, so the later one holds everything the earlier one did. That is the
// reclaim this package exists for and the fix must leave it alone.
func TestWholeFileReadStillSupersedesAnEarlierWholeFileRead(t *testing.T) {
	messages := conversationOf(
		readCall("toolu_1", "read_files", `{"path":"engine/engine.go"}`, strings.Repeat("first copy\n", 200)),
		readCall("toolu_2", "read_files", `{"path":"engine/engine.go"}`, strings.Repeat("second copy\n", 200)),
	)

	if n := DeduplicateFileRefs(messages); n != 1 {
		t.Fatalf("deduplicated %d results, want 1", n)
	}
	if got := resultFor(t, messages, "toolu_1"); !strings.Contains(got, "superseded") {
		t.Errorf("earlier whole-file read kept: %q", got)
	}
}
