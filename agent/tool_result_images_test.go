package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

var redSquare = ToolResultImage{MediaType: "image/png", Base64Data: "cmVk"}

func TestAToolResultsImagesGoAfterItsText(t *testing.T) {
	block := NewToolResultBlockWithImages("call_1", "saved", false, []ToolResultImage{redSquare})
	content := block.OfToolResult.Content
	if len(content) != 2 || content[0].OfText == nil || content[0].OfText.Text != "saved" {
		t.Fatalf("content = %+v", content)
	}
	if content[1].OfImage == nil || content[1].OfImage.Source.OfBase64.Data != "cmVk" || string(content[1].OfImage.Source.OfBase64.MediaType) != "image/png" {
		t.Fatalf("image block = %+v", content[1])
	}
}

// The OpenAI loop keeps its history in Anthropic's form; the images have to
// survive the conversion it makes before every call.
func TestToolResultImagesSurviveTheConversionToOpenAIMessages(t *testing.T) {
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(NewToolResultBlockWithImages("call_1", "saved", false, []ToolResultImage{redSquare})),
	}
	converted := ConvertAnthropicMessagesToOpenAI(messages)
	if len(converted) != 1 || converted[0].Role != "tool" || len(converted[0].ToolResultImages) != 1 || converted[0].ToolResultImages[0] != redSquare {
		t.Fatalf("converted = %+v", converted)
	}
}

// Chat completions take only text in a tool message, so the images go in a
// user message straight after the run of tool messages.
func TestChatCompletionsGetToolImagesInAUserMessageAfterTheToolMessages(t *testing.T) {
	messages := chatCompletionMessagesWithToolImages([]OpenAIMessage{
		{Role: "assistant", ToolCalls: []OpenAIToolCall{{ID: "call_1"}, {ID: "call_2"}}},
		{Role: "tool", ToolCallID: "call_1", Content: "saved", ToolResultImages: []ToolResultImage{redSquare}},
		{Role: "tool", ToolCallID: "call_2", Content: "plain"},
	})
	if len(messages) != 4 || messages[3].Role != "user" {
		t.Fatalf("messages = %+v", messages)
	}
	encoded, _ := json.Marshal(messages)
	if strings.Contains(string(encoded), "ToolResultImages") {
		t.Fatal("the internal image field was sent")
	}
	if !strings.Contains(string(encoded), `"image_url":{"url":"data:image/png;base64,cmVk"}`) || !strings.Contains(string(encoded), "call_1") {
		t.Fatalf("user message = %s", encoded)
	}
	if plain := chatCompletionMessagesWithToolImages([]OpenAIMessage{{Role: "tool", Content: "x"}}); len(plain) != 1 {
		t.Fatalf("messages without images changed: %+v", plain)
	}
}

// The subscription's Responses endpoint takes images inside the call's output.
func TestTheResponsesRequestPutsToolImagesInTheFunctionCallOutput(t *testing.T) {
	body, err := responsesRequestFromChatCompletion(OpenAIRequest{Messages: []OpenAIMessage{
		{Role: "tool", ToolCallID: "call_1", Content: "saved", ToolResultImages: []ToolResultImage{redSquare}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(body["input"])
	want := `[{"call_id":"call_1","output":[{"text":"saved","type":"input_text"},{"image_url":"data:image/png;base64,cmVk","type":"input_image"}],"type":"function_call_output"}]`
	if string(encoded) != want {
		t.Fatalf("input\n got %s\nwant %s", encoded, want)
	}
}

// recordingProvider answers from a list and keeps every request it was sent.
type recordingProvider struct {
	responses []*anthropic.Message
	requests  []anthropic.MessageNewParams
}

func (p *recordingProvider) Complete(ctx context.Context, params *anthropic.MessageNewParams) (*anthropic.Message, error) {
	p.requests = append(p.requests, *params)
	return p.responses[len(p.requests)-1], nil
}

func (p *recordingProvider) CompleteStreaming(ctx context.Context, params *anthropic.MessageNewParams) (StreamingResponse, error) {
	return nil, context.Canceled
}

// On the Anthropic path an image a tool attaches goes into its tool_result
// beside the text, which the model reads on the next call.
func TestOnTheAnthropicPathAToolsImageIsInItsToolResult(t *testing.T) {
	provider := &recordingProvider{responses: []*anthropic.Message{
		toolCallResponse(anthropic.Usage{}),
		finalTextResponse(anthropic.Usage{}),
	}}
	a := New(provider, "system")
	a.tools = []Tool{{
		Name: "spin", Description: "draws",
		Run: func(ctx context.Context, input string) (string, error) {
			AttachToolResultImage(ctx, redSquare)
			return "saved", nil
		},
	}}
	runOneTurn(t, a)

	if len(provider.requests) != 2 {
		t.Fatalf("the turn made %d calls, want 2", len(provider.requests))
	}
	encoded, _ := json.Marshal(provider.requests[1].Messages)
	if !strings.Contains(string(encoded), `"source":{"data":"cmVk","media_type":"image/png","type":"base64"}`) {
		t.Fatalf("the second call's messages carry no image: %s", encoded)
	}
}
