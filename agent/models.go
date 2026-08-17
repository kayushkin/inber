package agent

import (
	"log"
	"sync"

	modelstore "github.com/kayushkin/model-store"
)

// ModelInfo describes a model with metadata for tracking.
type ModelInfo struct {
	ID              string  // e.g., "claude-sonnet-4-5-20250929"
	ContextWindow   int     // max tokens
	InputCostPer1M  float64 // cost per 1M input tokens
	OutputCostPer1M float64 // cost per 1M output tokens
}

// Context window and prices used when the model-store cannot answer for a
// model. They are deliberately not zero: a zero context window means "no
// overflow guard" to Agent.SetContextWindow, so answering an unknown model with
// zero would turn a missing registry row into a silently unguarded request.
// They are the values the registry itself carries for a mid-range model, and
// unresolvedModelReported makes each miss say so once.
const (
	unknownModelContextWindow   = 200000
	unknownModelInputCostPer1M  = 3.00
	unknownModelOutputCostPer1M = 15.00
)

// anthropicProviderID is the provider id the model-store seeds Anthropic under.
const anthropicProviderID = "anthropic"

// anthropicContextWithoutTheLongContextBeta is the largest context an Anthropic
// request can carry without the long-context beta header. clients.go lists
// the betas inber sends and that one is not among them, so a registry row
// claiming a million tokens for an Anthropic model states what the *model* can
// do, not what *this client* can ask for.
//
// Guarding at the row's number would be worse than the hardcode it replaces:
// the conversation grows past what a request can carry, the API rejects the
// turn, and agent_run.go's single retry at half the window is still over the
// real ceiling, so the turn fails outright instead of being pruned. Capping
// keeps the guard honest without deciding whether to buy the beta's premium
// long-context pricing — that decision belongs to whoever pays for it, and
// taking it would raise the ceiling here on its own.
const anthropicContextWithoutTheLongContextBeta = 200000

// unresolvedModelReported remembers which model ids have already been reported
// as unresolvable, so a per-turn cost calculation on an unknown model warns once
// rather than on every call.
var unresolvedModelReported sync.Map

// GetModelInfo returns what the model-store knows about a model: the context
// window to guard requests against and the per-million-token prices to bill
// them at. It falls back to the unknownModel* constants above, loudly, when the
// registry has no row for the model.
//
// The lookup goes through Store.ResolveModel, which keys on the model's **id**
// and then on its aliases. It used to scan AllModelsWithStatus for a row whose
// display *name* equalled the id it was handed, and no Anthropic row can
// satisfy that: the id is "claude-sonnet-4-20250514" and the name is "Claude
// Sonnet 4". So every Anthropic model — every model inber actually runs on —
// missed the registry and took the unknown-model fallback. That is why inber
// billed Haiku at Sonnet's $3/$15, twelve times its registered $0.25/$1.25, and
// why every model on the box was priced the same whatever it cost.
//
// The context window comes from the registry too. The comment this replaces
// claimed the store did not carry one; it does, as Model.MaxTokens, filled from
// each provider's own API by `ms sync`. Of the 56 rows in the registry on this
// host only 11 are actually 200,000 — 26 are 400,000, 14 are a million or more,
// and 5 are 128,000. That last group is the direction that costs a request
// rather than wasting a window: Agent.BeforeRequest prunes at the window it is
// told, so a 128,000-token model told it had 200,000 is never pruned before the
// API rejects the turn. The window is bounded by what inber's client for that
// provider can request — see requestableContextWindow.
func GetModelInfo(modelID string, store *modelstore.Store) ModelInfo {
	if store == nil {
		return unknownModelInfo(modelID)
	}

	model, err := store.ResolveModel(modelID)
	if err != nil {
		reportUnresolvedModel(modelID, err)
		return unknownModelInfo(modelID)
	}
	if model.MaxTokens <= 0 {
		// The row exists but does not state a window. Prices are still the
		// registry's, only the window falls back — reporting zero here would
		// disarm the overflow guard for a model the registry does know.
		reportModelWithoutContextWindow(model.ID)
		return ModelInfo{
			ID:              model.ID,
			ContextWindow:   unknownModelContextWindow,
			InputCostPer1M:  model.InputCost,
			OutputCostPer1M: model.OutputCost,
		}
	}

	return ModelInfo{
		ID:              model.ID,
		ContextWindow:   requestableContextWindow(model),
		InputCostPer1M:  model.InputCost,
		OutputCostPer1M: model.OutputCost,
	}
}

// requestableContextWindow returns the model's registered window bounded by
// what inber's client for that provider can actually ask for. Only Anthropic is
// bounded today, and only upward — see
// anthropicContextWithoutTheLongContextBeta. A row smaller than the bound is
// taken as it stands, which is the direction that matters most: it arms the
// overflow guard earlier than the old hardcode did.
func requestableContextWindow(model *modelstore.Model) int {
	if model.Provider != anthropicProviderID {
		return model.MaxTokens
	}
	if model.MaxTokens <= anthropicContextWithoutTheLongContextBeta {
		return model.MaxTokens
	}
	reportCappedContextWindow(model.ID, model.MaxTokens)
	return anthropicContextWithoutTheLongContextBeta
}

func unknownModelInfo(modelID string) ModelInfo {
	return ModelInfo{
		ID:              modelID,
		ContextWindow:   unknownModelContextWindow,
		InputCostPer1M:  unknownModelInputCostPer1M,
		OutputCostPer1M: unknownModelOutputCostPer1M,
	}
}

func reportUnresolvedModel(modelID string, err error) {
	if _, alreadyReported := unresolvedModelReported.LoadOrStore(modelID, struct{}{}); alreadyReported {
		return
	}
	log.Printf("[models] model-store has no row for %q (%v); using the unknown-model window %d and prices $%.2f/$%.2f per 1M",
		modelID, err, unknownModelContextWindow, unknownModelInputCostPer1M, unknownModelOutputCostPer1M)
}

// reportCappedContextWindow says, once per model, that inber is guarding a
// smaller context than the registry records, and why.
func reportCappedContextWindow(modelID string, registeredWindow int) {
	if _, alreadyReported := unresolvedModelReported.LoadOrStore(modelID, struct{}{}); alreadyReported {
		return
	}
	log.Printf("[models] %q is registered at %d tokens but inber does not send Anthropic's long-context beta; guarding at %d",
		modelID, registeredWindow, anthropicContextWithoutTheLongContextBeta)
}

// reportModelWithoutContextWindow says, once per model, that the registry knows
// the model but not how big its context is. It shares unresolvedModelReported
// with reportUnresolvedModel: both mean "this model id is not fully answered",
// and a model can only be in one of the two states.
func reportModelWithoutContextWindow(modelID string) {
	if _, alreadyReported := unresolvedModelReported.LoadOrStore(modelID, struct{}{}); alreadyReported {
		return
	}
	log.Printf("[models] model-store row %q states no context window; guarding requests at the unknown-model %d instead",
		modelID, unknownModelContextWindow)
}

// DefaultModel is the model used when none is specified.
const DefaultModel = "claude-sonnet-4-20250514"
