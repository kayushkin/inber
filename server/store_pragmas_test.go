package server

import (
	"path/filepath"
	"testing"
)

// TestNewStoreAppliesPragmas pins that the gateway DB's DSN actually applies WAL
// and busy_timeout.
//
// modernc.org/sqlite silently ignores DSN keys it does not recognise, so the
// mattn-style ?_journal_mode=WAL&_busy_timeout=5000 this store shipped with opened
// cleanly and applied neither — the live ~/.inber/server/server.db was in
// journal_mode=delete with no busy timeout, while being written from concurrent
// HTTP handlers. Asserting on the pragmas rather than the DSN string is what makes
// this unfakeable.
func TestNewStoreAppliesPragmas(t *testing.T) {
	s, err := NewStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	defer s.Close()

	var journalMode string
	if err := s.db.QueryRow(`PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("journal_mode = %q, want %q (the DSN pragma was not applied)", journalMode, "wal")
	}

	var busyTimeout int
	if err := s.db.QueryRow(`PRAGMA busy_timeout`).Scan(&busyTimeout); err != nil {
		t.Fatalf("PRAGMA busy_timeout: %v", err)
	}
	if busyTimeout != 5000 {
		t.Errorf("busy_timeout = %d, want 5000 (the DSN pragma was not applied)", busyTimeout)
	}
}
