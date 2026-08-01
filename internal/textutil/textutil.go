// Package textutil cuts text down to a byte budget without splitting a rune.
//
// Every truncation in inber used to be a plain byte slice, `s[:n]`. That is
// correct only while the text is ASCII. One multibyte rune straddling the cut
// leaves a partial rune at the end of the result, which is invalid UTF-8 — and
// these cuts feed the model's prompt (pruned tool-call arguments, session
// summaries), the session database, the timeline files and the terminal alike.
// The rule is the one codex writes into its own tests: a byte cap is cut on a
// rune boundary.
//
// The budget is counted in bytes, not runes, because that is what every caller
// already meant: the cap exists to bound how much text is carried, and a rune
// costs between one and four bytes.
package textutil

import "unicode/utf8"

// Truncate returns the longest prefix of s that is at most maxBytes long and
// does not end inside a rune. It returns s unchanged when s already fits, and
// the empty string when maxBytes is zero or negative, or when the first rune
// alone is over budget — a partial rune is never a prefix worth having.
//
// Nothing is appended. A caller that wants an ellipsis appends its own, so the
// several spellings already in the tree ("...", "…") stay as they are.
func Truncate(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	// s[cut] is the first byte that will be dropped. While it is a
	// continuation byte it belongs to a rune that started before the cut, so
	// the cut is inside that rune and has to move back.
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

// TruncateTail is Truncate from the other end: the longest suffix of s that is
// at most maxBytes long and does not start inside a rune. It is what a head +
// tail display needs for its second half.
func TruncateTail(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	start := len(s) - maxBytes
	for start < len(s) && !utf8.RuneStart(s[start]) {
		start++
	}
	return s[start:]
}

// TruncateWith returns s when it fits in maxBytes, and otherwise the
// rune-safe prefix followed by marker. The marker is not counted against
// maxBytes, which is what the call sites this replaced already did.
func TruncateWith(s string, maxBytes int, marker string) string {
	if len(s) <= maxBytes {
		return s
	}
	return Truncate(s, maxBytes) + marker
}
