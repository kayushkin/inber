package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/kayushkin/inber/logger"
)

// Serve starts the HTTP API server. Blocks until ctx is cancelled.
func (g *Server) Serve(ctx context.Context) error {
	// Background auth sync: pick up OpenClaw's refreshed OAuth tokens every 30 min.
	if g.modelStore != nil {
		go g.authSyncLoop(ctx)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/api/run", g.handleRun)
	mux.HandleFunc("/api/spawn", g.handleSpawn)
	mux.HandleFunc("/api/fork-spawn", g.handleForkSpawn)
	mux.HandleFunc("/api/sessions", g.handleSessions)
	mux.HandleFunc("/api/sessions/history", g.handleSessionHistory)
	mux.HandleFunc("/api/sessions/", g.handleSessionDetail)
	mux.HandleFunc("/api/requests/", g.handleRequests)
	mux.HandleFunc("/api/models", g.handleModels)
	// mux.HandleFunc("/api/models/test", g.handleModelTest) // removed: stub
	mux.HandleFunc("/api/agents", g.handleAgents)
	mux.HandleFunc("/api/agents/config", g.handleAgentConfig)
	mux.HandleFunc("/api/memory", g.handleMemoryList)
	mux.HandleFunc("/api/memory/search", g.handleMemorySearch)
	mux.HandleFunc("/api/memory/stats", g.handleMemoryStats)
	mux.HandleFunc("/api/memory/compact", g.handleMemoryCompact)
	mux.HandleFunc("/api/memory/prune", g.handleMemoryPrune)
	mux.HandleFunc("/api/memory/decay", g.handleMemoryDecay)
	mux.HandleFunc("/api/memory/", g.handleMemoryDetail)
	mux.HandleFunc("/api/health", g.handleHealth)

	// llm-bridge compatible endpoints (root-level, no /api prefix).
	mux.HandleFunc("/health", g.handleBridgeHealth)
	mux.HandleFunc("/harnesses", g.handleBridgeHarnesses)
	mux.HandleFunc("/sessions", g.handleBridgeSessions)
	mux.HandleFunc("/sessions/", g.handleBridgeSessionRouter)

	server := &http.Server{
		Addr:    g.config.ListenAddr,
		Handler: mux,
	}

	// Shutdown on context cancellation.
	go func() {
		<-ctx.Done()
		server.Shutdown(context.Background())
	}()

	logger.WithComponent("api").Info("API server starting", map[string]interface{}{
		"address":      g.config.ListenAddr,
		"agent_count":  len(g.config.Agents),
		"default_agent": g.config.DefaultAgent,
	})

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