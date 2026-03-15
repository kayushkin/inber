package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/conversation"
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
}

// turn executes one turn on this session's engine.
// Drains any pending messages (from sub-agents that completed while idle)
// by prepending them to the input.
// onActive/onIdle are called on status transitions (for event publishing).
func (s *Session) turn(ctx context.Context, input string) (*agent.TurnResult, error) {
	s.mu.Lock()
	s.Status = Running
	ctx, s.cancel = context.WithCancel(ctx)
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

// ---------------------------------------------------------------------------
// Session creation
// ---------------------------------------------------------------------------

// getOrCreateSession returns an existing session or creates one.
func (g *Gateway) getOrCreateSession(key, agentName string, ac AgentConfig, onEvent func(StreamEvent)) (*Session, error) {
	if val, ok := g.sessions.Load(key); ok {
		return val.(*Session), nil
	}

	sess, err := g.createSession(key, agentName, ac, onEvent)
	if err != nil {
		return nil, err
	}

	// Store, but handle race (another goroutine may have created it).
	actual, loaded := g.sessions.LoadOrStore(key, sess)
	if loaded {
		// Someone else created it first. Close ours and use theirs.
		sess.close()
		return actual.(*Session), nil
	}
	return sess, nil
}

// createSession creates a new session with a fresh engine.
func (g *Gateway) createSession(key, agentName string, ac AgentConfig, onEvent func(StreamEvent)) (*Session, error) {
	injections := make(chan string, 10)

	cfg := engine.EngineConfig{
		AgentName:   agentName,
		RepoRoot:    ac.Workspace,
		Model:       ac.Model,
		Thinking:    ac.Thinking,
		CommandName: "serve",
		Injections:  injections,
		ExtraTools: []agent.Tool{
			g.SpawnAgentTool(key),
			g.SessionsListTool(key),
			g.SteerAgentTool(),
		},
	}

	// Pass shared model store.
	if g.modelStore != nil {
		// Engine will use this instead of opening its own.
		// TODO: Add ModelStore field to EngineConfig.
		// For now, engine opens its own. This is fine initially.
	}

	// Set up display hooks for streaming.
	if onEvent != nil {
		cfg.Display = &engine.DisplayHooks{
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
		}
	}

	// Try to load existing messages from persistence.
	msgs := g.loadPersistedMessages(key)
	if len(msgs) > 0 {
		// Repair interrupted sessions.
		msgs = conversation.RepairEmptyContent(msgs)
		msgs = conversation.RepairDanglingToolUse(msgs)
		msgs = conversation.RepairAlternation(msgs)
		msgs = agent.SanitizeMessageToolIDs(msgs)
		log.Printf("[gateway] resumed session %s (%d messages)", key, len(msgs))
	}

	eng, err := engine.NewEngine(cfg)
	if err != nil {
		return nil, fmt.Errorf("create engine for %s: %w", agentName, err)
	}

	// If we loaded persisted messages, set them on the engine.
	if len(msgs) > 0 {
		eng.Messages = msgs
	}

	return &Session{
		Key:        key,
		AgentName:  agentName,
		Engine:     eng,
		Status:     Idle,
		CreatedAt:  time.Now(),
		LastActive: time.Now(),
		injections: injections,
	}, nil
}

// loadPersistedMessages loads messages from the gateway data dir.
func (g *Gateway) loadPersistedMessages(key string) []anthropic.MessageParam {
	path := filepath.Join(g.config.DataDir, "sessions", key, "messages.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var msgs []anthropic.MessageParam
	if err := json.Unmarshal(data, &msgs); err != nil {
		log.Printf("[gateway] failed to load messages for %s: %v", key, err)
		return nil
	}
	return msgs
}

// ---------------------------------------------------------------------------
// Forking
// ---------------------------------------------------------------------------

// forkSession creates a child session with a deep copy of the parent's messages.
func (g *Gateway) forkSession(parent *Session, childKey, agentName string, ac AgentConfig, onEvent func(StreamEvent)) (*Session, error) {
	// Deep copy parent's messages.
	parent.mu.Lock()
	msgData, err := json.Marshal(parent.Engine.Messages)
	parent.mu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("copy messages: %w", err)
	}

	var parentMessages []anthropic.MessageParam
	if err := json.Unmarshal(msgData, &parentMessages); err != nil {
		return nil, fmt.Errorf("unmarshal messages: %w", err)
	}

	child, err := g.createSession(childKey, agentName, ac, onEvent)
	if err != nil {
		return nil, err
	}

	// Replace the empty messages with parent's history.
	child.Engine.Messages = parentMessages
	child.SpawnDepth = parent.SpawnDepth + 1
	child.ParentKey = parent.Key

	// Inject sub-agent context.
	taskContext := fmt.Sprintf("[System] You are a forked sub-agent. "+
		"You inherited your parent's conversation context. "+
		"Complete your assigned task and respond with your results. "+
		"Do not repeat context you already have.")
	child.Engine.Messages = append(child.Engine.Messages,
		anthropic.NewUserMessage(anthropic.NewTextBlock(taskContext)))

	return child, nil
}

// sessionKeyForChild generates a child session key.
func sessionKeyForChild(parentKey string) string {
	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%100000)
	return parentKey + ":sub:" + suffix
}

// truncate truncates a string for display.
func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n-3] + "..."
}
