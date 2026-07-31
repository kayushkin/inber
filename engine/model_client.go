package engine

import (
	"github.com/kayushkin/inber/agent"
)

// resolveModelClient makes e.modelClient match the model failover selected, and
// reports the model the turn will actually run on.
//
// The two answers have to be produced together. Everything downstream of a turn
// keys off the model string — the Anthropic request carries it (agent_run), the
// OpenAI/Anthropic routing branch reads the installed client, and
// recordModelHealth writes a success or an error against it in model-store,
// which is shared, persistent, cross-session state. So the string and the
// installed client must describe the same thing or every one of those readers is
// told something untrue.
//
// The case that forced this to be one function: building a client for the
// selected model can fail, and the commonest reason is that the model belongs to
// a provider this host has no credentials for (agent.NewModelClient returns
// "no credentials found for provider ..."). That is exactly what a failover
// hits, because a fallback chain crosses providers. Keeping the previously
// installed client while relabelling the turn with the model we could not build
// sends the old provider a request naming a model it has never heard of, and
// then records the *fallback* as unhealthy for the API's complaint about it — so
// one missing API key walks the chain and marks every model in it dead, with a
// LastError that never mentions credentials. Reporting the model actually in
// force keeps the health data true and leaves the fallback eligible to be
// retried once its credentials exist.
//
// The failure is not fatal: with no client installed at all the engine still has
// its own Anthropic client (see buildAgent), and that path is left exactly as it
// was. Either way the reason is logged rather than swallowed.
func (e *Engine) resolveModelClient(selected string) string {
	if e.modelClient != nil && e.modelClient.Model != nil && e.modelClient.Model.ID == selected {
		return selected
	}

	mc, err := agent.NewModelClient(selected, e.modelStore, e.authStore)
	if err == nil {
		e.modelClient = mc
		return selected
	}

	if e.modelClient != nil && e.modelClient.Model != nil {
		Log.Warn("no client for model %s (%v) — this turn runs on %s, and its health is recorded against that",
			selected, err, e.modelClient.Model.ID)
		return e.modelClient.Model.ID
	}

	// No client to fall back to. buildAgent uses the engine's own Anthropic
	// client here, which really does send `selected`, so the label is accurate.
	Log.Warn("no client for model %s (%v) — using the engine's default client", selected, err)
	return selected
}
