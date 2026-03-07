package engine

import (
	"time"

	modelstore "github.com/kayushkin/model-store"
)

const healthWindow = 30 * time.Minute

// selectModel picks the best available model based on health data.
// Returns the model to use and a timeout hint based on observed response times.
//
// Strategy:
//  1. If preferred model is healthy (responded in last 30min, no recent error) → use it
//  2. If preferred model is unknown (never tried) → use it (give it a chance)
//  3. If preferred model is unhealthy → try fallbacks in order, pick first healthy one
//  4. If nothing is healthy → try preferred anyway (everything might be down)
func (e *Engine) selectModel() (model string, timeoutHint time.Duration) {
	preferred := e.Model
	defaultTimeout := 3 * time.Minute

	if e.modelStore == nil {
		return preferred, defaultTimeout
	}

	health := e.modelStore.GetHealth(preferred)

	// Healthy or unknown → use it
	if health.IsHealthy(healthWindow) || health.IsUnknown() {
		timeout := timeoutFromHealth(health, defaultTimeout)
		return preferred, timeout
	}

	// Preferred is unhealthy — try fallbacks
	Log.Warn("model %s is unhealthy (last error: %s), trying fallbacks", preferred, health.LastError)

	for _, fallback := range e.fallbackChain() {
		if fallback == preferred {
			continue
		}
		fh := e.modelStore.GetHealth(fallback)
		if fh.IsHealthy(healthWindow) || fh.IsUnknown() {
			Log.Info("failover: %s → %s", preferred, fallback)
			return fallback, timeoutFromHealth(fh, defaultTimeout)
		}
	}

	// Nothing healthy — try preferred anyway
	Log.Warn("no healthy fallbacks, using %s anyway", preferred)
	return preferred, defaultTimeout
}

// fallbackChain returns the ordered list of fallback models.
// Prefers the DB chain (enabled models by priority), falls back to tiers config.
func (e *Engine) fallbackChain() []string {
	// Try DB-driven chain first
	if e.modelStore != nil {
		models, err := e.modelStore.FailoverChain()
		if err == nil && len(models) > 0 {
			chain := make([]string, len(models))
			for i, m := range models {
				chain[i] = m.ID
			}
			return chain
		}
	}

	// Fallback to tiers config
	if e.tiers == nil {
		return nil
	}
	var chain []string
	seen := make(map[string]bool)
	for _, m := range e.tiers.High {
		if !seen[m] {
			chain = append(chain, m)
			seen[m] = true
		}
	}
	for _, m := range e.tiers.Low {
		if !seen[m] {
			chain = append(chain, m)
			seen[m] = true
		}
	}
	return chain
}

// timeoutFromHealth calculates a reasonable timeout based on observed response times.
// Uses 3x the average response time, clamped between 30s and 5min.
func timeoutFromHealth(h *modelstore.ModelHealth, defaultTimeout time.Duration) time.Duration {
	if h == nil || h.AvgResponseMs == 0 {
		return defaultTimeout
	}
	timeout := time.Duration(h.AvgResponseMs*3) * time.Millisecond
	if timeout < 30*time.Second {
		timeout = 30 * time.Second
	}
	if timeout > 5*time.Minute {
		timeout = 5 * time.Minute
	}
	return timeout
}

// recordModelHealth updates health tracking after an API call.
func (e *Engine) recordModelHealth(model string, durationMs int64, err error) {
	if e.modelStore == nil {
		return
	}
	if err != nil {
		e.modelStore.RecordError(model, err.Error())
	} else {
		e.modelStore.RecordSuccess(model, durationMs)
	}
}
