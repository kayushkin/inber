package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// disabledToolsFileName is the sidecar written beside a session's messages.json,
// alongside the turn counter and the guard state, and for the same reason:
// messages.json is the wire format of the transcript itself, a
// []anthropic.MessageParam, and every reader unmarshals it as one, so nothing
// else can be put inside it.
const disabledToolsFileName = "disabled_tools.json"

// DisabledToolsPath is where SaveDisabledTools and LoadDisabledTools agree the
// sidecar for dir lives. It is exported so that anything which has to reach the
// file itself — a test that corrupts it, a tool that inspects it — asks here
// instead of keeping a second copy of the name.
func DisabledToolsPath(dir string) string {
	return filepath.Join(dir, disabledToolsFileName)
}

// SaveDisabledTools writes the names a session has taken off the wire into dir.
//
// The set is not recorded anywhere else and cannot be recovered from anything
// that is. It arrives on a POST /sessions/{id}/config call, lives in engine
// memory for one process lifetime, and is not a field of the request that
// starts a session — so a session rebuilt after a restart came back with every
// tool it had been given, no log line and no error, while the caller that took
// the tool away still believed it was gone.
//
// An empty set is written, not skipped. Sending no names is how a caller
// re-enables everything, and skipping the write would leave the previous file
// in place — so the one request that means "put the tools back" would be the
// one request a rebuild ignored. The file is the whole answer or it is not an
// answer.
func SaveDisabledTools(dir string, names []string) error {
	if names == nil {
		names = []string{}
	}
	data, err := json.Marshal(names)
	if err != nil {
		return fmt.Errorf("marshal disabled tools: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	path := DisabledToolsPath(dir)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// LoadDisabledTools reads the names previously written into dir.
//
// A missing file is not an error: a session that has never been persisted has
// none, and neither does one persisted before this file existed. Both mean the
// same thing — nothing is known to have been taken off the wire — and both
// yield no names. A file that exists and cannot be read or parsed is an error,
// because that is a tool a caller took away and the record of it being lost,
// which is the defect this file exists to close.
func LoadDisabledTools(dir string) ([]string, error) {
	path := DisabledToolsPath(dir)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var names []string
	if err := json.Unmarshal(data, &names); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return names, nil
}
