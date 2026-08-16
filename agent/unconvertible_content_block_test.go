package agent_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
)

// accumulateFrames replays raw stream frames into a Message the way
// Agent.executeAPICall's loop does, and reports what Accumulate said about each
// one. Building the fixture through the SDK rather than writing a
// ContentBlockUnion literal is deliberate: the defect is what the SDK does with
// a frame, so a hand-built block would be a fixture derived from the conclusion
// and would keep passing if Accumulate's behaviour changed underneath it.
func accumulateFrames(t *testing.T, rawFrames []string) (*anthropic.Message, []error) {
	t.Helper()
	var accumulated anthropic.Message
	var errs []error
	for _, raw := range rawFrames {
		var event anthropic.MessageStreamEventUnion
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			t.Fatalf("the fixture frame %s is not a stream event: %v", raw, err)
		}
		errs = append(errs, accumulated.Accumulate(event))
	}
	return &accumulated, errs
}

// TestAnUnknownContentBlockTypePanicsToParam is the premise this guard rests on.
// Without it, a green suite cannot tell "the guard works" from "there was
// nothing to guard against" — and the premise is not obvious, because the block
// arrives through an Accumulate call that reports no error at all.
//
// Measured against anthropic-sdk-go v1.35.0. If a later SDK returns an error
// here, or converts the block, this test says so at the bump instead of leaving
// a defence nobody can date.
func TestAnUnknownContentBlockTypePanicsToParam(t *testing.T) {
	message, accumulateErrs := accumulateFrames(t, []string{
		`{"type":"message_start","message":{"role":"assistant"}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"some_future_block","payload":"hi"}}`,
	})

	for i, err := range accumulateErrs {
		if err != nil {
			t.Fatalf("frame %d reported %v — this defect's whole point is that Accumulate stays silent", i, err)
		}
	}
	if len(message.Content) != 1 {
		t.Fatalf("accumulated %d blocks, want the one the start frame appended", len(message.Content))
	}
	if got := string(message.Content[0].Type); got != "some_future_block" {
		t.Fatalf("block type = %q, want the unknown type verbatim", got)
	}

	defer func() {
		if recover() == nil {
			t.Error("ToParam did not panic on an unknown block type — the SDK changed, and the guard in agent_run.go should be re-read against it")
		}
	}()
	_ = message.ToParam()
}

// TestAnUnknownContentBlockEndsTheTurnNotTheProcess is the curative test. Before
// the guard, this stream took the whole process down at agent.go's
// resp.ToParam(): the only recover() in agent, engine, server or session is on
// the bus path, so every other entry point exited.
func TestAnUnknownContentBlockEndsTheTurnNotTheProcess(t *testing.T) {
	result, err, messages, seenByUser := runWithFailingStream(t, []string{
		`{"type":"message_start","message":{"role":"assistant"}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Here is the answer. "}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"content_block_start","index":1,"content_block":{"type":"some_future_block","payload":"hi"}}`,
	}, nil)

	if err == nil {
		t.Fatal("the turn reported success while carrying a block that cannot be sent back")
	}
	if !errors.Is(err, agent.ErrUnconvertibleContentBlock) {
		t.Fatalf("err = %v, want it to wrap ErrUnconvertibleContentBlock so recordModelHealth can tell it apart", err)
	}
	if !strings.Contains(err.Error(), "some_future_block") {
		t.Errorf("err = %q, want it to name the block type — that name is the whole diagnosis", err)
	}
	if !strings.Contains(err.Error(), "block 1") {
		t.Errorf("err = %q, want it to name which block, since a response can carry several", err)
	}

	const want = "Here is the answer. "
	if seenByUser != want {
		t.Fatalf("the delta hook saw %q, want %q — the test's premise is wrong", seenByUser, want)
	}
	if result.Text != want {
		t.Errorf("result.Text = %q, want the text the user already read (%q)", result.Text, want)
	}
	if !result.Incomplete {
		t.Error("result.Incomplete must be set: the answer stopped short of what the model sent")
	}

	for _, message := range messages {
		for _, block := range message.Content {
			if block.OfText == nil && block.OfToolUse == nil && block.OfThinking == nil {
				t.Fatalf("an unconvertible block reached the conversation: %+v", block)
			}
		}
	}
}

// TestAContentBlockThatIsNotAnObjectEndsTheTurnToo covers the second route to
// the same nil interface: a content_block that is not an object leaves Type
// empty, and Accumulate reports nothing about that either. Two routes, one
// guard — which is why the guard asks AsAny rather than matching type names.
func TestAContentBlockThatIsNotAnObjectEndsTheTurnToo(t *testing.T) {
	_, err, _, _ := runWithFailingStream(t, []string{
		`{"type":"message_start","message":{"role":"assistant"}}`,
		`{"type":"content_block_start","index":0,"content_block":"not-an-object"}`,
	}, nil)

	if !errors.Is(err, agent.ErrUnconvertibleContentBlock) {
		t.Fatalf("err = %v, want ErrUnconvertibleContentBlock", err)
	}
	if !strings.Contains(err.Error(), `type ""`) {
		t.Errorf("err = %q, want it to show the empty type rather than imply a named one", err)
	}
}

// TestAnOrdinaryResponseIsNotRefused is the known-negative control, and it has
// to be reached rather than merely written: a guard that rejected every response
// would turn all three tests above green and every session red.
func TestAnOrdinaryResponseIsNotRefused(t *testing.T) {
	result, err, messages, _ := runWithFailingStream(t, []string{
		`{"type":"message_start","message":{"role":"assistant"}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"All done."}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3}}`,
	}, nil)

	if err != nil {
		t.Fatalf("an ordinary text response was refused: %v", err)
	}
	if result.Text != "All done." {
		t.Errorf("result.Text = %q, want the text that arrived", result.Text)
	}
	if result.Incomplete {
		t.Error("result.Incomplete must not be set on a response that finished")
	}
	if len(messages) != 2 {
		t.Fatalf("conversation has %d messages, want the user message plus the answer", len(messages))
	}
	if got := messages[1].Content[0].OfText; got == nil || got.Text != "All done." {
		t.Errorf("appended assistant block = %+v, want the text unchanged", got)
	}
}
