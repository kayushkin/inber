package engine

import (
	"context"
	"errors"
	"time"

	"github.com/kayushkin/inber/agent"
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

	// When --model was explicitly set, honor it without failover
	if e.modelExplicitlySet {
		health := e.modelStore.GetHealth(preferred)
		return preferred, timeoutFromHealth(health, defaultTimeout)
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

// fallbackChain returns enabled models ordered by priority from the model store.
func (e *Engine) fallbackChain() []string {
	if e.modelStore == nil {
		return nil
	}
	models, err := e.modelStore.FailoverChain()
	if err != nil || len(models) == 0 {
		return nil
	}
	chain := make([]string, len(models))
	for i, m := range models {
		chain[i] = m.ID
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

// recordModelHealth updates health tracking after an API call, for those
// outcomes that are evidence about the model.
//
// The filter is load-bearing, not tidiness. model-store's health table is
// host-shared, persistent and read across processes, and a single error flips a
// model to unhealthy — model-store/health.go marks it down whenever the last
// outcome it recorded was an error, with no threshold and no decay. selectModel
// then fails over on that. So an error inber raised about itself, written here,
// degrades model selection for every session on the machine until some other
// session happens to record a success against the same model.
//
// An outcome carrying no evidence writes nothing at all rather than a success.
// There is no honest success available: durationMs is the wall clock from the
// start of the API work to the moment inber gave up, which for a cancelled or
// capped turn is when the user pressed stop or a policy clock expired, not a
// response time. AvgResponseMs is a timeout estimator (timeoutFromHealth), so
// handing it a fabricated duration is worse than handing it nothing.
func (e *Engine) recordModelHealth(turnContext context.Context, model string, durationMs int64, err error) {
	if e.modelStore == nil {
		return
	}
	if err != nil {
		if !errorIsEvidenceAboutTheModel(turnContext, err) {
			Log.Info("model health: leaving %s unchanged — %q is inber reporting on inber, not on the provider", model, err)
			return
		}
		e.modelStore.RecordError(model, err.Error())
		return
	}
	e.modelStore.RecordSuccess(model, durationMs)
}

// errorIsEvidenceAboutTheModel reports whether a failed turn says anything at
// all about the model that ran it or the provider serving it.
//
// Three of the errors that reach recordModelHealth are raised by inber about
// inber, and the provider may have been answering perfectly throughout:
//
//   - context.Canceled — the user pressed stop. It arrives from Agent.Run's
//     cancellation check, which sees it because InterruptSession calls
//     Session.cancel; that is what the stop button in dash and llm-bridge does.
//     A cancel is always somebody deciding to stop, never a provider's answer,
//     so it is excluded whatever the turn context says.
//   - context.DeadlineExceeded — excluded only when inber's own clock is the one
//     that ran out, which is what turnContext.Err() reports. See below.
//   - agent.ErrMaxAPICallsExceeded — inber's own runaway cap. Hitting it means
//     the provider answered every one of those calls.
//
// The deadline case has to ask whose clock fired, because inber runs two kinds
// and only one of them is its own. Its own are on the turn context: a sub-agent
// spawn timeout, a session's max_duration, the bus handler's cap. The other one
// is not on any context — agent/openai.go gives the OpenAI-compatible client a
// flat 120s http.Client.Timeout, which serves openai, google, openrouter, ollama
// and the catch-all for every provider inber does not name. When that fires,
// net/http returns an error satisfying errors.Is(err, context.DeadlineExceeded)
// while the turn context is still perfectly live.
//
// Reading that as inber-reporting-on-inber discards the only signal a hung
// provider on that path ever produces: nothing is recorded, the model never goes
// unhealthy, selectModel keeps choosing it, and every turn burns 120s forever
// without failover firing once. So the test is not what the error looks like, it
// is whether inber's own clock actually ran out.
//
// turnContext must be the context the turn ran under, and must not be nil: a nil
// one would answer "no clock of mine fired" for every deadline there is, which is
// exactly the reading this function exists to stop being automatic.
//
// Everything else is deliberately left to record an error, including "unexpected
// stop reason". Whether a refusal, or a pause_turn inber has no branch for, is a
// provider fault, a model fault or a gap in inber is an open question on todo
// 4c511c8f; deciding it here would settle it by accident.
func errorIsEvidenceAboutTheModel(turnContext context.Context, err error) bool {
	switch {
	case errors.Is(err, context.Canceled),
		errors.Is(err, agent.ErrMaxAPICallsExceeded):
		return false
	case errors.Is(err, context.DeadlineExceeded):
		return turnContext.Err() == nil
	}
	return true
}
