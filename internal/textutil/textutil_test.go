package textutil

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateKeepsTextThatFits(t *testing.T) {
	for _, s := range []string{"", "abc", "héllo", "🦀"} {
		if got := Truncate(s, 64); got != s {
			t.Errorf("Truncate(%q, 64) = %q, want it unchanged", s, got)
		}
	}
}

func TestTruncateCutsOnARuneBoundary(t *testing.T) {
	// codex's own case for this rule: a run of two-byte runes with a
	// four-byte rune at the end, cut at a budget that lands inside a rune.
	s := strings.Repeat("é", 499) + "🦀"
	for maxBytes := 0; maxBytes <= len(s); maxBytes++ {
		got := Truncate(s, maxBytes)
		if !utf8.ValidString(got) {
			t.Fatalf("Truncate(s, %d) is not valid UTF-8: %q", maxBytes, got)
		}
		if len(got) > maxBytes {
			t.Fatalf("Truncate(s, %d) is %d bytes, over budget", maxBytes, len(got))
		}
		if !strings.HasPrefix(s, got) {
			t.Fatalf("Truncate(s, %d) is not a prefix of the input", maxBytes)
		}
		// Nothing under budget may have been given up: the next rune has to
		// be the one that would not fit.
		if len(got) < len(s) {
			_, size := utf8.DecodeRuneInString(s[len(got):])
			if len(got)+size <= maxBytes {
				t.Fatalf("Truncate(s, %d) stopped at %d bytes with room for the next rune", maxBytes, len(got))
			}
		}
	}
}

func TestTruncateRefusesAPartialRune(t *testing.T) {
	// A budget smaller than the first rune has no prefix to return.
	if got := Truncate("🦀tail", 3); got != "" {
		t.Errorf("Truncate(%q, 3) = %q, want empty — 🦀 is four bytes", "🦀tail", got)
	}
	if got := Truncate("é", 1); got != "" {
		t.Errorf("Truncate(%q, 1) = %q, want empty", "é", got)
	}
}

func TestTruncateOnAnEmptyOrNegativeBudget(t *testing.T) {
	if got := Truncate("abc", 0); got != "" {
		t.Errorf("Truncate(abc, 0) = %q, want empty", got)
	}
	// A negative budget reaches Truncate from call sites that subtract the
	// marker's length from a caller-supplied cap. It must not panic, which is
	// what the raw slice it replaced did.
	if got := Truncate("abc", -3); got != "" {
		t.Errorf("Truncate(abc, -3) = %q, want empty", got)
	}
}

func TestTruncateLeavesInvalidInputAlone(t *testing.T) {
	// Text that is already invalid UTF-8 must still come back bounded rather
	// than send the loop past the start of the string.
	s := "\xff\xff\xff\xff"
	got := Truncate(s, 2)
	if len(got) > 2 {
		t.Errorf("Truncate on invalid input returned %d bytes, over budget", len(got))
	}
}

func TestTruncateTailCutsOnARuneBoundary(t *testing.T) {
	s := "🦀" + strings.Repeat("é", 20)
	for maxBytes := 0; maxBytes <= len(s); maxBytes++ {
		got := TruncateTail(s, maxBytes)
		if !utf8.ValidString(got) {
			t.Fatalf("TruncateTail(s, %d) is not valid UTF-8: %q", maxBytes, got)
		}
		if len(got) > maxBytes {
			t.Fatalf("TruncateTail(s, %d) is %d bytes, over budget", maxBytes, len(got))
		}
		if !strings.HasSuffix(s, got) {
			t.Fatalf("TruncateTail(s, %d) is not a suffix of the input", maxBytes)
		}
	}
}

func TestTruncateWithAppendsOnlyWhenItCut(t *testing.T) {
	if got := TruncateWith("abc", 8, "..."); got != "abc" {
		t.Errorf("TruncateWith on text that fits = %q, want it unchanged and unmarked", got)
	}
	got := TruncateWith(strings.Repeat("é", 10), 5, "…")
	if !utf8.ValidString(got) {
		t.Errorf("TruncateWith produced invalid UTF-8: %q", got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("TruncateWith = %q, want the marker on the end", got)
	}
	if strings.TrimSuffix(got, "…") != "éé" {
		t.Errorf("TruncateWith = %q, want two runes before the marker", got)
	}
}
