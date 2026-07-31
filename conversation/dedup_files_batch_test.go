package conversation

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// readCall builds the assistant/user message pair for one file tool call.
func readCall(id, name, input, result string) []anthropic.MessageParam {
	return []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{{
				OfToolUse: &anthropic.ToolUseBlockParam{
					ID:    id,
					Name:  name,
					Input: json.RawMessage(input),
				},
			}},
		},
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{{
				OfToolResult: &anthropic.ToolResultBlockParam{
					ToolUseID: id,
					Content: []anthropic.ToolResultBlockParamContentUnion{
						{OfText: &anthropic.TextBlockParam{Text: result}},
					},
				},
			}},
		},
	}
}

func conversationOf(calls ...[]anthropic.MessageParam) []anthropic.MessageParam {
	var out []anthropic.MessageParam
	for _, c := range calls {
		out = append(out, c...)
	}
	return out
}

// resultFor returns the tool_result text for a tool_use id.
func resultFor(t *testing.T, messages []anthropic.MessageParam, id string) string {
	t.Helper()
	for _, msg := range messages {
		for _, block := range msg.Content {
			if block.OfToolResult != nil && block.OfToolResult.ToolUseID == id {
				return extractToolResultContent(block.OfToolResult.Content)
			}
		}
	}
	t.Fatalf("no tool_result for %s", id)
	return ""
}

// The batching form of read_files is the one the tool description tells the
// model to prefer, and its results are the largest in the transcript — N files
// concatenated. Reading the same file twice that way must supersede the first
// copy exactly as the single-path form does.
func TestDeduplicateFileRefsSupersedesBatchedReads(t *testing.T) {
	messages := conversationOf(
		readCall("toolu_1", "read_files", `{"paths":["engine/engine.go"]}`, strings.Repeat("first copy\n", 200)),
		readCall("toolu_2", "read_files", `{"paths":["engine/engine.go"]}`, strings.Repeat("second copy\n", 200)),
	)

	if n := DeduplicateFileRefs(messages); n != 1 {
		t.Fatalf("deduplicated %d batched reads of one file, want 1", n)
	}
	if got := resultFor(t, messages, "toolu_1"); !strings.Contains(got, "superseded") {
		t.Fatalf("the older batched read survived at full size: %.40q", got)
	}
	if got := resultFor(t, messages, "toolu_2"); !strings.Contains(got, "second copy") {
		t.Fatalf("the latest read was stubbed: %.40q", got)
	}
}

// A batched write is a write to every path it names, so it supersedes the reads
// of each of them — otherwise the transcript keeps content the write replaced.
func TestDeduplicateFileRefsBatchedWriteSupersedesEarlierReads(t *testing.T) {
	messages := conversationOf(
		readCall("toolu_1", "read_files", `{"path":"a.go"}`, "old a"),
		readCall("toolu_2", "read_files", `{"path":"b.go"}`, "old b"),
		readCall("toolu_3", "write_files",
			`{"files":[{"path":"a.go","content":"new a"},{"path":"b.go","content":"new b"}]}`, "wrote 2 files"),
	)

	if n := DeduplicateFileRefs(messages); n != 2 {
		t.Fatalf("deduplicated %d reads superseded by a batched write, want 2", n)
	}
	for _, id := range []string{"toolu_1", "toolu_2"} {
		if got := resultFor(t, messages, id); !strings.Contains(got, "superseded") {
			t.Fatalf("%s survived a later batched write: %.40q", id, got)
		}
	}
}

// The same for edits[], which carries its paths one level down as well.
func TestDeduplicateFileRefsBatchedEditSupersedesEarlierRead(t *testing.T) {
	messages := conversationOf(
		readCall("toolu_1", "read_files", `{"path":"a.go"}`, "old a"),
		readCall("toolu_2", "edit_files",
			`{"edits":[{"path":"a.go","old_text":"x","new_text":"y"}]}`, "edited"),
	)

	if n := DeduplicateFileRefs(messages); n != 1 {
		t.Fatalf("deduplicated %d, want 1", n)
	}
	if got := resultFor(t, messages, "toolu_1"); !strings.Contains(got, "superseded") {
		t.Fatalf("read survived a later batched edit: %.40q", got)
	}
}

// A multi-path call is only redundant once EVERY file it carries has been
// re-read. Stubbing it because one of its paths came up again would delete the
// only copy of the others.
func TestDeduplicateFileRefsKeepsBatchWithAnUnsupersededPath(t *testing.T) {
	messages := conversationOf(
		readCall("toolu_1", "read_files", `{"paths":["a.go","b.go"]}`, "a and b"),
		readCall("toolu_2", "read_files", `{"path":"a.go"}`, "a again"),
	)

	if n := DeduplicateFileRefs(messages); n != 0 {
		t.Fatalf("deduplicated %d; the batch still holds the only copy of b.go", n)
	}
	if got := resultFor(t, messages, "toolu_1"); strings.Contains(got, "superseded") {
		t.Fatalf("stubbed a batch whose b.go was never re-read: %.40q", got)
	}
}

// ...and once the last of its paths is re-read, it is redundant and goes.
func TestDeduplicateFileRefsStubsBatchOnceEveryPathIsSuperseded(t *testing.T) {
	messages := conversationOf(
		readCall("toolu_1", "read_files", `{"paths":["a.go","b.go"]}`, "a and b"),
		readCall("toolu_2", "read_files", `{"path":"a.go"}`, "a again"),
		readCall("toolu_3", "read_files", `{"path":"b.go"}`, "b again"),
	)

	if n := DeduplicateFileRefs(messages); n != 1 {
		t.Fatalf("deduplicated %d, want 1", n)
	}
	if got := resultFor(t, messages, "toolu_1"); !strings.Contains(got, "superseded") {
		t.Fatalf("batch survived after both its paths were re-read: %.40q", got)
	}
}

// offset and limit only take effect on a call that resolves to one path — a
// batch of two reads each file whole (tool-store tools/fs.go, ReadFile). So a
// stray offset alongside a multi-file batch must not demote that batch to a
// partial read, which would stop it superseding anything.
func TestDeduplicateFileRefsBatchWithStrayOffsetIsStillAFullRead(t *testing.T) {
	messages := conversationOf(
		readCall("toolu_1", "read_files", `{"paths":["a.go","b.go"]}`, "a and b"),
		readCall("toolu_2", "read_files", `{"paths":["a.go","b.go"],"offset":5}`, "a and b again"),
	)

	if n := DeduplicateFileRefs(messages); n != 1 {
		t.Fatalf("deduplicated %d; a batch read is whole-file whatever offset says", n)
	}
	if got := resultFor(t, messages, "toolu_1"); !strings.Contains(got, "superseded") {
		t.Fatalf("the older batch survived: %.40q", got)
	}
}

// The single-path form of the same read does honour offset, and stays partial.
func TestDeduplicateFileRefsSinglePathWithOffsetIsStillPartial(t *testing.T) {
	messages := conversationOf(
		readCall("toolu_1", "read_files", `{"path":"a.go"}`, "whole file"),
		readCall("toolu_2", "read_files", `{"paths":["a.go"],"offset":5}`, "from line 5"),
	)

	if n := DeduplicateFileRefs(messages); n != 0 {
		t.Fatalf("deduplicated %d; a partial read must not supersede the whole file", n)
	}
}

// CrossZoneDedup reads the same paths through the same extractor, so a batched
// read in the staging zone has to be recognised as re-reading a frozen file.
func TestCrossZoneDedupSeesBatchedReads(t *testing.T) {
	frozen := readCall("toolu_1", "read_files", `{"paths":["a.go","b.go"]}`, "a and b")
	staging := readCall("toolu_2", "read_files", `{"paths":["b.go"]}`, "b again")

	superseded := CrossZoneDedup(frozen, staging)
	if len(superseded) != 1 || superseded[0] != "b.go" {
		t.Fatalf("CrossZoneDedup reported %v, want [b.go]", superseded)
	}
}

// Complement: the single-path behaviour this function already had must still
// hold, so a fixture that stops exercising the batch shapes cannot turn the
// assertions above green by accident.
func TestDeduplicateFileRefsStillSupersedesSinglePathReads(t *testing.T) {
	messages := conversationOf(
		readCall("toolu_1", "read_files", `{"path":"a.go"}`, "first"),
		readCall("toolu_2", "read_files", `{"path":"a.go"}`, "second"),
	)

	if n := DeduplicateFileRefs(messages); n != 1 {
		t.Fatalf("deduplicated %d single-path reads, want 1", n)
	}
}

// Complement: a partial read must still not supersede another partial read of
// the same file — the ranges differ, so both are load-bearing.
func TestDeduplicateFileRefsStillKeepsBothPartialReads(t *testing.T) {
	messages := conversationOf(
		readCall("toolu_1", "read_files", `{"path":"a.go","offset":1,"limit":50}`, "lines 1-50"),
		readCall("toolu_2", "read_files", `{"path":"a.go","offset":51,"limit":50}`, "lines 51-100"),
	)

	if n := DeduplicateFileRefs(messages); n != 0 {
		t.Fatalf("deduplicated %d partial reads; they cover different ranges", n)
	}
}
