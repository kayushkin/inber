package session

import (
	"fmt"
	"runtime"
	"strings"
	"testing"
)

// TestSessionDoesNotRetainTruncatedToolOutput pins the whole point of
// truncating a tool result: the bytes that were cut are gone.
//
// They were not. LogToolResult used to copy the untruncated output into a
// map[string]string on the Session, so a session's memory grew with the total
// size of everything it had ever truncated — and the only way back out was an
// exported getter with no callers anywhere in the tree, tests included. The
// truncation still shrank the model's context; it just stopped shrinking the
// process.
//
// The assertion is a heap measurement rather than a look at the struct, because
// a look at the struct is exactly what a future reader would satisfy by giving
// the map a nicer name. What must stay true is the number: log 64 MiB of
// truncated output and the session must not be 64 MiB bigger afterwards.
func TestSessionDoesNotRetainTruncatedToolOutput(t *testing.T) {
	const (
		toolResults      = 64
		outputSize       = 1 << 20 // 1 MiB each, 64 MiB total
		retentionAllowed = 16 << 20
	)

	session, err := New(t.TempDir(), "test-model", "test-agent", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	session.SetTruncateConfig(TruncateConfig{
		Threshold:  100,
		HeadTokens: 20,
		TailTokens: 10,
		Strategy:   StrategyHeadTail,
	})

	// Prove the outputs really are being truncated before measuring what is left
	// of them. A session that quietly stopped truncating would keep no side copy
	// either, and would pass a retention check while failing at the thing the
	// check exists to protect.
	if displayed := session.TruncateToolResult("shell", largeToolOutput(0, outputSize), false); displayed == "" {
		t.Fatalf("test setup: an output of %d bytes was not truncated", outputSize)
	}

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)

	for i := 0; i < toolResults; i++ {
		session.LogToolResult(fmt.Sprintf("tool-%d", i), "shell", largeToolOutput(i, outputSize), false)
	}

	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	retained := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	if retained > retentionAllowed {
		t.Errorf("session retained %d bytes after logging %d truncated tool results of %d bytes each; "+
			"at most %d is expected, and %d is what holding every full output would cost. "+
			"Truncated output must be discarded, not moved to a side map",
			retained, toolResults, outputSize, int64(retentionAllowed), int64(toolResults)*outputSize)
	}
}

// largeToolOutput builds a distinct multi-line output of at least size bytes.
// Distinct per call so nothing in the runtime can share one backing array
// between iterations and make retention look like release.
func largeToolOutput(seq, size int) string {
	line := fmt.Sprintf("output %d: cannot use *ResponseX as type interface{MethodX()}\n", seq)
	return strings.Repeat(line, size/len(line)+1)
}
