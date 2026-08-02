package engine

import (
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
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

// TestInjectedSpawnToolReplacesTheRegistryBuiltOne is the control for the gate
// below, and it is what makes not building the registry's copy safe.
//
// Two differently-shaped tools are declared under the one name spawn_agent:
// the registry's (agent, orchestrator, task) and the server's (agent, task,
// model, fork, timeout_seconds). In a server session both are produced — the
// registry's by buildSpecialTool, the server's through EngineConfig.ExtraTools
// — and mergeExtraTools resolves the collision. The outcome is not a race: an
// extra replaces the first base entry of its name, so the server's always wins
// and the registry's is thrown away after being fully constructed.
//
// This states that determinism directly. If it ever fails, the gate below is
// dropping a tool the model would otherwise have got.
func TestInjectedSpawnToolReplacesTheRegistryBuiltOne(t *testing.T) {
	registryShaped := agent.Tool{
		Name:        spawnToolName,
		Description: "Delegate a task to another agent.",
		InputSchema: anthropic.ToolInputSchemaParam{
			Required: []string{"agent", "task"},
			Properties: map[string]any{
				"agent": map[string]any{"type": "string"}, "orchestrator": map[string]any{"type": "string"}, "task": map[string]any{"type": "string"},
			},
		},
	}
	serverShaped := agent.Tool{
		Name:        spawnToolName,
		Description: "Spawn a sub-agent to work on a task.",
		InputSchema: anthropic.ToolInputSchemaParam{
			Required: []string{"agent", "task"},
			Properties: map[string]any{
				"agent": map[string]any{"type": "string"}, "task": map[string]any{"type": "string"},
				"model": map[string]any{"type": "string"}, "fork": map[string]any{"type": "boolean"},
				"timeout_seconds": map[string]any{"type": "integer"},
			},
		},
	}

	got := mergeExtraTools([]agent.Tool{{Name: "read_files"}, registryShaped, {Name: "write_files"}}, []agent.Tool{serverShaped})

	var spawns []agent.Tool
	for _, tool := range got {
		if tool.Name == spawnToolName {
			spawns = append(spawns, tool)
		}
	}
	if len(spawns) != 1 {
		t.Fatalf("got %d tools named %q on the wire, want 1 — the model cannot be shown two schemas for one name", len(spawns), spawnToolName)
	}
	props, _ := spawns[0].InputSchema.Properties.(map[string]any)
	if _, ok := props["timeout_seconds"]; !ok {
		t.Errorf("the surviving %q has no timeout_seconds, so it is the registry's copy; the server's injection is supposed to win", spawnToolName)
	}
	if _, ok := props["orchestrator"]; ok {
		t.Errorf("the surviving %q still carries orchestrator, so the registry's schema leaked through the merge", spawnToolName)
	}
}

// TestNoRegistryIsBuiltWhenTheCallerInjectsSpawn is the cost half of the gate,
// stated on the other input the gate was ignoring.
//
// needsSpawnTools reads the agent's configured tool list and nothing else, so a
// server session for an agent that lists spawn_agent built a registry — an
// agent-store load, a model-store dial, a logs directory, and a fully
// constructed spawn tool including two HTTP round trips for its description —
// and then mergeExtraTools threw that tool away, because the server injects its
// own spawn_agent. The registry has exactly one reader, the spawn case in
// buildSpecialTool, so once its tool is discarded nothing else can read it.
//
// The control above pins that the injected tool wins, which is what makes
// declining to build the other one lossless.
func TestNoRegistryIsBuiltWhenTheCallerInjectsSpawn(t *testing.T) {
	listsSpawn := &registry.AgentConfig{Tools: []string{"read_files", spawnToolName, "write_files"}}

	if !needsAgentRegistry(listsSpawn, nil) {
		t.Errorf("needsAgentRegistry is false with no injected tools; the config lists %q and nothing else would build it, so the model would get no spawn tool at all", spawnToolName)
	}

	serverInjection := []agent.Tool{
		{Name: spawnToolName}, {Name: "steer_agent"}, {Name: "agents_status"},
	}
	if needsAgentRegistry(listsSpawn, serverInjection) {
		t.Errorf("needsAgentRegistry is true although the caller injects %q; the registry's tool would be built and then discarded by mergeExtraTools, and nothing else reads the registry", spawnToolName)
	}

	// An injected set that does not carry the name leaves the gate alone.
	if !needsAgentRegistry(listsSpawn, []agent.Tool{{Name: "steer_agent"}, {Name: "merge_workspace"}}) {
		t.Errorf("needsAgentRegistry is false for an injected set with no %q in it; those tools collide with nothing and must not suppress the registry", spawnToolName)
	}

	// A config that never asked for it is still refused, injection or not.
	noSpawn := &registry.AgentConfig{Tools: []string{"read_files"}}
	for _, extras := range [][]agent.Tool{nil, serverInjection} {
		if needsAgentRegistry(noSpawn, extras) {
			t.Errorf("needsAgentRegistry is true for a config that does not list %q", spawnToolName)
		}
		if needsAgentRegistry(nil, extras) {
			t.Errorf("needsAgentRegistry is true for a nil agent config")
		}
	}
}
