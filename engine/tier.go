package engine

// TierHigh is the expensive/complex reasoning tier.
const TierHigh = "high"

// TierLow is the cheap/routine execution tier.
const TierLow = "low"

// Note: Tier racing has been replaced by health-based failover (failover.go).
// The tiers config is still used to define the fallback chain.
// High models are tried first, low models are fallbacks.
