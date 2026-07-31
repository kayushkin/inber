package engine

import (
	"strings"
	"testing"

	"github.com/kayushkin/inber/guard"
)

// TestObserveModeRefusesTheToolsThatChangeThings drives the engine's own gate,
// which is the only caller guard.CheckTool has. Everything below the gate was
// already written and tested; what was missing was anybody asking.
func TestObserveModeRefusesTheToolsThatChangeThings(t *testing.T) {
	e := &Engine{Guard: guard.New(guard.Config{Mode: guard.Observe})}

	refusal := e.buildToolRefusal()
	if refusal == nil {
		t.Fatal("a session with a guard got no tool gate")
	}

	for _, tool := range []string{"shell_commands", "write_files", "edit_files", "deploy"} {
		reason := refusal(tool, "{}")
		if reason == "" {
			t.Errorf("observe mode allowed %s", tool)
			continue
		}
		if !strings.Contains(reason, "observe") {
			t.Errorf("refusal of %s reads %q, and does not say which mode refused it", tool, reason)
		}
	}

	for _, tool := range []string{"read_files", "list_files", "ripgrep"} {
		if reason := refusal(tool, "{}"); reason != "" {
			t.Errorf("observe mode refused the read tool %s: %s", tool, reason)
		}
	}
}

// TestAssistModeRefusesWhatItCannotAsk. Assist routes dangerous tools through
// an approver, and no session here has one — inber sets no ApprovalFunc and
// emits no approval event for llm-bridge to answer. A held call must be
// refused and said to be held, not quietly run.
func TestAssistModeRefusesWhatItCannotAsk(t *testing.T) {
	e := &Engine{Guard: guard.New(guard.Config{Mode: guard.Assist})}

	reason := e.buildToolRefusal()("shell_commands", "{}")

	if reason == "" {
		t.Fatal("assist mode ran a dangerous tool with no approver to ask")
	}
	if !strings.Contains(reason, "approv") {
		t.Errorf("refusal reads %q, and does not say the call was held for approval", reason)
	}
}

// TestASessionThatNamedNoModeRefusesNothing is the fail-closed regression this
// change had to avoid. Every session on this host runs without a mode; if the
// gate refused anything under Unset it would break all of them, which is worse
// than the hole it closes.
func TestASessionThatNamedNoModeRefusesNothing(t *testing.T) {
	mode, err := guard.ParseMode("")
	if err != nil {
		t.Fatalf("ParseMode(\"\"): %v", err)
	}
	e := &Engine{Guard: guard.New(guard.Config{Mode: mode})}

	refusal := e.buildToolRefusal()
	for _, tool := range []string{"shell_commands", "write_files", "read_files", "spawn_agent"} {
		if reason := refusal(tool, "{}"); reason != "" {
			t.Errorf("a session that named no mode refused %s: %s", tool, reason)
		}
	}
}

// TestASessionWithNoGuardHasNoGate. The guard is nil-checked everywhere else in
// the engine, and the gate has to answer the same way: no guard, no gate, and
// no closure that dereferences one.
func TestASessionWithNoGuardHasNoGate(t *testing.T) {
	e := &Engine{}

	if refusal := e.buildToolRefusal(); refusal != nil {
		t.Error("an engine with no guard produced a tool gate")
	}
}
