package guard

import "fmt"

// ParseMode turns the mode a caller named into the Mode a guard enforces. It is
// the inverse of Mode.String, and the empty string round-trips to Unset: a
// request that carries no mode is asking for nothing in particular, which
// CheckTool treats as full access.
//
// Anything else that is not a mode is an error, never a default. A mode is a
// trust boundary and its value arrives as user input, so a typo — "observer",
// "read-only", "safe" — must not quietly resolve to the mode that allows
// `bash -c`. The returned Mode on an error is Observe rather than the zero
// value, so that a caller who ignores the error still ends up with the
// strictest mode instead of the loosest one.
func ParseMode(name string) (Mode, error) {
	switch name {
	case "":
		return Unset, nil
	case "observe":
		return Observe, nil
	case "assist":
		return Assist, nil
	case "autonomous":
		return Autonomous, nil
	}
	return Observe, fmt.Errorf("unknown execution mode %q: want observe, assist or autonomous", name)
}
