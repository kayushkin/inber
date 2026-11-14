package agent

import (
	"context"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// The read cache answers a repeat read with "already in context" because it
// assumes a read made this turn is still in the transcript. A prune inside the
// turn can cut that read out (the pruner ages tool results by round trips, not
// by turns), so after an adopted prune the cache must not vouch for anything.

const readCachePrunedPath = "/tmp/read-cache-prune-probe/file.go"

func agentWithOneCachedRead(t *testing.T) *Agent {
	t.Helper()
	a := &Agent{readCache: NewReadCache()}
	a.SetContextWindow(200000)
	a.readCache.RecordFullRead(readCachePrunedPath, 120)
	if _, cached := a.readCache.Check(readCachePrunedPath); !cached {
		t.Fatal("the read was not recorded — this test proves nothing")
	}
	return a
}

func TestAnAdoptedPruneOnBuildForgetsEveryCachedRead(t *testing.T) {
	a := agentWithOneCachedRead(t)
	a.SetBeforeRequest(pruneRewritingContentInPlace(4000))
	messages := conversationOfLongTextMessages(4)

	a.buildRequest(context.Background(), "claude-sonnet-4-5-20250929", &messages, a.prepareTools(), false)

	if stub, cached := a.readCache.Check(readCachePrunedPath); cached {
		t.Fatalf("after a prune that freed tokens the cache still answers %q — "+
			"the model is told it has a file the prune may have cut", stub)
	}
}

// The control: a prune that changed nothing is not adopted, and must not cost
// the turn its dedup.
func TestAPruneThatDidNothingKeepsCachedReads(t *testing.T) {
	a := agentWithOneCachedRead(t)
	a.SetBeforeRequest(func(_ context.Context, messages []anthropic.MessageParam, _ int) ([]anthropic.MessageParam, int) {
		return messages, 0
	})
	messages := conversationOfLongTextMessages(4)

	a.buildRequest(context.Background(), "claude-sonnet-4-5-20250929", &messages, a.prepareTools(), false)

	if _, cached := a.readCache.Check(readCachePrunedPath); !cached {
		t.Fatal("a prune that freed nothing cleared the read cache")
	}
}

func TestAnAdoptedPruneOnContextLengthRetryForgetsEveryCachedRead(t *testing.T) {
	a := agentWithOneCachedRead(t)
	calls := 0
	prune := pruneRewritingContentInPlace(4000)
	a.SetBeforeRequest(func(ctx context.Context, messages []anthropic.MessageParam, window int) ([]anthropic.MessageParam, int) {
		calls++
		if calls == 1 {
			return messages, 0 // the build: nothing to free yet
		}
		return prune(ctx, messages, window)
	})
	messages := conversationOfLongTextMessages(4)
	params := a.buildRequest(context.Background(), "claude-sonnet-4-5-20250929", &messages, a.prepareTools(), false)
	if _, cached := a.readCache.Check(readCachePrunedPath); !cached {
		t.Fatal("the build's no-op prune already cleared the cache — the retry path is untested")
	}

	a.provider = contextLengthErrorProvider{}
	if _, err := a.executeAPICall(context.Background(), params, &messages); err == nil {
		t.Fatal("executeAPICall returned no error — the retry path was never reached")
	}
	if calls != 2 {
		t.Fatalf("BeforeRequest called %d times, want 2 (the build and the retry)", calls)
	}

	if stub, cached := a.readCache.Check(readCachePrunedPath); cached {
		t.Fatalf("after the retry's prune the cache still answers %q", stub)
	}
}
