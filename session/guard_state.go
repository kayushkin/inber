package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kayushkin/inber/guard"
)

// guardStateFileName is the sidecar written beside a session's messages.json,
// alongside the turn counter and for the same reason: messages.json is the wire
// format of the transcript itself, a []anthropic.MessageParam, and every reader
// unmarshals it as one, so nothing else can be put inside it.
const guardStateFileName = "guard_state.json"

// GuardStatePath is where SaveGuardState and LoadGuardState agree the sidecar
// for dir lives. It is exported so that anything which has to reach the file
// itself — a test that corrupts it, a tool that inspects it — asks here instead
// of keeping a second copy of the name.
func GuardStatePath(dir string) string {
	return filepath.Join(dir, guardStateFileName)
}

// SaveGuardState writes a session's safety limits and their running totals into
// dir.
//
// Neither half survives without being written. The caps come from the request
// that started the session and are not recorded anywhere else, and the totals
// cannot be recounted from the transcript: input tokens and dollars are
// properties of the API calls that produced the messages, not of the messages,
// and one RunTurn appends several user-role messages, so counting turns back
// out of the transcript inflates them by however many synthetic injections the
// session happened to hit.
func SaveGuardState(dir string, state guard.State) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal guard state: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	path := GuardStatePath(dir)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// LoadGuardState reads the safety limits and totals previously written into dir.
//
// A missing file is not an error: a session that has never been persisted has
// none, and neither does one persisted before this file existed. Both mean the
// same thing — nothing is known to have been capped or spent — and both yield
// the zero State, which ResumeState treats as having nothing to say. A file that
// exists and cannot be read or parsed is an error, because that is a cap that
// was recorded and then lost, and silently resuming uncapped is the defect this
// file exists to close.
func LoadGuardState(dir string) (guard.State, error) {
	path := GuardStatePath(dir)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return guard.State{}, nil
	}
	if err != nil {
		return guard.State{}, fmt.Errorf("read %s: %w", path, err)
	}
	var state guard.State
	if err := json.Unmarshal(data, &state); err != nil {
		return guard.State{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return state, nil
}
