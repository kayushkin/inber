package server

import (
	"fmt"
	"net/http"
	"strings"
)

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