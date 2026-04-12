package server

// api_bridge.go implements llm-bridge-compatible endpoints so that the
// llm-bridge frontend can drive inber directly.
//
// These endpoints live alongside the existing /api/* routes and translate
// between inber's internal types and the llm-bridge canonical msg types.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/kayushkin/llm-bridge/msg"
)

// ---------------------------------------------------------------------------
// GET /health — llm-bridge format
// ---------------------------------------------------------------------------

type bridgeHealthResponse struct {
	Status    string              `json:"status"`
	Harnesses []bridgeHarnessStatus `json:"harnesses"`
	Sessions  bridgeSessionCounts `json:"sessions"`
}

type bridgeHarnessStatus struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Binary    string `json:"binary,omitempty"`
}

type bridgeSessionCounts struct {
	Running   int `json:"running"`
	Idle      int `json:"idle"`
	Completed int `json:"completed"`
}

func (g *Server) handleBridgeHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	running, idle, completed := g.countSessionsByState()

	status := "ok"
	g.mu.RLock()
	lastErrAt := g.lastErrorAt
	g.mu.RUnlock()
	if !lastErrAt.IsZero() && time.Since(lastErrAt) < 5*time.Minute {
		status = "degraded"
	}

	jsonResponse(w, bridgeHealthResponse{
		Status: status,
		Harnesses: []bridgeHarnessStatus{
			{Name: "inber", Available: true},
		},
		Sessions: bridgeSessionCounts{
			Running:   running,
			Idle:      idle,
			Completed: completed,
		},
	})
}

// ---------------------------------------------------------------------------
// GET /harnesses
// ---------------------------------------------------------------------------

func (g *Server) handleBridgeHarnesses(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	jsonResponse(w, []bridgeHarnessStatus{
		{Name: "inber", Available: true},
	})
}

// ---------------------------------------------------------------------------
// GET /sessions  — list
// POST /sessions — create
// ---------------------------------------------------------------------------

// bridgeSession mirrors llm-bridge's store.Session JSON shape.
type bridgeSession struct {
	ID          string    `json:"id"`
	DisplayName string    `json:"display_name"`
	Harness     string    `json:"harness"`
	State       string    `json:"state"`
	AgentID     string    `json:"agent_id,omitempty"`
	SpawnerID   string    `json:"spawner_id,omitempty"`
	ParentID    string    `json:"parent_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func sessionInfoToBridge(s *SessionInfo) bridgeSession {
	return bridgeSession{
		ID:          s.Key,
		DisplayName: s.Agent,
		Harness:     "inber",
		State:       s.Status.String(),
		ParentID:    s.ParentKey,
		CreatedAt:   s.CreatedAt,
		UpdatedAt:   s.LastActive,
	}
}

func (g *Server) handleBridgeSessions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		sessions := g.ListSessions()
		result := make([]bridgeSession, 0, len(sessions))
		for _, s := range sessions {
			result = append(result, sessionInfoToBridge(s))
		}
		jsonResponse(w, result)

	case http.MethodPost:
		var req struct {
			Harness     string `json:"harness"`
			DisplayName string `json:"display_name,omitempty"`
			AgentID     string `json:"agent_id,omitempty"`
			AutoStart   bool   `json:"auto_start,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}

		agentName := req.AgentID
		if agentName == "" {
			agentName = g.config.DefaultAgent
		}
		if _, ok := g.GetAgentConfig(agentName); !ok {
			jsonError(w, fmt.Sprintf("unknown agent: %s", agentName), http.StatusBadRequest)
			return
		}

		sessionKey := fmt.Sprintf("agent:%s:bridge-%d", agentName, time.Now().UnixNano())
		g.store.UpsertSession(sessionKey, agentName, "main")

		sess := bridgeSession{
			ID:          sessionKey,
			DisplayName: req.DisplayName,
			Harness:     "inber",
			State:       string(msg.SessionIdle),
			AgentID:     agentName,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		if sess.DisplayName == "" {
			sess.DisplayName = agentName
		}

		w.WriteHeader(http.StatusCreated)
		jsonResponse(w, sess)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// ---------------------------------------------------------------------------
// /sessions/{id} sub-routes
// ---------------------------------------------------------------------------

func (g *Server) handleBridgeSessionRouter(w http.ResponseWriter, r *http.Request) {
	// Strip "/sessions/" prefix and parse the rest.
	rest := strings.TrimPrefix(r.URL.Path, "/sessions/")
	if rest == "" {
		jsonError(w, "session id required", http.StatusBadRequest)
		return
	}

	// Split into id and action.
	id, action, _ := strings.Cut(rest, "/")

	switch action {
	case "":
		g.handleBridgeGetSession(w, r, id)
	case "send":
		g.handleBridgeSend(w, r, id)
	case "events":
		g.handleBridgeEvents(w, r, id)
	case "history":
		g.handleBridgeHistory(w, r, id)
	case "stop":
		g.handleBridgeStop(w, r, id)
	case "interrupt":
		g.handleBridgeInterrupt(w, r, id)
	default:
		jsonError(w, "unknown action", http.StatusNotFound)
	}
}

// GET /sessions/{id}
func (g *Server) handleBridgeGetSession(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	val, ok := g.sessions.Load(id)
	if !ok {
		jsonError(w, "session not found", http.StatusNotFound)
		return
	}
	s := val.(*Session)
	s.mu.Lock()
	info := &SessionInfo{
		Key:        s.Key,
		Agent:      s.AgentName,
		Status:     s.Status,
		SpawnDepth: s.SpawnDepth,
		ParentKey:  s.ParentKey,
		Children:   s.Children,
		CreatedAt:  s.CreatedAt,
		LastActive: s.LastActive,
		Messages:   len(s.Engine.Messages),
	}
	s.mu.Unlock()
	jsonResponse(w, sessionInfoToBridge(info))
}

// POST /sessions/{id}/send
func (g *Server) handleBridgeSend(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Message == "" {
		jsonError(w, "message required", http.StatusBadRequest)
		return
	}

	// Resolve agent from session key.
	agentName := agentFromSessionKey(id)

	// Build a RunRequest targeting this specific session.
	runReq := RunRequest{
		Agent:      agentName,
		Message:    req.Message,
		SessionKey: id,
	}

	// If the client wants SSE, stream bridge-format events.
	if r.Header.Get("Accept") == "text/event-stream" {
		g.handleBridgeSendStream(w, r, runReq)
		return
	}

	// Synchronous: run and return a bridge-format result event.
	resp, err := g.Run(r.Context(), runReq)
	if err != nil {
		g.logError(agentName, "bridge", req.Message, err)
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, map[string]string{"status": "sent", "text": resp.Text})
}

// handleBridgeSendStream streams msg.Event over SSE.
func (g *Server) handleBridgeSendStream(w http.ResponseWriter, r *http.Request, req RunRequest) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		jsonError(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	err := g.Stream(r.Context(), req, func(event StreamEvent) {
		bridgeEvent := streamEventToBridge(event, req.SessionKey)
		data, _ := json.Marshal(bridgeEvent)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", bridgeEvent.Type, data)
		flusher.Flush()
	})
	if err != nil {
		g.logError(req.Agent, "bridge-stream", req.Message, err)
		errEvent := msg.Event{
			Type:      msg.EventError,
			Harness:   "inber",
			SessionID: req.SessionKey,
			Timestamp: time.Now(),
			Error:     &msg.ErrorEvent{Message: err.Error()},
		}
		data, _ := json.Marshal(errEvent)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", msg.EventError, data)
		flusher.Flush()
	}
}

// GET /sessions/{id}/events — persistent SSE subscription
func (g *Server) handleBridgeEvents(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	val, ok := g.sessions.Load(id)
	if !ok {
		jsonError(w, "session not found", http.StatusNotFound)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		jsonError(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Create a channel to receive events from this session.
	events := make(chan StreamEvent, 100)
	sess := val.(*Session)

	// Wrap the session's existing onEvent to fan out to our subscriber.
	sess.mu.Lock()
	origOnEvent := sess.onEvent
	sess.onEvent = func(e StreamEvent) {
		if origOnEvent != nil {
			origOnEvent(e)
		}
		select {
		case events <- e:
		default:
		}
	}
	sess.updateHooks()
	sess.mu.Unlock()

	defer func() {
		sess.mu.Lock()
		sess.onEvent = origOnEvent
		sess.updateHooks()
		sess.mu.Unlock()
	}()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				w.Write([]byte("event: close\ndata: {}\n\n"))
				flusher.Flush()
				return
			}
			bridgeEvent := streamEventToBridge(event, id)
			data, _ := json.Marshal(bridgeEvent)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", bridgeEvent.Type, data)
			flusher.Flush()
		}
	}
}

// GET /sessions/{id}/history
func (g *Server) handleBridgeHistory(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rows, err := g.store.RecentRequests(id, 100)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Convert request history to msg.Event list.
	var events []json.RawMessage
	for _, row := range rows {
		// User message event.
		if row.InputText != nil && *row.InputText != "" {
			userEvent := msg.Event{
				Type:      "user_message",
				Harness:   "inber",
				SessionID: id,
				Timestamp: timeOrNow(row.StartedAt),
				Result:    &msg.ResultEvent{Text: *row.InputText},
			}
			if data, err := json.Marshal(userEvent); err == nil {
				events = append(events, data)
			}
		}

		// Assistant result event.
		if row.OutputText != nil && *row.OutputText != "" {
			resultEvent := msg.Event{
				Type:      msg.EventResult,
				Harness:   "inber",
				SessionID: id,
				Timestamp: timeOrNow(row.CompletedAt),
				Result: &msg.ResultEvent{
					Text:     *row.OutputText,
					NumTurns: row.Turns,
					Usage: msg.TokenUsage{
						InputTokens:      row.InputTokens,
						OutputTokens:     row.OutputTokens,
						CacheReadTokens:  row.CacheReadTokens,
						CacheWriteTokens: row.CacheWriteTokens,
					},
					Cost: &msg.Cost{TotalUSD: row.Cost},
				},
			}
			if data, err := json.Marshal(resultEvent); err == nil {
				events = append(events, data)
			}
		}
	}

	if events == nil {
		events = []json.RawMessage{}
	}
	jsonResponse(w, events)
}

// POST /sessions/{id}/stop
func (g *Server) handleBridgeStop(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := g.StopSession(id); err != nil {
		jsonError(w, err.Error(), http.StatusNotFound)
		return
	}

	jsonResponse(w, bridgeSession{
		ID:      id,
		Harness: "inber",
		State:   string(msg.SessionAborted),
	})
}

// POST /sessions/{id}/interrupt
func (g *Server) handleBridgeInterrupt(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := g.StopSession(id); err != nil {
		jsonError(w, err.Error(), http.StatusNotFound)
		return
	}

	jsonResponse(w, bridgeSession{
		ID:      id,
		Harness: "inber",
		State:   string(msg.SessionIdle),
	})
}

// ---------------------------------------------------------------------------
// Type conversion helpers
// ---------------------------------------------------------------------------

// streamEventToBridge converts inber's StreamEvent to llm-bridge's msg.Event.
func streamEventToBridge(e StreamEvent, sessionID string) msg.Event {
	bridgeEvent := msg.Event{
		Harness:   "inber",
		SessionID: sessionID,
		Timestamp: time.Now(),
	}

	switch e.Kind {
	case "delta":
		bridgeEvent.Type = msg.EventStream
		bridgeEvent.Stream = &msg.HarnessStream{
			Delta: &msg.BlockDelta{
				Type: msg.DeltaText,
				Text: e.Text,
			},
		}

	case "thinking":
		bridgeEvent.Type = msg.EventThinking
		bridgeEvent.Thinking = &msg.ThinkingEvent{Text: e.Text}

	case "tool_call":
		bridgeEvent.Type = msg.EventToolCall
		bridgeEvent.ToolCall = &msg.ToolCallEvent{
			Name:  e.Tool,
			Input: json.RawMessage(e.Text),
		}

	case "tool_result":
		bridgeEvent.Type = msg.EventToolResult
		bridgeEvent.ToolResult = &msg.ToolResultEvent{
			Name:   e.Tool,
			Output: e.Text,
		}

	case "done":
		bridgeEvent.Type = msg.EventResult
		result := &msg.ResultEvent{Text: e.Text}
		if data, ok := e.Data.(map[string]any); ok {
			if tokens, ok := data["tokens"].(TokenUsage); ok {
				result.Usage = msg.TokenUsage{
					InputTokens:      tokens.Input,
					OutputTokens:     tokens.Output,
					CacheReadTokens:  tokens.CacheRead,
					CacheWriteTokens: tokens.CacheWrite,
				}
				result.Cost = &msg.Cost{TotalUSD: tokens.Cost}
			}
			if ms, ok := data["duration_ms"].(int64); ok {
				result.DurationMS = ms
			}
		}
		bridgeEvent.Result = result

	case "status":
		bridgeEvent.Type = msg.EventSystem
		bridgeEvent.System = &msg.SystemEvent{
			Subtype: "status",
			Message: e.Text,
		}

	case "error":
		bridgeEvent.Type = msg.EventError
		bridgeEvent.Error = &msg.ErrorEvent{Message: e.Text}

	default:
		bridgeEvent.Type = msg.EventSystem
		bridgeEvent.System = &msg.SystemEvent{
			Subtype: e.Kind,
			Message: e.Text,
		}
	}

	return bridgeEvent
}

// countSessionsByState counts sessions grouped by state.
func (g *Server) countSessionsByState() (running, idle, completed int) {
	g.sessions.Range(func(_, val any) bool {
		s := val.(*Session)
		s.mu.Lock()
		switch s.Status {
		case Running:
			running++
		case Idle:
			idle++
		case Completed:
			completed++
		}
		s.mu.Unlock()
		return true
	})
	return
}

// agentFromSessionKey extracts agent name from session keys like "agent:NAME:main".
func agentFromSessionKey(key string) string {
	parts := strings.SplitN(key, ":", 3)
	if len(parts) >= 2 {
		return parts[1]
	}
	return ""
}

func timeOrNow(t *time.Time) time.Time {
	if t != nil {
		return *t
	}
	return time.Now()
}
