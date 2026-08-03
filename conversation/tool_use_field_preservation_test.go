package conversation

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// The two places that rewrite an existing tool_use block — truncateToolCall in
// the age-based prune, and pass 4 of the file dedup — used to build a fresh
// ToolUseBlockParam from a literal naming ID, Name and Input. The SDK's block
// carries CacheControl and Caller as well, so both rewrites dropped them.
//
// replaceToolResultText, twenty lines away in the same package, already learned
// this on the tool_result side, where the field it silently dropped was
// is_error and a failed call aged into a successful one. These tests apply that
// rule to the tool_use side before it costs anything: the losses are latent
// today (agent_run.go re-places every history breakpoint after pruning, and
// nothing here sets Caller), so what they defend is the shape, and the shape is
// what the SDK keeps adding fields to.

func toolUseBlockWithCacheControl() anthropic.ContentBlockParamUnion {
	return anthropic.ContentBlockParamUnion{
		OfToolUse: &anthropic.ToolUseBlockParam{
			ID:           "call_1",
			Name:         "read_files",
			Input:        map[string]any{"paths": []string{"a.go"}},
			CacheControl: anthropic.NewCacheControlEphemeralParam(),
		},
	}
}

func TestPruningAToolCallKeepsItsCacheBreakpoint(t *testing.T) {
	pruned := truncateToolCall(toolUseBlockWithCacheControl())

	if pruned.OfToolUse == nil {
		t.Fatal("pruning turned a tool_use block into something else")
	}
	if pruned.OfToolUse.CacheControl.Type == "" {
		t.Error("the cache breakpoint was dropped; a rewrite of what a call says must not change what the call is")
	}
	if pruned.OfToolUse.ID != "call_1" || pruned.OfToolUse.Name != "read_files" {
		t.Errorf("identity changed: id %q name %q", pruned.OfToolUse.ID, pruned.OfToolUse.Name)
	}
	// The one field it is supposed to rewrite.
	summary, ok := pruned.OfToolUse.Input.(map[string]interface{})["_summary"].(string)
	if !ok || summary == "" {
		t.Errorf("the input was not summarized: %#v", pruned.OfToolUse.Input)
	}
}

func TestDedupingAToolCallKeepsItsCacheBreakpoint(t *testing.T) {
	block := toolUseBlockWithCacheControl()
	deduped := replaceToolUseInput(block, map[string]interface{}{"_deduped": true})

	if deduped.OfToolUse.CacheControl.Type == "" {
		t.Error("the cache breakpoint was dropped by the dedup rewrite")
	}
	if deduped.OfToolUse.Input.(map[string]interface{})["_deduped"] != true {
		t.Errorf("the dedup marker did not land: %#v", deduped.OfToolUse.Input)
	}
}

// The original block must not be edited through the pointer it was read from.
// Pruning runs over the live conversation, and a rewrite that mutated in place
// would prune the transcript the session keeps, not the copy being sent.
func TestRewritingAToolCallLeavesTheOriginalAlone(t *testing.T) {
	block := toolUseBlockWithCacheControl()

	_ = truncateToolCall(block)

	original, ok := block.OfToolUse.Input.(map[string]any)
	if !ok {
		t.Fatalf("the original input was replaced in place: %#v", block.OfToolUse.Input)
	}
	if _, summarized := original["_summary"]; summarized {
		t.Error("the original block was summarized in place")
	}
}
