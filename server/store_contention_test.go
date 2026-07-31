package server

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// NewStore used to carry the WAL conversion in its DSN, where the driver ran it
// while opening a connection and no retry was possible. A gateway starting on a
// fresh database while anything else held that file failed to open at all, and
// reported it as whichever statement happened to open the connection —
// "migrate server db: database is locked", which sends every reader to the schema.
//
// Holding a write transaction on the fresh file makes the race deterministic
// rather than roughly-half-the-time. Against the DSN version this fails on every
// run, and it fails instantly: the 0s is the assertion, because a store that had
// honoured its own 5s busy_timeout would have outlasted a 150ms lock.
func TestNewStoreWaitsOutAConversionItLoses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.db")

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
	store, err := NewStore(path)
	waited := time.Since(opened)
	if err != nil {
		t.Fatalf("NewStore gave up while another connection held the file, after %v: %v", waited, err)
	}
	defer store.Close()

	if waited < holdFor {
		t.Errorf("NewStore returned after %v, before the %v lock was released — it cannot have waited for it", waited, holdFor)
	}
}
