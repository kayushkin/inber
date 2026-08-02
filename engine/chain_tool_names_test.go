package engine

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/tools"
	toolstoretools "github.com/kayushkin/tool-store/tools"
)

// engineToolNames is every name a tool the engine can install actually answers
// to, derived from the constructors rather than restated. This mirrors
// guard.knownToolNames, which exists for the same reason: a renamed tool must
// move this set, not slip past it.
//
// The throwaway arguments are never used — only Name is read — so the nil store
// is never dereferenced.
func engineToolNames(t *testing.T) map[string]bool {
	t.Helper()

	names := make(map[string]bool)

	// tool-store auto-registers its zero-argument tools.
	for _, impl := range toolstoretools.All() {
		names[impl.Name] = true
	}
	// The engine's own default set, plus the context-taking tools it adds.
	engine := &Engine{}
	for _, built := range engine.buildDefaultTools() {
		names[built.Name] = true
	}
	for _, name := range []string{
		tools.RepoMap("", nil).Name,
		tools.RecentFiles("").Name,
		tools.Deploy().Name,
	} {
		names[name] = true
	}
	return names
}

// TestChainIsOfferedOnEveryToolTheEngineBuilds is the engine-side half of
// agent.TestTurnTerminatorSpecialCaseIsDormant. package agent cannot import
// package tools — tools imports agent — so the agent-side test restates the
// engine's tool names; this one derives them from the constructors and fails if
// that restatement has gone stale, or if end_turn ever becomes a real tool.
//
// AddChainAndSidebandFields withholds the chain field from one name and one name
// only. Nothing registers a tool under it, so today every tool the engine builds
// carries the field. Registering end_turn is a model-visible contract change and
// belongs to todo 87399239; this test makes it impossible to do by accident.
func TestChainIsOfferedOnEveryToolTheEngineBuilds(t *testing.T) {
	known := engineToolNames(t)
	if len(known) == 0 {
		t.Fatal("derived no tool names — this test would pass without checking anything")
	}

	// Each fixture carries a real properties map. A tool built with a nil map
	// cannot receive the chain field at all — injectFields returns the schema
	// untouched — so a nil-schema fixture would make every tool below look
	// withheld-or-skipped and this test would assert nothing.
	built := make([]agent.Tool, 0, len(known))
	for name := range known {
		built = append(built, agent.Tool{Name: name, InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{"path": map[string]any{"type": "string"}},
		}})
	}

	prepared := agent.AddChainAndSidebandFields(built)
	if len(prepared) != len(built) {
		t.Fatalf("preparation returned %d tools for %d inputs", len(prepared), len(built))
	}

	for _, one := range prepared {
		props, ok := one.InputSchema.Properties.(map[string]any)
		if !ok {
			t.Fatalf("%s: properties are not a map after preparation", one.Name)
		}
		_, present := props["then"]
		// end_turn is in this set because tool-store's catalogue publishes it,
		// not because inber installs it — TestEndTurnIsNotReachableFromAnyBuildPath
		// is the assertion that it is unreachable here. So it is the one name
		// that must be withheld, and every other name must be offered.
		switch {
		case one.Name == "end_turn" && present:
			t.Errorf("the chain field was offered to %q, which is the one name it must be withheld from — there is nothing to run after the end of a turn", one.Name)
		case one.Name != "end_turn" && !present:
			t.Errorf("the chain field was withheld from %q; only end_turn is meant to be withheld", one.Name)
		}
	}
}

// TestEndTurnIsNotReachableFromAnyBuildPath states the fact the dormant special
// case rests on, at the four places a tool can enter the engine. It is the
// measurement that turned "end_turn is a load-bearing name" from a claim into a
// defect: the schema was telling the model to call it on every single tool call.
func TestEndTurnIsNotReachableFromAnyBuildPath(t *testing.T) {
	const endTurn = "end_turn"
	engine := &Engine{}

	for _, built := range engine.buildDefaultTools() {
		if built.Name == endTurn {
			t.Errorf("buildDefaultTools now installs %q", endTurn)
		}
	}
	for _, built := range tools.All() {
		if built.Name == endTurn {
			t.Errorf("tools.All now includes %q", endTurn)
		}
	}
	for _, built := range tools.AllFromRegistry() {
		if built.Name == endTurn {
			t.Errorf("the default registry now includes %q", endTurn)
		}
	}
	if engine.findStandardTool(endTurn) != nil {
		t.Errorf("findStandardTool now resolves %q, so an agent config listing it would receive it", endTurn)
	}
	if engine.buildSpecialTool(endTurn) != nil {
		t.Errorf("buildSpecialTool now builds %q", endTurn)
	}
}
