package server

import (
	"encoding/json"
	"net/http"
)

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