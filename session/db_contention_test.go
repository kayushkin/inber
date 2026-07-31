package session

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kayushkin/inber/internal/sqlitewal"
	_ "modernc.org/sqlite"
)

// This database lives at <repo>/.inber/sessions.db, so every inber session
// running on one repo opens the same file — which is exactly the case that races
// to convert it the first time. The conversion used to ride in on the DSN, where
// the driver ran it while opening a connection and nothing could retry it, so a
// second session starting on a fresh repo failed to open at all.
//
// Holding a write transaction makes that deterministic rather than
// roughly-half-the-time. Against the DSN version this fails on every run, and it
// fails instantly: the 0s is the assertion, because a store that had honoured
// its own 5s busy_timeout would have outlasted a 150ms lock.
func TestOpenDBWaitsOutAConversionItLoses(t *testing.T) {
	repoRoot := t.TempDir()
	path := filepath.Join(repoRoot, ".inber", "sessions.db")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}

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

	opened := time.Now()
	store, err := OpenDB(repoRoot)
	waited := time.Since(opened)
	if err != nil {
		t.Fatalf("OpenDB gave up while another session held the file, after %v: %v", waited, err)
	}
	defer store.Close()

	if waited < holdFor {
		t.Errorf("OpenDB returned after %v, before the %v lock was released — it cannot have waited for it", waited, holdFor)
	}

	// Surviving is not enough: a session database left on the rollback journal
	// serialises the readers WAL exists to let run alongside a writer.
	var settled string
	if err := store.db.QueryRow("PRAGMA journal_mode").Scan(&settled); err != nil {
		t.Fatalf("read back journal_mode: %v", err)
	}
	if !strings.EqualFold(settled, sqlitewal.JournalMode) {
		t.Errorf("journal_mode settled on %q, want %s", settled, sqlitewal.JournalMode)
	}
}
