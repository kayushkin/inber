package session

import (
	"os"
	"testing"
	"time"
)

// openTestDB opens a sessions DB in a temp repo root.
func openTestDB(t *testing.T) *SQLiteStore {
	t.Helper()
	d, err := OpenDB(t.TempDir())
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

// TestOpenDBAppliesPragmas pins that WAL and busy_timeout really are on. They
// arrive by different routes — busy_timeout as a DSN pragma, WAL as a statement
// sqlitewal.SwitchToWAL runs once — and each route fails quietly in its own way.
// modernc.org/sqlite silently ignores DSN keys it does not recognise, so a
// mattn-style ?_journal=wal&_timeout=5000 opens cleanly and applies neither —
// which is how this database ran in journal_mode=delete with no busy timeout.
// A lost WAL conversion is quieter still: it can report the mode the file stayed
// on rather than an error. Asserting on the pragmas (not on the DSN string, and
// not on the absence of an error) is what makes that unfakeable.
func TestOpenDBAppliesPragmas(t *testing.T) {
	d := openTestDB(t)

	var journalMode string
	if err := d.db.QueryRow(`PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("journal_mode = %q, want %q (the DSN pragma was not applied)", journalMode, "wal")
	}

	var busyTimeout int
	if err := d.db.QueryRow(`PRAGMA busy_timeout`).Scan(&busyTimeout); err != nil {
		t.Fatalf("PRAGMA busy_timeout: %v", err)
	}
	if busyTimeout != 5000 {
		t.Errorf("busy_timeout = %d, want 5000 (the DSN pragma was not applied)", busyTimeout)
	}
}

// TestListActiveStatusReportsRealTimestamps is the regression test for sessions
// reporting a start time of epoch 0 (1970).
//
// The old query computed the epoch in SQL with strftime('%s', s.started_at). The
// driver writes a bound time.Time as Go's time.Time.String(), which SQLite's date
// functions cannot parse, so strftime returned NULL, and a COALESCE(..., 0) turned
// that NULL into 0 — every running agent's start time read back as 1970 while the
// query still succeeded.
func TestListActiveStatusReportsRealTimestamps(t *testing.T) {
	d := openTestDB(t)

	started := time.Now().Add(-2 * time.Hour)
	lastTurn := time.Now().Add(-5 * time.Minute)

	// PID must be alive or ListActiveStatus reaps the session as stale.
	if err := d.InsertSession(&SessionRow{
		ID: "s1", Agent: "claxon", Model: "sonnet", Command: "run",
		PID: os.Getpid(), StartedAt: started,
	}); err != nil {
		t.Fatalf("InsertSession: %v", err)
	}
	for i, ts := range []time.Time{started.Add(time.Minute), lastTurn} {
		if err := d.InsertTurn(&TurnRow{SessionID: "s1", Turn: i + 1, StartedAt: ts}); err != nil {
			t.Fatalf("InsertTurn: %v", err)
		}
	}

	active, err := d.ListActiveStatus()
	if err != nil {
		t.Fatalf("ListActiveStatus: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("got %d active sessions, want 1", len(active))
	}
	a := active[0]

	if a.Turns != 2 {
		t.Errorf("Turns = %d, want 2", a.Turns)
	}
	// Second granularity: the column round-trips sub-second precision, but the
	// point of the assertion is the date, not the nanoseconds.
	if d := a.StartedAt.Sub(started); d > time.Second || d < -time.Second {
		t.Errorf("StartedAt = %s, want ~%s (off by %s)", a.StartedAt, started, d)
	}
	if d := a.LastTurn.Sub(lastTurn); d > time.Second || d < -time.Second {
		t.Errorf("LastTurn = %s, want ~%s (off by %s)", a.LastTurn, lastTurn, d)
	}

	// The specific failure this replaces: epoch 0 read back as a plausible-looking
	// time.Time, and Duration then measured from 1970 — a ~56-year "running" time
	// injected into every agent's fleet-status prompt block.
	if a.StartedAt.Year() == 1970 {
		t.Errorf("StartedAt is epoch 0 (%s): the timestamp did not survive the round trip", a.StartedAt)
	}
	if a.Duration > 24*time.Hour {
		t.Errorf("Duration = %s, want ~2h: Duration is being measured from the wrong epoch", a.Duration)
	}
}

// TestListActiveStatusWithNoTurns covers the LEFT JOIN's NULL side: a session that
// has not taken a turn yet must report its own start as the latest activity, not a
// zero time.
func TestListActiveStatusWithNoTurns(t *testing.T) {
	d := openTestDB(t)

	started := time.Now().Add(-30 * time.Minute)
	if err := d.InsertSession(&SessionRow{
		ID: "s1", Agent: "claxon", Model: "sonnet", Command: "run",
		PID: os.Getpid(), StartedAt: started,
	}); err != nil {
		t.Fatalf("InsertSession: %v", err)
	}

	active, err := d.ListActiveStatus()
	if err != nil {
		t.Fatalf("ListActiveStatus: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("got %d active sessions, want 1", len(active))
	}
	a := active[0]

	if a.Turns != 0 {
		t.Errorf("Turns = %d, want 0", a.Turns)
	}
	if a.LastTurn.IsZero() || a.LastTurn.Year() == 1970 {
		t.Errorf("LastTurn = %s, want the session start (%s)", a.LastTurn, started)
	}
	if d := a.LastTurn.Sub(started); d > time.Second || d < -time.Second {
		t.Errorf("LastTurn = %s, want ~%s (the session start)", a.LastTurn, started)
	}
}

// TestListActiveStatusReadsLegacyGoEncodedTimestamps pins the compatibility
// guarantee the fix rests on.
//
// Every row already on disk stores its timestamp in Go's time.Time.String() form,
// monotonic-clock suffix and all (e.g. "... +0000 UTC m=+0.019650163"). That is why
// the fix computes the epoch in Go instead of pinning _time_format=sqlite: pinning
// the write format would leave existing rows in the old encoding and new rows in the
// new one, and no single SQL date expression could read both. The driver's decoder
// handles this encoding, so reading the bare column works for both eras — this test
// writes the raw legacy string past the driver's encoder to prove it.
func TestListActiveStatusReadsLegacyGoEncodedTimestamps(t *testing.T) {
	d := openTestDB(t)

	// Written as a raw string, exactly as the rows in ~/.inber/sessions.db are stored.
	const legacyStartedAt = "2026-03-25 05:35:45.192770721 +0000 UTC m=+0.019650163"
	const legacyTurnAt = "2026-03-25 06:10:00.123456789 +0000 UTC m=+2055.019650163"

	if _, err := d.db.Exec(`
		INSERT INTO sessions (id, agent, model, command, pid, started_at, status, log_file)
		VALUES ('s1', 'claxon', 'sonnet', 'run', ?, ?, 'running', '')`,
		os.Getpid(), legacyStartedAt,
	); err != nil {
		t.Fatalf("insert legacy session: %v", err)
	}
	if _, err := d.db.Exec(`
		INSERT INTO turns (session_id, turn, started_at) VALUES ('s1', 1, ?)`,
		legacyTurnAt,
	); err != nil {
		t.Fatalf("insert legacy turn: %v", err)
	}

	active, err := d.ListActiveStatus()
	if err != nil {
		t.Fatalf("ListActiveStatus: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("got %d active sessions, want 1", len(active))
	}
	a := active[0]

	wantStart := time.Date(2026, 3, 25, 5, 35, 45, 192770721, time.UTC)
	if !a.StartedAt.Equal(wantStart) {
		t.Errorf("StartedAt = %s, want %s (legacy Go-encoded timestamp did not decode)", a.StartedAt, wantStart)
	}
	wantTurn := time.Date(2026, 3, 25, 6, 10, 0, 123456789, time.UTC)
	if !a.LastTurn.Equal(wantTurn) {
		t.Errorf("LastTurn = %s, want %s (legacy Go-encoded timestamp did not decode)", a.LastTurn, wantTurn)
	}
}
