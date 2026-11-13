package agent

import (
	"errors"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// ErrEmptyReply ends a turn whose model finished (end_turn) without saying
// anything: no content blocks at all, only blank text, or only thinking.
//
// Anthropic documents this reply as ordinary, most often straight after a tool
// result, and the OpenAI-compatible route produces the same shape from a 200
// with an empty choices array. Both used to be appended to the conversation as
// they came. A content-less assistant message in the middle of a conversation
// is a 400 from the Messages API, so the next turn and every one after it
// failed until the session was rebuilt, and the turn itself reported success
// with empty text.
//
// The reply is not appended, so the conversation stays a valid shape: it ends
// on the user message the model was answering, and the next user message is
// merged into it (engine/turn_prepare.go). Nothing is retried and no
// placeholder is stored, because either would put words in the transcript the
// model or the user never wrote; the caller sees this error instead.
//
// It is not evidence that the model or provider is unhealthy — the provider
// answered — so engine.recordModelHealth leaves the model's record alone.
var ErrEmptyReply = errors.New("model ended its turn with an empty reply")

// ReplyAnswersNothing reports whether a response that ended the turn carries
// nothing the user can read and nothing the conversation could act on: every
// block is thinking, redacted thinking or blank text. A response with no
// blocks at all answers nothing too.
func ReplyAnswersNothing(response *anthropic.Message) bool {
	for _, block := range response.Content {
		switch block.Type {
		case "thinking", "redacted_thinking":
			continue
		case "text":
			if strings.TrimSpace(block.Text) != "" {
				return false
			}
		default:
			return false
		}
	}
	return true
}
