package engine

import (
	"strings"
	"testing"
)

// The memory_save arm of formatToolResult shortens the returned memory id for
// the inline display. Its only guard is `result.ID != ""`, so before the fix a
// cut of exactly eight bytes ran against an id of any non-zero length and
// panicked with "slice bounds out of range" on anything shorter than eight —
// inside the display path of an interactive run, where a panic takes the whole
// session down rather than printing a short line.
//
// These tests drive formatToolResult itself, not the truncation helper. The
// helper was never broken; the call site was the thing that never called one.

func TestMemorySaveIDPreviewSurvivesAnIDShorterThanTheCut(t *testing.T) {
	// A memory id of 1 to 7 bytes. Every one of these panicked before the fix.
	for _, id := range []string{"a", "ab", "abc", "abcd", "abcde", "abcdef", "abcdefg"} {
		t.Run(id, func(t *testing.T) {
			got := formatToolResult("memory_save", `{"id":"`+id+`"}`)
			want := "saved " + id
			if got != want {
				t.Errorf("formatToolResult(memory_save, id=%q) = %q, want %q", id, got, want)
			}
		})
	}
}

func TestMemorySaveIDPreviewCutsAnOverlongIDToEightBytes(t *testing.T) {
	// The truncating branch: the reason the cut is there at all. An id longer
	// than the budget is shortened, and shortened to the budget exactly.
	got := formatToolResult("memory_save", `{"id":"0123456789abcdef"}`)
	if want := "saved 01234567"; got != want {
		t.Errorf("formatToolResult(memory_save, 16-byte id) = %q, want %q", got, want)
	}
	preview := strings.TrimPrefix(got, "saved ")
	if len(preview) != 8 {
		t.Errorf("preview %q is %d bytes, want 8 — the cut is not being applied", preview, len(preview))
	}
}

func TestMemorySaveIDPreviewAtExactlyTheCutIsUnchanged(t *testing.T) {
	// Eight bytes is the one length at which a raw id[:8] and a rune-safe cut
	// return the identical string, so a fixture that only ever uses this length
	// cannot tell a guarded call site from an unguarded one. It is here as the
	// boundary case, not as the coverage.
	got := formatToolResult("memory_save", `{"id":"01234567"}`)
	if want := "saved 01234567"; got != want {
		t.Errorf("formatToolResult(memory_save, 8-byte id) = %q, want %q", got, want)
	}
}

func TestMemorySaveIDPreviewNeverSplitsARune(t *testing.T) {
	// Memory ids are ASCII in practice, which is why a reviewer hunting split
	// runes passes this site over. The repair is a rune-safe cut regardless, so
	// pin that it is.
	//
	// The fixture's leading "a" is load-bearing and is the whole reason this
	// test says anything. Without it, "мемори-1" is two bytes per rune and the
	// eight-byte cut lands exactly on a rune boundary — the test passed against
	// the raw, unguarded id[:8]. One odd-width byte in front shifts every later
	// boundary off the cut, so byte 8 now falls inside the fourth rune.
	got := formatToolResult("memory_save", `{"id":"aмемори-1"}`)
	if want := "saved aмем"; got != want {
		t.Errorf("formatToolResult(memory_save, multibyte id) = %q, want %q", got, want)
	}
	preview := strings.TrimPrefix(got, "saved ")
	if strings.ContainsRune(preview, '�') {
		t.Errorf("preview %q ends inside a rune", preview)
	}
}

func TestMemorySaveWithNoIDStillReportsTheByteCount(t *testing.T) {
	// The empty-id and unparseable-output arms fall through to the byte count.
	// They are the reason the panic was never noticed: the only guard on the
	// cut is the one that steers these two away from it.
	for name, output := range map[string]string{
		"empty id":     `{"id":""}`,
		"no id field":  `{"ok":true}`,
		"not json":     `saved fine`,
		"empty output": ``,
	} {
		t.Run(name, func(t *testing.T) {
			got := formatToolResult("memory_save", output)
			want := "saved (" + itoa(len(output)) + " bytes)"
			if got != want {
				t.Errorf("formatToolResult(memory_save, %q) = %q, want %q", output, got, want)
			}
		})
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
