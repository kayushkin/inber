package agent_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
)

// A reply that wrote some text, then began a tool call and ran out of output
// budget in the middle of its arguments.
var textThenTruncatedToolUseAtMaxTokens = []string{
	`{"type":"message_start","message":{"role":"assistant"}}`,
	`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
	`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Writing the file now."}}`,
	`{"type":"content_block_stop","index":0}`,
	`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_2","name":"look","input":{}}}`,
	`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"path\":\"a.go\",\"body\":\"package ma"}}`,
	`{"type":"message_delta","delta":{"stop_reason":"max_tokens"},"usage":{"output_tokens":16384}}`,
}

// assertEveryMessageMarshals is the property the defect broke: a truncated
// tool_use input is invalid JSON, so the next request could not be built and
// the transcript write failed.
func assertEveryMessageMarshals(t *testing.T, messages []anthropic.MessageParam) {
	t.Helper()
	for i, message := range messages {
		if len(message.Content) == 0 {
			t.Fatalf("message %d (%s) has no content — the next request is a 400", i, message.Role)
		}
		if _, err := json.Marshal(message); err != nil {
			t.Fatalf("message %d (%s) does not marshal: %v", i, message.Role, err)
		}
		for _, block := range message.Content {
			if message.Role == anthropic.MessageParamRoleAssistant && block.OfToolUse != nil && block.OfToolUse.ID == "toolu_2" {
				t.Fatalf("message %d keeps the tool call that was cut off and never ran", i)
			}
		}
	}
}

// TestAReplyCutOffAtMaxTokensKeepsOnlyItsTextAndIsAnError is the defect: the
// reply was appended whole, truncated tool call and all, and returned as a
// successful turn.
func TestAReplyCutOffAtMaxTokensKeepsOnlyItsTextAndIsAnError(t *testing.T) {
	result, err, messages := runToolThen(t, textThenTruncatedToolUseAtMaxTokens)

	if !errors.Is(err, agent.ErrReplyCutOffAtOutputLimit) {
		t.Fatalf("err = %v, want ErrReplyCutOffAtOutputLimit", err)
	}
	if !result.Incomplete {
		t.Error("result.Incomplete must be set: the reply stopped short")
	}
	if result.Text != "Writing the file now." {
		t.Errorf("result.Text = %q, want the text the model delivered", result.Text)
	}
	if len(result.APICalls) != 2 {
		t.Errorf("recorded %d API calls, want 2 — the cut-off reply was still billed", len(result.APICalls))
	}
	assertEveryMessageMarshals(t, messages)
	if len(messages) != 4 {
		t.Fatalf("conversation has %d messages, want user, tool_use, tool_result, cut-off text", len(messages))
	}
	last := messages[3]
	if last.Role != anthropic.MessageParamRoleAssistant || len(last.Content) != 2 ||
		last.Content[0].OfText == nil || last.Content[0].OfText.Text != "Writing the file now." ||
		last.Content[1].OfText == nil || !strings.Contains(last.Content[1].OfText.Text, "response cut off") {
		t.Fatalf("last message = %+v, want the text then a cut-off notice", last)
	}
}

// TestAReplyCutOffAtMaxTokensWithNoTextStoresNothing: a reply that was only a
// truncated tool call leaves the conversation ending on the tool result.
func TestAReplyCutOffAtMaxTokensWithNoTextStoresNothing(t *testing.T) {
	result, err, messages := runToolThen(t, []string{
		`{"type":"message_start","message":{"role":"assistant"}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_2","name":"look","input":{}}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"path\":\"a"}}`,
		`{"type":"message_delta","delta":{"stop_reason":"max_tokens"},"usage":{"output_tokens":16384}}`,
	})

	if !errors.Is(err, agent.ErrReplyCutOffAtOutputLimit) {
		t.Fatalf("err = %v, want ErrReplyCutOffAtOutputLimit", err)
	}
	if result.Incomplete || result.Text != "" {
		t.Errorf("Incomplete %v, Text %q; want neither, the user saw nothing", result.Incomplete, result.Text)
	}
	assertEveryMessageMarshals(t, messages)
	assertConversationIsSendable(t, messages)
}
