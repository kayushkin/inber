package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// truncateStr cuts both of the timeline's message previews — the user prompt
// and the assistant response — and that cut did not hold the rune boundary.
// TestReconstructTimelineFromJSONL reaches the line on both paths, but its
// fixtures are "hello world" and "Here's the output!", which never reach the
// budget, so it cannot tell a byte cut from a rune-safe one. Measured
// 2026-09-07 on branch test/the-spawn-tool-preview-is-asked: reverting
// session/timeline_utils.go:13 to the pre-9ce1666 byte cut reddened nothing in
// the whole repository.
//
// Both callers pass their budget in, so the fixtures are driven through
// ReconstructTimelineFromJSONL and the 120 stays in timeline_jsonl.go. A test
// that restates the budget goes quiet the day the budget moves.
//
// The assertion reads the event struct, not a re-marshalled timeline: passing a
// partial rune back through encoding/json rewrites it as U+FFFD, and an
// assertion downstream of that encoder can never fail.
func TestTheTimelineMessagePreviewsAreCutOnARuneBoundary(t *testing.T) {
	// One ASCII byte in front of a run of two-byte runes, so every rune in the
	// run starts at an odd offset and any even budget necessarily cuts inside
	// one. The straddle guard below fails loudly rather than passing if a
	// changed budget ever moves the cut out of that run.
	long := "x" + strings.Repeat("é", 150)

	req, err := json.Marshal(map[string]any{
		"messages": []map[string]any{{
			"role":    "user",
			"content": []map[string]any{{"type": "text", "text": long}},
		}},
	})
	if err != nil {
		t.Fatalf("build request fixture: %v", err)
	}

	logFile := filepath.Join(t.TempDir(), "session.jsonl")
	f, err := os.Create(logFile)
	if err != nil {
		t.Fatalf("create log: %v", err)
	}
	enc := json.NewEncoder(f)
	for _, e := range []Entry{
		{Timestamp: time.Date(2026, 9, 7, 6, 0, 0, 0, time.UTC), Turn: 1, Role: "request", Request: req},
		{Timestamp: time.Date(2026, 9, 7, 6, 0, 1, 0, time.UTC), Turn: 1, Role: "assistant", Content: long},
	} {
		if err := enc.Encode(e); err != nil {
			t.Fatalf("write log: %v", err)
		}
	}
	f.Close()

	events, _, err := ReconstructTimelineFromJSONL(logFile, nil)
	if err != nil {
		t.Fatalf("ReconstructTimelineFromJSONL: %v", err)
	}

	previews := map[string]string{}
	for _, ev := range events {
		switch ev.Type {
		case "prompt":
			previews["the user prompt"] = ev.UserMessage
		case "response":
			previews["the assistant response"] = ev.ResponseText
		}
	}
	if len(previews) != 2 {
		t.Fatalf("the timeline has %d of the 2 events that carry a cut preview, so this "+
			"case does not reach both call sites.\ngot %d events", len(previews), len(events))
	}

	for what, preview := range previews {
		cut := strings.TrimSuffix(preview, "...")
		if cut == preview {
			t.Errorf("%s preview of a %d-byte message was not cut at all, so this case "+
				"cannot see the cut.\ngot %q", what, len(long), preview)
			continue
		}
		if !straddledCut(cut) {
			t.Errorf("%s: the cut landed in the fixture's ASCII prefix, so a byte cut and "+
				"a rune-safe cut would agree and this case could not tell them apart. The "+
				"fixture needs rebuilding against the current budget.\ngot %q", what, preview)
			continue
		}
		if !utf8.ValidString(preview) {
			t.Errorf("%s preview in the reconstructed timeline is not valid UTF-8: the cut "+
				"ended inside a rune.\ngot %q", what, preview)
		}
	}
}
