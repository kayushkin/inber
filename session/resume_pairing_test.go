package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// The defect these tests pin: a session JSONL is an append-only record of what
// happened, and a process that exits while a tool call is in flight leaves a
// tool_call entry with nothing after it. Reconstructing that record block for
// block produces an assistant message holding a tool_use no tool_result
// answers, and the Anthropic API refuses any request shaped that way — so the
// transcript loads and the very first send fails.
//
// Measured on this host when the todo was filed: of 23 session directories, 9
// carry only a session.jsonl (the rest also carry a messages.json, which
// LoadMessagesFromDir prefers), and 4 hold a tool_call with no tool_result. In
// all 4 the unpaired entry is the last line in the file and the tool is
// spawn_agent — an interrupted spawn, which is what writeInterruptedSpawn
// below reproduces.

// writeSessionLog writes JSONL lines to a temp file and returns its path.
func writeSessionLog(t *testing.T, lines ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "session.jsonl")
	var body string
	for _, line := range lines {
		body += line + "\n"
	}
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("write session log: %v", err)
	}
	return path
}

// unansweredToolUseIDs returns the ID of every tool_use block that no later
// tool_result answers. This is the condition the API enforces, stated once.
func unansweredToolUseIDs(messages []anthropic.MessageParam) []string {
	answered := map[string]bool{}
	for _, message := range messages {
		for _, block := range message.Content {
			if block.OfToolResult != nil {
				answered[block.OfToolResult.ToolUseID] = true
			}
		}
	}
	var unanswered []string
	for _, message := range messages {
		for _, block := range message.Content {
			if block.OfToolUse != nil && !answered[block.OfToolUse.ID] {
				unanswered = append(unanswered, block.OfToolUse.ID)
			}
		}
	}
	return unanswered
}

func countToolResults(messages []anthropic.MessageParam) int {
	total := 0
	for _, message := range messages {
		for _, block := range message.Content {
			if block.OfToolResult != nil {
				total++
			}
		}
	}
	return total
}

const (
	userTurn        = `{"ts":"2026-03-04T19:05:12Z","role":"user","content":"split this up"}`
	assistantTurn   = `{"ts":"2026-03-04T19:05:14Z","role":"assistant","content":"Spawning a sub-agent."}`
	spawnCall       = `{"ts":"2026-03-04T19:05:15Z","role":"tool_call","tool_name":"spawn_agent","tool_id":"toolu_01spawn","tool_input":{"agent":"brigid","task":"index the repo"}}`
	spawnResult     = `{"ts":"2026-03-04T19:06:02Z","role":"tool_result","tool_name":"spawn_agent","tool_id":"toolu_01spawn","content":"done"}`
	followUpTurn    = `{"ts":"2026-03-04T19:06:03Z","role":"assistant","content":"That is indexed."}`
	readCall        = `{"ts":"2026-03-04T19:05:20Z","role":"tool_call","tool_name":"read_files","tool_id":"toolu_02read","tool_input":{"paths":["README.md"]}}`
	readResult      = `{"ts":"2026-03-04T19:05:21Z","role":"tool_result","tool_name":"read_files","tool_id":"toolu_02read","content":"# inber"}`
	laterUserTurn   = `{"ts":"2026-03-04T19:07:00Z","role":"user","content":"carry on"}`
	laterAssistant  = `{"ts":"2026-03-04T19:07:01Z","role":"assistant","content":"Carrying on."}`
	interruptedText = "[session interrupted — tool call was not completed]"
)

// TestAnInterruptedSpawnLoadsAsAConversationTheAPIWillAccept is the shape all 4
// unpaired sessions on this host have: the log ends on the tool_call.
func TestAnInterruptedSpawnLoadsAsAConversationTheAPIWillAccept(t *testing.T) {
	path := writeSessionLog(t, userTurn, assistantTurn, spawnCall)

	messages, err := LoadMessages(path)
	if err != nil {
		t.Fatalf("LoadMessages: %v", err)
	}

	if unanswered := unansweredToolUseIDs(messages); len(unanswered) != 0 {
		t.Fatalf("the API would refuse this request: tool_use %v has no tool_result", unanswered)
	}
	if got := countToolResults(messages); got != 1 {
		t.Fatalf("want exactly one synthesised result for the one interrupted call, got %d", got)
	}
}

// TestTheSynthesisedResultSaysTheCallWasInterrupted pins what the model is told,
// which is the half that decides whether it retries the call or invents an
// outcome for it. The wording is conversation.RepairDanglingToolUse's, not a
// second copy of it — that this test reads the same string is the point.
func TestTheSynthesisedResultSaysTheCallWasInterrupted(t *testing.T) {
	path := writeSessionLog(t, userTurn, assistantTurn, spawnCall)

	messages, err := LoadMessages(path)
	if err != nil {
		t.Fatalf("LoadMessages: %v", err)
	}

	var found bool
	for _, message := range messages {
		for _, block := range message.Content {
			result := block.OfToolResult
			if result == nil || result.ToolUseID != "toolu_01spawn" {
				continue
			}
			found = true
			if !result.IsError.Valid() || !result.IsError.Value {
				t.Error("the synthesised result must be an error — the call did not succeed")
			}
			if len(result.Content) == 0 || result.Content[0].OfText == nil ||
				result.Content[0].OfText.Text != interruptedText {
				t.Errorf("result text = %#v, want %q", result.Content, interruptedText)
			}
		}
	}
	if !found {
		t.Fatal("no tool_result was synthesised for the interrupted spawn")
	}
}

// TestAnUnpairedCallMidConversationIsAnsweredToo covers the case the corpus does
// not hold today: a call that went unanswered with the conversation carrying on
// past it. Whether that is an interruption or a logging bug, the reconstruction
// still has to hand back something the API will take.
func TestAnUnpairedCallMidConversationIsAnsweredToo(t *testing.T) {
	path := writeSessionLog(t, userTurn, assistantTurn, spawnCall, laterUserTurn, laterAssistant)

	messages, err := LoadMessages(path)
	if err != nil {
		t.Fatalf("LoadMessages: %v", err)
	}

	if unanswered := unansweredToolUseIDs(messages); len(unanswered) != 0 {
		t.Fatalf("the API would refuse this request: tool_use %v has no tool_result", unanswered)
	}
}

// TestOnlyTheUnansweredCallGetsASynthesisedResult is the other half, and the one
// that fails if the repair is applied indiscriminately: a session that ran to
// completion must come back exactly as it was recorded.
func TestOnlyTheUnansweredCallGetsASynthesisedResult(t *testing.T) {
	path := writeSessionLog(t, userTurn, assistantTurn, spawnCall, spawnResult, followUpTurn)

	messages, err := LoadMessages(path)
	if err != nil {
		t.Fatalf("LoadMessages: %v", err)
	}

	if got := countToolResults(messages); got != 1 {
		t.Fatalf("want the one real result and nothing invented, got %d results", got)
	}
	for _, message := range messages {
		for _, block := range message.Content {
			if block.OfToolResult == nil || len(block.OfToolResult.Content) == 0 {
				continue
			}
			if text := block.OfToolResult.Content[0].OfText; text != nil && text.Text == interruptedText {
				t.Error("a completed call was reported to the model as interrupted")
			}
		}
	}
}

// TestOneOfTwoCallsInATurnIsAnswered covers a turn that issued two calls and
// only logged one result — the partial interruption, which is answered by
// RepairDanglingToolUse's second pass rather than its first.
func TestOneOfTwoCallsInATurnIsAnswered(t *testing.T) {
	path := writeSessionLog(t, userTurn, assistantTurn, spawnCall, readCall, readResult)

	messages, err := LoadMessages(path)
	if err != nil {
		t.Fatalf("LoadMessages: %v", err)
	}

	if unanswered := unansweredToolUseIDs(messages); len(unanswered) != 0 {
		t.Fatalf("the API would refuse this request: tool_use %v has no tool_result", unanswered)
	}
	if got := countToolResults(messages); got != 2 {
		t.Fatalf("want the real result plus one synthesised, got %d", got)
	}
}

// TestLoadMessagesFromDirPrefersTheSnapshot pins the reason only 9 of this
// host's 23 session directories reach the reconstruction at all.
func TestLoadMessagesFromDirPrefersTheSnapshot(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "session.jsonl"),
		[]byte(userTurn+"\n"+assistantTurn+"\n"+spawnCall+"\n"), 0644); err != nil {
		t.Fatalf("write session log: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "messages.json"),
		[]byte(`[{"role":"user","content":[{"type":"text","text":"from the snapshot"}]}]`), 0644); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}

	messages, err := LoadMessagesFromDir(dir)
	if err != nil {
		t.Fatalf("LoadMessagesFromDir: %v", err)
	}
	if len(messages) != 1 || len(messages[0].Content) != 1 ||
		messages[0].Content[0].OfText == nil ||
		messages[0].Content[0].OfText.Text != "from the snapshot" {
		t.Fatalf("the snapshot should win over the JSONL, got %+v", messages)
	}
}
