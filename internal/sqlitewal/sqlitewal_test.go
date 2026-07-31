package sqlitewal

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// Holding a write transaction on the fresh file makes the race deterministic
// rather than roughly-half-the-time. Against a DSN-carried conversion this fails
// on every run, and it fails instantly — the 0s is the finding, because a
// connection that had honoured its own 5s busy_timeout would have outlasted a
// 150ms lock.
func TestSwitchToWALWaitsOutAConversionItLoses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "waited.db")

	// Stand in for the process that reached the fresh file first and is mid-write.
	// The holder's DSN is spelled out rather than built by ConnectionDSN: it has
	// to leave the fresh file on the rollback journal so the store under test is
	// the one that must convert it. Sharing the helper would let a change to
	// ConnectionDSN quietly convert the file here instead, and the race this test
	// exists for would stop happening.
	holder, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()
	if _, err := holder.Exec("CREATE TABLE IF NOT EXISTS seed (id TEXT)"); err != nil {
		t.Fatal(err)
	}
	held, err := holder.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := held.Exec("INSERT INTO seed VALUES ('x')"); err != nil {
		t.Fatal(err)
	}

	const holdFor = 150 * time.Millisecond
	releasing := time.AfterFunc(holdFor, func() { held.Commit() })
	defer releasing.Stop()

	loser, err := sql.Open("sqlite", ConnectionDSN(path))
	if err != nil {
		t.Fatal(err)
	}
	defer loser.Close()

	started := time.Now()
	err = SwitchToWAL(loser)
	waited := time.Since(started)
	if err != nil {
		t.Fatalf("SwitchToWAL gave up while another connection held the file, after %v: %v", waited, err)
	}
	if waited < holdFor {
		t.Errorf("SwitchToWAL returned after %v, before the %v lock was released — it cannot have waited for it", waited, holdFor)
	}

	var settled string
	if err := loser.QueryRow("PRAGMA journal_mode").Scan(&settled); err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(settled, JournalMode) {
		t.Errorf("journal mode settled on %q, want %s", settled, JournalMode)
	}
}

// The settled-mode branch of SwitchToWAL has no test here on purpose. A lost
// race that comes back reporting the mode it stayed on, rather than an error,
// was reasoned about and never observed: every lost race measured in memory-store
// and here failed loudly. Attempts to stage one — a memory journal under an
// exclusive lock — convert to WAL anyway, so the only test available would be a
// stub asserting against itself.

// The conversion has to be a statement this package runs, not a DSN pragma the
// driver replays on every connection it opens, because only the statement can
// retry. Pinning the DSN keeps the two from drifting back together.
func TestConnectionDSNCarriesBusyTimeoutAndNotTheConversion(t *testing.T) {
	dsn := ConnectionDSN("/tmp/example.db")
	if !strings.Contains(dsn, "_pragma=busy_timeout(5000)") {
		t.Errorf("DSN %q does not carry busy_timeout — modernc drops keys it does not recognise and says nothing", dsn)
	}
	if strings.Contains(dsn, "journal_mode") {
		t.Errorf("DSN %q carries the journal_mode conversion; it belongs in SwitchToWAL, where it can wait", dsn)
	}
}
