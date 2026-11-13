package agent

import (
	"errors"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
)

// ErrReplyStoppedByContentFilter ends a turn whose reply an OpenAI-compatible
// provider stopped with its content filter: finish_reason "content_filter" on
// Chat Completions, or incomplete_details.reason "content_filter" on the
// Responses routes (see chatCompletionFromResponsesStream).
//
// mapOpenAIFinishReason used to send that reason to its default case,
// end_turn, so the OpenAI turn loop returned a reply the provider stopped part
// way as a finished success. It now maps to anthropic.StopReasonRefusal, the
// Anthropic name for the same event, and the OpenAI loop handles it as it
// handles a reply cut off at max_tokens: only the text is stored, followed by
// a notice, Text and Incomplete are set, tool calls are dropped (none ran),
// and the caller gets this error.
//
// It is not evidence that the model or provider is unavailable, which is all
// ModelHealth can express: engine.recordModelHealth leaves the model's record
// alone. What a refusal on the Anthropic route should do is still open (todo
// 6b4a9ab5); this error does not decide it.
var ErrReplyStoppedByContentFilter = errors.New("model reply stopped by the provider's content filter")

// ContentFilteredReplyMessage builds what a conversation keeps of a reply the
// provider's content filter stopped: its text, then a notice. When the reply
// carried no text, keep is false and nothing should be appended.
func ContentFilteredReplyMessage(response *anthropic.Message) (message anthropic.MessageParam, text string, keep bool) {
	text = deliveredText(response)
	if text == "" {
		return anthropic.MessageParam{}, "", false
	}
	return anthropic.NewAssistantMessage(
		anthropic.NewTextBlock(text),
		anthropic.NewTextBlock(fmt.Sprintf(incompleteResponseNotice, ErrReplyStoppedByContentFilter)),
	), text, true
}
