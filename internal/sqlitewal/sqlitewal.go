// Package sqlitewal opens SQLite databases in write-ahead-log mode, waiting out
// the conversion when another process reaches a fresh file first.
//
// Switching a database from the rollback journal to WAL takes a brief exclusive
// lock, and that one statement is the only part of opening a store that
// busy_timeout cannot mediate. Measured in memory-store, four connections racing
// to convert one fresh file, 160 opens per row:
//
//	journal_mode alone              120 failed
//	busy_timeout + journal_mode       1 failed, after 2ms
//	busy_timeout(30000) + same        2 failed, after 1ms
//
// The third row is the finding: six times the timeout changes nothing and the
// failure still lands in a millisecond, so the wait is never being entered.
// SQLite declines to run the busy handler when a connection has to upgrade a
// lock it already holds, because waiting there is how two connections deadlock;
// it returns SQLITE_BUSY on the spot instead. A longer timeout has nothing to
// give, so the wait has to be ours.
package sqlitewal

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// BusyTimeoutMilliseconds is how long a contended statement waits for a lock.
const BusyTimeoutMilliseconds = 5000

// JournalMode is the journal every database opened through this package ends up
// in. WAL lets readers run while a writer holds the file, which is the whole
// reason a store converts at all.
const JournalMode = "WAL"

// ConnectionDSN builds the connection string for a database at dbPath.
//
// modernc.org/sqlite only understands _pragma=NAME(VALUE); it silently ignores
// DSN keys it does not recognise, so a mattn-style ?_journal_mode=WAL&_busy_timeout=5000
// applies neither pragma and reports no error.
//
// journal_mode is deliberately not in here with busy_timeout. The driver
// replays every DSN pragma on every connection the pool opens, so putting the
// conversion there runs it in a place that cannot retry, and reports the failure
// as whatever statement happened to open that connection — in memory-store it
// arrived for months as "create schema: database is locked", which sent every
// reader to the schema. It belongs in SwitchToWAL, once, where it can wait.
func ConnectionDSN(dbPath string) string {
	return fmt.Sprintf("%s?_pragma=busy_timeout(%d)", dbPath, BusyTimeoutMilliseconds)
}

// SwitchToWAL moves the database file into JournalMode, retrying for as long as
// a contended statement is already allowed to wait.
//
// Retrying converges because the race is only ever over the first conversion:
// once any process has won it the file is in WAL, and every later connection
// reads the mode back instead of changing it — 240 of 240 concurrent opens
// against an already-converted file succeeded, against 6 retries needed in total
// to convert 60 fresh ones.
//
// This is not a retry loop around ordinary reads and writes. That was tried in
// memory-store and measured worse than doing nothing, because it tries to do
// WAL's job by waiting. This runs once per store, on the one statement that
// turns WAL on, and everything after it relies on WAL rather than on waiting.
func SwitchToWAL(db *sql.DB) error {
	// The conversion gets the same patience as any other contended statement.
	// busy_timeout already answers "how long is this store willing to wait for a
	// lock" and there is no reason for a second number.
	deadline := time.Now().Add(BusyTimeoutMilliseconds * time.Millisecond)

	var settled string
	var err error
	for backoff := time.Millisecond; ; backoff += time.Millisecond {
		err = db.QueryRow("PRAGMA journal_mode(" + JournalMode + ")").Scan(&settled)
		if err == nil && strings.EqualFold(settled, JournalMode) {
			return nil
		}
		if !time.Now().Add(backoff).Before(deadline) {
			break
		}
		time.Sleep(backoff)
	}

	if err != nil {
		return fmt.Errorf("switch journal mode to %s: %w", JournalMode, err)
	}
	// A lost race can also come back quietly, reporting the mode it stayed on
	// rather than an error, and a database left on the rollback journal is the
	// serialized queue this package exists to avoid.
	return fmt.Errorf("journal mode settled on %q, want %s", settled, JournalMode)
}
