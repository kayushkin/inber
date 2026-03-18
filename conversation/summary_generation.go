package conversation

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// generateSummary creates a concise summary using the specified LLM model
func generateSummary(
	ctx context.Context,
	client *anthropic.Client,
	conversationText string,
	model string,
	maxTokens int,
) (string, error) {
	
	systemPrompt := `You are an expert at summarizing conversations between a user and an AI assistant.

Your task is to create a concise summary that captures:
1. The main topics discussed
2. Key decisions made
3. Important information shared
4. Current project status/context
5. Any unresolved questions or next steps

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
		return "", fmt.Errorf("summarization API call failed: %w", err)
	}

	// Extract text from response
	var summaryParts []string
	for _, block := range response.Content {
		if block.Type == "text" {
			summaryParts = append(summaryParts, block.Text)
		}
	}

	if len(summaryParts) == 0 {
		return "", fmt.Errorf("no text content in summarization response")
	}

	return strings.Join(summaryParts, "\n"), nil
}

// mechanicalSummary creates a simple summary without using an LLM
func mechanicalSummary(messages []anthropic.MessageParam) string {
	userMsgCount := 0
	assistantMsgCount := 0
	toolCalls := 0
	
	topics := make(map[string]bool)
	
	// Simple analysis of message content
	for _, msg := range messages {
		switch msg.Role {
		case anthropic.MessageParamRoleUser:
			userMsgCount++
			// Try to extract basic topics from user messages
			for _, block := range msg.Content {
				if block.OfText != nil {
					words := strings.Fields(strings.ToLower(block.OfText.Text))
					for _, word := range words {
						if len(word) > 6 && !strings.Contains(word, "please") && !strings.Contains(word, "could") {
							topics[word] = true
							if len(topics) >= 5 { // Limit topics
								break
							}
						}
					}
				}
			}
			
		case anthropic.MessageParamRoleAssistant:
			assistantMsgCount++
			// Count tool calls
			for _, block := range msg.Content {
				if block.OfToolUse != nil {
					toolCalls++
				}
			}
		}
	}
	
	summary := fmt.Sprintf("Conversation summary: %d conversation turns, %d assistant responses", userMsgCount, assistantMsgCount)
	
	if toolCalls > 0 {
		summary += fmt.Sprintf(", %d tool calls", toolCalls)
	}
	
	if len(topics) > 0 {
		var topicList []string
		for topic := range topics {
			topicList = append(topicList, topic)
		}
		summary += fmt.Sprintf(". Topics discussed: %s", strings.Join(topicList, ", "))
	}
	
	return summary
}