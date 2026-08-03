package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/engine"
)

// compact_turn_in_flight_test.go — POST /sessions/{id}/compact is a WRITER of
// the conversation, and these pin that it never writes one a turn is writing.
//
// The distinction the handler used to miss is that compaction is not a reader.
// Every other cross-goroutine toucher of Engine.Messages in this package reads
// it — the session list, the history endpoint, the state persister. Compaction
// replaces it. Two writers on one slice from two goroutines is not a stale
// read, it is a lost write: whichever assignment lands second discards the
// other, and the one that can lose is the turn's.

// sessionForCompact builds a session holding a short conversation, in the
// status the caller wants to test.
func sessionForCompact(t *testing.T, status SessionStatus) (*Server, *Session) {
	t.Helper()

	g := &Server{}
	g.config.DataDir = t.TempDir()

	eng := &engine.Engine{}
	// Short on purpose. A longer conversation would widen the race window, but
	// it also crosses the summarizer's threshold, and summarizing calls a model
	// this test has no credentials for. So the window is widened with
	// concurrency in the race test below rather than with size here.
	eng.Messages = []anthropic.MessageParam{
		anthropic.NewUserMessage(anthropic.NewTextBlock("first")),
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("second")),
	}

	s := &Session{
		Key:        "agent:test:bridge-compact",
		AgentName:  "test",
		Engine:     eng,
		Status:     status,
		injections: make(chan string, 10),
	}
	g.sessions.Store(s.Key, s)
	return g, s
}

func postCompact(g *Server, key string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/sessions/"+key+"/compact", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	g.handleBridgeCompact(rec, req, key)
	return rec
}

// TestCompactRefusesASessionWithATurnInFlight is the subject: a compaction
// asked for while the session is running is refused rather than run.
func TestCompactRefusesASessionWithATurnInFlight(t *testing.T) {
	g, s := sessionForCompact(t, Running)

	before := len(s.Engine.Messages)
	rec := postCompact(g, s.Key)

	if rec.Code != http.StatusConflict {
		t.Fatalf("compact during a turn answered %d, want %d — it ran against a conversation the turn was writing",
			rec.Code, http.StatusConflict)
	}
	if got := len(s.Engine.Messages); got != before {
		t.Errorf("refused compact still changed the conversation: %d messages, want %d", got, before)
	}
	if !strings.Contains(rec.Body.String(), "turn is in flight") {
		t.Errorf("refusal does not say why: %s", rec.Body.String())
	}
}

// TestCompactRunsOnAnIdleSession is the control, and it is what stops the
// refusal above from being satisfied by a handler that refuses everything.
func TestCompactRunsOnAnIdleSession(t *testing.T) {
	g, s := sessionForCompact(t, Idle)

	rec := postCompact(g, s.Key)

	if rec.Code != http.StatusOK {
		t.Fatalf("compact on an idle session answered %d (%s), want %d",
			rec.Code, strings.TrimSpace(rec.Body.String()), http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "compacted") {
		t.Errorf("idle compact did not report a compaction: %s", rec.Body.String())
	}
}

// TestCompactDoesNotRaceALiveTurn is the one that measures rather than asserts.
// It drives the real handler against a real turn — Session.turn into
// Engine.RunTurn into prepareInput, which is where the conversation is
// appended to — and is meaningful only under `go test -race`.
//
// Sabotage that proves it is not self-satisfying: delete the Status == Running
// test from handleBridgeCompact and this reddens with a DATA RACE between
// engine.(*Engine).prepareInput and server.(*Server).handleBridgeCompact. The
// two tests above stay green under that sabotage, which is why this one exists
// alongside them rather than instead of them.
func TestCompactDoesNotRaceALiveTurn(t *testing.T) {
	g, s := sessionForCompact(t, Idle)

	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			runTurnAsFarAsATestCanTakeIt(s)
		}
	}()

	// Several compacters rather than one. The window a sabotage reopens is the
	// few microseconds between releasing s.mu and finishing the marshal, and one
	// caller hits it rarely enough that a reopened race escaped roughly one run
	// in five. Measured with the sabotage in place, this shape caught it on
	// every run.
	var compacters sync.WaitGroup
	var compactions atomic.Int64
	deadline := time.Now().Add(3 * time.Second)
	for i := 0; i < 4; i++ {
		compacters.Add(1)
		go func() {
			defer compacters.Done()
			for time.Now().Before(deadline) {
				postCompact(g, s.Key)
				compactions.Add(1)
			}
		}()
	}

	compacters.Wait()
	close(stop)
	wg.Wait()

	if compactions.Load() == 0 {
		t.Fatal("no compaction was attempted, so the race was never given a chance to fire")
	}
}

// runTurnAsFarAsATestCanTakeIt drives the real Session.turn and swallows the
// one failure that is an artefact of the test rather than of the code.
//
// A turn on an engine built in a test reaches the provider and dereferences a
// client nothing configured, because there are no credentials here and there
// must not be — a test that reached Anthropic would be billing the user to
// assert a lock. What matters is that the write this test is about happens
// BEFORE that point: Engine.RunTurn appends the turn's user message in
// prepareInput (engine/turn_prepare.go), several steps above the model call. So
// the racing writer is real and is the production one; only its ending is
// stubbed.
//
// Session.turn's own deferred func returns the session to Idle on the way out
// of a panic exactly as it does on the way out of an error, so the session is
// left in the same state either way and the next iteration is a real one.
func runTurnAsFarAsATestCanTakeIt(s *Session) {
	defer func() { _ = recover() }()
	//nolint:errcheck // the turn is expected not to finish; see above.
	s.turn(context.Background(), "hello")
}
