package session

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

// logstackFixture stands in for a real logstack, and it is pinned to logstack's
// own protocol rather than to whatever inber happens to send.
//
// The method and the path are checked and anything else is refused, because a
// fixture that answers every request identically cannot notice a wrong URL —
// it would report a client posting to the wrong route as a healthy forward.
// The route, the 201 and the response body are logstack's
// `Handler.SetupRoutes`/`IngestLog`, probed against a real logstack binary on
// 2026-08-29, not read off inber's own client.
func logstackFixture(t *testing.T, posted chan<- logstackmodels.LogEntry) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.RequestURI != "/api/v1/logs" {
			t.Errorf("request went to %s %s, want POST /api/v1/logs — logstack has no other ingest route", r.Method, r.RequestURI)
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
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
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"id":      entry.ID,
			"status":  "created",
			"message": "Log entry created successfully",
		})
	}))
}

// collectSessionEntries drives a real Session against the fixture and returns
// every entry that reached the wire, including the closing one.
func collectSessionEntries(t *testing.T, drive func(*Session)) []logstackmodels.LogEntry {
	t.Helper()
	posted := make(chan logstackmodels.LogEntry, 32)
	server := logstackFixture(t, posted)
	defer server.Close()

	t.Setenv("LOGSTACK_URL", server.URL)

	sess, err := New(t.TempDir(), "claude-sonnet-4-5", "claxon", "", nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	drive(sess)

	// Log is fire-and-forget on its own goroutine, so the close entry can race
	// the turn entries onto the wire. Drain until the closing entry arrives.
	sess.Close()

	// Two waits, and they are different questions. The closing entry says the
	// session is finished; the idle window says the turn entries that raced it
	// have landed. Log is fire-and-forget on its own goroutine, so the closing
	// entry regularly overtakes the turns it summarises — draining only up to it
	// undercounts, which in this test would read as the defect being absent.
	var entries []logstackmodels.LogEntry
	deadline := time.After(10 * time.Second)
	closed := false
	for {
		select {
		case entry := <-posted:
			entries = append(entries, entry)
			if content, ok := entry.Content.(string); ok && strings.HasPrefix(content, "session complete") {
				closed = true
			}
		case <-time.After(500 * time.Millisecond):
			if closed {
				return entries
			}
		case <-deadline:
			t.Fatalf("the session's closing entry never reached logstack; got %d entries", len(entries))
		}
	}
}

// End to end through the real adapter: what a session actually puts on the wire.
//
// The unit tests above all call the mapper directly. This one drives
// LogAssistant on a real Session with LOGSTACK_URL set, and reads the HTTP body
// logstack would have received — so it also covers the doorway, where the cache
// counts were being dropped before.
func TestSessionPostsCountableAssistantTurn(t *testing.T) {
	entries := collectSessionEntries(t, func(sess *Session) {
		sess.LogAssistant("the reply", TurnTokens{Input: 120, Output: 40, CacheRead: 9000, CacheWrite: 500}, 2)
	})

	for _, entry := range entries {
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
	}
	t.Fatal("no outbound entry reached logstack")
}

// A session reports each of its tokens to logstack exactly once.
//
// The closing entry is a system entry carrying the session's *cumulative*
// totals, and every one of those tokens has already been reported by the
// per-turn assistant entries. Typing it lifecycle keeps it out of Usage and
// MaxUsage, which both select outbound — but logstack's third reader,
// FileStore.Stats behind GET /api/v1/stats, sums TokensIn and TokensOut over
// every entry with no type filter at all. Measured against a real logstack on
// 2026-08-29: three turns of 100 in / 10 out were reported by /api/v1/stats as
// 600 in / 60 out, exactly double, while /api/v1/usage read 300/30 correctly.
//
// This is the rule the mapper already applies to Cost — a cumulative figure in
// a per-turn field is counted once per turn by anything that sums it — carried
// to the three counts it was never applied to.
func TestASessionReportsItsTokensToLogstackExactlyOnce(t *testing.T) {
	const turns = 3
	entries := collectSessionEntries(t, func(sess *Session) {
		for turn := 1; turn <= turns; turn++ {
			sess.LogAssistant("reply", TurnTokens{Input: 100, Output: 10}, turn)
		}
	})

	var summedIn, summedOut int
	for _, entry := range entries {
		summedIn += entry.TokensIn
		summedOut += entry.TokensOut
		if entry.Stats != nil {
			if entry.Type != logstackmodels.TypeOutbound {
				t.Errorf("a %s entry carries a Stats block (%+v) — only a turn logstack counts may carry one",
					entry.Type, entry.Stats)
			}
		}
	}

	if summedIn != turns*100 || summedOut != turns*10 {
		t.Errorf("logstack's untyped readers see in %d out %d, want %d/%d — the session's tokens are reported more than once",
			summedIn, summedOut, turns*100, turns*10)
	}
}

// A trailing slash on LOGSTACK_URL does not silently unplug the session.
//
// logstack's client appends "/api/v1/logs" to the base URL it is handed, and
// gin does not route "//api/v1/logs" — measured against a real logstack on
// 2026-08-29, 404 with the slash and 201 without. Log discards its error into a
// log line, so the only symptom of a mis-set variable was the whole session
// being absent from logstack. Nothing could see this while the fixture answered
// 201 to every path it was given.
func TestATrailingSlashOnTheLogstackURLStillReachesTheIngestRoute(t *testing.T) {
	posted := make(chan logstackmodels.LogEntry, 8)
	server := logstackFixture(t, posted)
	defer server.Close()

	adapter := NewLogstackAdapter(server.URL+"/", "claxon", "session-1")
	if adapter == nil {
		t.Fatal("NewLogstackAdapter returned nil for a URL that differs only by a trailing slash")
	}
	adapter.Log(Entry{Timestamp: time.Now(), Role: "assistant", Content: "the reply",
		Model: "claude-sonnet-4-5", InputTokens: 120, OutputTokens: 40})

	select {
	case entry := <-posted:
		if entry.Type != logstackmodels.TypeOutbound {
			t.Fatalf("posted type = %q, want outbound", entry.Type)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the entry never reached logstack's ingest route")
	}
}

// A URL that is nothing but slashes is no URL, and the adapter stays off.
//
// The cry-wolf half of the trim above: TrimRight must not turn "/" into a live
// adapter pointed at the empty string, which would post to a relative URL and
// fail on every entry.
func TestALogstackURLOfOnlySlashesLeavesTheAdapterOff(t *testing.T) {
	if adapter := NewLogstackAdapter("///", "claxon", "session-1"); adapter != nil {
		t.Fatalf("adapter built from a slash-only URL: %+v", adapter)
	}
}
