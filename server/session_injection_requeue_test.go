package server

import (
	"context"
	"strings"
	"sync"
	"testing"
)

// The window these tests pin is the one offerToTurnInFlightLocked's comment
// already describes, in the past tense, as though it were closed:
//
//	"With no turn running there is no reader, and the message sat buffered
//	until some unrelated later turn happened to make a second API call."
//
// Holding s.mu across the status test and the send closed the half where a
// delivery raced Status = Idle. It did not close the half where the turn was
// genuinely running, genuinely took the message, and then ended without ever
// reading it — which is every turn the model answers without calling a tool,
// because Agent.Run reads the channel only from its second API call onward.

// TestATurnThatNeverReadsRequeuesItsInjections is the defect, driven directly.
// A message accepted mid-turn by a turn that makes one API call must not stay
// in the channel: the contract this package pins is that a delivered message
// reaches the turn in flight or reaches the pending queue, and this one reached
// neither.
func TestATurnThatNeverReadsRequeuesItsInjections(t *testing.T) {
	s := &Session{Key: "test", Status: Running, injections: make(chan string, 10)}

	if route := s.deliver("actually, stop and check the branch first"); route != DeliveredMidTurn {
		t.Fatalf("route = %q, want %q — the setup is wrong, not the fix", route, DeliveredMidTurn)
	}

	// Stand in for Session.turn's deferred func: the turn ends without
	// Agent.Run ever having reached a second API call.
	s.mu.Lock()
	s.Status = Idle
	s.requeueInjectionsTheTurnNeverReadLocked()
	s.mu.Unlock()

	if got := len(s.injections); got != 0 {
		t.Errorf("%d message(s) still buffered in the channel; a later unrelated turn would read them", got)
	}
	if len(s.pendingMessages) != 1 {
		t.Fatalf("pendingMessages = %v, want the one message requeued", s.pendingMessages)
	}
	if !strings.Contains(s.pendingMessages[0], "check the branch first") {
		t.Errorf("pendingMessages[0] = %q, want the delivered text verbatim", s.pendingMessages[0])
	}
}

// TestSessionTurnActuallyRunsTheRequeue pins the WIRING, which every other test
// in this file takes on trust by calling the drain directly. A correct drain
// that Session.turn never calls fixes nothing, and nothing else here would go
// red if the call were deleted.
//
// A nil Engine makes RunTurn panic, which is a turn ending the hard way — and
// turn's deferred func runs during the unwind, which is exactly what needs
// asserting. Recovering here lets the test read the state that deferred func
// left behind.
func TestSessionTurnActuallyRunsTheRequeue(t *testing.T) {
	s := &Session{Key: "test", Status: Idle, injections: make(chan string, 10)}
	s.injections <- "a message the turn will never read"

	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("expected RunTurn to panic on a nil Engine; if it no longer does, this test needs a different way to end a turn")
			}
		}()
		_, _ = s.turn(context.Background(), "go")
	}()

	if got := len(s.injections); got != 0 {
		t.Errorf("%d message(s) left in the channel — Session.turn's deferred func is not calling the requeue", got)
	}
	if len(s.pendingMessages) != 1 || s.pendingMessages[0] != "a message the turn will never read" {
		t.Errorf("pendingMessages = %v, want the stranded message requeued by Session.turn", s.pendingMessages)
	}
}

// TestRequeuePreservesOrderAndTheAlreadyPendingQueue: the drain appends, so a
// message the session was already holding for the next turn keeps its place
// ahead of one the ending turn never read. Session.turn joins pendingMessages
// in slice order, so this is the order the next turn's input is built in.
func TestRequeuePreservesOrderAndTheAlreadyPendingQueue(t *testing.T) {
	s := &Session{Key: "test", Status: Running, injections: make(chan string, 10)}
	s.pendingMessages = []string{"queued before the turn ended"}

	for _, m := range []string{"first mid-turn", "second mid-turn"} {
		if route := s.deliver(m); route != DeliveredMidTurn {
			t.Fatalf("deliver(%q) route = %q, want %q", m, route, DeliveredMidTurn)
		}
	}

	s.mu.Lock()
	s.Status = Idle
	s.requeueInjectionsTheTurnNeverReadLocked()
	s.mu.Unlock()

	want := []string{"queued before the turn ended", "first mid-turn", "second mid-turn"}
	if len(s.pendingMessages) != len(want) {
		t.Fatalf("pendingMessages = %v, want %v", s.pendingMessages, want)
	}
	for i := range want {
		if s.pendingMessages[i] != want[i] {
			t.Errorf("pendingMessages[%d] = %q, want %q", i, s.pendingMessages[i], want[i])
		}
	}
}

// TestRequeueLeavesNothingBehindWhenTheTurnDidRead: the ordinary case. A turn
// that reached its second API call has already drained the channel, so the
// requeue must be a no-op rather than a second delivery of the same message.
func TestRequeueLeavesNothingBehindWhenTheTurnDidRead(t *testing.T) {
	s := &Session{Key: "test", Status: Running, injections: make(chan string, 10)}

	if route := s.deliver("mid-turn steer"); route != DeliveredMidTurn {
		t.Fatalf("route = %q, want %q", route, DeliveredMidTurn)
	}

	// Stand in for Agent.Run's InjectCheck draining from inside the turn.
	<-s.injections

	s.mu.Lock()
	s.Status = Idle
	s.requeueInjectionsTheTurnNeverReadLocked()
	s.mu.Unlock()

	if len(s.pendingMessages) != 0 {
		t.Errorf("pendingMessages = %v, want empty — the turn already read it, requeueing would deliver it twice", s.pendingMessages)
	}
}

// TestRequeueToleratesASessionWithNoInjectionChannel: not every session is
// built with one — only the server path supplies it (server/session_creation.go),
// so CLI and chat sessions carry a nil channel and the deferred drain runs on
// them too.
func TestRequeueToleratesASessionWithNoInjectionChannel(t *testing.T) {
	s := &Session{Key: "test", Status: Running}

	s.mu.Lock()
	s.Status = Idle
	s.requeueInjectionsTheTurnNeverReadLocked()
	s.mu.Unlock()

	if len(s.pendingMessages) != 0 {
		t.Errorf("pendingMessages = %v, want empty", s.pendingMessages)
	}
}

// TestNoMessageIsAdmittedAfterTheRequeueDrains is why the drain shares the
// critical section that sets Status = Idle rather than running beside it. A
// delivery that got in after the drain but while the status still said Running
// would land in a channel with no reader left — the exact leak this fix is
// closing, reintroduced at a narrower width.
//
// One joiner cannot reach that window, so this drives many concurrent
// deliveries against the transition and asserts the conservation law: every
// message ends up in pendingMessages or was read, and none is left in the
// channel once the session is idle.
func TestNoMessageIsAdmittedAfterTheRequeueDrains(t *testing.T) {
	const rounds = 300
	const deliverers = 16

	for round := 0; round < rounds; round++ {
		s := &Session{Key: "test", Status: Running, injections: make(chan string, deliverers)}

		var senders sync.WaitGroup
		for i := 0; i < deliverers; i++ {
			senders.Add(1)
			go func() {
				defer senders.Done()
				s.deliver("m")
			}()
		}

		// The turn ends concurrently with those deliveries.
		var ender sync.WaitGroup
		ender.Add(1)
		go func() {
			defer ender.Done()
			s.mu.Lock()
			s.Status = Idle
			s.requeueInjectionsTheTurnNeverReadLocked()
			s.mu.Unlock()
		}()

		senders.Wait()
		ender.Wait()

		s.mu.Lock()
		buffered := len(s.injections)
		queued := len(s.pendingMessages)
		s.mu.Unlock()

		if buffered != 0 {
			t.Fatalf("round %d: %d message(s) stranded in the channel after the session went idle", round, buffered)
		}
		if queued != deliverers {
			t.Fatalf("round %d: %d of %d messages accounted for in pendingMessages", round, queued, deliverers)
		}
	}
}
