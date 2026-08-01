package session

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	logstackmodels "github.com/kayushkin/logstack/models"
)

// An assistant turn has to be filed as outbound, because that is the only type
// logstack's usage readers select.
//
// This is the whole defect in one assertion. inber built a complete entry,
// posted it, and got a 201 back, so from its own side forwarding looked
// healthy — but the entry was typed TypeMessage, Usage and MaxUsage both query
// outbound and nothing else, and inber's usage came back as count 0 for three
// months.
func TestAssistantTurnIsTypedOutbound(t *testing.T) {
	entry := Entry{
		Timestamp:    time.Now(),
		Turn:         3,
		Role:         "assistant",
		Content:      "the reply",
		Model:        "claude-sonnet-4-5",
		InputTokens:  120,
		OutputTokens: 40,
	}

	got := entry.toLogstackEntry("claxon", "session-1").Type
	if got != logstackmodels.TypeOutbound {
		t.Fatalf("assistant entry typed %q, want %q — no usage reader queries anything else", got, logstackmodels.TypeOutbound)
	}
}

// Every role maps to the type that describes it, and the ones that carry no
// usage stay out of the outbound bucket.
//
// The system case is the one worth stating out loud: a session's closing entry
// is a system entry carrying the session's *cumulative* totals, so typing it
// outbound would add a whole session's usage on top of the per-turn assistant
// entries that already reported it.
func TestRolesMapToTheirLogstackType(t *testing.T) {
	for role, want := range map[string]string{
		"user":        logstackmodels.TypeInbound,
		"assistant":   logstackmodels.TypeOutbound,
		"tool_call":   logstackmodels.TypeToolCall,
		"tool_result": logstackmodels.TypeToolResult,
		"system":      logstackmodels.TypeLifecycle,
		"thinking":    logstackmodels.TypeMessage,
		"request":     logstackmodels.TypeMessage,
		"compaction":  logstackmodels.TypeMessage,
		"summarize":   logstackmodels.TypeMessage,
		"stash":       logstackmodels.TypeMessage,
		"prune":       logstackmodels.TypeMessage,
	} {
		if got := entryType(role); got != want {
			t.Errorf("role %q typed %q, want %q", role, got, want)
		}
		if role != "assistant" && entryType(role) == logstackmodels.TypeOutbound {
			t.Errorf("role %q is counted as usage and should not be", role)
		}
	}
}

// The four counts reach logstack in the Stats block, which is the only place it
// reads them from.
//
// TokensIn/TokensOut/Metadata are all marked deprecated by logstack's own model
// and no usage reader looks at them, so an entry with those filled and Stats
// nil is dropped whole — cache counts, dollars and the API call with it.
func TestStatsCarryTheFourCounts(t *testing.T) {
	entry := Entry{
		Timestamp:    time.Now(),
		Turn:         1,
		Role:         "assistant",
		Content:      "the reply",
		Model:        "claude-sonnet-4-5",
		InputTokens:  120,
		OutputTokens: 40,
		CacheRead:    9000,
		CacheWrite:   500,
		TotalCost:    0.42,
	}

	stats := entry.toLogstackEntry("claxon", "session-1").Stats
	if stats == nil {
		t.Fatal("Stats is nil on a turn that reported usage — logstack computes usage from Stats alone")
	}
	if stats.InputTokens != 120 || stats.OutputTokens != 40 || stats.CacheReadTokens != 9000 {
		t.Errorf("counts = in %d out %d cache_read %d, want 120/40/9000",
			stats.InputTokens, stats.OutputTokens, stats.CacheReadTokens)
	}
	if stats.Model != "claude-sonnet-4-5" {
		t.Errorf("Stats.Model = %q, want the model that ran the turn", stats.Model)
	}
}

// Cache writes go in exactly one field, because logstack reads the two as a sum.
//
// MaxUsage computes cacheWrite as CacheCreationTokens + CacheWriteTokens. The
// names are near-synonyms and filling both is the natural mistake; it doubles
// inber's cache-write figure in every cost report that reads it.
func TestCacheWritesAreReportedOnce(t *testing.T) {
	entry := Entry{
		Timestamp:  time.Now(),
		Role:       "assistant",
		Model:      "claude-sonnet-4-5",
		CacheWrite: 500,
	}

	stats := entry.toLogstackEntry("claxon", "session-1").Stats
	if stats == nil {
		t.Fatal("Stats is nil on a turn that wrote cache")
	}
	if stats.CacheCreationTokens+stats.CacheWriteTokens != 500 {
		t.Fatalf("cache writes sum to %d, want 500 — logstack adds CacheCreationTokens and CacheWriteTokens together",
			stats.CacheCreationTokens+stats.CacheWriteTokens)
	}
}

// An entry that reports no usage carries no Stats block.
//
// A zeroed Stats on every user message, tool call and lifecycle line would put
// them all in logstack's counted path with zero tokens, inflating the API-call
// count that MaxUsage derives from the number of entries it reads.
func TestEntriesWithoutUsageCarryNoStats(t *testing.T) {
	for _, role := range []string{"user", "tool_call", "tool_result", "system", "request"} {
		entry := Entry{Timestamp: time.Now(), Role: role, Content: "x"}
		if stats := entry.toLogstackEntry("claxon", "session-1").Stats; stats != nil {
			t.Errorf("role %q carries a Stats block with no usage in it: %+v", role, stats)
		}
	}
}

// Cost stays out of Stats, because the number inber has is the wrong one.
//
// Entry.TotalCost is the session's running total, re-reported on every turn. In
// TurnStats.Cost — a per-turn field — a consumer summing it over a session
// counts that session's cost once per turn. It stays in Metadata, where it is
// labelled cumulative, and MaxUsage prices the tokens itself.
func TestCumulativeCostIsNotReportedAsTurnCost(t *testing.T) {
	entry := Entry{
		Timestamp:    time.Now(),
		Role:         "assistant",
		Model:        "claude-sonnet-4-5",
		InputTokens:  120,
		OutputTokens: 40,
		TotalCost:    12.34,
	}

	logEntry := entry.toLogstackEntry("claxon", "session-1")
	if logEntry.Stats.Cost != 0 {
		t.Errorf("Stats.Cost = %v, want 0 — Entry.TotalCost is cumulative, not this turn's", logEntry.Stats.Cost)
	}
	if logEntry.Metadata["cost_usd"] != 12.34 {
		t.Errorf("Metadata[cost_usd] = %v, want the cumulative figure to stay where it is labelled", logEntry.Metadata["cost_usd"])
	}
}

// End to end through the real adapter: what a session actually puts on the wire.
//
// The unit tests above all call the mapper directly. This one drives
// LogAssistant on a real Session with LOGSTACK_URL set, and reads the HTTP body
// logstack would have received — so it also covers the doorway, where the cache
// counts were being dropped before.
func TestSessionPostsCountableAssistantTurn(t *testing.T) {
	posted := make(chan logstackmodels.LogEntry, 8)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			return
		}
		var entry logstackmodels.LogEntry
		if err := json.Unmarshal(body, &entry); err != nil {
			t.Errorf("unmarshal entry: %v", err)
			return
		}
		posted <- entry
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	t.Setenv("LOGSTACK_URL", server.URL)

	sess, err := New(t.TempDir(), "claude-sonnet-4-5", "claxon", "", nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer sess.Close()

	sess.LogAssistant("the reply", TurnTokens{Input: 120, Output: 40, CacheRead: 9000, CacheWrite: 500}, 2)

	deadline := time.After(5 * time.Second)
	for {
		select {
		case entry := <-posted:
			if entry.Type != logstackmodels.TypeOutbound {
				continue // session start and the like
			}
			if entry.Stats == nil {
				t.Fatal("the assistant turn reached logstack with no Stats block")
			}
			if entry.Stats.InputTokens != 120 || entry.Stats.OutputTokens != 40 {
				t.Fatalf("posted counts = in %d out %d, want 120/40", entry.Stats.InputTokens, entry.Stats.OutputTokens)
			}
			if entry.Stats.CacheReadTokens != 9000 {
				t.Fatalf("posted cache_read = %d, want 9000", entry.Stats.CacheReadTokens)
			}
			if entry.Stats.CacheCreationTokens+entry.Stats.CacheWriteTokens != 500 {
				t.Fatalf("posted cache writes sum to %d, want 500",
					entry.Stats.CacheCreationTokens+entry.Stats.CacheWriteTokens)
			}
			if entry.Orchestrator != "inber" {
				t.Fatalf("posted orchestrator = %q, want inber", entry.Orchestrator)
			}
			return
		case <-deadline:
			t.Fatal("no outbound entry reached logstack within 5s")
		}
	}
}
