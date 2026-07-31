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
		s.Engine.SetDisplayHooks(&engine.DisplayHooks{
			OnThinking: func(text string) {
				onEvent(StreamEvent{Kind: "thinking", Text: text, Turn: sess.CurrentTurn()})
			},
			OnTextDelta: func(text string) {
				onEvent(StreamEvent{Kind: "delta", Text: text, Turn: sess.CurrentTurn()})
			},
			OnToolCall: func(name, input string) {
				onEvent(StreamEvent{Kind: "tool_call", Tool: name, Text: input, Turn: sess.CurrentTurn()})
			},
			OnToolResult: func(name, output string, isError bool) {
				onEvent(StreamEvent{Kind: "tool_result", Tool: name, Text: output, Turn: sess.CurrentTurn()})
			},
			OnStatus: func(text string) {
				onEvent(StreamEvent{Kind: "status", Text: text, Turn: sess.CurrentTurn()})
			},
		})
	} else {
		s.Engine.SetDisplayHooks(nil)
	}
}

// turnContext derives the context a turn actually runs under from the context
// of whoever asked for the turn.
//
// It keeps the caller's deadline and drops the caller's cancellation. The
// deadline is load-bearing: a sub-agent spawn bounds its child with
// context.WithTimeout, and that bound only stops anything if it survives into
// the turn. The cancellation is deliberately dropped, because the caller is
// almost always an HTTP request — a browser tab closing mid-turn, or a proxy
// hitting its read timeout, must not abort work the session would otherwise
// finish and keep. A turn in flight is stopped by interrupt and stop, and by
// nothing else.
func turnContext(ctx context.Context) (context.Context, context.CancelFunc) {
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
	turnCtx, cancel := turnContext(ctx)
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

// queuePending adds a message to be delivered on the next turn.
func (s *Session) queuePending(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pendingMessages = append(s.pendingMessages, msg)
}

// inject sends a message into the session (for mid-run injection).
func (s *Session) inject(message string) {
	if s.injections != nil {
		select {
		case s.injections <- message:
		default:
			logger.WithComponent("session").Warn("injection buffer full, dropping message", map[string]interface{}{
				"session_key": s.Key,
			})
		}
	}
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
