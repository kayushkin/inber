package conversation

import (
	"fmt"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// A pruned tool result used to be labelled by sniffing its content, and the
// first guess — "more than five lines means a shell command" — swallowed
// everything else. A file read must be reported as a file read.
func TestPrunedResultNamesTheToolThatProducedIt(t *testing.T) {
	content := repeatLines("line %d of a Go source file", 30)
	summary := summarizeToolResult("read_files", content)

	if !strings.HasPrefix(summary, "[read_files:") {
		t.Fatalf("a read_files result was summarised as %q", summary)
	}
	if strings.Contains(summary, "shell") {
		t.Fatalf("a read_files result was labelled shell: %q", summary)
	}
}

// Every summary states how much was dropped. Three of the old branches
// ([shell: N lines], [listed N files], [search: N results]) gave a line count
// and no byte count, so the transcript could not say what the result cost.
func TestEverySummaryKeepsBothCounts(t *testing.T) {
	cases := []struct{ name, content string }{
		{"shell_commands", "make: entering directory\nbuilding\nlinking\ndone\nexit code 0\nand more output\n"},
		{"list_files", repeatLines("/home/user/repo/pkg/file%d.go", 40)},
		{"ripgrep", repeatLines("pkg/file%d.go:42: found a match here", 12)},
		{"memory_search", "found 3 results\nresults follow\nfirst\nsecond"},
		{"write_files", "wrote 4096 bytes to /tmp/out"},
		{"web_fetch", strings.Repeat("a paragraph of prose fetched from the web. ", 10)},
		{"", "an unpaired result with no tool_use to name it, padded out past a hundred bytes so it is summarised"},
	}
	for _, c := range cases {
		summary := summarizeToolResult(c.name, c.content)
		if !strings.Contains(summary, fmt.Sprintf("%d bytes", len(c.content))) {
			t.Errorf("%q summary lost the byte count: %q", c.name, summary)
		}
		if !strings.Contains(summary, "lines") {
			t.Errorf("%q summary lost the line count: %q", c.name, summary)
		}
	}
}

// When the pairing is missing the summary says so. It never falls back to
// guessing a tool from the content — that guess is the defect.
func TestUnpairedResultIsNamedAsUnknown(t *testing.T) {
	summary := summarizeToolResult("", "exit code 0\nand five\nmore\nlines\nof\noutput\n")
	if !strings.HasPrefix(summary, "[tool result:") {
		t.Fatalf("an unpaired result was summarised as %q", summary)
	}
}

// The old code had a [read file: ...] branch that could never be produced: it
// required more than 20 lines, and more than 5 lines had already been claimed
// by the shell branch above it. Nothing in the package may reintroduce a label
// that no input reaches — this test sweeps the whole shape space and asserts
// every summary names its tool.
func TestSummaryLabelIsAlwaysTheToolName(t *testing.T) {
	bodies := []string{"plain text", "x", "a/b path", "found results", "wrote it", "exit code 1", "error: no"}
	for lines := 1; lines <= 60; lines++ {
		for _, body := range bodies {
			content := strings.TrimSuffix(strings.Repeat(body+"\n", lines), "\n")
			summary := summarizeToolResult("shell_commands", content)
			if !strings.HasPrefix(summary, "[shell_commands: ") {
				t.Fatalf("%d lines of %q summarised as %q", lines, body, summary)
			}
		}
	}
}

// Dropping a result outright is the biggest reduction of all, so the marker
// left behind names the tool and the size rather than only saying something
// was here.
func TestDroppedResultRecordsTheToolAndTheSize(t *testing.T) {
	content := repeatLines("output line %d", 40)
	block := toolResultBlock("toolu_drop", content)
	cfg := DefaultManagementConfig()

	pruned, wasPruned := pruneToolResult(block, cfg.ToolResultDrop, cfg, "ripgrep")
	if !wasPruned {
		t.Fatal("an over-age result was not dropped")
	}
	text := resultText(t, pruned)
	if !strings.Contains(text, "ripgrep") {
		t.Errorf("drop marker does not name the tool: %q", text)
	}
	if !strings.Contains(text, fmt.Sprintf("%d bytes", len(content))) {
		t.Errorf("drop marker does not say how much was dropped: %q", text)
	}
}

// The live path: ManageConversation must pair the ids itself. A result whose
// call is several messages back still gets named.
func TestManageConversationNamesPrunedResults(t *testing.T) {
	cfg := DefaultManagementConfig()
	cfg.KeepRecentTurns = 1
	cfg.ManageInterval = 0
	content := repeatLines("line %d of a source file being read", 30)

	messages := []anthropic.MessageParam{
		{Role: anthropic.MessageParamRoleUser, Content: []anthropic.ContentBlockParamUnion{textBlock("read the file")}},
		{Role: anthropic.MessageParamRoleAssistant, Content: []anthropic.ContentBlockParamUnion{toolUseBlock("toolu_read", "read_files")}},
		{Role: anthropic.MessageParamRoleUser, Content: []anthropic.ContentBlockParamUnion{toolResultBlock("toolu_read", content)}},
	}
	for i := 0; i < cfg.ToolResultKeepFull+1; i++ {
		messages = append(messages,
			anthropic.MessageParam{Role: anthropic.MessageParamRoleAssistant, Content: []anthropic.ContentBlockParamUnion{textBlock("thinking")}},
			anthropic.MessageParam{Role: anthropic.MessageParamRoleUser, Content: []anthropic.ContentBlockParamUnion{textBlock("carry on")}},
		)
	}

	managed, _, err := ManageConversation(t.Context(), messages, nil, "sess-1", cfg)
	if err != nil {
		t.Fatal(err)
	}

	summary := findResultText(managed, "toolu_read")
	if summary == "" {
		t.Fatal("the tool result disappeared from the managed conversation")
	}
	if summary == content {
		t.Fatalf("the fixture never reached the pruning thresholds; summary is the original content")
	}
	if !strings.Contains(summary, "read_files") {
		t.Fatalf("the live pruning path lost the tool name: %q", summary)
	}
}

// The staging zone is pruned separately, and a result there can answer a call
// made back in the frozen zone. Pairing over the staging slice alone loses the
// name for exactly those results.
func TestStagingPairsAcrossTheFrozenBoundary(t *testing.T) {
	cfg := DefaultManagementConfig()
	content := repeatLines("line %d of a file read before the freeze point", 30)

	messages := []anthropic.MessageParam{
		{Role: anthropic.MessageParamRoleUser, Content: []anthropic.ContentBlockParamUnion{textBlock("read it")}},
		{Role: anthropic.MessageParamRoleAssistant, Content: []anthropic.ContentBlockParamUnion{toolUseBlock("toolu_frozen", "read_files")}},
		// frozen boundary falls here: the call is behind it, the result ahead
		{Role: anthropic.MessageParamRoleUser, Content: []anthropic.ContentBlockParamUnion{toolResultBlock("toolu_frozen", content)}},
	}
	for i := 0; i < cfg.ToolResultKeepFull+1; i++ {
		messages = append(messages,
			anthropic.MessageParam{Role: anthropic.MessageParamRoleAssistant, Content: []anthropic.ContentBlockParamUnion{textBlock("thinking")}},
			anthropic.MessageParam{Role: anthropic.MessageParamRoleUser, Content: []anthropic.ContentBlockParamUnion{textBlock("carry on")}},
		)
	}

	ManageStaging(messages, 2, cfg)

	summary := findResultText(messages, "toolu_frozen")
	if summary == content {
		t.Fatal("the fixture never reached the pruning thresholds; the result was left whole")
	}
	if !strings.Contains(summary, "read_files") {
		t.Fatalf("a staged result whose call is in the frozen zone lost its name: %q", summary)
	}
}

// A pruned tool call keeps its id and name, so a second management pass can
// still pair results with it.
func TestPairingSurvivesATruncatedToolCall(t *testing.T) {
	block := truncateToolCall(toolUseBlock("toolu_x", "shell_commands"))
	names := toolNamesByUseID([]anthropic.MessageParam{
		{Role: anthropic.MessageParamRoleAssistant, Content: []anthropic.ContentBlockParamUnion{block}},
	})
	if names["toolu_x"] != "shell_commands" {
		t.Fatalf("pairing after truncation gave %q", names["toolu_x"])
	}
}

// A call with no id must not become a pairing: the entry would answer to every
// result that also has no id and give it a name it never had.
func TestIdlessCallIsNotPaired(t *testing.T) {
	names := toolNamesByUseID([]anthropic.MessageParam{
		{Role: anthropic.MessageParamRoleAssistant, Content: []anthropic.ContentBlockParamUnion{toolUseBlock("", "shell_commands")}},
	})
	if len(names) != 0 {
		t.Fatalf("an id-less call was paired: %v", names)
	}
}

// A result whose first line is blank must not leave a trailing space on the
// summary — the marker is the whole content of the block.
func TestSummaryHasNoTrailingSpace(t *testing.T) {
	summary := summarizeToolResult("read_files", "\n"+repeatLines("line %d", 20))
	if strings.HasSuffix(summary, " ") {
		t.Fatalf("summary ends in a space: %q", summary)
	}
}

func repeatLines(format string, n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, format+"\n", i)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func textBlock(text string) anthropic.ContentBlockParamUnion {
	return anthropic.ContentBlockParamUnion{OfText: &anthropic.TextBlockParam{Text: text}}
}

func toolUseBlock(id, name string) anthropic.ContentBlockParamUnion {
	return anthropic.ContentBlockParamUnion{
		OfToolUse: &anthropic.ToolUseBlockParam{ID: id, Name: name, Input: map[string]interface{}{"path": "/tmp/x"}},
	}
}

func toolResultBlock(id, content string) anthropic.ContentBlockParamUnion {
	return anthropic.ContentBlockParamUnion{
		OfToolResult: &anthropic.ToolResultBlockParam{
			ToolUseID: id,
			Content: []anthropic.ToolResultBlockParamContentUnion{
				{OfText: &anthropic.TextBlockParam{Text: content}},
			},
		},
	}
}

func resultText(t *testing.T, block anthropic.ContentBlockParamUnion) string {
	t.Helper()
	if block.OfToolResult == nil {
		t.Fatal("pruning dropped the tool_result block")
	}
	return extractToolResultContent(block.OfToolResult.Content)
}

func findResultText(messages []anthropic.MessageParam, toolUseID string) string {
	for _, msg := range messages {
		for _, block := range msg.Content {
			if block.OfToolResult != nil && block.OfToolResult.ToolUseID == toolUseID {
				return extractToolResultContent(block.OfToolResult.Content)
			}
		}
	}
	return ""
}
