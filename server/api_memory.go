package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kayushkin/inber/memory"
)

// GET /api/memory/search?q=<query>&limit=<n>&agent=<name>
func (g *Server) handleMemorySearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		jsonError(w, "q parameter required", http.StatusBadRequest)
		return
	}

	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	store, err := g.memoryStoreForRequest(r)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer store.Close()

	results, err := store.Search(query, limit)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, results)
}

// handleMemoryList handles GET (list) and POST (save) on /api/memory.
func (g *Server) handleMemoryList(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		g.handleMemorySave(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	var minImportance float64
	if v := r.URL.Query().Get("min_importance"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			minImportance = f
		}
	}

	store, err := g.memoryStoreForRequest(r)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer store.Close()

	results, err := store.ListRecent(limit, minImportance)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, results)
}

// handleMemoryDetail routes GET and DELETE for /api/memory/<id>.
func (g *Server) handleMemoryDetail(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		g.handleMemoryGet(w, r)
	case http.MethodDelete:
		g.handleMemoryForget(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// GET /api/memory/<id>?agent=<name>
func (g *Server) handleMemoryGet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/memory/")
	if id == "" {
		jsonError(w, "memory id required", http.StatusBadRequest)
		return
	}

	store, err := g.memoryStoreForRequest(r)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer store.Close()

	m, err := store.Get(id)
	if err != nil {
		jsonError(w, err.Error(), http.StatusNotFound)
		return
	}
	jsonResponse(w, m)
}

// POST /api/memory — save a memory
// Body: {"content": "...", "tags": ["a", "b"], "importance": 0.5, "source": "user", "agent": "claxon"}
func (g *Server) handleMemorySave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Content    string   `json:"content"`
		Tags       []string `json:"tags"`
		Importance float64  `json:"importance"`
		Source     string   `json:"source"`
		Agent      string   `json:"agent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Content == "" {
		jsonError(w, "content required", http.StatusBadRequest)
		return
	}

	workspace := g.workspaceForAgent(req.Agent)
	if workspace == "" {
		jsonError(w, "unknown agent or no workspace configured", http.StatusBadRequest)
		return
	}

	store, err := memory.OpenOrCreate(workspace)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer store.Close()

	// An absent source stays absent. This used to become "user", so an HTTP
	// caller that said nothing about provenance had its memory recorded as
	// something the user had said — the most trusted value the field carries.
	// memory-store's own handler had the identical guess and dropped it for the
	// same reason; the two must not disagree about what an empty source means,
	// because they write into the same store.
	if req.Importance == 0 {
		req.Importance = 0.5
	}

	m := memory.Memory{
		ID:         uuid.New().String(),
		Content:    req.Content,
		Tags:       req.Tags,
		Importance: req.Importance,
		Source:     req.Source,
	}

	if err := store.Save(m); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, map[string]string{"id": m.ID})
}

// DELETE /api/memory/<id>?agent=<name>
func (g *Server) handleMemoryForget(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/memory/")
	if id == "" {
		jsonError(w, "memory id required", http.StatusBadRequest)
		return
	}

	store, err := g.memoryStoreForRequest(r)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer store.Close()

	if err := store.Forget(id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, map[string]string{"status": "forgotten"})
}

// GET /api/memory/stats?agent=<name>
func (g *Server) handleMemoryStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	store, err := g.memoryStoreForRequest(r)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer store.Close()

	all, err := store.ListRecent(10000, 0)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	tagCounts := make(map[string]int)
	var totalImportance float64
	importanceBuckets := make(map[string]int)

	for _, m := range all {
		for _, tag := range m.Tags {
			tagCounts[tag]++
		}
		totalImportance += m.Importance
		bucket := strconv.Itoa(int(m.Importance * 10))
		importanceBuckets[bucket]++
	}

	var avgImportance float64
	if len(all) > 0 {
		avgImportance = totalImportance / float64(len(all))
	}

	jsonResponse(w, map[string]any{
		"total":              len(all),
		"average_importance": avgImportance,
		"tags":               tagCounts,
		"importance_buckets": importanceBuckets,
	})
}

// POST /api/memory/compact?agent=<name>&age=<duration>&min_access=<n>
func (g *Server) handleMemoryCompact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	age := 7 * 24 * time.Hour // default 7 days
	if v := r.URL.Query().Get("age"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			age = d
		}
	}

	minAccess := 2
	if v := r.URL.Query().Get("min_access"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			minAccess = n
		}
	}

	store, err := g.memoryStoreForRequest(r)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer store.Close()

	results, err := store.Compact(age, minAccess)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, results)
}

// POST /api/memory/prune?agent=<name>&dry_run=true
func (g *Server) handleMemoryPrune(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	dryRun := r.URL.Query().Get("dry_run") == "true"

	store, err := g.memoryStoreForRequest(r)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer store.Close()

	all, err := store.ListRecent(1000, 0)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var prunable []memory.Memory
	for _, m := range all {
		if m.Importance < 0.3 && m.AccessCount < 2 {
			prunable = append(prunable, m)
		}
	}

	if !dryRun {
		for _, m := range prunable {
			store.Forget(m.ID)
		}
	}

	jsonResponse(w, map[string]any{
		"prunable": len(prunable),
		"dry_run":  dryRun,
		"pruned":   !dryRun,
	})
}

// POST /api/memory/decay?agent=<name>
func (g *Server) handleMemoryDecay(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	store, err := g.memoryStoreForRequest(r)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer store.Close()

	if err := store.DecayImportance(); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, map[string]string{"status": "decay_applied"})
}

// memoryStoreForRequest opens a memory store for the agent specified in the request.
func (g *Server) memoryStoreForRequest(r *http.Request) (memory.MemoryStore, error) {
	agentName := r.URL.Query().Get("agent")
	workspace := g.workspaceForAgent(agentName)
	if workspace == "" {
		// Fall back to default agent's workspace.
		workspace = g.workspaceForAgent(g.config.DefaultAgent)
	}
	if workspace == "" {
		return nil, fmt.Errorf("no workspace found for agent %q", agentName)
	}
	return memory.OpenOrCreate(workspace)
}

// workspaceForAgent returns the workspace path for the given agent name.
func (g *Server) workspaceForAgent(name string) string {
	if ac, ok := g.config.Agents[name]; ok {
		return ac.Workspace
	}
	return ""
}
