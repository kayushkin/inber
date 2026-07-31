package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/guard"
	toolstoretools "github.com/kayushkin/tool-store/tools"
)

// The OpenAI-served turn used to dispatch tools in a loop of its own, which
// knew nothing about the "then" chain or the done/note/split sideband. Both are
// fields a model writes into a tool call and that are taken out of the input
// whatever happens next, so on that path a chain and a sideband field went into
// the tool's own arguments, encoding/json dropped them as unknown, and the
// model was told nothing. These tests drive runOpenAITurn against a fake
// OpenAI-compatible server and pin that the two paths now carry the same
// instructions.

// fakeOpenAI serves canned chat completions in order and records every request
// body it was sent.
type fakeOpenAI struct {
	mu        sync.Mutex
	responses []agent.OpenAIResponse
	requests  []agent.OpenAIRequest
	server    *httptest.Server
}

func newFakeOpenAI(t *testing.T, responses ...agent.OpenAIResponse) *fakeOpenAI {
	t.Helper()
	f := &fakeOpenAI{responses: responses}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req agent.OpenAIRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f.mu.Lock()
		f.requests = append(f.requests, req)
		var resp agent.OpenAIResponse
		if len(f.responses) > 0 {
			resp = f.responses[0]
			f.responses = f.responses[1:]
		} else {
			resp = textResponse("no canned response left")
		}
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeOpenAI) request(i int) agent.OpenAIRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[i]
}

func textResponse(text string) agent.OpenAIResponse {
	return agent.OpenAIResponse{
		Choices: []agent.OpenAIChoice{{
			Message:      agent.OpenAIMessage{Role: "assistant", Content: text},
			FinishReason: "stop",
		}},
	}
}

func toolCallResponse(id, name, arguments string) agent.OpenAIResponse {
	return agent.OpenAIResponse{
		Choices: []agent.OpenAIChoice{{
			Message: agent.OpenAIMessage{
				Role: "assistant",
				ToolCalls: []agent.OpenAIToolCall{{
					ID:       id,
					Type:     "function",
					Function: agent.OpenAIFunctionCall{Name: name, Arguments: arguments},
				}},
			},
			FinishReason: "tool_calls",
		}},
	}
}

// recordingTool answers with a fixed output and remembers every input it was
// handed, which is how these tests see whether the injected fields reached the
// tool instead of being taken out for the chain and the sideband.
func recordingTool(name, output string, seen *[]string) agent.Tool {
	return agent.Tool{
		Name:        name,
		Description: name,
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"path": map[string]any{"type": "string"},
			},
		},
		Run: func(ctx context.Context, input string) (string, error) {
			*seen = append(*seen, input)
			return output, nil
		},
	}
}

func openAIEngine(t *testing.T, f *fakeOpenAI, tools ...agent.Tool) *Engine {
	t.Helper()
	return &Engine{
		agentTools:  tools,
		modelClient: &agent.ModelClient{OpenAIClient: agent.NewOpenAIClient(f.server.URL, "test-key", "test-model")},
	}
}

// TestAnOpenAIServedTurnAdvertisesTheChainAndSidebandFields. Advertising the
// fields and honouring them are the same decision: a path that sends the plain
// schemas leaves the model no way to know the chain exists, and one that sends
// them without dispatching through the chain drops what the model writes.
func TestAnOpenAIServedTurnAdvertisesTheChainAndSidebandFields(t *testing.T) {
	var seen []string
	f := newFakeOpenAI(t, textResponse("done"))
	e := openAIEngine(t, f, recordingTool("read_it", "contents", &seen), recordingTool("write_it", "written", &seen))

	if _, err := e.runOpenAITurn(context.Background(), nil); err != nil {
		t.Fatalf("runOpenAITurn: %v", err)
	}

	req := f.request(0)
	if len(req.Tools) != 2 {
		t.Fatalf("sent %d tool schemas, want 2", len(req.Tools))
	}
	for _, tool := range req.Tools {
		props, ok := tool.Function.Parameters["properties"].(map[string]any)
		if !ok {
			t.Fatalf("%s: schema has no properties: %#v", tool.Function.Name, tool.Function.Parameters)
		}
		for _, field := range []string{"then", "done", "note", "split"} {
			if _, present := props[field]; !present {
				t.Errorf("%s: schema does not offer %q, so a model served by this provider cannot know it exists", tool.Function.Name, field)
			}
		}
		then, _ := props["then"].(map[string]any)
		thenProps, _ := then["properties"].(map[string]any)
		toolField, _ := thenProps["tool"].(map[string]any)
		names, _ := toolField["enum"].([]any)
		if len(names) != 2 {
			t.Errorf("%s: the chain's tool enum lists %v, and the turn has 2 tools", tool.Function.Name, names)
		}
	}
}

// TestAnOpenAIServedTurnRunsTheChainAndReportsTheSideband is the defect itself:
// one tool_use block carrying a primary call, a "then" chain and a "done" the
// engine has nothing to apply it to. All three have to be answered for.
func TestAnOpenAIServedTurnRunsTheChainAndReportsTheSideband(t *testing.T) {
	var readInputs, writeInputs []string
	f := newFakeOpenAI(t,
		toolCallResponse("call_1", "read_it", `{"path":"a.txt","then":{"tool":"write_it","input":{"path":"b.txt"}},"done":[0]}`),
		textResponse("finished"),
	)
	e := openAIEngine(t, f,
		recordingTool("read_it", "contents of a", &readInputs),
		recordingTool("write_it", "wrote b", &writeInputs),
	)

	if _, err := e.runOpenAITurn(context.Background(), nil); err != nil {
		t.Fatalf("runOpenAITurn: %v", err)
	}

	if len(writeInputs) != 1 {
		t.Fatalf("the chained call ran %d times, want 1 — the \"then\" field was dropped", len(writeInputs))
	}
	if !strings.Contains(writeInputs[0], "b.txt") {
		t.Errorf("the chained call got %q, not the input the chain named", writeInputs[0])
	}

	if len(readInputs) != 1 {
		t.Fatalf("the primary call ran %d times, want 1", len(readInputs))
	}
	for _, field := range []string{"then", "done"} {
		if strings.Contains(readInputs[0], `"`+field+`"`) {
			t.Errorf("the primary tool was handed the %q field in its own arguments: %s", field, readInputs[0])
		}
	}

	report := lastToolResultText(t, e.Messages)
	if !strings.Contains(report, "--- then(write_it) ---") {
		t.Errorf("the tool result does not mark the chained call:\n%s", report)
	}
	if !strings.Contains(report, "wrote b") {
		t.Errorf("the tool result does not carry the chained call's output:\n%s", report)
	}
	// No repo, so no task list — which the model has to be told, because the
	// field is deleted from its input either way.
	if !strings.Contains(report, "done ignored") {
		t.Errorf("a \"done\" with nothing behind it was swallowed instead of reported:\n%s", report)
	}
}

// TestAnOpenAIServedSidebandReachesTheEnginesOwnCallbacks. Reporting an
// unapplicable field is only half of it: where the engine does have somewhere
// to put a sideband field, the field has to land there. This one goes all the
// way to the scratchpad on disk.
func TestAnOpenAIServedSidebandReachesTheEnginesOwnCallbacks(t *testing.T) {
	repo := t.TempDir()
	var readInputs []string
	f := newFakeOpenAI(t,
		toolCallResponse("call_1", "read_it", `{"path":"a.txt","note":{"key":"layout","value":"engine/ holds the turn loops"}}`),
		textResponse("finished"),
	)
	e := openAIEngine(t, f, recordingTool("read_it", "contents of a", &readInputs))
	e.repoRoot = repo
	e.AgentName = "test-agent"

	if _, err := e.runOpenAITurn(context.Background(), nil); err != nil {
		t.Fatalf("runOpenAITurn: %v", err)
	}

	notes := toolstoretools.LoadScratchpadNotes(repo, "test-agent")
	if notes["layout"] != "engine/ holds the turn loops" {
		t.Errorf("the note the model wrote into the call never reached the scratchpad: %#v", notes)
	}
	if report := lastToolResultText(t, e.Messages); !strings.Contains(report, "noted") {
		t.Errorf("the saved note was not reported back to the model:\n%s", report)
	}
}

// TestAnOpenAIServedChainIsGatedLikeThePrimaryCall. The gate was wired into
// this loop by hand and asked about the primary call only. Once the loop
// carries chains, a chain that skipped the gate would be a way around it.
func TestAnOpenAIServedChainIsGatedLikeThePrimaryCall(t *testing.T) {
	var readInputs, writeInputs []string
	f := newFakeOpenAI(t,
		toolCallResponse("call_1", "read_files", `{"path":"a.txt","then":{"tool":"write_files","input":{"path":"b.txt"}}}`),
		textResponse("finished"),
	)
	e := openAIEngine(t, f,
		recordingTool("read_files", "contents of a", &readInputs),
		recordingTool("write_files", "wrote b", &writeInputs),
	)
	e.Guard = guard.New(guard.Config{Mode: guard.Observe})

	if _, err := e.runOpenAITurn(context.Background(), nil); err != nil {
		t.Fatalf("runOpenAITurn: %v", err)
	}

	if len(writeInputs) != 0 {
		t.Fatalf("observe mode let a chained write run: %v", writeInputs)
	}
	report := lastToolResultText(t, e.Messages)
	if !strings.Contains(report, "refused") || !strings.Contains(report, "write_files") {
		t.Errorf("the refused chain was not reported on the result it rode in on:\n%s", report)
	}
}

// lastToolResultText returns the text of the tool_result blocks in the last
// user message, which is what the model reads back.
func lastToolResultText(t *testing.T, messages []anthropic.MessageParam) string {
	t.Helper()
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != anthropic.MessageParamRoleUser {
			continue
		}
		encoded, err := json.Marshal(messages[i].Content)
		if err != nil {
			t.Fatalf("marshal tool results: %v", err)
		}
		return string(encoded)
	}
	t.Fatal("no user message carrying tool results")
	return ""
}
