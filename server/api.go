package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
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
	// mux.HandleFunc("/api/models/test", g.handleModelTest) // removed: stub
	mux.HandleFunc("/api/agents", g.handleAgents)

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