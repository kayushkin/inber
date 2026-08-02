package engine

import (
	"testing"

	"github.com/kayushkin/inber/agent/registry"
)

// TestSpawnGateAdmitsOnlyBuildableNames is the assertion the two halves of the
// spawn path never had between them.
//
// setupAgentRegistry gates on needsSpawnTools; buildSpecialTool is the only
// thing that turns a configured name into a spawn tool, and the registry the
// gate builds is read at exactly one place inside it. So a name the gate admits
// and the builder does not answer to buys an agent registry — a dial to
// agent-store and model-store, a memory-store handle, a logs directory — that
// nothing can ever read, and still hands the model no spawn tool.
//
// "spawn_*" was such a name. It appeared in the gate and nowhere else in the
// repository; no code expands a wildcard in a tool list.
func TestSpawnGateAdmitsOnlyBuildableNames(t *testing.T) {
	// A registry instance is enough for buildSpecialTool to take the branch;
	// only the returned tool's presence is read here, never its behaviour.
	engine := &Engine{agentRegistry: &registry.Registry{}}

	// Every name that has ever been written into the spawn gate.
	for _, name := range []string{"spawn_agent", "spawn_*"} {
		admitted := needsSpawnTools([]string{name})
		buildable := engine.buildSpecialTool(name) != nil

		if admitted && !buildable {
			t.Errorf("needsSpawnTools admits %q but buildSpecialTool cannot build it — the registry this gate creates would be unreachable and the model would get no spawn tool", name)
		}
		if buildable && !admitted {
			t.Errorf("buildSpecialTool builds %q but needsSpawnTools does not admit it — the registry it needs is never created, so the tool silently falls out of the set", name)
		}
	}
}

// TestSpawnGateIsNotDuplicated states the property that made the divergence
// possible. Engine carried a second needsSpawnTools as a method, with no callers
// and a narrower condition than the live one: the package function admitted
// "spawn_*" and the method did not. Two same-named predicates that disagree are
// one edit away from the wrong one being called.
//
// There is nothing to assert about a deleted method, so this asserts the
// property that replaced it: the gate and the builder agree because they read
// the same constant.
func TestSpawnGateAndBuilderShareOneName(t *testing.T) {
	engine := &Engine{agentRegistry: &registry.Registry{}}

	if !needsSpawnTools([]string{spawnToolName}) {
		t.Errorf("needsSpawnTools rejects %q, the name the builder answers to", spawnToolName)
	}
	built := engine.buildSpecialTool(spawnToolName)
	if built == nil {
		t.Fatalf("buildSpecialTool cannot build %q, the name the gate admits", spawnToolName)
	}
	if built.Name != spawnToolName {
		t.Errorf("buildSpecialTool(%q) returned a tool named %q; the gate keys off the configured name, so the two must match", spawnToolName, built.Name)
	}
}

// TestNoRegistryIsBuiltForAnUnbuildableName is the cost half, stated on the
// gate rather than on the tool set: a config that names no buildable spawn tool
// must not construct a registry at all.
func TestNoRegistryIsBuiltForAnUnbuildableName(t *testing.T) {
	for _, tools := range [][]string{
		{"spawn_*"},
		{"read_files", "spawn_*"},
		{"read_files"},
		{},
	} {
		if needsSpawnTools(tools) {
			t.Errorf("needsSpawnTools(%v) is true, so setupAgentRegistry would build a registry; none of these names reaches the builder's spawn case", tools)
		}
	}
}
