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

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/llm-bridge/msg"
)

// ---------------------------------------------------------------------------
// GET /health — llm-bridge format
// ---------------------------------------------------------------------------

type bridgeHealthResponse struct {
	Status    string                `json:"status"`
	Harnesses []bridgeHarnessStatus `json:"harnesses"`
	Sessions  bridgeSessionCounts   `json:"sessions"`
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

// bridgeSession mirrors llm-bridge's ManagedSession JSON shape (3-ID model).
type bridgeSession struct {
	SessionID   string    `json:"session_id"`
	HarnessID   string    `json:"harness_id,omitempty"`
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
		SessionID:   s.Key,
		HarnessID:   s.Key,
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

		now := time.Now()
		sess := bridgeSession{
			SessionID:   sessionKey,
			DisplayName: req.DisplayName,
			Harness:     "inber",
			State:       string(msg.SessionIdle),
			AgentID:     agentName,
			CreatedAt:   now,
			UpdatedAt:   now,
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
	case "resume":
		g.handleBridgeResume(w, r, id)
	case "fork":
		g.handleBridgeFork(w, r, id)
	case "config":
		g.handleBridgeConfig(w, r, id)
	case "compact":
		g.handleBridgeCompact(w, r, id)
	case "discover":
		g.handleBridgeDiscover(w, r, id)
	case "messages":
		g.handleBridgeMessages(w, r, id)
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
			Type:            msg.EventError,
			Harness:         "inber",
			BridgeSessionID: req.SessionKey,
			Timestamp:       time.Now(),
			Error:           &msg.ErrorEvent{Message: err.Error()},
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
				Type:            "user_message",
				Harness:         "inber",
				BridgeSessionID: id,
				Timestamp:       timeOrNow(row.StartedAt),
				Result:          &msg.ResultEvent{Text: *row.InputText},
			}
			if data, err := json.Marshal(userEvent); err == nil {
				events = append(events, data)
			}
		}

		// Assistant result event.
		if row.OutputText != nil && *row.OutputText != "" {
			resultEvent := msg.Event{
				Type:            msg.EventResult,
				Harness:         "inber",
				BridgeSessionID: id,
				Timestamp:       timeOrNow(row.CompletedAt),
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
		SessionID: id,
		Harness:  "inber",
		State:    string(msg.SessionAborted),
	})
}

// POST /sessions/{id}/interrupt
func (g *Server) handleBridgeInterrupt(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := g.InterruptSession(id); err != nil {
		jsonError(w, err.Error(), http.StatusNotFound)
		return
	}

	jsonResponse(w, bridgeSession{
		SessionID: id,
		Harness:  "inber",
		State:    string(msg.SessionIdle),
	})
}

// ---------------------------------------------------------------------------
// POST /sessions/{id}/resume — reconnect to an idle session
// ---------------------------------------------------------------------------

func (g *Server) handleBridgeResume(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// If the session is already loaded, just return it.
	if val, ok := g.sessions.Load(id); ok {
		s := val.(*Session)
		s.mu.Lock()
		info := sessionInfoToBridge(&SessionInfo{
			Key:        s.Key,
			Agent:      s.AgentName,
			Status:     s.Status,
			SpawnDepth: s.SpawnDepth,
			ParentKey:  s.ParentKey,
			Children:   s.Children,
			CreatedAt:  s.CreatedAt,
			LastActive: s.LastActive,
			Messages:   len(s.Engine.Messages),
		})
		s.mu.Unlock()
		jsonResponse(w, info)
		return
	}

	// Not loaded — try to recreate from persisted messages.
	agentName := agentFromSessionKey(id)
	if agentName == "" {
		jsonError(w, "cannot determine agent from session key", http.StatusBadRequest)
		return
	}
	ac, ok := g.GetAgentConfig(agentName)
	if !ok {
		jsonError(w, fmt.Sprintf("unknown agent: %s", agentName), http.StatusBadRequest)
		return
	}

	sess, err := g.createSession(r.Context(), id, agentName, ac, RunRequest{}, nil)
	if err != nil {
		jsonError(w, fmt.Sprintf("resume failed: %v", err), http.StatusInternalServerError)
		return
	}

	actual, loaded := g.sessions.LoadOrStore(id, sess)
	if loaded {
		sess.close()
		sess = actual.(*Session)
	}

	jsonResponse(w, sessionInfoToBridge(&SessionInfo{
		Key:        sess.Key,
		Agent:      sess.AgentName,
		Status:     sess.Status,
		CreatedAt:  sess.CreatedAt,
		LastActive: sess.LastActive,
		Messages:   len(sess.Engine.Messages),
	}))
}

// ---------------------------------------------------------------------------
// POST /sessions/{id}/fork — branch a conversation
// ---------------------------------------------------------------------------

func (g *Server) handleBridgeFork(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	val, ok := g.sessions.Load(id)
	if !ok {
		jsonError(w, "session not found", http.StatusNotFound)
		return
	}
	parent := val.(*Session)

	var req struct {
		DisplayName string `json:"display_name,omitempty"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	childKey := sessionKeyForChild(id)
	agentName := parent.AgentName
	ac, _ := g.GetAgentConfig(agentName)

	child, err := g.forkSession(r.Context(), parent, childKey, agentName, ac, nil)
	if err != nil {
		jsonError(w, fmt.Sprintf("fork failed: %v", err), http.StatusInternalServerError)
		return
	}

	g.sessions.Store(childKey, child)
	g.store.UpsertSession(childKey, agentName, "fork")

	// Track as child of parent.
	parent.mu.Lock()
	parent.Children = append(parent.Children, childKey)
	parent.mu.Unlock()

	displayName := req.DisplayName
	if displayName == "" {
		displayName = agentName + " (fork)"
	}

	w.WriteHeader(http.StatusCreated)
	jsonResponse(w, bridgeSession{
		SessionID:   childKey,
		DisplayName: displayName,
		Harness:     "inber",
		State:       string(msg.SessionIdle),
		AgentID:     agentName,
		ParentID:    id,
		CreatedAt:   child.CreatedAt,
		UpdatedAt:   child.LastActive,
	})
}

// ---------------------------------------------------------------------------
// POST /sessions/{id}/config — update model, effort, tools, budget
// ---------------------------------------------------------------------------

func (g *Server) handleBridgeConfig(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	val, ok := g.sessions.Load(id)
	if !ok {
		jsonError(w, "session not found", http.StatusNotFound)
		return
	}
	s := val.(*Session)

	var req struct {
		Model         string   `json:"model,omitempty"`
		Effort        string   `json:"effort,omitempty"` // "high", "medium", "low" or raw token count
		DisabledTools []string `json:"disabled_tools,omitempty"`
		MaxBudget     int      `json:"max_budget,omitempty"` // max input tokens
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if req.Model != "" {
		s.Engine.SetModel(req.Model)
	}

	if req.Effort != "" {
		var budget int64
		switch req.Effort {
		case "high":
			budget = 32000
		case "medium":
			budget = 10000
		case "low":
			budget = 0
		default:
			// Try parsing as raw token count.
			if n, err := parseIntFromString(req.Effort); err == nil {
				budget = int64(n)
			}
		}
		s.Engine.SetThinkingBudget(budget)
	}

	if len(req.DisabledTools) > 0 {
		s.Engine.SetDisabledTools(req.DisabledTools)
	}

	if req.MaxBudget > 0 && s.Engine.Guard != nil {
		s.Engine.Guard.SetMaxInputTokens(req.MaxBudget)
	}

	jsonResponse(w, map[string]string{
		"status": "updated",
		"model":  s.Engine.Model,
	})
}

// ---------------------------------------------------------------------------
// POST /sessions/{id}/compact — compress conversation context
// ---------------------------------------------------------------------------

func (g *Server) handleBridgeCompact(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	val, ok := g.sessions.Load(id)
	if !ok {
		jsonError(w, "session not found", http.StatusNotFound)
		return
	}
	s := val.(*Session)

	var req struct {
		Summary string `json:"summary,omitempty"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	s.mu.Lock()
	before := len(s.Engine.Messages)
	removed, err := s.Engine.CompactContext(req.Summary)
	after := len(s.Engine.Messages)
	s.mu.Unlock()

	// Persist before reporting the error: a compaction can fail at the summary and
	// still have pruned, and the durable copy has to match what the engine now holds.
	g.persistSessionState(s)

	if err != nil {
		jsonError(w, fmt.Sprintf("compact failed: %v", err), http.StatusInternalServerError)
		return
	}

	jsonResponse(w, map[string]any{
		"status":           "compacted",
		"messages_before":  before,
		"messages_after":   after,
		"messages_removed": removed,
	})
}

// ---------------------------------------------------------------------------
// GET /sessions/{id}/discover — find stored sessions for this agent
// ---------------------------------------------------------------------------

func (g *Server) handleBridgeDiscover(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	agentName := agentFromSessionKey(id)

	// List all stored sessions and filter by agent.
	rows, err := g.store.ListSessions("")
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var filtered []any
	for _, row := range rows {
		if agentName == "" || row.Agent == agentName {
			filtered = append(filtered, row)
		}
	}
	if filtered == nil {
		filtered = []any{}
	}

	jsonResponse(w, filtered)
}

// ---------------------------------------------------------------------------
// GET /sessions/{id}/messages — materialized message list
// ---------------------------------------------------------------------------

func (g *Server) handleBridgeMessages(w http.ResponseWriter, r *http.Request, id string) {
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
	msgs := s.Engine.Messages
	s.mu.Unlock()

	// Return a lightweight materialized view.
	type materializedMsg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	var result []materializedMsg
	for _, m := range msgs {
		text := extractMessageText(m)
		result = append(result, materializedMsg{
			Role:    string(m.Role),
			Content: text,
		})
	}

	jsonResponse(w, result)
}

// ---------------------------------------------------------------------------
// Type conversion helpers
// ---------------------------------------------------------------------------

// extractMessageText extracts the text content from an anthropic MessageParam.
func extractMessageText(m anthropic.MessageParam) string {
	var parts []string
	for _, block := range m.Content {
		if block.OfText != nil {
			parts = append(parts, block.OfText.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// parseIntFromString tries to parse a string as an integer.
func parseIntFromString(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

// streamEventToBridge converts inber's StreamEvent to llm-bridge's msg.Event.
func streamEventToBridge(e StreamEvent, sessionID string) msg.Event {
	bridgeEvent := msg.Event{
		Harness:         "inber",
		BridgeSessionID: sessionID,
		Timestamp:       time.Now(),
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
