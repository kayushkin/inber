package mcp

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

// A tool that returns an image puts the whole base64 picture on one line, and
// bufio's default line limit is 64 KiB. Past it the reader stopped, and every
// call after that failed. A reply of a few hundred KiB has to arrive whole.
func TestAReplyLongerThan64KiBArrivesWhole(t *testing.T) {
	server := newTestServer(t)
	server.client.tools["read_it"] = ToolInfo{Name: "read_it"}
	long := strings.Repeat("x", 300*1024)

	type outcome struct {
		out string
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		out, err := server.client.CallTool(context.Background(), "read_it", `{}`)
		done <- outcome{out, err}
	}()
	request := server.nextRequest(t)
	text, _ := json.Marshal(long)
	server.reply(t, request.ID, `{"content":[{"type":"text","text":`+string(text)+`}]}`)

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("call failed: %v", got.err)
		}
		if len(got.out) != len(long) {
			t.Fatalf("reply is %d bytes, want %d", len(got.out), len(long))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the call never returned")
	}
}

// Nothing read a server's stderr, so a server that wrote more than a pipe
// holds blocked and stopped answering. The fixture's stderr is an unbuffered
// pipe, so without a reader the write below never returns.
func TestAServerWritingToStderrIsNotBlocked(t *testing.T) {
	requestReader, requestWriter := io.Pipe()
	responseReader, responseWriter := io.Pipe()
	stderrReader, stderrWriter := io.Pipe()
	client := newClientOverStreams(requestWriter, responseReader, stderrReader)
	t.Cleanup(func() {
		stderrWriter.Close()
		responseWriter.Close()
		requestReader.Close()
		client.Close()
	})

	written := make(chan error, 1)
	go func() {
		_, err := io.WriteString(stderrWriter, strings.Repeat("warning: something noisy\n", 4000))
		written <- err
	}()
	select {
	case err := <-written:
		if err != nil {
			t.Fatalf("write to stderr: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the server's stderr write blocked: nothing reads it")
	}
}
