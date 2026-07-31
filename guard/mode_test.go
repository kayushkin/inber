package guard

import "testing"

// TestEveryModeRoundTripsThroughItsOwnName. ParseMode and Mode.String are the
// two halves of how a mode is written down and read back — the persisted state
// stores the name and the resume parses it — so a mode whose name does not
// round-trip is a mode that cannot survive a rebuild.
func TestEveryModeRoundTripsThroughItsOwnName(t *testing.T) {
	for _, mode := range []Mode{Unset, Observe, Assist, Autonomous} {
		parsed, err := ParseMode(mode.String())
		if err != nil {
			t.Errorf("ParseMode(%q) — the name %v gives itself — failed: %v", mode.String(), mode, err)
			continue
		}
		if parsed != mode {
			t.Errorf("ParseMode(%q) = %v, want %v", mode.String(), parsed, mode)
		}
	}
}

// TestAModeNobodyNamedAllowsEverything pins the default, and it is the
// assertion most worth having. Almost nothing on this box names a mode, so a
// parse that failed closed — reading "" as Observe, the tightest mode — would
// refuse every write and every shell command in every session on the host. That
// is a far worse outcome than the hole this gate was built to close.
func TestAModeNobodyNamedAllowsEverything(t *testing.T) {
	mode, err := ParseMode("")
	if err != nil {
		t.Fatalf("ParseMode(\"\"): %v", err)
	}

	g := New(Config{Mode: mode})
	for _, tool := range []string{"shell_commands", "write_files", "edit_files", "read_files"} {
		if verdict := g.CheckTool(tool, "{}"); verdict != Allowed {
			t.Errorf("with no mode named, CheckTool(%q) = %v, want Allowed", tool, verdict)
		}
	}
}

// TestAMisspelledModeIsAnErrorNotAGuess. The value arrives as user input on an
// HTTP request. A near miss resolved to a default would resolve to the default
// that allows `bash -c` with the full environment, which is the opposite of
// what somebody typing "observer" is asking for.
func TestAMisspelledModeIsAnErrorNotAGuess(t *testing.T) {
	for _, name := range []string{"observer", "read-only", "safe", "OBSERVE", "auto"} {
		mode, err := ParseMode(name)
		if err == nil {
			t.Errorf("ParseMode(%q) = %v with no error — a mode nobody defined was accepted", name, mode)
			continue
		}
		if mode != Observe {
			t.Errorf("ParseMode(%q) returned %v alongside its error, want Observe — a caller who ignores the error must not be handed the loosest mode", name, mode)
		}
	}
}
