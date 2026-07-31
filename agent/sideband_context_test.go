package agent

import (
	"context"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// Completing the last task in the plan fires the project's build command, so
// the done sideband is the one callback that starts a subprocess. It used to be
// called with no context at all, which left the turn's interrupt with nothing
// to reach: the ten-minute `go build ./...` ran to the end whatever the user
// pressed.
func TestCompleteTasksReceivesTheTurnsContext(t *testing.T) {
	type contextKey string
	const marker contextKey = "turn"

	turnContext := context.WithValue(context.Background(), marker, "this turn")

	var received context.Context
	agentUnderTest := &Agent{
		sidebandCallbacks: &SidebandCallbacks{
			CompleteTasks: func(ctx context.Context, _ []int) error {
				received = ctx
				return nil
			},
		},
	}

	runBlocksWithContext(t, turnContext, agentUnderTest,
		toolUseBlock("t1", "noop", `{"done":[0]}`))

	if received == nil {
		t.Fatal("CompleteTasks was never called")
	}
	if received.Value(marker) != "this turn" {
		t.Errorf("CompleteTasks got a context that is not the turn's: %v", received)
	}
}

// The turn's context is only worth threading if cancelling it is visible at the
// callback, so pin that rather than the identity alone.
func TestCancellingTheTurnIsVisibleToCompleteTasks(t *testing.T) {
	turnContext, cancel := context.WithCancel(context.Background())
	cancel()

	var cancelledAtCallback bool
	agentUnderTest := &Agent{
		sidebandCallbacks: &SidebandCallbacks{
			CompleteTasks: func(ctx context.Context, _ []int) error {
				cancelledAtCallback = ctx.Err() != nil
				return nil
			},
		},
	}

	runBlocksWithContext(t, turnContext, agentUnderTest,
		toolUseBlock("t1", "noop", `{"done":[0]}`))

	if !cancelledAtCallback {
		t.Error("the turn was cancelled but the sideband callback could not tell")
	}
}

// runBlocksWithContext is runBlocks with the turn's context under the caller's
// control; runBlocks itself always uses context.Background(), which is exactly
// the thing these tests need to vary.
func runBlocksWithContext(t *testing.T, ctx context.Context, a *Agent, blocks ...anthropic.ContentBlockUnion) {
	t.Helper()

	noop := Tool{
		Name: "noop",
		Run:  func(context.Context, string) (string, error) { return "ok", nil },
	}
	a.tools = []Tool{noop}
	toolMap := map[string]Tool{noop.Name: noop}

	resp := &anthropic.Message{Content: blocks}
	if _, _, err := a.executeTools(ctx, resp, &toolInfo{toolMap: toolMap}, &TurnResult{}); err != nil {
		t.Fatalf("executeTools: %v", err)
	}
}
