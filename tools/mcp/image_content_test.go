package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/kayushkin/inber/agent"
)

// The client used to keep only a reply's text, so a model never saw an image
// a tool returned. An image item now reaches the call's tool result.
func TestAnImageInAReplyReachesTheCallsToolResult(t *testing.T) {
	server := newTestServer(t)
	server.client.tools["draw"] = ToolInfo{Name: "draw"}
	ctx, attached := agent.ToolResultImagesFor(context.Background())

	done := make(chan string, 1)
	go func() {
		out, err := server.client.CallTool(ctx, "draw", `{}`)
		if err != nil {
			t.Errorf("call: %v", err)
		}
		done <- out
	}()
	request := server.nextRequest(t)
	server.reply(t, request.ID, `{"content":[{"type":"text","text":"saved to a.png"},{"type":"image","data":"aW1hZ2U=","mimeType":"image/png"}]}`)

	select {
	case out := <-done:
		if out != "saved to a.png" {
			t.Errorf("text = %q", out)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the call never returned")
	}
	images := attached()
	if len(images) != 1 || images[0].MediaType != "image/png" || images[0].Base64Data != "aW1hZ2U=" {
		t.Fatalf("attached images = %+v", images)
	}
}

// A failed call reports its text as the error and attaches nothing.
func TestAFailedCallAttachesNoImage(t *testing.T) {
	server := newTestServer(t)
	server.client.tools["draw"] = ToolInfo{Name: "draw"}
	ctx, attached := agent.ToolResultImagesFor(context.Background())

	done := make(chan error, 1)
	go func() {
		_, err := server.client.CallTool(ctx, "draw", `{}`)
		done <- err
	}()
	request := server.nextRequest(t)
	server.reply(t, request.ID, `{"isError":true,"content":[{"type":"text","text":"no"},{"type":"image","data":"aW1hZ2U=","mimeType":"image/png"}]}`)
	if err := <-done; err == nil {
		t.Fatal("a failed call returned no error")
	}
	if images := attached(); len(images) != 0 {
		t.Fatalf("a failed call attached %d images", len(images))
	}
}
