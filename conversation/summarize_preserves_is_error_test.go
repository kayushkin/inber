package conversation

import (
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// messagesToText builds the ONLY thing the summarizer ever sees. Whatever it
// leaves out is gone: SummarizeConversation exchanges the old turns for the
// summary, so a fact absent from this text is absent from the conversation from
// then on, and from the memory archive as well.
//
// pruneToolResult already learned this lesson — see replaceToolResultText and
// prune_preserves_is_error_test.go. is_error is the one structured signal that a
// call failed, and a rendering that omits it hands the summarizer a failed call
// wearing a successful one's clothes.

func failedResultMessages(content string) []anthropic.MessageParam {
	return []anthropic.MessageParam{
		{Role: anthropic.MessageParamRoleAssistant, Content: []anthropic.ContentBlockParamUnion{
			toolUseBlock("toolu_fail", "shell_commands"),
		}},
		{Role: anthropic.MessageParamRoleUser, Content: []anthropic.ContentBlockParamUnion{
			anthropic.NewToolResultBlock("toolu_fail", content, true),
		}},
	}
}

// The headline case. A command that failed is rendered indistinguishably from
// one that succeeded, so the summary records its partial output as a result.
func TestSummaryTextMarksAFailedToolResult(t *testing.T) {
	text := messagesToText(failedResultMessages("build succeeded for 3 packages\nmake: *** [all] Error 2"))

	if !strings.Contains(text, "tool_result") {
		t.Fatalf("the tool result did not reach the summarizer at all: %q", text)
	}
	if !strings.Contains(text, "failed") {
		t.Fatalf("a failed tool result reads as an ordinary result in the summarizer's input: %q", text)
	}
}

// The control: a rendering that says "failed" on everything is no better than
// one that says it on nothing. This is green before the fix, and it is what
// stops the case above from being satisfiable by a constant.
func TestSummaryTextLeavesASuccessUnmarked(t *testing.T) {
	messages := []anthropic.MessageParam{
		{Role: anthropic.MessageParamRoleAssistant, Content: []anthropic.ContentBlockParamUnion{
			toolUseBlock("toolu_ok", "read_files"),
		}},
		{Role: anthropic.MessageParamRoleUser, Content: []anthropic.ContentBlockParamUnion{
			toolResultBlock("toolu_ok", "package main\n\nfunc main() {}"),
		}},
	}

	if text := messagesToText(messages); strings.Contains(text, "failed") {
		t.Fatalf("a successful tool result was reported to the summarizer as a failure: %q", text)
	}
}

// renderedToolResultLimit measures the renderer's own cut length instead of
// assuming it. The cut is a bare literal inside extractToolResultText, so a
// fixture sized against a remembered number stops truncating the moment that
// literal is raised — and this test used to answer that by skipping, which
// reported a change in the code under test as drift in its own data and left
// the package green.
//
// A renderer that does not truncate at all has no length to find. That is a
// failure and not a reason to skip: the case below would go unexercised.
func renderedToolResultLimit(t *testing.T) int {
	t.Helper()
	const probeLength = 1 << 16
	probe := anthropic.NewToolResultBlock("toolu_probe", strings.Repeat("x", probeLength), false)
	rendered := extractToolResultText(probe)
	if len(rendered) >= probeLength {
		t.Fatalf("the renderer no longer truncates: a %d-character result rendered to %d characters, so the truncation case cannot be exercised",
			probeLength, len(rendered))
	}
	return len(rendered)
}

// The sharp version, and the one arXiv:2607.13071 describes: a command that
// prints a lot and then fails. The renderer cuts the output, so the error text
// at the end is lost and the partial stdout is all that survives. Truncation is
// fine; losing the fact of the failure with it is not.
//
// The stdout is built to overrun whatever the live cut length is, so the error
// text is always past it. That makes reaching the truncation branch guaranteed
// rather than lucky.
func TestAFailureSurvivesTruncationOfItsOutput(t *testing.T) {
	limit := renderedToolResultLimit(t)
	stdout := strings.Repeat("compiling module ... ok\n", limit/len("compiling module ... ok")+2)
	content := stdout + "make: *** [all] Error 2"
	text := messagesToText(failedResultMessages(content))

	if strings.Contains(text, "Error 2") {
		t.Fatalf("stdout of %d characters did not overrun the %d-character cut, so truncation never happened and the case was not exercised: %q",
			len(stdout), limit, text)
	}
	if !strings.Contains(text, "failed") {
		t.Fatalf("truncation removed the only trace that the call failed: %q", text)
	}
}
