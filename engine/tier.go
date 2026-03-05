package engine

// TierHigh is the expensive/complex reasoning tier.
const TierHigh = "high"

// TierLow is the cheap/routine tier.
const TierLow = "low"

// SetTier switches the active model tier. Use TierHigh for planning/complex
// reasoning, TierLow for routine tool calls and simple responses.
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

// activeModels returns the model list for the current tier.
// Falls back to the single e.Model if no tiers configured.
func (e *Engine) activeModels() []string {
	if e.tiers == nil {
		return []string{e.Model}
	}
	switch e.activeTier {
	case TierHigh:
		if len(e.tiers.High) > 0 {
			return e.tiers.High
		}
	case TierLow:
		if len(e.tiers.Low) > 0 {
			return e.tiers.Low
		}
	}
	// Fallback to single model
	return []string{e.Model}
}
