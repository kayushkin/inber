package checkpoint

import (
	"errors"
	"testing"
)

// Every entry point must report that it does nothing. The shape this guards
// against is the one the package shipped with: a nil error and a zero value,
// which a caller cannot distinguish from "the snapshot was taken and this
// session has no checkpoints yet".
func TestEveryEntryPointReportsThatItIsNotImplemented(t *testing.T) {
	manager, err := New(t.TempDir(), "session-1")
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("New returned %v; want ErrNotImplemented", err)
	}
	if manager != nil {
		t.Error("New handed back a Manager, so a caller can go on to use one that does nothing")
	}

	// The methods are checked on a bare Manager rather than on New's return,
	// which is nil: an implementation lands method by method, and each one has
	// to keep telling the truth until it is the one that got built.
	m := &Manager{}

	if _, err := m.Take("before turn 3", 3); !errors.Is(err, ErrNotImplemented) {
		t.Errorf("Take returned %v; want ErrNotImplemented", err)
	}
	if _, err := m.List(); !errors.Is(err, ErrNotImplemented) {
		t.Errorf("List returned %v; want ErrNotImplemented", err)
	}
	if _, err := m.Diff(1, 2); !errors.Is(err, ErrNotImplemented) {
		t.Errorf("Diff returned %v; want ErrNotImplemented", err)
	}
	if _, err := m.DiffFromPrevious(2); !errors.Is(err, ErrNotImplemented) {
		t.Errorf("DiffFromPrevious returned %v; want ErrNotImplemented", err)
	}
	// Restore is the dangerous one: it used to report that an agent's file
	// changes had been rolled back without opening a file.
	if err := m.Restore(1); !errors.Is(err, ErrNotImplemented) {
		t.Errorf("Restore returned %v; want ErrNotImplemented", err)
	}
}

// DiffFromPrevious used to answer ("", nil) for num <= 1 before reaching Diff,
// so the guard above could pass on the boundary for the wrong reason.
func TestDiffFromPreviousDoesNotReportSuccessAtTheFirstCheckpoint(t *testing.T) {
	m := &Manager{}
	for _, num := range []int{0, 1} {
		if _, err := m.DiffFromPrevious(num); !errors.Is(err, ErrNotImplemented) {
			t.Errorf("DiffFromPrevious(%d) returned %v; want ErrNotImplemented", num, err)
		}
	}
}
