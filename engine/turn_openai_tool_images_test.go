package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kayushkin/inber/agent"
)

// An image a tool attaches reaches the model with the next request. Over chat
// completions that is a user message after the tool message, since a tool
// message there may hold only text.
func TestAnImageAToolAttachesReachesTheNextRequest(t *testing.T) {
	drawing := agent.Tool{
		Name:        "draw",
		Description: "draws",
		Run: func(ctx context.Context, input string) (string, error) {
			agent.AttachToolResultImage(ctx, agent.ToolResultImage{MediaType: "image/png", Base64Data: "cmVk"})
			return "saved to a.png", nil
		},
	}
	f := newFakeOpenAI(t, toolCallResponse("call_1", "draw", `{}`), textResponse("it is red"))
	e := openAIEngine(t, f, drawing)

	if _, err := e.runOpenAITurn(context.Background(), nil); err != nil {
		t.Fatalf("runOpenAITurn: %v", err)
	}
	if len(f.requests) != 2 {
		t.Fatalf("the loop made %d requests, want 2", len(f.requests))
	}
	encoded, _ := json.Marshal(f.request(1).Messages)
	if !strings.Contains(string(encoded), `"image_url":{"url":"data:image/png;base64,cmVk"}`) {
		t.Fatalf("the second request carries no image: %s", encoded)
	}
}
