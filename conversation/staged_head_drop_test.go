package conversation

import "testing"

func TestShiftAfterHeadDropMovesTheBoundaryByTheNumberDropped(t *testing.T) {
	staged := NewStagedConversation(5)
	staged.FrozenIdx = 40

	staged.ShiftAfterHeadDrop(14)

	if staged.FrozenIdx != 26 {
		t.Errorf("FrozenIdx = %d after dropping 14 from 40, want 26", staged.FrozenIdx)
	}
}

func TestShiftAfterHeadDropCollapsesABoundaryThatFellOffTheFront(t *testing.T) {
	staged := NewStagedConversation(5)
	staged.FrozenIdx = 6

	staged.ShiftAfterHeadDrop(20)

	// Not -14: a negative boundary would make StagingSlice and ManageStaging index
	// out of range, and "nothing is frozen" is the honest answer once every frozen
	// message has been dropped.
	if staged.FrozenIdx != 0 {
		t.Errorf("FrozenIdx = %d after dropping past the boundary, want 0", staged.FrozenIdx)
	}
}

func TestShiftAfterHeadDropOfNothingLeavesTheBoundaryAlone(t *testing.T) {
	staged := NewStagedConversation(5)
	staged.FrozenIdx = 9

	staged.ShiftAfterHeadDrop(0)

	if staged.FrozenIdx != 9 {
		t.Errorf("FrozenIdx = %d with nothing dropped, want 9", staged.FrozenIdx)
	}
}
