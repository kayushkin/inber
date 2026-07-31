package engine

import (
	"testing"

	"github.com/kayushkin/inber/agent"
	modelstore "github.com/kayushkin/model-store"
)

// clientFor builds a ModelClient the way the engine does, using the env-var
// credential path (modelStore and authStore nil), so these tests need no
// network, no fixtures and no mocking.
func clientFor(t *testing.T, modelID string) *agent.ModelClient {
	t.Helper()
	mc, err := agent.NewModelClient(modelID, nil, nil)
	if err != nil {
		t.Fatalf("building a client for %s: %v", modelID, err)
	}
	return mc
}

// TestResolveModelClient_KeepsTheModelItCanActuallyRun is the regression test for
// the defect: a failover to a model with no credentials used to leave the old
// client installed while relabelling the turn with the new model, so the request
// went to the old provider naming a model it does not serve and the *fallback*
// was recorded unhealthy for it.
func TestResolveModelClient_KeepsTheModelItCanActuallyRun(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-key")
	t.Setenv("OPENAI_API_KEY", "")

	e := &Engine{modelClient: clientFor(t, "claude-sonnet-4-5")}

	// Failover picks an OpenAI model; this host has no OpenAI credentials.
	inForce := e.resolveModelClient("gpt-4o")

	if inForce != "claude-sonnet-4-5" {
		t.Fatalf("turn reported as running on %q, but no client for it could be built; "+
			"recordModelHealth would mark it unhealthy for an error it never caused", inForce)
	}
	if e.modelClient == nil || e.modelClient.Model == nil {
		t.Fatal("the previously installed client was dropped")
	}
	if e.modelClient.Model.ID != inForce {
		t.Fatalf("installed client is for %q but the turn is labelled %q — the two must agree",
			e.modelClient.Model.ID, inForce)
	}
}

// TestResolveModelClient_InstallsClientForANewModel is the complement: when the
// credentials *are* there, failover must actually take effect. Without this a
// resolver that always returned the incumbent would pass the test above.
func TestResolveModelClient_InstallsClientForANewModel(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-key")
	t.Setenv("OPENAI_API_KEY", "sk-openai-test-key")

	e := &Engine{modelClient: clientFor(t, "claude-sonnet-4-5")}

	inForce := e.resolveModelClient("gpt-4o")

	if inForce != "gpt-4o" {
		t.Fatalf("credentials exist for gpt-4o, so the failover should have taken it; got %q", inForce)
	}
	if e.modelClient.Model.ID != "gpt-4o" {
		t.Fatalf("client not swapped: still on %q", e.modelClient.Model.ID)
	}
	if !e.modelClient.IsOpenAI() {
		t.Fatal("installed client is not the OpenAI one, so executeAgent would route this turn to Anthropic")
	}
}

// TestResolveModelClient_ReusesTheInstalledClient pins that an unchanged model
// does not rebuild — a rebuild re-resolves credentials and stands up another
// HTTP client on every turn.
func TestResolveModelClient_ReusesTheInstalledClient(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-key")

	before := clientFor(t, "claude-sonnet-4-5")
	e := &Engine{modelClient: before}

	inForce := e.resolveModelClient("claude-sonnet-4-5")

	if inForce != "claude-sonnet-4-5" {
		t.Fatalf("got %q", inForce)
	}
	if e.modelClient != before {
		t.Fatal("client was rebuilt for a model that had not changed")
	}
}

// TestResolveModelClient_NoIncumbentKeepsTheDefaultClientPath pins the
// backward-compatible path unchanged: with no client installed, buildAgent falls
// back to the engine's own Anthropic client, which really does send `selected`,
// so `selected` is the honest label even though the build failed.
func TestResolveModelClient_NoIncumbentKeepsTheDefaultClientPath(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	e := &Engine{}

	inForce := e.resolveModelClient("gpt-4o")

	if inForce != "gpt-4o" {
		t.Fatalf("with no incumbent client the selected model is what actually gets sent; got %q", inForce)
	}
	if e.modelClient != nil {
		t.Fatal("a failed build must not install a client")
	}
}

// TestResolveModelClient_RebuildsWhenTheIncumbentHasNoModel covers the guard the
// old inline condition got backwards: `e.modelClient.Model != nil && ID != selected`
// evaluated false for a client carrying no model, so such a client was reused for
// every model forever. agent.NewModelClient always sets Model, so this is not
// reachable today; the test states the intent so a second constructor cannot
// reintroduce it silently.
func TestResolveModelClient_RebuildsWhenTheIncumbentHasNoModel(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-key")

	e := &Engine{modelClient: &agent.ModelClient{Provider: "anthropic"}}

	inForce := e.resolveModelClient("claude-sonnet-4-5")

	if inForce != "claude-sonnet-4-5" {
		t.Fatalf("got %q", inForce)
	}
	if e.modelClient.Model == nil || e.modelClient.Model.ID != "claude-sonnet-4-5" {
		t.Fatal("a client with no model must be replaced, not reused")
	}
}

// TestNewModelClient_FailsOnAMissingProviderKey pins the premise the whole fix
// rests on: the build failure this guards against is a *credential* failure, and
// it is the one a cross-provider failover chain walks into.
func TestNewModelClient_FailsOnAMissingProviderKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")

	if _, err := agent.NewModelClient("gpt-4o", nil, nil); err == nil {
		t.Fatal("expected a credential error for a provider with no key configured")
	}
}

// TestResolveModelClient_KeepsHealthAttributionHonest states the consequence in
// the terms model-store sees, because that is the shared state the defect
// corrupted: whatever executeAgent passes to recordModelHealth must name the
// model the request was actually sent as.
func TestResolveModelClient_KeepsHealthAttributionHonest(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "sk-ant-test-key")
	t.Setenv("OPENAI_API_KEY", "")

	e := &Engine{modelClient: clientFor(t, "claude-sonnet-4-5")}

	// executeAgent's sequence, minus the API call.
	selected := "gpt-4o"
	modelUsed := e.resolveModelClient(selected)
	e.Model = modelUsed

	// recordModelHealth is called with modelUsed; assert it cannot be the model
	// whose client failed to build.
	if modelUsed == selected {
		t.Fatal("health would be recorded against gpt-4o, which this turn never reached")
	}
	if e.Model != modelUsed {
		t.Fatalf("e.Model (%q) and the health key (%q) disagree", e.Model, modelUsed)
	}

	// And the request itself carries e.Model, so it must name the installed
	// client's provider.
	var _ *modelstore.Model = e.modelClient.Model
	if e.modelClient.Model.ID != e.Model {
		t.Fatalf("request would carry model %q on a client for %q", e.Model, e.modelClient.Model.ID)
	}
}
