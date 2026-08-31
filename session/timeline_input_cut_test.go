package session

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

// summarizeToolInput cuts on two of its branches and neither cut held the rune
// boundary. TestSummarizeToolInput's table reaches both lines, but every one of
// its fixtures is ASCII — `strings.Repeat("x", 100)` for the generic branch —
// and on ASCII a byte cut and a rune-safe cut agree, so the table cannot tell
// them apart. Measured 2026-08-31 on branch
// test/the-truncatewith-call-sites-are-asked: reverting either line to the
// pre-9ce1666 byte cut reddened nothing in the whole repository.
//
//	session/timeline_utils.go:27  the shell command, to 100 bytes
//	session/timeline_utils.go:51  the generic raw input, to 80 bytes
//
// :27 was not even in the case list that judged these sites -- that list was
// enumerated from `grep -n TruncateWith` and :27 calls textutil.Truncate.
//
// These cuts feed the timeline file, not the model, so they are the cheap end
// of the ranking in card 3bec041f. They are pinned here because the fixture is
// the only thing missing: the call sites are already executed by a test.
//
// Both fixtures put one ASCII byte in front of a run of two-byte runes, so
// every rune in the run starts at an ODD offset and an even budget necessarily
// cuts inside one. The straddle guard below fails loudly rather than passing if
// a changed budget ever moves the cut out of that run.

// straddledCut reports whether `cut` -- the part of the summary before the
// marker -- ends inside the fixture's multibyte run. A case whose cut landed in
// the ASCII prefix cannot tell a byte cut from a rune-safe one.
func straddledCut(cut string) bool {
	return cut != "" && cut[len(cut)-1] >= 0x80
}

func TestTheGenericToolInputCutLandsOnARuneBoundary(t *testing.T) {
	raw := "x" + strings.Repeat("é", 60)

	got := summarizeToolInput("other", raw)

	cut := strings.TrimSuffix(got, "...")
	if cut == got {
		t.Fatalf("the generic branch did not cut a %d-byte input at all, so this case "+
			"cannot see the cut.\ngot %q", len(raw), got)
	}
	if !straddledCut(cut) {
		t.Fatalf("the cut landed in the fixture's ASCII prefix, so a byte cut and a "+
			"rune-safe cut would agree and this case could not tell them apart. The "+
			"fixture needs rebuilding against the current budget.\ngot %q", got)
	}
	if !utf8.ValidString(got) {
		t.Errorf("the generic tool-input summary written to the timeline is not valid "+
			"UTF-8: the cut ended inside a rune.\ngot %q", got)
	}
}

func TestTheShellCommandCutLandsOnARuneBoundary(t *testing.T) {
	command := "x" + strings.Repeat("é", 80)
	raw, err := json.Marshal(map[string]string{"command": command})
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}

	got := summarizeToolInput("shell", string(raw))

	// The shell branch returns "`" + cut + "...`" when it cuts, and "`" + whole
	// + "`" when it does not.
	cut := strings.TrimSuffix(strings.TrimPrefix(got, "`"), "...`")
	if cut == strings.TrimPrefix(got, "`") {
		t.Fatalf("the shell branch did not cut a %d-byte command at all, so this case "+
			"cannot see the cut.\ngot %q", len(command), got)
	}
	if !straddledCut(cut) {
		t.Fatalf("the cut landed in the fixture's ASCII prefix, so a byte cut and a "+
			"rune-safe cut would agree and this case could not tell them apart. The "+
			"fixture needs rebuilding against the current budget.\ngot %q", got)
	}
	if !utf8.ValidString(got) {
		t.Errorf("the shell-command summary written to the timeline is not valid UTF-8: "+
			"the cut ended inside a rune.\ngot %q", got)
	}
}

// TestTheShellCommandIsCutToItsBudget is the does-it-cut-at-all claim for :27
// on its own. The generic branch's half of this claim is already held by
// TestSummarizeToolInput/generic_truncate, which is why there is no sibling for
// it here.
//
// Written as a comparison between two commands rather than a match against a
// remembered length: a short command must survive whole and a long one must not.
func TestTheShellCommandIsCutToItsBudget(t *testing.T) {
	summarize := func(command string) string {
		raw, err := json.Marshal(map[string]string{"command": command})
		if err != nil {
			t.Fatalf("marshal fixture: %v", err)
		}
		return summarizeToolInput("shell", string(raw))
	}

	short := strings.Repeat("s", 20)
	if got := summarize(short); !strings.Contains(got, short) {
		t.Errorf("a %d-byte command is inside the budget and should reach the timeline "+
			"whole; it did not.\ngot %q", len(short), got)
	}

	long := strings.Repeat("L", 500)
	if got := summarize(long); strings.Contains(got, long) {
		t.Errorf("a %d-byte command reached the timeline uncut, so the cut at "+
			"timeline_utils.go:27 bounds nothing.\ngot %d bytes", len(long), len(got))
	}
}
