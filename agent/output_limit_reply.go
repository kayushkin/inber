package agent

import (
	"errors"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
)

// ErrReplyCutOffAtOutputLimit ends a turn whose reply stopped on max_tokens:
// the model ran out of output budget before it finished.
//
// Both turn loops used to treat that stop like end_turn. They appended the
// reply as it came and returned success, so a long answer cut mid-sentence
// looked finished, and a tool_use block cut mid-arguments went into the
// conversation with JSON that does not parse. That block was never
// dispatched, so it had no tool_result, and its input could not be marshalled:
// the next request failed to build and the transcript write dropped the turn.
//
// A reply cut off this way is now handled like a stream that failed part way
// (see deliveredText): only its text is stored, followed by a notice that it
// was cut off, Text and Incomplete are set on the result, and the caller gets
// this error. Tool calls in the reply are dropped, because none of them ran.
//
// It is not evidence that the model or provider is unhealthy: the limit is the
// MaxTokens inber put on the request. engine.recordModelHealth leaves the
// model's record alone.
var ErrReplyCutOffAtOutputLimit = errors.New("model reply cut off at its output token limit (max_tokens)")

// CutOffReplyMessage builds what a conversation keeps of a reply that stopped
// on max_tokens: its text, then a notice that it was cut off. It also returns
// that text. When the reply carried no text, keep is false and nothing should
// be appended; the conversation then ends on the message the model was
// answering, as it does after ErrEmptyReply.
func CutOffReplyMessage(response *anthropic.Message) (message anthropic.MessageParam, text string, keep bool) {
	text = deliveredText(response)
	if text == "" {
		return anthropic.MessageParam{}, "", false
	}
	return anthropic.NewAssistantMessage(
		anthropic.NewTextBlock(text),
		anthropic.NewTextBlock(fmt.Sprintf(incompleteResponseNotice, ErrReplyCutOffAtOutputLimit)),
	), text, true
}
