package session

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// The prompts/*.md breakdown is where a human goes to find out where a turn's
// context went. Until 2026-08-07 it rendered its own estimator, which charged
// a tool_use or a tool_result a flat 50 tokens: one 20KB read_files result
// was reported as 117 tokens where the honest estimator reports 7,066. These
// tests pin the breakdown to the shared estimator so that copy cannot come
// back.

func breakdownFor(t *testing.T, params *anthropic.MessageNewParams) (turn string, dir string) {
	t.Helper()

	sess, err := New(t.TempDir(), "claude-sonnet-4-20250514", "test", "", nil)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	t.Cleanup(func() { sess.Close() })

	if err := sess.WritePromptBreakdown(1, params, nil); err != nil {
		t.Fatalf("WritePromptBreakdown: %v", err)
	}

	promptsDir := filepath.Join(filepath.Dir(sess.FilePath()), "prompts")
	data, err := os.ReadFile(filepath.Join(promptsDir, "turn-1.md"))
	if err != nil {
		t.Fatalf("read turn-1.md: %v", err)
	}
	return string(data), promptsDir
}

// tokensInRow pulls the number out of a `| label | ~N (…) |` row.
func tokensInRow(t *testing.T, doc, label string) int {
	t.Helper()
	re := regexp.MustCompile(`(?m)^\|[^|\n]*` + regexp.QuoteMeta(label) + `[^|\n]*\|[^0-9\n]*([0-9]+)`)
	m := re.FindStringSubmatch(doc)
	if m == nil {
		t.Fatalf("no %q row in breakdown:\n%s", label, doc)
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("unparseable token count in %q row: %v", label, err)
	}
	return n
}

func requestWithABigToolResult(size int) *anthropic.MessageNewParams {
	return &anthropic.MessageNewParams{
		Model:  "claude-sonnet-4-20250514",
		System: []anthropic.TextBlockParam{{Text: strings.Repeat("y", 6000)}},
		Tools: []anthropic.ToolUnionParam{
			{OfTool: &anthropic.ToolParam{
				Name:        "read_files",
				Description: anthropic.String("Read files from disk"),
				InputSchema: anthropic.ToolInputSchemaParam{
					Properties: map[string]any{
						"paths": map[string]any{"type": "string", "description": strings.Repeat("d", 3000)},
					},
				},
			}},
		},
		Messages: []anthropic.MessageParam{
			{Role: anthropic.MessageParamRoleUser, Content: []anthropic.ContentBlockParamUnion{
				{OfText: &anthropic.TextBlockParam{Text: "read a.go"}},
			}},
			{Role: anthropic.MessageParamRoleUser, Content: []anthropic.ContentBlockParamUnion{
				{OfToolResult: &anthropic.ToolResultBlockParam{
					ToolUseID: "call_1",
					Content: []anthropic.ToolResultBlockParamContentUnion{
						{OfText: &anthropic.TextBlockParam{Text: strings.Repeat("x", size)}},
					},
				}},
			}},
		},
	}
}

// The defect, stated as the thing that must not come back.
func TestBreakdownCountsAToolResultsWholeSize(t *testing.T) {
	doc, _ := breakdownFor(t, requestWithABigToolResult(20000))

	// 20,000 bytes at ~3 chars/token is ~6,600. The old walk answered 117.
	if got := tokensInRow(t, doc, "Messages"); got < 5000 {
		t.Errorf("Messages row reports %d tokens for a 20KB tool result — it is being charged a constant", got)
	}
}

// The Total is asked of the request estimator while the rows are asked of the
// section estimators. If those ever stop agreeing, the table stops adding up
// in front of the person reading it.
func TestBreakdownRowsSumToItsTotal(t *testing.T) {
	doc, _ := breakdownFor(t, requestWithABigToolResult(9000))

	system := tokensInRow(t, doc, "System prompt")
	tools := tokensInRow(t, doc, "Tools")
	messages := tokensInRow(t, doc, "Messages")
	total := tokensInRow(t, doc, "**Total**")

	if system+tools+messages != total {
		t.Errorf("rows sum to %d (system %d + tools %d + messages %d) but Total says %d",
			system+tools+messages, system, tools, messages, total)
	}
	if system == 0 || tools == 0 || messages == 0 {
		t.Errorf("a section priced at zero: system %d, tools %d, messages %d", system, tools, messages)
	}
}

// The per-message rows in the same document are what a reader uses to find
// WHICH message is heavy. They were the fourth copy of the wrong walk.
func TestPerMessageRowsAgreeWithTheMessagesTotal(t *testing.T) {
	doc, _ := breakdownFor(t, requestWithABigToolResult(12000))

	rows := regexp.MustCompile(`(?m)^\| [0-9]+ \| (?:user|assistant) \| ([0-9]+) \|`).FindAllStringSubmatch(doc, -1)
	if len(rows) != 2 {
		t.Fatalf("expected 2 message rows, got %d:\n%s", len(rows), doc)
	}

	sum := 0
	for _, r := range rows {
		n, err := strconv.Atoi(r[1])
		if err != nil {
			t.Fatalf("unparseable per-message count %q: %v", r[1], err)
		}
		sum += n
	}

	if want := tokensInRow(t, doc, "Messages"); sum != want {
		t.Errorf("per-message rows sum to %d, Messages row says %d", sum, want)
	}
}

// prompts/system.md carries its own per-block table and its own Total. Both
// come from the shared estimator now, so they have to agree with each other
// and with the turn file's System prompt row.
func TestSystemIndexAgreesWithTheTurnFile(t *testing.T) {
	params := requestWithABigToolResult(2000)
	params.System = []anthropic.TextBlockParam{
		{Text: strings.Repeat("a", 4000)},
		{Text: strings.Repeat("b", 1200)},
	}

	doc, promptsDir := breakdownFor(t, params)

	data, err := os.ReadFile(filepath.Join(promptsDir, "system.md"))
	if err != nil {
		t.Fatalf("read system.md: %v", err)
	}
	index := string(data)

	blockRows := regexp.MustCompile(`(?m)^\| [0-9]+ \| \[[^\]]*\]\([^)]*\) \| ~([0-9]+) \|`).FindAllStringSubmatch(index, -1)
	if len(blockRows) != 2 {
		t.Fatalf("expected 2 block rows in system.md, got %d:\n%s", len(blockRows), index)
	}
	sum := 0
	for _, r := range blockRows {
		n, _ := strconv.Atoi(r[1])
		sum += n
	}

	indexTotal := regexp.MustCompile(`\*\*Total:\*\* ~([0-9]+) tokens`).FindStringSubmatch(index)
	if indexTotal == nil {
		t.Fatalf("no Total in system.md:\n%s", index)
	}
	got, _ := strconv.Atoi(indexTotal[1])

	if sum != got {
		t.Errorf("system.md block rows sum to %d, its Total says %d", sum, got)
	}
	if turnRow := tokensInRow(t, doc, "System prompt"); turnRow != got {
		t.Errorf("system.md Total is %d, turn-1.md System prompt row is %d", got, turnRow)
	}
}
