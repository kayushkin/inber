package conversation

import (
	"encoding/json"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/internal/textutil"
	"github.com/kayushkin/inber/memory"
)

// truncateToOneLine flattens text onto one line and cuts it to maxLen bytes,
// on a rune boundary. What it returns goes into the model's prompt, so a
// half-rune here is invalid UTF-8 in the request.
func truncateToOneLine(text string, maxLen int) string {
	// Remove newlines
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.ReplaceAll(text, "\r", "")
	text = strings.TrimSpace(text)
	
	return textutil.TruncateWith(text, maxLen, "...")
}

// truncateToSummary extracts first 2-3 sentences from text
func truncateToSummary(text string) string {
	// Split into sentences (simple approach)
	sentences := strings.FieldsFunc(text, func(r rune) bool {
		return r == '.' || r == '!' || r == '?'
	})
	
	var result []string
	charCount := 0
	for i, sentence := range sentences {
		if i >= 3 { // Max 3 sentences
			break
		}
		sentence = strings.TrimSpace(sentence)
		if len(sentence) < 10 { // Skip very short fragments
			continue
		}
		result = append(result, sentence)
		charCount += len(sentence)
		if charCount > 300 { // Max ~300 chars
			break
		}
	}
	
	if len(result) == 0 {
		// Fallback: just take first 300 bytes
		return textutil.TruncateWith(text, 300, "...")
	}
	
	return strings.Join(result, ". ") + "."
}

// extractTextBlockContent joins the prose in a message's text blocks, and
// deliberately sees nothing else: no tool result, no tool call input, no
// thinking. Its one caller reads assistant prose looking for decisions worth
// saving to memory, so tool traffic would be noise there.
//
// It used to be what estimateMessageTokens measured as well, which is the bug
// that function's comment describes. Do not reach for this to size anything.
func extractTextBlockContent(content []anthropic.ContentBlockParamUnion) string {
	var parts []string
	for _, block := range content {
		if block.OfText != nil {
			parts = append(parts, block.OfText.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// estimateMessageTokens estimates how much of a request's budget a message
// occupies. It measures the message as it will be SENT — every content block
// marshalled the way the SDK marshals it into the request — rather than the
// text blocks alone.
//
// Counting only text blocks was the bug this replaces, and it was not the
// "3-4x undercount" the old ShouldPrune comment claimed. A tool result and a
// tool call's input are the two largest things in an agentic conversation and
// a text-block walk sees neither: a 24-message conversation carrying 20KB of
// read_files output per turn estimated at 48 tokens. Both readers of this
// number compare it against a budget derived from the model's context window
// — ShouldPrune's token branch, and the emergency flush in
// engine.pruneIfNeeded — so neither could fire on the conversations they exist
// for, and neither could the prune that agent.executeAPICall retries with
// after the API rejects a turn for length.
//
// Marshalling rather than enumerating is what keeps this honest as the SDK
// grows. ContentBlockParamUnion carries eighteen block kinds today; picking
// the text-bearing ones out by hand would silently count the nineteenth as
// zero, which is exactly how this got here. A block that fails to marshal is
// counted as zero because it cannot be sent either.
//
// The number this returns runs slightly high — JSON field names and escaping
// are not tokens — and that is the safe direction for an overflow guard: it
// prunes a little early rather than letting a request exceed the window.
func estimateMessageTokens(msg anthropic.MessageParam) int {
	total := 0
	for _, block := range msg.Content {
		encoded, err := json.Marshal(block)
		if err != nil {
			continue
		}
		total += memory.EstimateTokens(string(encoded))
	}
	return total
}

// EstimateTokens returns a rough token count for all messages in a conversation.
func EstimateTokens(messages []anthropic.MessageParam) int {
	total := 0
	for _, msg := range messages {
		total += estimateMessageTokens(msg)
	}
	return total
}