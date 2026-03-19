package server

import (
	"encoding/json"
	"fmt"
	"net/http"
)

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
	// Accept "text" as alias for "message" (backward compat with bus-agent).
	if req.Message == "" && req.Text != "" {
		req.Message = req.Text
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