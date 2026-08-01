package server

import (
	"strings"
	"testing"
)

// The sub-agent completion message is read by a model, not by a person, and the
// model reading it is the one deciding whether to spawn again. It reported
// `%d in`, taken from TokenUsage.Input.
//
// Those four counts are disjoint: Input is the part of the prompt that was
// neither read from the cache nor written to it. inber caches deliberately, so
// on the server's own store Input is about 4% of what a child actually sent —
// the three worst live sub-sessions each pushed over a million prompt tokens
// and announced themselves as ~40k. The cost on the same line has priced all
// four since commit 09848b9, so the parent was reading a dollar figure it had
// no way to explain from the tokens beside it.

// TestSpawnTokenLineRecordsCacheTraffic is the defect this file was written
// for, asserted as a difference: a child that moved a million tokens through
// the cache must not report the same line as one that moved none.
func TestSpawnTokenLineRecordsCacheTraffic(t *testing.T) {
	fresh := describeSpawnTokens(TokenUsage{Input: 1_000, Output: 500, Cost: 0.01})
	cached := describeSpawnTokens(TokenUsage{
		Input: 1_000, Output: 500, CacheRead: 900_000, CacheWrite: 100_000, Cost: 0.42,
	})

	if fresh == cached {
		t.Fatalf("a child that read 900k tokens from the cache and wrote 100k into it "+
			"reported %q, the same line as a child with no cache traffic at all", cached)
	}
}

// TestSpawnTokenLineNamesTheWholePrompt pins what the line has to say, not
// merely that it changed. A total is what a parent needs to compare one child
// against another; the breakdown is what lets it be checked against the
// provider's own four numbers.
func TestSpawnTokenLineNamesTheWholePrompt(t *testing.T) {
	line := describeSpawnTokens(TokenUsage{
		Input: 1_000, Output: 500, CacheRead: 900_000, CacheWrite: 100_000, Cost: 0.42,
	})

	for _, part := range []string{
		"prompt=1001000", "fresh=1000", "cache_read=900000", "cache_write=100000", "out=500",
	} {
		if !strings.Contains(line, part) {
			t.Errorf("token line %q is missing %q", line, part)
		}
	}
}

// TestSpawnTokenLineTotalCoversEveryCount is the guard against the total being
// the fresh count under a wider name. Each of the three prompt buckets must move
// it on its own, or a bucket has been left out of the sum.
func TestSpawnTokenLineTotalCoversEveryCount(t *testing.T) {
	base := TokenUsage{Input: 1_000, Output: 500}
	baseline := describeSpawnTokens(base)

	for name, usage := range map[string]TokenUsage{
		"fresh input": {Input: base.Input + 7_000, Output: base.Output},
		"cache read":  {Input: base.Input, Output: base.Output, CacheRead: 7_000},
		"cache write": {Input: base.Input, Output: base.Output, CacheWrite: 7_000},
	} {
		widened := describeSpawnTokens(usage)
		if !strings.Contains(widened, "prompt=8000") {
			t.Errorf("adding 7000 tokens of %s gave %q, want a prompt total of 8000 "+
				"(baseline was %q) — that bucket is not in the sum", name, widened, baseline)
		}
	}
}
