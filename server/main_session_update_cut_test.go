package server

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// updateMainSession builds the "[Context update]" message that a finished spawn
// injects into its parent's main session, and it cuts twice on the way:
//
//	server/spawn_delivery.go:214  the child's summary, to 497 bytes + "..."
//	server/spawn_delivery.go:220  the task, via server/session_utils.go:23
//
// Both cuts reach the model. queuePending puts the message on the parent
// session's pendingMessages, and the next turn prefixes it onto the prompt.
//
// Measured 2026-08-31 on branch test/the-truncatewith-call-sites-are-asked:
// neither site had a test of any kind. Reverting either to the pre-9ce1666 byte
// cut, or deleting either truncation outright, reddened nothing in the whole
// repository. They were invisible to the sweep that judged these sites because
// that case list was enumerated from `grep -n TruncateWith`, and both call
// textutil.Truncate instead. Card 3388f950, residual 3bec041f.
//
// session_utils.go:23 is the busiest cut in inber — ten call sites — and this
// is the first test to reach any of them.

// queuedMainSessionUpdate runs updateMainSession against a parent session that
// exists, and hands back the single message it queued.
func queuedMainSessionUpdate(t *testing.T, task, summary string) string {
	t.Helper()

	g := &Server{}
	parent := &Session{Key: "agent:claxon:main", AgentName: "claxon"}
	g.sessions.Store(parent.Key, parent)

	g.updateMainSession("claxon", task, "completed", summary)

	parent.mu.Lock()
	defer parent.mu.Unlock()
	if len(parent.pendingMessages) != 1 {
		t.Fatalf("updateMainSession queued %d messages, want exactly 1",
			len(parent.pendingMessages))
	}
	return parent.pendingMessages[0]
}

// fieldOf pulls one "Name: value" line out of the context-update message.
func fieldOf(t *testing.T, msg, name string) string {
	t.Helper()
	for _, line := range strings.Split(msg, "\n") {
		if strings.HasPrefix(line, name+": ") {
			return strings.TrimPrefix(line, name+": ")
		}
	}
	t.Fatalf("no %q line in the queued context update:\n%s", name, msg)
	return ""
}

// straddles reports whether `field` was cut inside a run of multibyte runes,
// which is the only state in which a case can tell a byte cut from a rune-safe
// one. The byte just before the "..." marker must not be ASCII: if it is, the
// cut landed in the fixture's ASCII prefix and the case is vacuous.
func straddles(field string) bool {
	cut := strings.TrimSuffix(field, "...")
	if cut == field || cut == "" {
		return false
	}
	return cut[len(cut)-1] >= 0x80
}

// multibyteFixture is an ASCII prefix of `prefix` bytes followed by `runes`
// copies of a two-byte rune. Shifting `prefix` by one byte moves the cut's
// offset within the multibyte run by one, so sweeping four consecutive values
// covers both residues modulo the rune width. That is deliberate: the cut
// indexes here (497, 197) are constants in the code, and a fixture engineered
// to straddle today's value goes quiet in the flattering direction the day one
// of them is changed.
func multibyteFixture(prefix, runes int) string {
	return strings.Repeat("a", prefix) + strings.Repeat("é", runes)
}

func TestTheSpawnSummaryCutIntoTheMainSessionLandsOnARuneBoundary(t *testing.T) {
	for pad := 0; pad < 4; pad++ {
		msg := queuedMainSessionUpdate(t, "a short task", multibyteFixture(400+pad, 100))
		summary := fieldOf(t, msg, "Summary")

		if !straddles(summary) {
			t.Fatalf("pad=%d: the summary cut did not land inside the multibyte run, "+
				"so this case cannot tell a byte cut from a rune-safe one. The fixture "+
				"needs rebuilding against the current budget.\nSummary %q", pad, summary)
		}
		if !utf8.ValidString(msg) {
			t.Errorf("pad=%d: the context update injected into the parent session is not "+
				"valid UTF-8: the spawn summary was cut inside a rune.\nSummary %q",
				pad, summary)
		}
	}
}

func TestTheSpawnSummaryIsCutBeforeItReachesTheMainSession(t *testing.T) {
	short := strings.Repeat("s", 100)
	if got := fieldOf(t, queuedMainSessionUpdate(t, "t", short), "Summary"); got != short {
		t.Errorf("a %d-byte summary is inside the budget and should reach the parent "+
			"whole; it did not.\nSummary %q", len(short), got)
	}

	long := strings.Repeat("L", 5000)
	got := fieldOf(t, queuedMainSessionUpdate(t, "t", long), "Summary")

	// Contains, not equality. The call site is `Truncate(...) + "..."`, so a
	// mutation that stops the cut but keeps the marker yields `long + "..."` --
	// unequal to `long`, and an equality check calls that a pass. Measured: the
	// scorer's own notrunc arm has exactly that shape, and this assertion was
	// written as equality first and reported SURVIVED against it.
	if strings.Contains(got, long) {
		t.Errorf("the whole %d-byte summary reached the parent session, so the cut at "+
			"spawn_delivery.go:214 bounds nothing.\nSummary is %d bytes", len(long), len(got))
	}
}

func TestTheSpawnTaskCutIntoTheMainSessionLandsOnARuneBoundary(t *testing.T) {
	for pad := 0; pad < 4; pad++ {
		msg := queuedMainSessionUpdate(t, multibyteFixture(100+pad, 100), "a short summary")
		task := fieldOf(t, msg, "Task")

		if !straddles(task) {
			t.Fatalf("pad=%d: the task cut did not land inside the multibyte run, so "+
				"this case cannot tell a byte cut from a rune-safe one. The fixture "+
				"needs rebuilding against the current budget.\nTask %q", pad, task)
		}
		if !utf8.ValidString(msg) {
			t.Errorf("pad=%d: the context update injected into the parent session is not "+
				"valid UTF-8: the spawn task was cut inside a rune by the shared "+
				"truncate helper in session_utils.go.\nTask %q", pad, task)
		}
	}
}

func TestTheSpawnTaskIsCutBeforeItReachesTheMainSession(t *testing.T) {
	short := strings.Repeat("s", 50)
	if got := fieldOf(t, queuedMainSessionUpdate(t, short, "x"), "Task"); got != short {
		t.Errorf("a %d-byte task is inside the budget and should reach the parent "+
			"whole; it did not.\nTask %q", len(short), got)
	}

	long := strings.Repeat("L", 5000)
	got := fieldOf(t, queuedMainSessionUpdate(t, long, "x"), "Task")

	// Contains, not equality -- same reason as the summary case above.
	if strings.Contains(got, long) {
		t.Errorf("the whole %d-byte task reached the parent session, so the shared "+
			"truncate helper bounds nothing at this call site.\nTask is %d bytes",
			len(long), len(got))
	}
}
