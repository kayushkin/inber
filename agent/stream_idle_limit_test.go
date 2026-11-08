package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
)

// stallingStream replays its events, each after a pause, and then goes silent:
// Next blocks until the context the stream was opened under is done, which is
// what a provider holding an open socket and sending nothing looks like. Like
// the SDK's stream, it reports the context's error once that happens.
type stallingStream struct {
	ctx    context.Context
	events []anthropic.MessageStreamEventUnion
	pause  time.Duration
	index  int
	err    error
}

func (s *stallingStream) Next() bool {
	if s.index < len(s.events) {
		select {
		case <-time.After(s.pause):
			s.index++
			return true
		case <-s.ctx.Done():
			s.err = s.ctx.Err()
			return false
		}
	}
	<-s.ctx.Done()
	s.err = s.ctx.Err()
	return false
}

func (s *stallingStream) Current() anthropic.MessageStreamEventUnion { return s.events[s.index-1] }

func (s *stallingStream) Err() error { return s.err }

type stallingStreamProvider struct {
	rawEvents []string
	pause     time.Duration
}

func (p *stallingStreamProvider) Complete(ctx context.Context, params *anthropic.MessageNewParams) (*anthropic.Message, error) {
	return nil, errors.New("not used")
}

func (p *stallingStreamProvider) CompleteStreaming(ctx context.Context, params *anthropic.MessageNewParams) (agent.StreamingResponse, error) {
	events := make([]anthropic.MessageStreamEventUnion, 0, len(p.rawEvents))
	for _, raw := range p.rawEvents {
		var event anthropic.MessageStreamEventUnion
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return &stallingStream{ctx: ctx, events: events, pause: p.pause}, nil
}

func runAgainstStallingStream(t *testing.T, provider *stallingStreamProvider, idleLimit time.Duration, turnContext context.Context) (*agent.TurnResult, error, string, time.Duration) {
	t.Helper()
	a := agent.New(provider, "system")
	a.SetStreamIdleLimit(idleLimit)
	var seenByUser strings.Builder
	a.SetHooks(&agent.Hooks{OnTextDelta: func(text string) { seenByUser.WriteString(text) }})
	messages := []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("explain"))}

	type outcome struct {
		result *agent.TurnResult
		err    error
	}
	done := make(chan outcome, 1)
	start := time.Now()
	go func() {
		result, err := a.Run(turnContext, "claude-sonnet-4-5-20250929", &messages)
		done <- outcome{result, err}
	}()
	select {
	case got := <-done:
		return got.result, got.err, seenByUser.String(), time.Since(start)
	case <-time.After(10 * time.Second):
		t.Fatal("the turn is still running 10s after the stream went silent — nothing bounds the silence")
		return nil, nil, "", 0
	}
}

var textThenSilence = []string{
	`{"type":"message_start","message":{"role":"assistant"}}`,
	`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
	`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Half an answer"}}`,
}

// TestASilentStreamEndsTheTurnWithErrStreamIdle is card 0769c4db: a provider
// that opens a stream and goes quiet used to hold the turn forever. Now the
// call fails with ErrStreamIdle, and the text the user already saw is kept
// through the same partial path a dropped connection takes.
func TestASilentStreamEndsTheTurnWithErrStreamIdle(t *testing.T) {
	provider := &stallingStreamProvider{rawEvents: textThenSilence, pause: time.Millisecond}
	result, err, seenByUser, _ := runAgainstStallingStream(t, provider, 100*time.Millisecond, context.Background())

	if !errors.Is(err, agent.ErrStreamIdle) {
		t.Fatalf("err = %v, want ErrStreamIdle", err)
	}
	if errors.Is(err, context.Canceled) {
		t.Errorf("err = %v wraps context.Canceled, which inber reads as the user pressing stop", err)
	}
	if seenByUser != "Half an answer" || result.Text != "Half an answer" {
		t.Errorf("user saw %q, result.Text = %q; the partial text must be kept", seenByUser, result.Text)
	}
	if !result.Incomplete {
		t.Error("result.Incomplete must be set for a stream cut off by silence")
	}
}

// TestEveryEventPushesTheIdleLimitBack: a stream whose total length is far past
// the limit, but whose events never stop for longer than it, is alive and must
// not be cut off. Without the reset the watchdog would fire at 150ms.
func TestEveryEventPushesTheIdleLimitBack(t *testing.T) {
	events := []string{`{"type":"message_start","message":{"role":"assistant"}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`}
	for i := 0; i < 8; i++ {
		events = append(events, `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"x"}}`)
	}
	provider := &stallingStreamProvider{rawEvents: events, pause: 50 * time.Millisecond}
	result, err, _, elapsed := runAgainstStallingStream(t, provider, 150*time.Millisecond, context.Background())

	if !errors.Is(err, agent.ErrStreamIdle) {
		t.Fatalf("err = %v, want ErrStreamIdle once the events stop", err)
	}
	if result.Text != "xxxxxxxx" {
		t.Errorf("result.Text = %q, want all eight deltas: the watchdog fired while events were still arriving", result.Text)
	}
	if elapsed < 500*time.Millisecond {
		t.Errorf("turn ended after %s, before the ten events (500ms) had arrived", elapsed)
	}
}

// TestStopDuringASilentStreamIsStillAStop: when the user cancels the turn, the
// error must stay a cancellation and must not be reported as the provider's
// silence.
func TestStopDuringASilentStreamIsStillAStop(t *testing.T) {
	turnContext, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	provider := &stallingStreamProvider{rawEvents: textThenSilence, pause: time.Millisecond}
	_, err, _, _ := runAgainstStallingStream(t, provider, time.Hour, turnContext)

	if errors.Is(err, agent.ErrStreamIdle) {
		t.Fatalf("err = %v: a user stop was reported as a silent stream", err)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
