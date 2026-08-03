package session

import (
	"os"
	"strings"
	"testing"
)

func equalNames(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// TestDisabledToolsAreReadBackAsWritten. The set reaches an engine on one HTTP
// call and lives in that process's memory; this sidecar is the only thing that
// carries it to the next one.
func TestDisabledToolsAreReadBackAsWritten(t *testing.T) {
	dir := t.TempDir()

	if err := SaveDisabledTools(dir, []string{"shell", "write_files"}); err != nil {
		t.Fatalf("SaveDisabledTools: %v", err)
	}

	names, err := LoadDisabledTools(dir)
	if err != nil {
		t.Fatalf("LoadDisabledTools: %v", err)
	}
	if want := []string{"shell", "write_files"}; !equalNames(names, want) {
		t.Errorf("got %v, want %v", names, want)
	}
}

// TestReEnablingEverythingIsRecordedRatherThanSkipped is the trap an
// only-write-when-non-empty save walks into. Sending no names is how a caller
// re-enables every tool, so skipping that write would leave the previous file
// in place and a rebuild would take the tools away again — the one request that
// means "put them back" would be the one request that did not survive.
func TestReEnablingEverythingIsRecordedRatherThanSkipped(t *testing.T) {
	dir := t.TempDir()

	if err := SaveDisabledTools(dir, []string{"shell"}); err != nil {
		t.Fatalf("SaveDisabledTools: %v", err)
	}
	// The re-enable-everything request, in both spellings the wire allows.
	for _, empty := range [][]string{{}, nil} {
		if err := SaveDisabledTools(dir, empty); err != nil {
			t.Fatalf("SaveDisabledTools(%v): %v", empty, err)
		}

		names, err := LoadDisabledTools(dir)
		if err != nil {
			t.Fatalf("LoadDisabledTools: %v", err)
		}
		if len(names) != 0 {
			t.Fatalf("after re-enabling everything with %v, the record still says %v is off the wire", empty, names)
		}
	}
}

// TestASessionThatDisabledNothingIsNotAnError. Every session is this on its
// first turn, and so is one persisted before this sidecar existed. Both mean
// nothing was taken off the wire.
func TestASessionThatDisabledNothingIsNotAnError(t *testing.T) {
	names, err := LoadDisabledTools(t.TempDir())

	if err != nil {
		t.Fatalf("a session with no record is ordinary: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("got %v, want nothing", names)
	}
}

// TestAnUnreadableRecordIsReportedRatherThanTreatedAsEmpty. A truncated write
// or a half-finished copy read as "nothing was disabled" would put a tool a
// caller took away back on the wire, which is the whole defect this file
// closes, and it would do it silently.
func TestAnUnreadableRecordIsReportedRatherThanTreatedAsEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(DisabledToolsPath(dir), []byte(`["shell"`), 0o644); err != nil {
		t.Fatalf("write record: %v", err)
	}

	names, err := LoadDisabledTools(dir)

	if err == nil {
		t.Fatal("a corrupt record read as an empty one; the rebuild would re-enable the tool and say nothing")
	}
	if len(names) != 0 {
		t.Errorf("no names should come back from a failed read, got %v", names)
	}
	if !strings.Contains(err.Error(), disabledToolsFileName) {
		t.Errorf("the error should name the file to move aside: %v", err)
	}
}
