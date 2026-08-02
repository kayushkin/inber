package server

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/engine"
	"github.com/kayushkin/inber/logger"
)

// SessionStatus represents the current state of a session.
type SessionStatus int

const (
	Idle SessionStatus = iota
	Running
	Completed
	Error
)

func (s SessionStatus) String() string {
	switch s {
	case Idle:
		return "idle"
	case Running:
		return "running"
	case Completed:
		return "completed"
	case Error:
		return "error"
	default:
		return "unknown"
	}
}

func (s SessionStatus) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.String())
}

// Session represents one ongoing conversation with an agent.
type Session struct {
	Key        string
	AgentName  string
	Engine     *engine.Engine
	Status     SessionStatus
	SpawnDepth int
	ParentKey  string
	Children   []string
	CreatedAt  time.Time
	LastActive time.Time

	// WorkspaceRoots is the forge workspace this session works in, empty for a
	// session that works in its agent's ordinary repository. It is set once, by
	// createSession, from whichever source answered — so a fork can hand its
	// parent's workspace to its child, and recordChildSession can write down
	// where the child works without being told.
	WorkspaceRoots []engine.WorkspaceRoot

	mu              sync.Mutex
	cancel          context.CancelFunc
	injections      chan string
	pendingMessages []string          // results queued while session was idle
	onEvent         func(StreamEvent) // current request's event callback (updated per-turn)
}

// setOnEvent sets the event callback for the current request.
// Must be called before turn() so hooks point to the right writer.
func (s *Session) setOnEvent(onEvent func(StreamEvent)) {
	s.mu.Lock()
	s.onEvent = onEvent
	s.mu.Unlock()
}

// getOnEvent returns the current event callback (may be nil).
func (s *Session) getOnEvent() func(StreamEvent) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.onEvent
}

// updateHooks updates the engine's display hooks to use the current onEvent.
// Must be called with s.mu held.
func (s *Session) updateHooks() {
	onEvent := s.onEvent
	if s.Engine == nil {
		return
	}
	if onEvent != nil {
		sess := s.Engine.Session // for turn number access
		eng := s.Engine          // for the message these events belong to
		s.Engine.SetDisplayHooks(&engine.DisplayHooks{
			OnThinking: func(text string) {
				onEvent(StreamEvent{Kind: "thinking", Text: text, Turn: sess.CurrentTurn(), MessageID: eng.CurrentMessageID()})
			},
			OnTextDelta: func(text string) {
				onEvent(StreamEvent{Kind: "delta", Text: text, Turn: sess.CurrentTurn(), MessageID: eng.CurrentMessageID()})
			},
			OnToolCall: func(name, input string) {
				onEvent(StreamEvent{Kind: "tool_call", Tool: name, Text: input, Turn: sess.CurrentTurn(), MessageID: eng.CurrentMessageID()})
			},
			OnToolResult: func(name, output string, isError bool) {
				onEvent(StreamEvent{Kind: "tool_result", Tool: name, Text: output, Turn: sess.CurrentTurn(), MessageID: eng.CurrentMessageID()})
			},
			OnStatus: func(text string) {
				onEvent(StreamEvent{Kind: "status", Text: text, Turn: sess.CurrentTurn()})
			},
		})
	} else {
		s.Engine.SetDisplayHooks(nil)
	}
}

// withoutCallerCancellation derives the context session work actually runs
// under from the context of whoever asked for that work.
//
// It keeps the caller's deadline and drops the caller's cancellation. The
// deadline is load-bearing: a sub-agent spawn bounds its child with
// context.WithTimeout, and that bound only stops anything if it survives into
// the work. The cancellation is deliberately dropped, because the caller is
// almost always an HTTP request — a browser tab closing, or a proxy hitting its
// read timeout, must not abort work the session would otherwise finish and
// keep. A turn in flight is stopped by interrupt and stop, and by nothing else.
//
// Both things a session does under a caller's context use this rule: running a
// turn, and being built in the first place. Building a session reads the
// workspace — the memory store's preparation scans it for recently modified
// files — and a request that has gone away is not a reason to abandon a session
// the next request will ask for anyway.
func withoutCallerCancellation(ctx context.Context) (context.Context, context.CancelFunc) {
	turnCtx := context.WithoutCancel(ctx)
	if deadline, ok := ctx.Deadline(); ok {
		return context.WithDeadline(turnCtx, deadline)
	}
	return context.WithCancel(turnCtx)
}

// turn executes one turn on this session's engine.
// Drains any pending messages (from sub-agents that completed while idle)
// by prepending them to the input.
// onActive/onIdle are called on status transitions (for event publishing).
func (s *Session) turn(ctx context.Context, input string) (*agent.TurnResult, error) {
	turnCtx, cancel := withoutCallerCancellation(ctx)
	defer cancel()

	s.mu.Lock()
	s.Status = Running
	s.cancel = cancel
	// Update engine's display hooks to point to current request's onEvent.
	s.updateHooks()
	// Drain pending messages and prepend to input.
	if len(s.pendingMessages) > 0 {
		prefix := strings.Join(s.pendingMessages, "\n\n---\n\n")
		input = prefix + "\n\n---\n\n" + input
		logger.WithComponent("session").Debug("delivering pending messages", map[string]interface{}{
			"session_key": s.Key,
			"count":       len(s.pendingMessages),
		})
		s.pendingMessages = nil
	}
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.Status = Idle
		s.LastActive = time.Now()
		s.cancel = nil
		s.requeueInjectionsTheTurnNeverReadLocked()
		s.mu.Unlock()
	}()

	result, err := s.Engine.RunTurn(turnCtx, input)
	if err != nil {
		s.mu.Lock()
		s.Status = Error
		s.mu.Unlock()
		// Hand the result back even though the turn failed: it carries whatever
		// text the user already saw, which the caller records against the
		// request.
		return result, err
	}
	return result, nil
}

// DeliveryRoute names the path a message took into a session. Every message
// handed to deliver takes one of them; there is deliberately no route that
// means "lost", because the caller reports the route to a user.
type DeliveryRoute string

const (
	// DeliveredMidTurn: the turn in flight accepted the message and will read
	// it before its next API call.
	DeliveredMidTurn DeliveryRoute = "mid-turn"
	// DeliveredNextTurn: the message is queued and will be prepended to the
	// input of the session's next turn.
	DeliveredNextTurn DeliveryRoute = "next-turn"
)

// requeueInjectionsTheTurnNeverReadLocked moves anything still sitting in the
// injection channel onto the pending queue, as the turn that accepted it ends.
// s.mu must be held.
//
// offerToTurnInFlightLocked reports DeliveredMidTurn the moment the message is
// in the channel, but the only reader is Agent.Run, and it reads from its
// SECOND API call onward. A turn the model answers without calling a tool makes
// exactly one, so it never reads, and until this drain existed the message
// stayed buffered until some unrelated later turn happened to make a second
// call — surfacing there, out of context, hours later, wrapped in "[New message
// from user while you were working]".
//
// That is the same failure offerToTurnInFlightLocked's comment describes in the
// past tense. Holding the lock across the status test and the send closed the
// half where a delivery raced Status = Idle; this closes the half where the
// reader was real, took the message, and then finished without reading it.
//
// The pending queue is where both other overflow paths already go — a full
// buffer and a session that is not running — so this adds no new semantics: the
// message is answered by the next turn, as its own input, instead of appearing
// mid-way through an unrelated one.
//
// Draining here is safe because RunTurn has returned, so Agent.Run is no longer
// reading.
//
// What makes it correct against a concurrent deliverer is that it shares ONE
// hold of s.mu with Status = Idle — not the order of the two inside that hold.
// Measured by sabotage: swapping them, or moving the drain after the unlock,
// both stay green, because a deliverer can only run wholly before the critical
// section (its message enters the channel and this drains it) or wholly after
// (Status is Idle, so offerToTurnInFlightLocked refuses and it queues itself).
// Split them into two holds with the drain first and it strands messages within
// ~130 rounds — that is the shape to keep out, so do not "tidy" this into its
// own lock hold above the status write.
func (s *Session) requeueInjectionsTheTurnNeverReadLocked() {
	if s.injections == nil {
		return
	}
	for {
		select {
		case msg := <-s.injections:
			s.pendingMessages = append(s.pendingMessages, msg)
			logger.WithComponent("session").Debug("requeued a mid-turn message the turn never read", map[string]interface{}{
				"session_key": s.Key,
			})
		default:
			return
		}
	}
}

// queuePending adds a message to be delivered on the next turn.
func (s *Session) queuePending(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingMessages = append(s.pendingMessages, msg)
}

// offerToTurnInFlightLocked offers the message to the turn currently running on
// this session, and reports whether that turn took it. s.mu must be held.
//
// The status test and the send have to happen under one hold of the lock,
// because they used to happen under two and the gap between them lost
// messages. Session.turn's deferred func sets Status = Idle under this same
// lock, so a caller that read Running, unlocked, and then sent was sending
// into a channel nothing would ever drain: s.injections is read only by
// Agent.Run, via Engine.buildInjectCheck, and only from its second API call
// onward. With no turn running there is no reader, and the message sat
// buffered until some unrelated later turn happened to make a second API call.
//
// Holding s.mu across the send is safe, and that is worth stating because the
// fear of it is why the lock was dropped: the send cannot block. It is a
// select with a default, and the drain side is a non-blocking receive in a
// package that holds no reference to this lock.
func (s *Session) offerToTurnInFlightLocked(message string) bool {
	if s.Status != Running || s.injections == nil {
		return false
	}
	select {
	case s.injections <- message:
		return true
	default:
		logger.WithComponent("session").Warn("injection buffer full, the turn in flight will not see this message", map[string]interface{}{
			"session_key": s.Key,
			"capacity":    cap(s.injections),
		})
		return false
	}
}

// injectIfRunning hands a message to the turn in flight and reports whether
// that turn took it. A false return means the message was NOT delivered and
// the caller still owns it — that is the whole point of the return value, and
// callers must not treat it as advisory.
func (s *Session) injectIfRunning(message string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.offerToTurnInFlightLocked(message)
}

// deliver hands a message to the session by whichever route is open, and
// names the route it used. It never drops the message: a session that is not
// running, and one whose injection buffer is full, both fall back to the
// pending queue that Session.turn drains at the start of its next turn.
func (s *Session) deliver(message string) DeliveryRoute {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.offerToTurnInFlightLocked(message) {
		return DeliveredMidTurn
	}
	s.pendingMessages = append(s.pendingMessages, message)
	return DeliveredNextTurn
}

// interrupt cancels the current turn but keeps the session alive for future turns.
func (s *Session) interrupt() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.Status = Idle
	s.LastActive = time.Now()
}

// stop cancels the current run and marks the session as completed (terminal).
func (s *Session) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
	}
	s.Status = Completed
}

// close releases engine resources.
func (s *Session) close() {
	s.stop()
	if s.Engine != nil {
		s.Engine.Close()
	}
}
