// Package toolid normalises tool-call identifiers to the shape the Anthropic
// API accepts.
package toolid

import (
	"strconv"
	"strings"
)

// Sanitize rewrites a tool ID to match Anthropic's pattern ^[a-zA-Z0-9_-]+$.
// OpenAI and GLM mint IDs containing dots, colons and other characters that
// the Anthropic API rejects.
//
// The mapping is many-to-one and therefore lossy: "call:1" and "call.1" both
// become "call_1". No caller checks for a collision, and two colliding IDs in
// one turn produce duplicate tool_use IDs and mispaired tool_results. Choosing
// between disambiguating a collision and rejecting it is an open question:
// noteboard todo c114a30b-acc0-4c0f-8d0f-00761348dea9.
//
// The "tool_" fallback below looks like a second, worse source of collisions,
// keyed on length. It is not: the loop writes one rune for every input rune,
// legal or not, so the result is empty only when the input is empty — and then
// the length is zero. The fallback has exactly one reachable value, "tool_0".
func Sanitize(id string) string {
	var b strings.Builder
	b.Grow(len(id))
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	result := b.String()
	if result == "" {
		return "tool_" + strconv.Itoa(len(id))
	}
	return result
}
