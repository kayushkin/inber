package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// turnCounterFileName is the sidecar written beside a session's messages.json.
// It is a sidecar rather than a field inside messages.json because that file is
// the wire format of the transcript itself — a []anthropic.MessageParam — and
// every reader unmarshals it as one.
const turnCounterFileName = "turn_counter.json"

// PersistedTurnCounter carries the engine's turn count across invocations.
//
// It is written next to the transcript it counts and it has to be written,
// because the count cannot be recovered from the transcript. One RunTurn
// appends several user-role messages: the input, plus any synthetic injection
// ("[New message from user while you were working]", "[BUDGET LIMIT REACHED]",
// a summary and its canned acknowledgement). After a round trip through
// messages.json those are indistinguishable from real user turns, so counting
// them back would inflate the number by however many injections a session
// happened to hit.
type PersistedTurnCounter struct {
	Counter int `json:"counter"`
}

// SaveTurnCounter writes the turn count into dir, beside messages.json.
func SaveTurnCounter(dir string, counter int) error {
	data, err := json.Marshal(PersistedTurnCounter{Counter: counter})
	if err != nil {
		return fmt.Errorf("marshal turn counter: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	path := filepath.Join(dir, turnCounterFileName)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// LoadTurnCounter reads the turn count previously written into dir.
//
// A missing file is not an error: a session that has never been resumed has
// none, and neither does one persisted before this file existed. Both mean the
// same thing — no turns are known to have run — and both yield 0. A file that
// exists and cannot be read or parsed is an error, because that is a count that
// was recorded and then lost.
func LoadTurnCounter(dir string) (int, error) {
	path := filepath.Join(dir, turnCounterFileName)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", path, err)
	}
	var persisted PersistedTurnCounter
	if err := json.Unmarshal(data, &persisted); err != nil {
		return 0, fmt.Errorf("parse %s: %w", path, err)
	}
	return persisted.Counter, nil
}

// ClearTurnCounter removes the sidecar. It belongs wherever the transcript it
// counts is cleared: a counter left behind by a discarded transcript would
// tell the next session it is fifty turns into a conversation that has no
// messages in it.
func ClearTurnCounter(dir string) {
	os.Remove(filepath.Join(dir, turnCounterFileName))
}
