package toolid

import "testing"

func TestSanitizeKeepsLegalIdentifiers(t *testing.T) {
	for _, id := range []string{"toolu_01ABC", "call-7", "a", "A1_-"} {
		if got := Sanitize(id); got != id {
			t.Errorf("Sanitize(%q) = %q, want it unchanged", id, got)
		}
	}
}

func TestSanitizeReplacesIllegalRunes(t *testing.T) {
	cases := map[string]string{
		"call:1":     "call_1",
		"call.1":     "call_1",
		"fc/abc def": "fc_abc_def",
		"ключ":       "____",
	}
	for in, want := range cases {
		if got := Sanitize(in); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

// The mapping is many-to-one. This is not the behaviour we want, but it is the
// behaviour that ships, and moving the function must not quietly change it.
// The fix for the collision itself is noteboard todo c114a30b.
func TestSanitizeIsLossyAndCollides(t *testing.T) {
	if Sanitize("call:1") != Sanitize("call.1") {
		t.Fatal("expected the documented collision between call:1 and call.1")
	}
	// Two all-illegal IDs of the same rune count collide too, but through the
	// substitution, not through the fallback below.
	if Sanitize("::") != Sanitize("..") {
		t.Fatal("expected :: and .. to collide on __")
	}
	if got := Sanitize("::"); got != "__" {
		t.Fatalf("Sanitize(\"::\") = %q, want __", got)
	}
}

// The "tool_"+length fallback reads like a second source of collisions. It is
// not reachable as one: the loop writes a rune for every input rune, so an
// empty result implies an empty input, and the length is then always zero.
func TestSanitizeFallbackIsReachableOnlyForTheEmptyString(t *testing.T) {
	if got := Sanitize(""); got != "tool_0" {
		t.Fatalf("Sanitize(\"\") = %q, want tool_0", got)
	}
	for _, id := range []string{"::", "..", "。。", "\x00", " ", "ключ"} {
		if got := Sanitize(id); got == "" {
			t.Errorf("Sanitize(%q) produced an empty result, which should be impossible", id)
		}
	}
}
