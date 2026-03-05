package engine

// TierHigh is the expensive/complex reasoning tier.
const TierHigh = "high"

// TierLow is the cheap/routine execution tier.
const TierLow = "low"

// escalateThreshold is how many consecutive errors trigger an automatic
// escalation from low → high tier.
const escalateThreshold = 3

// SetTier manually switches the active model tier.
func (e *Engine) SetTier(tier string) {
	if e.tiers == nil {
		return
	}
	if tier != TierHigh && tier != TierLow {
		Log.Warn("unknown tier %q, ignoring", tier)
		return
	}
	if tier != e.activeTier {
		Log.Info("tier: %s → %s", e.activeTier, tier)
		e.activeTier = tier
	}
}

// ActiveTier returns the current tier name.
func (e *Engine) ActiveTier() string {
	if e.tiers == nil {
		return ""
	}
	return e.activeTier
}

// autoTier determines the tier for this turn based on state:
//   - Turn 1: always high (planning)
//   - Consecutive errors >= 3: escalate to high (stuck, need bigger model)
//   - Otherwise: low (execution)
//
// After a successful turn following escalation, drops back to low.
func (e *Engine) autoTier() {
	if e.tiers == nil {
		return
	}

	prev := e.activeTier

	switch {
	case e.TurnCounter <= 1:
		// First turn is always high — make the plan
		e.activeTier = TierHigh

	case e.consecutiveErrors >= escalateThreshold:
		// Stuck with repeated errors — escalate
		if e.activeTier != TierHigh {
			Log.Warn("tier: escalating to high (%d consecutive errors)", e.consecutiveErrors)
		}
		e.activeTier = TierHigh

	default:
		// Normal execution — use low tier
		// This also handles recovery: after escalation fixes the issue,
		// consecutiveErrors resets and we drop back to low
		e.activeTier = TierLow
	}

	if e.activeTier != prev {
		Log.Info("tier: %s → %s (turn %d, errors %d)", prev, e.activeTier, e.TurnCounter, e.consecutiveErrors)
	}
}

// activeModels returns the model list for the current tier.
// Falls back to the single e.Model if no tiers configured.
// High tier automatically appends the last model from Low tier as a
// final fallback (e.g. glm5) if not already present.
func (e *Engine) activeModels() []string {
	if e.tiers == nil {
		return []string{e.Model}
	}
	switch e.activeTier {
	case TierHigh:
		if len(e.tiers.High) > 0 {
			models := e.tiers.High
			// Append best low-tier model as last-resort fallback
			if len(e.tiers.Low) > 0 {
				fallback := e.tiers.Low[0]
				if models[len(models)-1] != fallback {
					models = append(append([]string{}, models...), fallback)
				}
			}
			return models
		}
	case TierLow:
		if len(e.tiers.Low) > 0 {
			return e.tiers.Low
		}
	}
	return []string{e.Model}
}
