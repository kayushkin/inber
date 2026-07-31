package agent_test

import (
	"path/filepath"
	"testing"

	"github.com/kayushkin/inber/agent"
	modelstore "github.com/kayushkin/model-store"
)

// registryShapedLikeTheLiveOne builds a store carrying the exact shape the
// registry has on this host: an id that is the API model id and a name that is
// a human display string. The two differing is the whole point — a lookup that
// keys on the name cannot find any of these rows.
func registryShapedLikeTheLiveOne(t *testing.T) *modelstore.Store {
	t.Helper()

	store, err := modelstore.Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	for _, provider := range []modelstore.Provider{
		{ID: "anthropic", Name: "Anthropic"},
		{ID: "openai", Name: "OpenAI"},
	} {
		if err := store.AddProvider(provider); err != nil {
			t.Fatalf("add provider %s: %v", provider.ID, err)
		}
	}

	models := []modelstore.Model{
		// An Anthropic row larger than what inber's client can request.
		{ID: "claude-sonnet-4-20250514", Provider: "anthropic", Name: "Claude Sonnet 4",
			Aliases: []string{"sonnet"}, MaxTokens: 1_000_000, InputCost: 3.0, OutputCost: 15.0, Enabled: true},
		// A non-Anthropic row larger than the old hardcode, which inber's
		// OpenAI client can request in full.
		{ID: "gpt-5.1-2025-11-13", Provider: "openai", Name: "GPT-5.1",
			MaxTokens: 400_000, InputCost: 1.25, OutputCost: 10.0, Enabled: true},
		// A window smaller than the old hardcode — the direction that loses a
		// request.
		{ID: "gpt-4o-2024-11-20", Provider: "openai", Name: "GPT-4o",
			MaxTokens: 128_000, InputCost: 2.5, OutputCost: 10.0, Enabled: true},
		// A model whose real window IS the old hardcode, so the assertions
		// below cannot pass by simply never returning 200000.
		{ID: "claude-haiku-4-5-20251001", Provider: "anthropic", Name: "Claude Haiku 4.5",
			MaxTokens: 200_000, InputCost: 0.8, OutputCost: 4.0, Enabled: true},
		// A row that states no window at all.
		{ID: "local-model", Provider: "openai", Name: "Local Model",
			MaxTokens: 0, InputCost: 0, OutputCost: 0, Enabled: true},
	}
	for _, m := range models {
		if err := store.AddModel(m); err != nil {
			t.Fatalf("add model %s: %v", m.ID, err)
		}
	}
	return store
}

// TestGetModelInfoFindsARowWhoseNameIsNotItsID is the defect this file was
// written for: keyed on the display name, every Anthropic model missed.
func TestGetModelInfoFindsARowWhoseNameIsNotItsID(t *testing.T) {
	store := registryShapedLikeTheLiveOne(t)

	info := agent.GetModelInfo("claude-haiku-4-5-20251001", store)

	if info.InputCostPer1M != 0.8 || info.OutputCostPer1M != 4.0 {
		t.Fatalf("prices = $%.2f/$%.2f per 1M, want the registry's $0.80/$4.00 — "+
			"$3.00/$15.00 means the lookup missed the row and took the unknown-model fallback",
			info.InputCostPer1M, info.OutputCostPer1M)
	}
	if info.ID != "claude-haiku-4-5-20251001" {
		t.Fatalf("ID = %q, want the canonical registry id", info.ID)
	}
}

// TestGetModelInfoReportsTheRegistrysContextWindow pins the second half: the
// window is the registry's, not a constant. Both directions are asserted,
// because only the small one can lose a request.
func TestGetModelInfoReportsTheRegistrysContextWindow(t *testing.T) {
	store := registryShapedLikeTheLiveOne(t)

	for _, tc := range []struct {
		modelID string
		want    int
	}{
		{"gpt-5.1-2025-11-13", 400_000},
		{"gpt-4o-2024-11-20", 128_000},
		// The complement: a model whose window really is 200,000 still gets
		// 200,000, so these assertions are not just "anything but the old
		// hardcode".
		{"claude-haiku-4-5-20251001", 200_000},
	} {
		if got := agent.GetModelInfo(tc.modelID, store).ContextWindow; got != tc.want {
			t.Errorf("%s context window = %d, want %d", tc.modelID, got, tc.want)
		}
	}
}

// TestGetModelInfoCapsAnAnthropicWindowAtWhatTheClientCanRequest — the registry
// records what the model can do; inber does not send the long-context beta, so
// guarding at a million tokens would grow a conversation past what the request
// can carry and lose the turn rather than prune it.
func TestGetModelInfoCapsAnAnthropicWindowAtWhatTheClientCanRequest(t *testing.T) {
	store := registryShapedLikeTheLiveOne(t)

	if got := agent.GetModelInfo("claude-sonnet-4-20250514", store).ContextWindow; got != 200_000 {
		t.Fatalf("context window = %d, want the requestable 200000", got)
	}
	// The cap is Anthropic-specific: a larger non-Anthropic row is untouched,
	// so this is a bound on the client and not a second hardcode.
	if got := agent.GetModelInfo("gpt-5.1-2025-11-13", store).ContextWindow; got != 400_000 {
		t.Fatalf("non-Anthropic context window = %d, want the registry's 400000", got)
	}
	// The prices are the registry's either way — capping the window must not
	// drag the model back onto the unknown-model fallback.
	if info := agent.GetModelInfo("claude-sonnet-4-20250514", store); info.ID != "claude-sonnet-4-20250514" {
		t.Fatalf("ID = %q, want the canonical registry id", info.ID)
	}
}

// TestGetModelInfoResolvesAnAlias — ResolveModel is the registry's own
// id-then-alias resolver, and using it means an alias works for free.
func TestGetModelInfoResolvesAnAlias(t *testing.T) {
	store := registryShapedLikeTheLiveOne(t)

	info := agent.GetModelInfo("sonnet", store)

	if info.ID != "claude-sonnet-4-20250514" {
		t.Fatalf("ID = %q, want the alias resolved to its canonical id", info.ID)
	}
	if info.InputCostPer1M != 3.0 || info.OutputCostPer1M != 15.0 {
		t.Fatalf("prices = $%.2f/$%.2f, want the aliased row's $3.00/$15.00",
			info.InputCostPer1M, info.OutputCostPer1M)
	}
}

// TestGetModelInfoFallsBackForAModelTheRegistryDoesNotHave keeps the
// unknown-model path honest, and asserts the window is non-zero: zero means "no
// overflow guard" to Agent.SetContextWindow, so a missing row must not disarm
// the guard.
func TestGetModelInfoFallsBackForAModelTheRegistryDoesNotHave(t *testing.T) {
	store := registryShapedLikeTheLiveOne(t)

	info := agent.GetModelInfo("model-nobody-registered", store)

	if info.ContextWindow != 200_000 {
		t.Fatalf("context window = %d, want the unknown-model 200000", info.ContextWindow)
	}
	if info.InputCostPer1M != 3.0 || info.OutputCostPer1M != 15.0 {
		t.Fatalf("prices = $%.2f/$%.2f, want the unknown-model $3.00/$15.00",
			info.InputCostPer1M, info.OutputCostPer1M)
	}
}

// TestGetModelInfoKeepsTheGuardArmedForARowWithNoWindow — a row that states no
// window still supplies its prices, and the window falls back rather than
// reporting zero.
func TestGetModelInfoKeepsTheGuardArmedForARowWithNoWindow(t *testing.T) {
	store := registryShapedLikeTheLiveOne(t)

	info := agent.GetModelInfo("local-model", store)

	if info.ContextWindow != 200_000 {
		t.Fatalf("context window = %d, want the unknown-model 200000 rather than an unguarded 0",
			info.ContextWindow)
	}
	if info.InputCostPer1M != 0 || info.OutputCostPer1M != 0 {
		t.Fatalf("prices = $%.2f/$%.2f, want the registry's own zeros",
			info.InputCostPer1M, info.OutputCostPer1M)
	}
}

// TestGetModelInfoWithoutAStore — cost accounting runs with a nil store in
// places, and must not panic.
func TestGetModelInfoWithoutAStore(t *testing.T) {
	info := agent.GetModelInfo("claude-sonnet-4-20250514", nil)

	if info.ContextWindow != 200_000 || info.InputCostPer1M != 3.0 || info.OutputCostPer1M != 15.0 {
		t.Fatalf("nil store gave %+v, want the unknown-model defaults", info)
	}
}
