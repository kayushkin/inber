package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// testServer is a fake MCP server wired to a client over in-memory pipes. It
// hands every request it reads to the test and writes back whatever the test
// tells it to, in whatever order the test chooses.
type testServer struct {
	client   *Client
	requests chan MCPRequest

	toClient   *io.PipeWriter
	fromClient *io.PipeReader

	closeOnce sync.Once
}

func newTestServer(t *testing.T) *testServer {
	t.Helper()

	requestReader, requestWriter := io.Pipe()
	responseReader, responseWriter := io.Pipe()
	stderrReader, stderrWriter := io.Pipe()
	t.Cleanup(func() { stderrWriter.Close() })

	server := &testServer{
		requests:   make(chan MCPRequest, 16),
		toClient:   responseWriter,
		fromClient: requestReader,
	}
	server.client = newClientOverStreams(requestWriter, responseReader, stderrReader)

	go func() {
		defer close(server.requests)
		scanner := bufio.NewScanner(requestReader)
		for scanner.Scan() {
			var request MCPRequest
			if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
				continue
			}
			server.requests <- request
		}
	}()

	t.Cleanup(server.Close)
	return server
}

// nextRequest returns the next request the client sent, failing the test if
// none arrives.
func (s *testServer) nextRequest(t *testing.T) MCPRequest {
	t.Helper()
	select {
	case request, ok := <-s.requests:
		if !ok {
			t.Fatal("client closed its request stream before sending a request")
		}
		return request
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the client to send a request")
		return MCPRequest{}
	}
}

// reply writes a successful JSON-RPC response for the given request id.
func (s *testServer) reply(t *testing.T, id string, result string) {
	t.Helper()
	s.writeLine(t, fmt.Sprintf(`{"jsonrpc":"2.0","id":%q,"result":%s}`, id, result))
}

func (s *testServer) writeLine(t *testing.T, line string) {
	t.Helper()
	if _, err := io.WriteString(s.toClient, line+"\n"); err != nil {
		t.Fatalf("failed to write %q to the client: %v", line, err)
	}
}

// endOutput closes the server's output, as a server that exits would.
func (s *testServer) endOutput() {
	s.toClient.Close()
}

func (s *testServer) Close() {
	s.closeOnce.Do(func() {
		s.toClient.Close()
		s.fromClient.Close()
		s.client.Close()
	})
}

// waiterCount reports how many calls the client still has outstanding.
func waiterCount(client *Client) int {
	client.mu.Lock()
	defer client.mu.Unlock()
	return len(client.waiters)
}

// TestConcurrentCallsEachGetTheirOwnReply is the defect this transport was
// rewritten for: two calls in flight on one client, answered out of order.
// Reading the stream inside a call let each one consume the other's reply.
func TestConcurrentCallsEachGetTheirOwnReply(t *testing.T) {
	server := newTestServer(t)

	type outcome struct {
		asked  string
		result json.RawMessage
		err    error
	}
	results := make(chan outcome, 2)
	for _, method := range []string{"alpha", "beta"} {
		go func(method string) {
			result, err := server.client.call(context.Background(), method, nil)
			results <- outcome{asked: method, result: result, err: err}
		}(method)
	}

	// Answer in the reverse of the order the requests arrived, so a client that
	// matched replies by arrival rather than by id would swap them.
	first := server.nextRequest(t)
	second := server.nextRequest(t)
	server.reply(t, second.ID, fmt.Sprintf(`{"method":%q}`, second.Method))
	server.reply(t, first.ID, fmt.Sprintf(`{"method":%q}`, first.Method))

	answered := map[string]bool{}
	for i := 0; i < 2; i++ {
		select {
		case got := <-results:
			if got.err != nil {
				t.Fatalf("call failed: %v", got.err)
			}
			var payload struct {
				Method string `json:"method"`
			}
			if err := json.Unmarshal(got.result, &payload); err != nil {
				t.Fatalf("failed to parse result %s: %v", got.result, err)
			}
			if payload.Method != got.asked {
				t.Errorf("call %q was answered with the reply to %q", got.asked, payload.Method)
			}
			answered[got.asked] = true
		case <-time.After(2 * time.Second):
			t.Fatal("a call never received its reply")
		}
	}

	for _, method := range []string{"alpha", "beta"} {
		if !answered[method] {
			t.Errorf("call %q did not get the reply to its own request", method)
		}
	}
	if count := waiterCount(server.client); count != 0 {
		t.Errorf("expected no outstanding waiters after both calls returned, got %d", count)
	}
}

// TestCallIgnoresTrafficItDidNotAskFor pins that unrelated output cannot answer
// or starve a call: log lines, replies to ids nobody is waiting on, and
// server-initiated requests all pass by until the real reply arrives.
func TestCallIgnoresTrafficItDidNotAskFor(t *testing.T) {
	server := newTestServer(t)

	result := make(chan json.RawMessage, 1)
	failed := make(chan error, 1)
	go func() {
		payload, err := server.client.call(context.Background(), "tools/call", nil)
		if err != nil {
			failed <- err
			return
		}
		result <- payload
	}()

	request := server.nextRequest(t)
	server.writeLine(t, "starting up, listening on stdio")
	server.writeLine(t, `{"jsonrpc":"2.0","id":"999","result":{"method":"someone else's call"}}`)
	// A request of the server's own, carrying the same id as our pending call:
	// the two directions number their requests independently, so this collision
	// is ordinary rather than hostile.
	server.writeLine(t, fmt.Sprintf(`{"jsonrpc":"2.0","id":%q,"method":"roots/list"}`, request.ID))
	server.writeLine(t, `{"jsonrpc":"2.0","method":"notifications/progress","params":{}}`)
	server.reply(t, request.ID, `{"method":"mine"}`)

	select {
	case payload := <-result:
		var got struct {
			Method string `json:"method"`
		}
		if err := json.Unmarshal(payload, &got); err != nil {
			t.Fatalf("failed to parse result %s: %v", payload, err)
		}
		if got.Method != "mine" {
			t.Errorf("call was answered with another message: %s", payload)
		}
	case err := <-failed:
		t.Fatalf("call failed: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("call never received its reply")
	}
}

// TestCallStopsAtItsDeadlineWhileTheServerIsSilent pins that the deadline
// bounds the operation. Reading inside the call left it blocked in a read that
// a silent server never returns from, so the timeout could not fire at all.
func TestCallStopsAtItsDeadlineWhileTheServerIsSilent(t *testing.T) {
	server := newTestServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	started := time.Now()
	_, err := server.client.call(ctx, "tools/call", nil)
	if err == nil {
		t.Fatal("expected the call to fail, got a result")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected a deadline error, got %v", err)
	}
	if waited := time.Since(started); waited > 2*time.Second {
		t.Errorf("call took %s to honour a 150ms deadline", waited)
	}
	if count := waiterCount(server.client); count != 0 {
		t.Errorf("expected the abandoned call to leave no waiter behind, got %d", count)
	}
}

// TestChatterDoesNotPostponeTheDeadline pins the same bound against a server
// that is talkative rather than silent: a stream of lines that are not the
// reply must not keep the call alive past its deadline.
func TestChatterDoesNotPostponeTheDeadline(t *testing.T) {
	server := newTestServer(t)

	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			if _, err := io.WriteString(server.toClient, "still working\n"); err != nil {
				return
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	started := time.Now()
	if _, err := server.client.call(ctx, "tools/call", nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected a deadline error, got %v", err)
	}
	if waited := time.Since(started); waited > 2*time.Second {
		t.Errorf("call took %s to honour a 150ms deadline against a chatty server", waited)
	}
}

// TestCallFailsWhenTheServerOutputEnds pins that a call is released, with an
// error naming its request, when the server stops talking for good.
func TestCallFailsWhenTheServerOutputEnds(t *testing.T) {
	server := newTestServer(t)

	failed := make(chan error, 1)
	go func() {
		_, err := server.client.call(context.Background(), "tools/call", nil)
		failed <- err
	}()

	request := server.nextRequest(t)
	server.endOutput()

	select {
	case err := <-failed:
		if err == nil {
			t.Fatal("expected the call to fail when the server output ended")
		}
		if !strings.Contains(err.Error(), request.ID) {
			t.Errorf("expected the error to name request %s, got %v", request.ID, err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("call was left waiting after the server output ended")
	}
}

// TestLateReplyDoesNotDisturbTheNextCall pins that a reply arriving after its
// call gave up is dropped rather than handed to whoever asks next.
func TestLateReplyDoesNotDisturbTheNextCall(t *testing.T) {
	server := newTestServer(t)

	abandonCtx, cancelAbandoned := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancelAbandoned()
	if _, err := server.client.call(abandonCtx, "abandoned", nil); err == nil {
		t.Fatal("expected the first call to time out")
	}
	abandoned := server.nextRequest(t)

	result := make(chan json.RawMessage, 1)
	failed := make(chan error, 1)
	go func() {
		payload, err := server.client.call(context.Background(), "second", nil)
		if err != nil {
			failed <- err
			return
		}
		result <- payload
	}()

	second := server.nextRequest(t)
	server.reply(t, abandoned.ID, `{"method":"late"}`)
	server.reply(t, second.ID, `{"method":"second"}`)

	select {
	case payload := <-result:
		var got struct {
			Method string `json:"method"`
		}
		if err := json.Unmarshal(payload, &got); err != nil {
			t.Fatalf("failed to parse result %s: %v", payload, err)
		}
		if got.Method != "second" {
			t.Errorf("second call was answered with the abandoned call's reply: %s", payload)
		}
	case err := <-failed:
		t.Fatalf("second call failed: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("second call never received its reply")
	}
}

// TestCloseReleasesAWaitingCall pins that shutting the client down does not
// strand a call that has no deadline of its own.
func TestCloseReleasesAWaitingCall(t *testing.T) {
	server := newTestServer(t)

	failed := make(chan error, 1)
	go func() {
		_, err := server.client.call(context.Background(), "tools/call", nil)
		failed <- err
	}()

	server.nextRequest(t)
	server.Close()

	select {
	case err := <-failed:
		if err == nil {
			t.Fatal("expected the call to fail once the client was closed")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("call was left waiting after Close")
	}
}

// TestErrorReplyReachesItsOwnCall pins that an error reply is routed by id like
// any other, rather than failing whichever call happens to be reading.
func TestErrorReplyReachesItsOwnCall(t *testing.T) {
	server := newTestServer(t)

	failed := make(chan error, 1)
	go func() {
		_, err := server.client.call(context.Background(), "tools/call", nil)
		failed <- err
	}()

	request := server.nextRequest(t)
	server.writeLine(t, fmt.Sprintf(`{"jsonrpc":"2.0","id":%q,"error":{"code":-32601,"message":"no such tool"}}`, request.ID))

	select {
	case err := <-failed:
		if err == nil || !strings.Contains(err.Error(), "no such tool") {
			t.Fatalf("expected the server's error to reach the call, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("call never received the error reply")
	}
}
