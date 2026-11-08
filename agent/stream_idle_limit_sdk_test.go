package agent_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/kayushkin/inber/agent"
)

// TestTheRealSDKStreamEndsWhenTheIdleWatchdogFires checks the assumption the
// watchdog rests on: cancelling the context a real SDK stream was opened under
// unblocks a Next that is waiting on a silent socket. The server sends the
// opening events and then holds the connection open, sending nothing.
func TestTheRealSDKStreamEndsWhenTheIdleWatchdogFires(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"m\",\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n")
		fmt.Fprint(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
		fmt.Fprint(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Half an answer\"}}\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	defer close(release)

	client := anthropic.NewClient(option.WithAPIKey("test"), option.WithBaseURL(server.URL), option.WithMaxRetries(0))
	provider := agent.NewAnthropicProvider(&client)

	a := agent.New(provider, "system")
	a.SetStreamIdleLimit(200 * time.Millisecond)
	var seen string
	a.SetHooks(&agent.Hooks{OnTextDelta: func(text string) { seen += text }})
	messages := []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("explain"))}

	done := make(chan error, 1)
	var result *agent.TurnResult
	go func() {
		var err error
		result, err = a.Run(t.Context(), "claude-sonnet-4-5-20250929", &messages)
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, agent.ErrStreamIdle) {
			t.Fatalf("err = %v, want ErrStreamIdle", err)
		}
		if seen != "Half an answer" || result.Text != "Half an answer" {
			t.Errorf("seen %q, result.Text %q; want the partial text kept", seen, result.Text)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the real SDK stream did not end when the idle watchdog cancelled its context")
	}
}
