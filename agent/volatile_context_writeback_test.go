package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// deepCopyMessages is forkSession's copy, verbatim: marshal the parent's
// messages and unmarshal them into the child's.
func deepCopyMessages(t *testing.T, messages []anthropic.MessageParam) []anthropic.MessageParam {
	t.Helper()
	data, err := json.Marshal(messages)
	if err != nil {
		t.Fatalf("marshal parent messages: %v", err)
	}
	var copied []anthropic.MessageParam
	if err := json.Unmarshal(data, &copied); err != nil {
		t.Fatalf("unmarshal into child: %v", err)
	}
	return copied
}

// The volatile context is not request-local: buildRequest writes it into the
// caller's message slice, and the caller is the engine passing &e.Messages.
//
// That is what makes it a fork question. A block that only ever reached the
// wire would describe the moment it was sent and then be gone; this one is
// appended to the conversation, persisted with it, and deep-copied into every
// child forkSession makes. Whatever the block said about the fleet, the
// server's other sessions and the files recently touched is then read by the
// child as if it were about the child's own moment, forever, because nothing
// rewrites an inherited block and only the orchestrator agent has injectors to
// produce a newer one.
//
// Pinned here rather than left as a source reading because the write-back is
// one line (`(*messages)[lastIdx].Content = newContent`) inside a function
// whose name says it builds a request.
func TestVolatileContextIsWrittenIntoTheCallersConversation(t *testing.T) {
	messages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("what is the state of the fleet?")),
	}

	agent := New(nil, "system")
	agent.VolatileContext = "[Context]\n# Agent Fleet\n- **brigid** (brigid) — kayushkin [working: \"the task of this moment\"]"

	agent.buildRequest(context.Background(), "model", &messages, agent.prepareTools(), false)

	var got []string
	for _, block := range messages[0].Content {
		if block.OfText != nil {
			got = append(got, block.OfText.Text)
		}
	}
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "the task of this moment") {
		t.Fatalf("the volatile context did not survive in the caller's slice, so it would not cross a fork: %q", joined)
	}
	if len(got) != 2 {
		t.Fatalf("want the original text block and the injected one, got %d: %q", len(got), joined)
	}
}

// The same write-back seen from the fork's side: a child that copies its
// parent's messages copies the parent's injected blocks with them.
//
// forkSession's copy is a json.Marshal/Unmarshal round trip of
// parent.Engine.Messages, so it is verbatim by construction — this asserts the
// part that is not obvious, that there is something usage-shaped in there to
// copy after a single turn has been built.
func TestAForkedTranscriptCarriesTheParentsInjectedContext(t *testing.T) {
	parentMessages := []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("parent turn")),
	}

	parent := New(nil, "system")
	parent.VolatileContext = "[Context]\n# Server Sessions\n\n- **claxon** [running] — agent:claxon:main, 412 msgs, last active 2s ago"
	parent.buildRequest(context.Background(), "model", &parentMessages, parent.prepareTools(), false)

	// What forkSession hands the child.
	childMessages := deepCopyMessages(t, parentMessages)

	var carried bool
	for _, message := range childMessages {
		for _, block := range message.Content {
			if block.OfText != nil && strings.Contains(block.OfText.Text, "412 msgs") {
				carried = true
			}
		}
	}
	if !carried {
		t.Fatal("the parent's session listing did not reach the child, so this audit's finding would be stale")
	}
}
