package conversation

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// generateSummary creates a concise summary using the specified LLM model.
//
// The second return says the model ran out of output tokens before it finished
// writing. That is not the same as an error and it is deliberately not returned
// as one: what compaction should do about a half-written summary is an open
// question (abort, retry with a larger ceiling, or accept it and say so in the
// injected block), and each answer has a cost the caller is the one to weigh.
//
// What is not open is that the fact must leave this function. Compaction is
// destructive — the caller assigns the summarized array over e.Messages and
// writes it across both messages.json copies — so a summary cut off mid-sentence
// becomes the session's only transcript. Before this, the one signal that said
// so was read by nobody: the call returned err == nil and the engine logged the
// same success line it logs for a complete summary.
func generateSummary(
	ctx context.Context,
	client *anthropic.Client,
	conversationText string,
	model string,
	maxTokens int,
) (summary string, cutOffAtTokenLimit bool, err error) {
	// Stop before the round-trip, not during it. The SDK would reject a
	// cancelled context anyway, but only after building the request and opening
	// the connection, and a caller that has already been told to stop should not
	// be assembling a prompt out of the whole transcript to do it.
	if err := ctx.Err(); err != nil {
		return "", false, fmt.Errorf("summarization cancelled before the API call: %w", err)
	}

	systemPrompt := `You are an expert at summarizing conversations between a user and an AI assistant.

Your task is to create a concise summary that captures:
1. The main topics discussed
2. Key decisions made
3. Important information shared
4. Current project status/context
5. Any unresolved questions or next steps

A tool result written as [tool_result failed: ...] is a call that did NOT succeed. Its output may read like a result, and it may have been cut short before the error text. Never record such a call as a confirmed result — say what was attempted and that it failed, so the work is not treated as done later.

Focus on actionable information and context that would be useful for continuing the conversation. Avoid unnecessary details while preserving important context.

Keep the summary under the token limit. Write in clear, bullet-point format when appropriate.`

	userPrompt := fmt.Sprintf(`Please summarize this conversation:

%s

Create a concise summary that captures the key points and context needed to continue this conversation effectively.`, conversationText)

	// Construct the API request
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: int64(maxTokens),
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userPrompt)),
		},
	}

	// Make the API call
	response, err := client.Messages.New(ctx, params)
	if err != nil {
		return "", false, fmt.Errorf("summarization API call failed: %w", err)
	}

	// Extract text from response
	var summaryParts []string
	for _, block := range response.Content {
		if block.Type == "text" {
			summaryParts = append(summaryParts, block.Text)
		}
	}

	if len(summaryParts) == 0 {
		return "", false, fmt.Errorf("no text content in summarization response")
	}

	// MaxTokens is the expected case rather than the edge here: the coder role
	// condenses 40 messages into 800 output tokens and the orchestrator 80 into
	// 1024, so the ceiling is reached by an ordinary long session, not only by a
	// runaway one.
	cutOffAtTokenLimit = response.StopReason == anthropic.StopReasonMaxTokens

	return strings.Join(summaryParts, "\n"), cutOffAtTokenLimit, nil
}
