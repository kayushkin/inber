package conversation

import (
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// failedResultBlock is what the engine actually appends when a tool call fails:
// anthropic.NewToolResultBlock(id, output, true), which sets is_error.
func failedResultBlock(id, content string) anthropic.ContentBlockParamUnion {
	return anthropic.NewToolResultBlock(id, content, true)
}

func isErrorOf(t *testing.T, block anthropic.ContentBlockParamUnion) (value, present bool) {
	t.Helper()
	if block.OfToolResult == nil {
		t.Fatal("rewriting the result dropped the tool_result block")
	}
	flag := block.OfToolResult.IsError
	return flag.Or(false), flag.Valid()
}

// A pruned result says less about what happened. It must not say something
// different. is_error is the model's only structured signal that a call failed,
// and rebuilding the block field by field used to drop it, so a failed call
// aged into a successful one.
func TestSummarisedResultStaysAnError(t *testing.T) {
	block := failedResultBlock("toolu_fail", repeatLines("compile error on line %d", 30))
	cfg := DefaultManagementConfig()

	pruned, wasPruned := pruneToolResult(block, cfg.ToolResultSummary, cfg, "shell_commands")
	if !wasPruned {
		t.Fatal("a middle-aged result was not summarised")
	}
	value, present := isErrorOf(t, pruned)
	if !present {
		t.Fatal("summarising a failed tool result dropped is_error entirely")
	}
	if !value {
		t.Fatal("summarising a failed tool result reported it as a success")
	}
}

// The drop path is the worse of the two: it deletes the error text as well, so
// if is_error goes with it there is nothing left anywhere saying the call failed.
func TestDroppedResultStaysAnError(t *testing.T) {
	content := repeatLines("compile error on line %d", 40)
	block := failedResultBlock("toolu_fail", content)
	cfg := DefaultManagementConfig()

	pruned, wasPruned := pruneToolResult(block, cfg.ToolResultDrop, cfg, "shell_commands")
	if !wasPruned {
		t.Fatal("an over-age result was not dropped")
	}
	if text := resultText(t, pruned); strings.Contains(text, "compile error") {
		t.Fatalf("the drop path kept the original text: %q", text)
	}
	value, present := isErrorOf(t, pruned)
	if !present {
		t.Fatal("dropping a failed tool result dropped is_error entirely")
	}
	if !value {
		t.Fatal("dropping a failed tool result reported it as a success")
	}
}

// A succeeding call must not be relabelled either: the flag is copied, not
// asserted.
func TestPruningKeepsASuccessASuccess(t *testing.T) {
	block := anthropic.NewToolResultBlock("toolu_ok", repeatLines("line %d", 40), false)
	cfg := DefaultManagementConfig()

	pruned, wasPruned := pruneToolResult(block, cfg.ToolResultDrop, cfg, "read_files")
	if !wasPruned {
		t.Fatal("an over-age result was not dropped")
	}
	if value, _ := isErrorOf(t, pruned); value {
		t.Fatal("dropping a successful tool result reported it as an error")
	}
}

// The file-dedup stub rebuilt the block the same way, so a failed read that was
// later superseded lost its flag on a different code path.
func TestDedupStubKeepsIsError(t *testing.T) {
	messages := []anthropic.MessageParam{
		{Role: anthropic.MessageParamRoleAssistant, Content: []anthropic.ContentBlockParamUnion{
			toolUseBlock("toolu_1", "read_files"),
		}},
		{Role: anthropic.MessageParamRoleUser, Content: []anthropic.ContentBlockParamUnion{
			failedResultBlock("toolu_1", repeatLines("/tmp/x line %d", 30)),
		}},
		{Role: anthropic.MessageParamRoleAssistant, Content: []anthropic.ContentBlockParamUnion{
			toolUseBlock("toolu_2", "read_files"),
		}},
		{Role: anthropic.MessageParamRoleUser, Content: []anthropic.ContentBlockParamUnion{
			toolResultBlock("toolu_2", repeatLines("/tmp/x line %d", 30)),
		}},
	}

	// Fail rather than skip: the fixture is declared right here, so nothing
	// superseded means the code under test stopped stubbing failed reads, and
	// this test is the only one that covers that. Measured 2026-08-10 — make
	// DeduplicateFileRefs pass over a result carrying is_error and this test
	// reported --- SKIP while the package reported ok. (noteboard f0453cc6)
	if n := DeduplicateFileRefs(messages); n == 0 {
		t.Fatal("nothing was superseded in this fixture, so the is_error claim below was never exercised; fix the code or rewrite the fixture")
	}
	value, present := isErrorOf(t, messages[1].Content[0])
	if !present {
		t.Fatal("stubbing a superseded failed read dropped is_error entirely")
	}
	if !value {
		t.Fatal("stubbing a superseded failed read reported it as a success")
	}
}

// The already-stubbed check used to sniff for a "[" and the word "superseded".
// Any tool can return both — reading this file returns both — and a result that
// did was skipped and never deduplicated. The check compares against the stub
// for that tool id now, so ordinary content carrying the words is still pruned.
func TestContentThatLooksLikeAStubIsStillDeduplicated(t *testing.T) {
	forged := "[/tmp/x superseded — see later read_files]\n" + repeatLines("real line %d", 30)
	messages := []anthropic.MessageParam{
		{Role: anthropic.MessageParamRoleAssistant, Content: []anthropic.ContentBlockParamUnion{
			toolUseBlock("toolu_1", "read_files"),
		}},
		{Role: anthropic.MessageParamRoleUser, Content: []anthropic.ContentBlockParamUnion{
			toolResultBlock("toolu_1", forged),
		}},
		{Role: anthropic.MessageParamRoleAssistant, Content: []anthropic.ContentBlockParamUnion{
			toolUseBlock("toolu_2", "read_files"),
		}},
		{Role: anthropic.MessageParamRoleUser, Content: []anthropic.ContentBlockParamUnion{
			toolResultBlock("toolu_2", repeatLines("real line %d", 30)),
		}},
	}

	if n := DeduplicateFileRefs(messages); n == 0 {
		t.Fatal("a result whose text merely looks like a stub was skipped by the idempotency check")
	}
	if text := resultText(t, messages[1].Content[0]); strings.Contains(text, "real line 0") {
		t.Fatalf("the superseded read was not replaced by a stub: %q", text)
	}
}

// Deduplicating twice must stub nothing the second time. Two mechanisms enforce
// that independently, and either alone suffices: pass 4 rewrites the superseded
// call's input to {"_deduped": true} so it stops being a file reference, and
// pass 3's check skips a result already equal to its stub. As the code stands
// pass 4 gets there first, so pass 3's check is the fallback. Deleting one
// leaves the property standing and this test green; the test fails only when
// both go. Pinned because pass 4 reads as a token saving and is not one.
func TestASecondDedupPassStubsNothing(t *testing.T) {
	messages := []anthropic.MessageParam{
		{Role: anthropic.MessageParamRoleAssistant, Content: []anthropic.ContentBlockParamUnion{
			toolUseBlock("toolu_1", "read_files"),
		}},
		{Role: anthropic.MessageParamRoleUser, Content: []anthropic.ContentBlockParamUnion{
			toolResultBlock("toolu_1", repeatLines("/tmp/x line %d", 30)),
		}},
		{Role: anthropic.MessageParamRoleAssistant, Content: []anthropic.ContentBlockParamUnion{
			toolUseBlock("toolu_2", "read_files"),
		}},
		{Role: anthropic.MessageParamRoleUser, Content: []anthropic.ContentBlockParamUnion{
			toolResultBlock("toolu_2", repeatLines("/tmp/x line %d", 30)),
		}},
	}

	first := DeduplicateFileRefs(messages)
	if first == 0 {
		t.Fatal("the first pass stubbed nothing, so the second-pass claim below was never exercised; fix the code or rewrite the fixture")
	}
	if second := DeduplicateFileRefs(messages); second != 0 {
		t.Fatalf("a second pass re-stubbed %d already-stubbed results", second)
	}
}
