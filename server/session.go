package server

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/engine"
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
	pendingMessages []string // results queued while session was idle
	onEvent         func(StreamEvent) // current request's event callback (updated per-turn)
}

// setOnEvent sets the event callback for the current request.
// Must be called before turn() so hooks point to the right writer.
func (s *Session) setOnEvent(onEvent func(StreamEvent)) {
	s.mu.Lock()
	s.onEvent = onEvent
	s.mu.Unlock()
}

// updateHooks updates the engine's display hooks to use the current onEvent.
// Must be called with s.mu held.
func (s *Session) updateHooks() {
	onEvent := s.onEvent
	if s.Engine == nil {
		return
	}
	if onEvent != nil {
		s.Engine.SetDisplayHooks(&engine.DisplayHooks{
			OnThinking: func(text string) {
				onEvent(StreamEvent{Kind: "thinking", Text: text})
			},
			OnTextDelta: func(text string) {
				onEvent(StreamEvent{Kind: "delta", Text: text})
			},
			OnToolCall: func(name, input string) {
				onEvent(StreamEvent{Kind: "tool_call", Tool: name, Text: input})
			},
			OnToolResult: func(name, output string, isError bool) {
				onEvent(StreamEvent{Kind: "tool_result", Tool: name, Text: output})
			},
		})
	} else {
		s.Engine.SetDisplayHooks(nil)
	}
}

// turn executes one turn on this session's engine.
// Drains any pending messages (from sub-agents that completed while idle)
// by prepending them to the input.
// onActive/onIdle are called on status transitions (for event publishing).
func (s *Session) turn(ctx context.Context, input string) (*agent.TurnResult, error) {
	s.mu.Lock()
	s.Status = Running
	ctx, s.cancel = context.WithCancel(ctx)
	// Update engine's display hooks to point to current request's onEvent.
	s.updateHooks()
	// Drain pending messages and prepend to input.
	if len(s.pendingMessages) > 0 {
		prefix := strings.Join(s.pendingMessages, "\n\n---\n\n")
		input = prefix + "\n\n---\n\n" + input
		log.Printf("[session] %s: delivering %d pending messages", s.Key, len(s.pendingMessages))
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

	result, err := s.Engine.RunTurn(input)
	if err != nil {
		s.mu.Lock()
		s.Status = Error
		s.mu.Unlock()
		return nil, err
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
			log.Printf("[session] injection buffer full for %s, dropping", s.Key)
		}
	}
}

// stop cancels the current run.
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