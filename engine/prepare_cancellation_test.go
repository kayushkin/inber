package engine

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// summarizableTranscript builds a transcript that really does reach the
// summarizer: past the 60-message trigger, and made of plain user turns rather
// than tool-result batches, which findTurnBoundary does not count as turns at
// all. A fixture that stops short of the API call would let every test below
// pass for the wrong reason.
func summarizableTranscript(turns int) []anthropic.MessageParam {
	var messages []anthropic.MessageParam
	for i := 0; i < turns; i++ {
		messages = append(messages,
			anthropic.NewUserMessage(anthropic.NewTextBlock(fmt.Sprintf("question %d", i))),
			anthropic.NewAssistantMessage(anthropic.NewTextBlock(fmt.Sprintf("answer %d", i))),
		)
	}
	return messages
}

// The prepare phase is the half of a turn that looks like bookkeeping and is
// not: summarization is a full API call, made before the turn's own call. These
// tests pin that it stops when its caller stops, and they use an engine with no
// client so a call that should not have happened panics instead of quietly
// going out over the network.

func TestSummarizeIfNeededStopsOnACancelledTurn(t *testing.T) {
	messages := summarizableTranscript(40) // 80 messages, past the 60-message trigger
	e := &Engine{Messages: messages}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := e.summarizeIfNeeded(ctx)
	if err == nil {
		t.Fatal("summarizeIfNeeded returned nil on a cancelled context — the summary API call was made anyway")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want one wrapping context.Canceled", err)
	}
	if len(e.Messages) != len(messages) {
		t.Fatalf("a cancelled summarization still rewrote the conversation: %d → %d messages",
			len(messages), len(e.Messages))
	}
}

// The complement, so the test above cannot pass merely because this fixture
// never triggers summarization. On a live context the same engine reaches the
// API call and dies on its nil client — which is exactly the work a cancelled
// turn used to pay for.
func TestSummarizeIfNeededOnALiveContextReachesTheAPICall(t *testing.T) {
	e := &Engine{Messages: summarizableTranscript(40)}

	defer func() {
		if recover() == nil {
			t.Fatal("summarizeIfNeeded did not reach the API call on a live context — " +
				"the cancellation test above proves nothing")
		}
	}()

	_ = e.summarizeIfNeeded(context.Background())
}

// An explicit compact is not a step inside a turn, so a turn interrupt does not
// reach it. What stops it is the context of whoever asked for it — the HTTP
// request, in the one caller that exists.
func TestCompactContextStopsOnItsOwnCallersContext(t *testing.T) {
	e := &Engine{Messages: summarizableTranscript(40)}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := e.CompactContext(ctx, "")
	if err == nil {
		t.Fatal("CompactContext returned nil on a cancelled request context — " +
			"a client that hung up still paid for a summary")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want one wrapping context.Canceled", err)
	}
}
