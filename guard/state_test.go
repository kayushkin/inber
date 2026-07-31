package guard

import "testing"

// TestAGuardReportsTheCapsItIsEnforcing pins the read half. Nothing outside this
// package could see a guard's caps at all before State existed, so a session
// that had lost them had no way to notice, and neither did a test.
func TestAGuardReportsTheCapsItIsEnforcing(t *testing.T) {
	g := New(Config{Mode: Autonomous, MaxTurns: 7, MaxInputTokens: 90_000, MaxCost: 5.00})
	g.RecordTurn(1_000)
	g.RecordCost(1.25)

	state := g.State()

	if state.MaxTurns != 7 || state.MaxInputTokens != 90_000 || state.MaxCost != 5.00 {
		t.Errorf("caps reported as %+v, want max turns 7, max input tokens 90000, max cost 5.00", state)
	}
	if state.Turns != 1 || state.InputTokens != 1_000 || state.Cost != 1.25 {
		t.Errorf("totals reported as %+v, want 1 turn, 1000 input tokens, $1.25", state)
	}
}

// TestARestoredGuardStopsWhereTheOldOneWouldHave is the point of the whole
// mechanism. A guard rebuilt for a session that has already spent $4.80 of its
// $5.00 must be over its limit immediately, not $5.00 away from it.
func TestARestoredGuardStopsWhereTheOldOneWouldHave(t *testing.T) {
	rebuilt := New(Config{Mode: Autonomous})

	if exceeded, _ := rebuilt.CheckLimits(); exceeded {
		t.Fatal("a fresh uncapped guard reports a limit exceeded before anything was restored")
	}

	rebuilt.RestoreState(State{MaxCost: 5.00, Cost: 4.80})
	if exceeded, _ := rebuilt.CheckLimits(); exceeded {
		t.Error("$4.80 spent against a $5.00 cap reads as exceeded — the cap trips a turn early")
	}

	rebuilt.RecordCost(0.30)
	exceeded, reason := rebuilt.CheckLimits()
	if !exceeded {
		t.Fatal("$5.10 spent against a $5.00 cap did not trip the cap — the restored total is not being counted")
	}
	if reason != "max cost exceeded" {
		t.Errorf("tripped with reason %q, want %q", reason, "max cost exceeded")
	}
}

// TestRestoringACapWithoutItsSpendHandsTheBudgetBack states the failure the
// second half of State exists to prevent, by showing what the caps-only version
// of this fix would have done. It is the assertion that a green test could most
// easily have encoded backwards: a session resumed with its cap restored and its
// spend forgotten passes any test that only asks whether the cap survived.
func TestRestoringACapWithoutItsSpendHandsTheBudgetBack(t *testing.T) {
	capsOnly := New(Config{Mode: Autonomous})
	capsOnly.RestoreState(State{MaxCost: 5.00}) // the spend deliberately dropped

	capsOnly.RecordCost(4.90)
	if exceeded, _ := capsOnly.CheckLimits(); exceeded {
		t.Fatal("test setup is wrong: $4.90 should not trip a $5.00 cap")
	}

	// The same session, restored whole, is already over.
	whole := New(Config{Mode: Autonomous})
	whole.RestoreState(State{MaxCost: 5.00, Cost: 4.80})
	whole.RecordCost(4.90)
	if exceeded, _ := whole.CheckLimits(); !exceeded {
		t.Error("a fully restored guard did not trip at $9.70 against a $5.00 cap")
	}
}

// TestResumeStateTakesTheTotalsFromTheRecord — a rebuild cannot know them, so
// there is nothing to weigh the record against.
func TestResumeStateTakesTheTotalsFromTheRecord(t *testing.T) {
	recorded := State{MaxCost: 5.00, Turns: 12, InputTokens: 400_000, Cost: 4.80}

	resumed := ResumeState(recorded, State{MaxCost: 9.00})

	if resumed.Turns != 12 || resumed.InputTokens != 400_000 || resumed.Cost != 4.80 {
		t.Errorf("totals came back as %+v, want 12 turns, 400000 input tokens, $4.80", resumed)
	}
}

// TestResumeStateFillsACapTheRebuildDoesNotHave is the defect this fix closes.
// The rebuild path passes a zero RunRequest, so it configures no caps at all,
// and every cap has to come from the record or the session comes back unlimited.
func TestResumeStateFillsACapTheRebuildDoesNotHave(t *testing.T) {
	recorded := State{MaxTurns: 7, MaxInputTokens: 90_000, MaxCost: 5.00}

	resumed := ResumeState(recorded, State{})

	if resumed.MaxTurns != 7 || resumed.MaxInputTokens != 90_000 || resumed.MaxCost != 5.00 {
		t.Errorf("a rebuild that configured nothing produced %+v, want every cap from the record", resumed)
	}
}

// TestResumeStatePrefersTheCapAskedForNow — a caller who names a limit on this
// request is asking about this run, and outranks the record. Including when the
// new figure is looser: refusing it would make the first cap a session ever had
// permanent, which is a different feature from remembering the last one.
func TestResumeStatePrefersTheCapAskedForNow(t *testing.T) {
	recorded := State{MaxTurns: 7, MaxInputTokens: 90_000, MaxCost: 5.00, Cost: 1.00}

	tighter := ResumeState(recorded, State{MaxCost: 2.00})
	if tighter.MaxCost != 2.00 {
		t.Errorf("MaxCost = %v, want the 2.00 this rebuild asked for", tighter.MaxCost)
	}
	if tighter.MaxTurns != 7 || tighter.MaxInputTokens != 90_000 {
		t.Errorf("naming one cap dropped the others: %+v", tighter)
	}

	looser := ResumeState(recorded, State{MaxCost: 20.00})
	if looser.MaxCost != 20.00 {
		t.Errorf("MaxCost = %v, want the 20.00 this rebuild asked for", looser.MaxCost)
	}
}

// TestResumeStateOnAnEmptyRecordChangesNothing covers a session being created
// rather than rebuilt. It has no record, and the caps it was just configured
// with must survive being merged with nothing.
func TestResumeStateOnAnEmptyRecordChangesNothing(t *testing.T) {
	configured := State{MaxTurns: 7, MaxInputTokens: 90_000, MaxCost: 5.00}

	resumed := ResumeState(State{}, configured)

	if resumed != configured {
		t.Errorf("a fresh session came back as %+v, want its own configuration %+v", resumed, configured)
	}
}

// TestRestoreStateLeavesTheWiringAlone. The repetition threshold and the
// approval hook describe how the guard was built for this run rather than how
// far the session has got, so a restore must not touch them.
//
// This test used to make the same claim about the mode, and a record that named
// no mode is still held to it: the mode a rebuild was configured with survives.
// What changed is the case the old claim got backwards. It reasoned that
// restoring a recorded Observe onto an Autonomous rebuild "would silently
// change what tools are legal" — but a rebuild is Autonomous because nobody
// said otherwise, not because anybody asked, so leaving the record out is what
// silently changes what tools are legal, in the direction that opens the
// session up. See TestARecordedModeSurvivesARebuildThatNamesNone.
func TestRestoreStateLeavesTheWiringAlone(t *testing.T) {
	g := New(Config{Mode: Observe, RepetitionThreshold: 9})

	if err := g.RestoreState(State{MaxCost: 5.00, Cost: 1.00}); err != nil {
		t.Fatalf("RestoreState: %v", err)
	}

	if g.CheckTool("shell_commands", "") != Denied {
		t.Error("mode changed: a record that named no mode dropped the guard out of Observe")
	}
	for i := 0; i < 8; i++ {
		g.RecordToolCall("ripgrep", "same", "")
	}
	if g.IsRepeating() {
		t.Error("repetition threshold changed: 8 identical calls tripped a threshold of 9")
	}
}

// TestARecordedModeSurvivesARebuildThatNamesNone is the whole reason the mode
// is persisted. handleBridgeResume rebuilds a session with a zero RunRequest,
// so a session created to observe is rebuilt with no mode named at all, and the
// mode nobody names is the one that allows `bash -c`.
func TestARecordedModeSurvivesARebuildThatNamesNone(t *testing.T) {
	recorded := State{Mode: Observe.String(), MaxCost: 5.00, Cost: 1.00}

	rebuilt := New(Config{}) // a resume names nothing: Unset, which allows everything
	if err := rebuilt.RestoreState(ResumeState(recorded, rebuilt.State())); err != nil {
		t.Fatalf("RestoreState: %v", err)
	}

	if verdict := rebuilt.CheckTool("shell_commands", "{}"); verdict != Denied {
		t.Errorf("a resumed observe session answers %v to shell_commands, want Denied — the session came back able to run the shell it was created not to run", verdict)
	}
}

// TestARebuildThatNamesAModeOutranksTheRecord is the complement. The record is
// what was said last time; a mode named on this rebuild is somebody asking now,
// and asking now wins — otherwise a session could never be let out of Observe.
func TestARebuildThatNamesAModeOutranksTheRecord(t *testing.T) {
	recorded := State{Mode: Observe.String()}

	rebuilt := New(Config{Mode: Autonomous})
	if err := rebuilt.RestoreState(ResumeState(recorded, rebuilt.State())); err != nil {
		t.Fatalf("RestoreState: %v", err)
	}

	if verdict := rebuilt.CheckTool("shell_commands", "{}"); verdict != Allowed {
		t.Errorf("a rebuild that asked for autonomous answers %v to shell_commands, want Allowed", verdict)
	}
}

// TestAnUnreadableRecordedModeRestoresObserve pins the corrupt-record case. The
// record says a mode was in force and cannot say which, so the restore reports
// the problem and holds the session to the mode that can grant nothing extra.
func TestAnUnreadableRecordedModeRestoresObserve(t *testing.T) {
	g := New(Config{Mode: Autonomous})

	err := g.RestoreState(State{Mode: "read-only"})

	if err == nil {
		t.Fatal("RestoreState accepted an unreadable mode silently")
	}
	if verdict := g.CheckTool("shell_commands", "{}"); verdict != Denied {
		t.Errorf("after an unreadable mode the guard answers %v to shell_commands, want Denied", verdict)
	}
}
