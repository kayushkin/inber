package engine

import (
	"context"
	"testing"

	"github.com/kayushkin/inber/agent"
)

// TestTheSecondRequestOfAToolTurnCarriesTheFirstReplyIntact. The loop stores
// each reply as history with ToParam, which the SDK fills from the JSON a
// message was decoded from. The converted reply used to be assembled as Go
// structs, so ToParam blanked it: the stored text was "" and the tool call had
// no id, name or arguments. OpenAI refused the second request of every tool
// turn for the empty function name.
func TestTheSecondRequestOfAToolTurnCarriesTheFirstReplyIntact(t *testing.T) {
	var seen []string
	firstReply := toolCallResponse("call_1", "read_it", `{"path":"a.txt"}`)
	firstReply.Choices[0].Message.Content = "Reading the file first."
	f := newFakeOpenAI(t, firstReply, textResponse("done"))
	e := openAIEngine(t, f, recordingTool("read_it", "contents", &seen))

	if _, err := e.runOpenAITurn(context.Background(), nil); err != nil {
		t.Fatalf("runOpenAITurn: %v", err)
	}
	if len(f.requests) != 2 {
		t.Fatalf("the loop made %d requests, want 2", len(f.requests))
	}

	var assistant *agent.OpenAIMessage
	for i, message := range f.request(1).Messages {
		if message.Role == "assistant" {
			assistant = &f.request(1).Messages[i]
		}
	}
	if assistant == nil {
		t.Fatal("the second request carried no assistant message")
	}
	if text, _ := assistant.Content.(string); text != "Reading the file first." {
		t.Errorf("the stored assistant text is %q, want the reply's text", text)
	}
	if len(assistant.ToolCalls) != 1 {
		t.Fatalf("the stored assistant message has %d tool calls, want 1", len(assistant.ToolCalls))
	}
	call := assistant.ToolCalls[0]
	if call.ID == "" || call.Function.Name != "read_it" || call.Function.Arguments != `{"path":"a.txt"}` {
		t.Errorf("the stored tool call is id=%q name=%q arguments=%q, want the call as the model made it", call.ID, call.Function.Name, call.Function.Arguments)
	}
}

// TestAToolCallWithArgumentsThatAreNotJSONFailsTheTurn. Such a call used to be
// stored with a blank input. Now the turn stops and says why.
func TestAToolCallWithArgumentsThatAreNotJSONFailsTheTurn(t *testing.T) {
	var seen []string
	f := newFakeOpenAI(t, toolCallResponse("call_1", "read_it", `{"path":`))
	e := openAIEngine(t, f, recordingTool("read_it", "contents", &seen))

	if _, err := e.runOpenAITurn(context.Background(), nil); err == nil {
		t.Fatal("runOpenAITurn succeeded on a tool call whose arguments are not JSON")
	}
	if len(seen) != 0 {
		t.Errorf("the tool ran %d times on arguments that are not JSON", len(seen))
	}
}
