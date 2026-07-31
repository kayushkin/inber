package engine

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// A tool call's arguments reach messageContent as a json.RawMessage. Rendering
// those bytes with %v produces about 3.7 characters per byte of decimal codes,
// so the blueprint's token estimate for the message inflates by the same
// factor.
func TestMessageContentEstimatesToolCallTokensFromItsText(t *testing.T) {
	input := json.RawMessage(`{"agent":"brigid","task":"summarise the release notes for v2"}`)
	msg := anthropic.MessageParam{
		Role: anthropic.MessageParamRoleAssistant,
		Content: []anthropic.ContentBlockParamUnion{
			{OfToolUse: &anthropic.ToolUseBlockParam{
				ID:    "toolu_01",
				Name:  "spawn_agent",
				Input: input,
			}},
		},
	}

	content := messageContent(msg)
	if strings.Contains(content, "[123 34") {
		t.Fatalf("tool input rendered as decimal byte codes: %q", content)
	}
	if !strings.Contains(content, `spawn_agent:{"agent":"brigid"`) {
		t.Fatalf("tool input did not render as its JSON text: %q", content)
	}

	// name + ":" + the arguments verbatim, so the estimate tracks what the
	// model is actually charged for.
	if want := len("spawn_agent:") + len(input); len(content) != want {
		t.Fatalf("content length = %d, want %d: %q", len(content), want, content)
	}
	if got, want := estimateTokensStr(content), 19; got != want {
		t.Fatalf("token estimate = %d, want %d", got, want)
	}
}
