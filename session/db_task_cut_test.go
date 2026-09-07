package session

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// SetTask cuts the first user message to 200 bytes and writes it into the
// sessions table, and that cut did not hold the rune boundary. Nothing in the
// repository asked: measured 2026-09-07 on branch
// test/the-spawn-tool-preview-is-asked, reverting session/db_sessions.go:32 to
// the pre-9ce1666 byte cut reddened nothing anywhere.
//
// This is the one cut in the residue that PERSISTS. A partial rune in a log
// line is gone with the line; a partial rune in `sessions.task` is read back by
// every session listing from then on, and re-cutting it later cannot repair it
// because the dropped bytes are not in the row.
//
// The assertion reads the value back out of SQLite rather than trusting
// SetTask's return, because the claim is about what a reader of the table sees.
// SQLite does not validate UTF-8 in a TEXT column and the driver hands the
// bytes back unchanged, so the invalid sequence survives the round trip — which
// is exactly why it has to be tested here and not at the helper.
func TestTheStoredSessionTaskIsCutOnARuneBoundary(t *testing.T) {
	d := openTestDB(t)

	if err := d.InsertSession(&SessionRow{
		ID:        "s1",
		Agent:     "test",
		Model:     "claude-sonnet-4-20250514",
		StartedAt: time.Now(),
		LogFile:   "session.jsonl",
	}); err != nil {
		t.Fatalf("InsertSession: %v", err)
	}

	// One ASCII byte in front of a run of two-byte runes, so every rune in the
	// run starts at an odd offset and any even budget necessarily cuts inside
	// one. The straddle guard below fails loudly rather than passing if a
	// changed budget ever moves the cut out of that run.
	long := "x" + strings.Repeat("é", 200)
	if err := d.SetTask("s1", long); err != nil {
		t.Fatalf("SetTask: %v", err)
	}

	var stored string
	if err := d.db.QueryRow(`SELECT task FROM sessions WHERE id = ?`, "s1").Scan(&stored); err != nil {
		t.Fatalf("read task back: %v", err)
	}

	cut := strings.TrimSuffix(stored, "…")
	if cut == stored {
		t.Fatalf("a %d-byte task was stored without being cut at all, so this case "+
			"cannot see the cut.\ngot %q", len(long), stored)
	}
	if !straddledCut(cut) {
		t.Fatalf("the cut landed in the fixture's ASCII prefix, so a byte cut and a "+
			"rune-safe cut would agree and this case could not tell them apart. The "+
			"fixture needs rebuilding against the current budget.\ngot %q", stored)
	}
	if !utf8.ValidString(stored) {
		t.Errorf("the task persisted in the sessions table is not valid UTF-8: the "+
			"cut ended inside a rune, and every later reader of the row sees it.\ngot %q", stored)
	}
}
