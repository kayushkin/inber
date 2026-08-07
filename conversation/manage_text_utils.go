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
		total += estimateMarshalled(block)
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

// EstimateMessageTokens is estimateMessageTokens for callers outside this
// package. It exists so that anything reporting a per-message number reports
// the same one the prune gates weigh, rather than walking the content blocks
// again with a different rule.
func EstimateMessageTokens(msg anthropic.MessageParam) int {
	return estimateMessageTokens(msg)
}

// estimateMarshalled prices one value the way it will be sent: marshalled to
// the JSON the SDK puts on the wire, then measured with memory-store's
// estimator. Every estimator in this file is this function with a different
// argument, which is what keeps a system block, a tool definition and a
// content block from being counted by three different rules.
//
// A value that fails to marshal is counted as zero because it cannot be sent
// either.
func estimateMarshalled(v any) int {
	encoded, err := json.Marshal(v)
	if err != nil {
		return 0
	}
	return memory.EstimateTokens(string(encoded))
}

// EstimateSystemBlockTokens prices one system prompt block.
func EstimateSystemBlockTokens(block anthropic.TextBlockParam) int {
	return estimateMarshalled(block)
}

// EstimateSystemTokens prices a request's whole system prompt.
func EstimateSystemTokens(blocks []anthropic.TextBlockParam) int {
	total := 0
	for _, block := range blocks {
		total += EstimateSystemBlockTokens(block)
	}
	return total
}

// EstimateToolTokens prices one tool definition — its name, description and
// the whole of its input schema.
//
// Marshalling rather than adding up a flat per-parameter constant is what
// makes this survive the schema injection inber does to every tool: the
// `then` chain and the sideband fields are added to a tool's schema on the
// way out (agent.AddChainAndSidebandFields), so a count derived from the
// registered parameter list prices a schema that is not the one being sent.
func EstimateToolTokens(tool anthropic.ToolUnionParam) int {
	return estimateMarshalled(tool)
}

// EstimateToolsTokens prices a request's whole tools block.
func EstimateToolsTokens(tools []anthropic.ToolUnionParam) int {
	total := 0
	for _, tool := range tools {
		total += EstimateToolTokens(tool)
	}
	return total
}

// EstimateRequestTokens prices everything a request puts in front of the
// model: its system prompt, its tools block and its messages.
//
// This is the number the message-list estimators cannot produce. The system
// blocks, the memory blocks folded into them and the tools block are several
// thousand tokens that no gate in the tree could see, so "does this prompt
// fit in the context window" was not a question inber could ask about itself.
//
// ⚠️ The prune gates deliberately do NOT call this yet. ShouldPrune's token
// branch and engine.pruneIfNeeded's emergency flush both compare against
// thresholds (TokenBudget, TokenBudget*2) that were tuned against the
// message-list number; feeding them a number several thousand tokens larger
// would move every one of those thresholds without anyone choosing to. What
// the gates should do once they can see the whole request is the open half of
// noteboard todo `939d5fdc` and is the user's call.
//
// MaxTokens and the thinking budget are not counted: both bound the model's
// OUTPUT, and this measures what is sent.
func EstimateRequestTokens(params *anthropic.MessageNewParams) int {
	if params == nil {
		return 0
	}
	return EstimateSystemTokens(params.System) +
		EstimateToolsTokens(params.Tools) +
		EstimateTokens(params.Messages)
}