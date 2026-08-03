package mcp

import (
	"context"
	"strings"
	"testing"
)

// callToolAgainst registers one tool on the client, issues a CallTool for it,
// and answers with the raw result object the test supplies.
func callToolAgainst(t *testing.T, server *testServer, toolName, result string) (string, error) {
	t.Helper()
	server.client.tools[toolName] = ToolInfo{Name: toolName}

	type outcome struct {
		output string
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		output, err := server.client.CallTool(context.Background(), toolName, `{}`)
		done <- outcome{output, err}
	}()

	request := server.nextRequest(t)
	server.reply(t, request.ID, result)

	got := <-done
	return got.output, got.err
}

// MCP reports a tool that ran and failed with isError on an otherwise
// successful JSON-RPC response. Ignoring the field hands the model the failure
// text as if it were the tool's output, with a nil error — so the turn counts
// it as a success and the model builds on work that did not happen.
func TestAToolThatReportsIsErrorFailsTheCall(t *testing.T) {
	server := newTestServer(t)

	output, err := callToolAgainst(t, server, "write_file", `{"content":[{"type":"text","text":"permission denied: /etc/hosts"}],"isError":true}`)
	if err == nil {
		t.Fatalf("a tool result with isError:true returned no error (output %q)", output)
	}
	// The server's own text is the only description of what went wrong, so it
	// has to survive into the error rather than be replaced by a generic one.
	if !strings.Contains(err.Error(), "permission denied: /etc/hosts") {
		t.Errorf("error drops the server's explanation: %v", err)
	}
	if !strings.Contains(err.Error(), "write_file") {
		t.Errorf("error does not name the tool that failed: %v", err)
	}
}

// The control: an ordinary result must still come back as output with no error,
// or the check above passes against a CallTool that fails everything.
func TestAToolWithoutIsErrorStillReturnsItsOutput(t *testing.T) {
	server := newTestServer(t)

	output, err := callToolAgainst(t, server, "read_file", `{"content":[{"type":"text","text":"file contents"}]}`)
	if err != nil {
		t.Fatalf("an ordinary tool result returned an error: %v", err)
	}
	if output != "file contents" {
		t.Errorf("output = %q, want %q", output, "file contents")
	}
}

// isError:false is a real value the protocol allows, and it means success. A
// check written against presence rather than value would fail this.
func TestIsErrorFalseIsASuccess(t *testing.T) {
	server := newTestServer(t)

	output, err := callToolAgainst(t, server, "list_dir", `{"content":[{"type":"text","text":"a\nb"}],"isError":false}`)
	if err != nil {
		t.Fatalf("isError:false returned an error: %v", err)
	}
	if output != "a\nb" {
		t.Errorf("output = %q, want %q", output, "a\nb")
	}
}

// A failing tool that sends no text must still fail, and must say that the
// server explained nothing rather than reporting an empty reason.
func TestAFailedToolWithNoTextStillFails(t *testing.T) {
	server := newTestServer(t)

	_, err := callToolAgainst(t, server, "deploy", `{"content":[],"isError":true}`)
	if err == nil {
		t.Fatal("a tool result with isError:true and no content returned no error")
	}
	if !strings.Contains(err.Error(), "no text explaining why") {
		t.Errorf("error does not say the server sent no explanation: %v", err)
	}
}
