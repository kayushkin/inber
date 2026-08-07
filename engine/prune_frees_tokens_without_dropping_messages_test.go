package engine

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/conversation"
)

// The fact these tests pin, and the reason BeforeRequest reports freed tokens
// at all: the pruner frees most of a conversation while handing back a slice of
// exactly the same length. Both of Agent's call sites used to ask "did the
// slice get shorter", so in the band between the point where pruning starts
// (KeepRecentTurns+1 messages) and the point where the head-drop starts
// (KeepRecentTurns*2+1), every freed token was thrown away.

// conversationOfHeavyToolResults builds `turns` assistant(tool_use) /
// user(tool_result) pairs whose results are large enough to be worth pruning.
// This is the shape the measurement in noteboard `0db2e05c` used: read_files
// output at roughly 20KB a result.
func conversationOfHeavyToolResults(turns int) []anthropic.MessageParam {
	const resultBytes = 20 * 1024
	messages := make([]anthropic.MessageParam, 0, turns*2)
	for turn := 0; turn < turns; turn++ {
		id := fmt.Sprintf("toolu_%d", turn)
		messages = append(messages, anthropic.NewAssistantMessage(
			anthropic.NewToolUseBlock(id, map[string]string{"path": fmt.Sprintf("file%d.go", turn)}, "read_files"),
		))
		body := strings.Repeat(fmt.Sprintf("line of file%d.go\n", turn), resultBytes/20)
		messages = append(messages, anthropic.NewUserMessage(
			anthropic.NewToolResultBlock(id, body, false),
		))
	}
	return messages
}

// TestThePrunerFreesTokensWithoutChangingTheMessageCount is the control for the
// whole change. If this ever goes red because the pruner started dropping
// messages, the length test the call sites used would have been adequate after
// all and pruneDidSomething's freed-token arm would be dead weight.
func TestThePrunerFreesTokensWithoutChangingTheMessageCount(t *testing.T) {
	pruneConfig := conversation.DefaultPruneConfig()

	// Inside the band: past KeepRecentTurns, short of the KeepRecentTurns*2
	// head-drop. Nothing here can change the message count.
	messages := conversationOfHeavyToolResults(pruneConfig.KeepRecentTurns/2 + 1)
	if len(messages) <= pruneConfig.KeepRecentTurns {
		t.Fatalf("test rig: %d messages does not reach the pruning threshold of %d",
			len(messages), pruneConfig.KeepRecentTurns)
	}
	if len(messages) > pruneConfig.KeepRecentTurns*2 {
		t.Fatalf("test rig: %d messages reaches the head-drop threshold of %d, so this "+
			"test would pass on the message count and prove nothing",
			len(messages), pruneConfig.KeepRecentTurns*2)
	}

	engine := &Engine{staged: conversation.NewStagedConversation(pruneConfig.ManageInterval)}
	testAgent := &agent.Agent{}
	engine.configureContextPruning(testAgent)

	before := conversation.EstimateTokens(messages)
	pruned, tokensFreed := testAgent.BeforeRequest(context.Background(), messages, 200000)

	if len(pruned) != len(messages) {
		t.Fatalf("pruner returned %d messages from %d — this test is about the band where "+
			"the count CANNOT change; if that changed, re-read pruneDidSomething",
			len(pruned), len(messages))
	}
	if tokensFreed <= 0 {
		t.Fatalf("pruner freed %d tokens from a %d-token conversation of 20KB tool results — "+
			"this is the defect: same length, nothing reported, caller discards it",
			tokensFreed, before)
	}

	// The freed count has to describe the messages actually handed back, not a
	// number the pruner made up. Otherwise the gate opens on a lie.
	after := conversation.EstimateTokens(pruned)
	if after >= before {
		t.Fatalf("pruner reported %d tokens freed but the conversation went from %d to %d tokens",
			tokensFreed, before, after)
	}
}

// TestALongToolOnlyRunNeverChangesTheMessageCountHoweverLongItGets widens the
// case above, and it is the sharpest form of the defect.
//
// The head-drop is supposed to be the escape hatch that eventually makes the
// message count move. It is not, for the conversations that most need pruning:
// it refuses to cut mid-turn, and StartsUserTurn reports false for a user
// message carrying nothing but tool_result blocks — correctly, since answering
// a tool call continues the assistant's turn rather than starting a new one. So
// a long uninterrupted tool loop offers the boundary walk nowhere to cut, the
// count never changes at any length, and the old length gate discarded every
// freed token no matter how big the conversation got. The band had no ceiling.
func TestALongToolOnlyRunNeverChangesTheMessageCountHoweverLongItGets(t *testing.T) {
	pruneConfig := conversation.DefaultPruneConfig()

	// Comfortably PAST the head-drop threshold, where the count was supposed to
	// start moving.
	messages := conversationOfHeavyToolResults(pruneConfig.KeepRecentTurns + 1)
	if len(messages) <= pruneConfig.KeepRecentTurns*2 {
		t.Fatalf("test rig: %d messages does not clear the head-drop threshold of %d",
			len(messages), pruneConfig.KeepRecentTurns*2)
	}

	engine := &Engine{staged: conversation.NewStagedConversation(pruneConfig.ManageInterval)}
	testAgent := &agent.Agent{}
	engine.configureContextPruning(testAgent)

	pruned, tokensFreed := testAgent.BeforeRequest(context.Background(), messages, 200000)

	if len(pruned) != len(messages) {
		t.Fatalf("the head drop cut %d messages out of a pure tool loop — it is not supposed "+
			"to find a turn boundary here; re-read StartsUserTurn before trusting this test",
			len(messages)-len(pruned))
	}
	if tokensFreed <= 0 {
		t.Fatalf("freed %d tokens from %d messages of 20KB tool results", tokensFreed, len(messages))
	}
}

// TestAHeadDropIsReportedAsFreedTokensToo covers the other arm. The head-drop
// is the one thing that shortens the slice, and it is not part of
// PruneConversation's own TokensFreed, so it has to be added or a caller
// switching to the freed count would lose the case the old length test caught.
func TestAHeadDropIsReportedAsFreedTokensToo(t *testing.T) {
	pruneConfig := conversation.DefaultPruneConfig()
	messages := conversationOfAlternatingTurns(pruneConfig.KeepRecentTurns + 20)

	engine := &Engine{staged: conversation.NewStagedConversation(pruneConfig.ManageInterval)}
	testAgent := &agent.Agent{}
	engine.configureContextPruning(testAgent)

	// A huge context window keeps the token branch out of it, so the only thing
	// that can free anything here is the message-count head drop.
	shortened, tokensFreed := testAgent.BeforeRequest(context.Background(), messages, 1<<24)

	if len(shortened) >= len(messages) {
		t.Fatalf("expected a head drop from %d messages, got %d", len(messages), len(shortened))
	}
	if tokensFreed <= 0 {
		t.Fatalf("dropped %d messages off the head and reported %d tokens freed",
			len(messages)-len(shortened), tokensFreed)
	}
}

// TestAConversationWithNothingToFreeReportsNothingFreed is the negative
// control. Without it, a BeforeRequest that returned a fixed positive number
// would satisfy every test above while telling the caller nothing.
func TestAConversationWithNothingToFreeReportsNothingFreed(t *testing.T) {
	pruneConfig := conversation.DefaultPruneConfig()
	// Short and light: under the pruning threshold and nowhere near the head drop.
	messages := conversationOfAlternatingTurns(pruneConfig.KeepRecentTurns / 4)

	engine := &Engine{staged: conversation.NewStagedConversation(pruneConfig.ManageInterval)}
	testAgent := &agent.Agent{}
	engine.configureContextPruning(testAgent)

	unchanged, tokensFreed := testAgent.BeforeRequest(context.Background(), messages, 200000)

	if len(unchanged) != len(messages) {
		t.Fatalf("expected no drop from %d messages, got %d", len(messages), len(unchanged))
	}
	if tokensFreed != 0 {
		t.Fatalf("reported %d tokens freed from a conversation nothing touched", tokensFreed)
	}
}
