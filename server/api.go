package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
)

// Serve starts the HTTP API server. Blocks until ctx is cancelled.
func (g *Server) Serve(ctx context.Context) error {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/run", g.handleRun)
	mux.HandleFunc("/api/spawn", g.handleSpawn)
	mux.HandleFunc("/api/fork-spawn", g.handleForkSpawn)
	mux.HandleFunc("/api/sessions", g.handleSessions)
	mux.HandleFunc("/api/sessions/", g.handleSessionDetail)
	mux.HandleFunc("/api/requests/", g.handleRequests)
	mux.HandleFunc("/api/models", g.handleModels)
	mux.HandleFunc("/api/models/test", g.handleModelTest)

	server := &http.Server{
		Addr:    g.config.ListenAddr,
		Handler: mux,
	}

	// Shutdown on context cancellation.
	go func() {
		<-ctx.Done()
		server.Shutdown(context.Background())
	}()

	log.Printf("[server] API listening on %s", g.config.ListenAddr)
	log.Printf("[server] %d agents configured, default=%s", len(g.config.Agents), g.config.DefaultAgent)

	err := server.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

// POST /api/run
func (g *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Message == "" {
		jsonError(w, "message required", http.StatusBadRequest)
		return
	}

	// Check if client wants streaming (SSE).
	if r.Header.Get("Accept") == "text/event-stream" {
		g.handleRunStream(w, r, req)
		return
	}

	resp, err := g.Run(r.Context(), req)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, resp)
}

// handleRunStream sends SSE events during a streaming run.
func (g *Server) handleRunStream(w http.ResponseWriter, r *http.Request, req RunRequest) {
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
		data, _ := json.Marshal(event)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Kind, data)
		flusher.Flush()
	})
	if err != nil {
		data, _ := json.Marshal(StreamEvent{Kind: "error", Text: err.Error()})
		fmt.Fprintf(w, "event: error\ndata: %s\n\n", data)
		flusher.Flush()
	}
}

// POST /api/spawn
func (g *Server) handleSpawn(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SpawnRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.ParentKey == "" || req.Agent == "" || req.Task == "" {
		jsonError(w, "parent_key, agent, and task required", http.StatusBadRequest)
		return
	}

	resp, err := g.Spawn(r.Context(), req)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, resp)
}

// POST /api/fork-spawn
func (g *Server) handleForkSpawn(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ParentKey string         `json:"parent_key"`
		Tasks     []SpawnRequest `json:"tasks"`
		Model     string         `json:"model,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.ParentKey == "" || len(req.Tasks) == 0 {
		jsonError(w, "parent_key and tasks required", http.StatusBadRequest)
		return
	}

	// Apply model override to all tasks.
	for i := range req.Tasks {
		if req.Model != "" && req.Tasks[i].Model == "" {
			req.Tasks[i].Model = req.Model
		}
	}

	responses, err := g.ForkAndSpawn(r.Context(), req.ParentKey, req.Tasks)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, responses)
}

// SessionWithRequests combines in-memory session info with recent request history.
type SessionWithRequests struct {
	*SessionInfo
	ActiveRequest  *RequestRow   `json:"active_request,omitempty"`
	RecentRequests []RequestRow  `json:"recent_requests,omitempty"`
}

// GET /api/sessions
func (g *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessions := g.ListSessions()

	// Enrich with DB data if requested.
	if r.URL.Query().Get("requests") == "true" {
		var enriched []SessionWithRequests
		for _, s := range sessions {
			swr := SessionWithRequests{SessionInfo: s}
			if active, _ := g.store.ActiveRequest(s.Key); active != nil {
				swr.ActiveRequest = active
			}
			if recent, _ := g.store.RecentRequests(s.Key, 5); len(recent) > 0 {
				swr.RecentRequests = recent
			}
			enriched = append(enriched, swr)
		}
		jsonResponse(w, enriched)
		return
	}

	jsonResponse(w, sessions)
}

// GET/DELETE /api/sessions/:key
func (g *Server) handleSessionDetail(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	if key == "" {
		jsonError(w, "session key required", http.StatusBadRequest)
		return
	}

	// Handle inject.
	if strings.HasSuffix(key, "/inject") {
		key = strings.TrimSuffix(key, "/inject")
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var body struct {
			Message string `json:"message"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if err := g.Inject(key, body.Message); err != nil {
			jsonError(w, err.Error(), http.StatusNotFound)
			return
		}
		jsonResponse(w, map[string]string{"status": "injected"})
		return
	}

	switch r.Method {
	case http.MethodGet:
		val, ok := g.sessions.Load(key)
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
		jsonResponse(w, info)

	case http.MethodDelete:
		if err := g.StopSession(key); err != nil {
			jsonError(w, err.Error(), http.StatusNotFound)
			return
		}
		jsonResponse(w, map[string]string{"status": "stopped"})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// GET /api/requests/:session_key — get request history for a session
func (g *Server) handleRequests(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionKey := strings.TrimPrefix(r.URL.Path, "/api/requests/")
	if sessionKey == "" {
		jsonError(w, "session key required", http.StatusBadRequest)
		return
	}

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}

	requests, err := g.store.RecentRequests(sessionKey, limit)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, requests)
}

// GET /api/models — proxy to model store
func (g *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if g.modelStore == nil {
		jsonError(w, "model store not available", http.StatusServiceUnavailable)
		return
	}
	statuses, err := g.modelStore.AllModelsWithStatus()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, statuses)
}

// POST /api/models/test
func (g *Server) handleModelTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// TODO: implement model testing
	jsonError(w, "not implemented", http.StatusNotImplemented)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func jsonResponse(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
