package server

import (
	"context"
	"time"

	"github.com/kayushkin/inber/logger"
)

// authSyncLoop periodically imports OAuth tokens from OpenClaw's auth-profiles.json
// into model-store. This ensures inber always uses the latest tokens that OpenClaw
// may have refreshed independently via pi-ai.
func (g *Server) authSyncLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := g.modelStore.SyncFromAuthProfiles("")
			if err != nil {
				logger.WithComponent("auth-sync").Warn("sync from auth-profiles failed", map[string]interface{}{
					"error": err,
				})
				continue
			}
			if n > 0 {
				logger.WithComponent("auth-sync").Info("synced credentials from auth-profiles", map[string]interface{}{
					"updated": n,
				})
				// Push back so both sides stay consistent
				if syncErr := g.modelStore.SyncToAuthProfiles(""); syncErr != nil {
					logger.WithComponent("auth-sync").Warn("sync to auth-profiles failed", map[string]interface{}{
						"error": syncErr,
					})
				}
			}
		}
	}
}
