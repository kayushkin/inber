package server

import (
	"runtime"
	"sync"
	"testing"
)

// The contract these tests pin: a message handed to Session.deliver is never
// lost. It reaches the turn in flight, or it reaches the pending queue that the
// next turn drains, and the route returned names which of the two happened so
// the caller can describe the delivery truthfully.
//
// Before this, delivery was a Status read, an unlock, and then a send on a
// capacity-10 channel with a select/default. Both the gap and the default
// branch dropped the message in silence, and the HTTP caller was told
// "[Message injected into running session — agent will see it during current
// work]" either way.

// TestDeliverToIdleSessionQueuesForTheNextTurn: with no turn running there is
// no reader for the injection channel — Agent.Run drains it, and only from its
// second API call onward — so an idle session's message belongs in the pending
// queue, which Session.turn prepends to the input of its next turn.
func TestDeliverToIdleSessionQueuesForTheNextTurn(t *testing.T) {
	s := &Session{Key: "test", Status: Idle, injections: make(chan string, 10)}

	if route := s.deliver("are you there"); route != DeliveredNextTurn {
		t.Errorf("route = %q, want %q", route, DeliveredNextTurn)
	}

	if len(s.injections) != 0 {
		t.Errorf("%d message(s) reached the injection channel of a session with no turn to read them", len(s.injections))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.pendingMessages) != 1 || s.pendingMessages[0] != "are you there" {
		t.Errorf("pendingMessages = %q, want [\"are you there\"]", s.pendingMessages)
	}
}

// TestDeliverFallsBackToTheQueueWhenTheInjectionBufferIsFull is the core of the
// fix. The buffer holds 10 and nothing ever made that number load-bearing; the
// eleventh message used to take the select's default branch, log one warn line
// and vanish, while its caller reported it delivered.
func TestDeliverFallsBackToTheQueueWhenTheInjectionBufferIsFull(t *testing.T) {
	s := &Session{Key: "test", Status: Running, injections: make(chan string, 2)}

	if route := s.deliver("first"); route != DeliveredMidTurn {
		t.Fatalf("route = %q for the first message, want %q", route, DeliveredMidTurn)
	}
	if route := s.deliver("second"); route != DeliveredMidTurn {
		t.Fatalf("route = %q for the second message, want %q", route, DeliveredMidTurn)
	}

	// The buffer is now full. The third message must survive it.
	if route := s.deliver("third"); route != DeliveredNextTurn {
		t.Errorf("route = %q for the message that overflowed the buffer, want %q", route, DeliveredNextTurn)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.pendingMessages) != 1 || s.pendingMessages[0] != "third" {
		t.Errorf("pendingMessages = %q — the overflowed message was dropped instead of deferred", s.pendingMessages)
	}
}

// TestInjectIfRunningRefusesAnIdleSessionAndKeepsTheMessage: injectIfRunning is
// the half of the contract that does NOT fall back, because two of its callers
// have a better path of their own (injectIfBusy runs the message as a turn,
// deliverResult starts a turn to carry the result). It must therefore leave the
// message entirely alone when it refuses — writing it anywhere would duplicate
// it against whatever the caller does next.
func TestInjectIfRunningRefusesAnIdleSessionAndKeepsTheMessage(t *testing.T) {
	s := &Session{Key: "test", Status: Idle, injections: make(chan string, 10)}

	if s.injectIfRunning("hello") {
		t.Fatal("injectIfRunning claimed an idle session's turn accepted the message")
	}
	if len(s.injections) != 0 {
		t.Errorf("%d message(s) reached the injection channel after a refusal", len(s.injections))
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.pendingMessages) != 0 {
		t.Errorf("pendingMessages = %q — a refusal must not also queue, or the caller's own path delivers it twice", s.pendingMessages)
	}
}

// TestInjectIfRunningRefusesAFullBuffer: the other refusal. A running session
// whose buffer is full has no room for the message, and reporting success would
// be the original bug with a return value bolted on.
func TestInjectIfRunningRefusesAFullBuffer(t *testing.T) {
	s := &Session{Key: "test", Status: Running, injections: make(chan string, 1)}

	if !s.injectIfRunning("first") {
		t.Fatal("injectIfRunning refused a running session with room in its buffer")
	}
	if s.injectIfRunning("second") {
		t.Error("injectIfRunning reported a full buffer had taken the message")
	}
}

// TestDeliverLosesNothingWhileTheStatusFlips is the race the fix exists for,
// driven directly. Session.turn's deferred func sets Status = Idle under s.mu,
// so a delivery that tests the status, unlocks, and then sends can be sending
// into a channel that has just lost its only reader. Run with -race.
//
// The assertion is a conservation law rather than a route: whichever way each
// message goes is a matter of timing, but every one of them must come out
// somewhere.
func TestDeliverLosesNothingWhileTheStatusFlips(t *testing.T) {
	const messages = 200

	s := &Session{Key: "test", Status: Running, injections: make(chan string, 8)}

	var readers sync.WaitGroup
	stopDraining := make(chan struct{})
	drained := 0

	// Stand in for Agent.Run's InjectCheck, which drains the channel from
	// inside a running turn.
	readers.Add(1)
	go func() {
		defer readers.Done()
		for {
			select {
			case <-s.injections:
				drained++
			case <-stopDraining:
				for len(s.injections) > 0 {
					<-s.injections
					drained++
				}
				return
			}
		}
	}()

	// Stand in for turn's status transitions.
	var flippers sync.WaitGroup
	stopFlipping := make(chan struct{})
	flippers.Add(1)
	go func() {
		defer flippers.Done()
		for {
			select {
			case <-stopFlipping:
				return
			default:
			}
			s.mu.Lock()
			if s.Status == Running {
				s.Status = Idle
			} else {
				s.Status = Running
			}
			s.mu.Unlock()
		}
	}()

	var senders sync.WaitGroup
	for i := 0; i < messages; i++ {
		senders.Add(1)
		go func() {
			defer senders.Done()
			s.deliver("m")
		}()
	}
	senders.Wait()

	close(stopFlipping)
	flippers.Wait()
	close(stopDraining)
	readers.Wait()

	s.mu.Lock()
	queued := len(s.pendingMessages)
	s.mu.Unlock()

	if drained+queued != messages {
		t.Errorf("%d delivered mid-turn + %d queued = %d, want %d — %d message(s) were lost",
			drained, queued, drained+queued, messages, messages-(drained+queued))
	}
}

// TestNothingReachesTheInjectionChannelOfAnIdleSession pins the atomicity
// itself, rather than a consequence of it, and it is the test that fails for
// the original shape of this bug.
//
// The delivery used to read Status under s.mu, release the lock, and send
// afterwards — so the send happened with no lock held at all and could land on
// a session that had gone idle in between. Here the whole critical section runs
// while this goroutine holds s.mu with Status = Idle: a delivery that decides
// under the lock provably cannot send into the channel, and one that decided
// before taking it provably can.
func TestNothingReachesTheInjectionChannelOfAnIdleSession(t *testing.T) {
	s := &Session{Key: "test", Status: Running, injections: make(chan string, 64)}

	var senders sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 8; i++ {
		senders.Add(1)
		go func() {
			defer senders.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				s.deliver("m")
			}
		}()
	}

	// Let them get going, then take the lock away from them.
	for i := 0; i < 64; i++ {
		runtime.Gosched()
	}

	var leaked int
	for round := 0; round < 200; round++ {
		s.mu.Lock()
		s.Status = Idle
		for len(s.injections) > 0 {
			<-s.injections
		}
		// Still holding s.mu, still Idle. Nothing may arrive.
		for i := 0; i < 32; i++ {
			runtime.Gosched()
		}
		leaked += len(s.injections)
		s.Status = Running
		s.pendingMessages = nil
		s.mu.Unlock()

		for len(s.injections) > 0 {
			<-s.injections
		}
	}

	close(stop)
	senders.Wait()

	if leaked > 0 {
		t.Errorf("%d message(s) reached the injection channel of an idle session — the status test and the send are not one act, so a turn that ends between them takes the message with it", leaked)
	}
}
