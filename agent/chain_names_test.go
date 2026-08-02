package agent

import (
	"regexp"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// A tool name is load-bearing control flow in several places in this repository
// and nothing has been checking that those names refer to a tool that exists.
// guard/classification_test.go already learned this once — isDangerous kept
// matching write_file after tool-store renamed it write_files, so Assist mode
// would have waved writes through — and wrote the rule down: "a name no tool has
// is not a safe default, it is a hole". That check was never applied outside
// package guard.
//
// The chain schema is the site where the hole is not latent. Its descriptions
// are read by the model on every single tool call, so a tool name written there
// is an instruction, and an instruction naming a tool that cannot be dispatched
// is paid for on every turn the model obeys it.

// toolNameShaped matches a snake_case identifier of the shape every tool in this
// repository uses (read_files, shell_commands, memory_search, end_turn). Ordinary
// English prose in these descriptions contains no such token, so anything this
// finds is a tool name — which is the whole point: a name in prose cannot be
// checked against the enum unless something goes looking for it.
var toolNameShaped = regexp.MustCompile(`\b[a-z]+(?:_[a-z]+)+\b`)

// descriptionsIn collects every "description" string reachable in a schema map,
// at any depth, so a name added to a nested property is covered too.
func descriptionsIn(node any) []string {
	var found []string
	switch typed := node.(type) {
	case map[string]any:
		for key, value := range typed {
			if key == "description" {
				if text, ok := value.(string); ok {
					found = append(found, text)
				}
				continue
			}
			found = append(found, descriptionsIn(value)...)
		}
	case []any:
		for _, element := range typed {
			found = append(found, descriptionsIn(element)...)
		}
	}
	return found
}

// TestThenDescriptionsNameOnlyOfferedTools is the assertion that was missing.
//
// buildThenSchema's two descriptions told the model, in prose, to chain a tool
// called end_turn. Nothing registers a tool under that name: tools.All(),
// engine.buildDefaultTools and the DefaultRegistry all omit it, and
// engine.findStandardTool returns nil for it, so even an agent config that lists
// end_turn does not receive it. The name therefore never appeared in this
// schema's own enum.
//
// A model that obeyed the instruction sent then={"tool":"end_turn"}, which
// executeWithChain resolves against toolMap, misses, and reports back as
// `--- then(end_turn) not run: no tool of that name ---`. That note is handed to
// hooks.OnToolResult flagged as an error whenever the primary call succeeded,
// and engine's OnToolResult increments Turn.ConsecutiveErrors, which drives the
// error-recovery context ladder in engine.contextBudget: one error widens memory
// recall from 6,000 tokens to 20,000, three to 35,000, five to 50,000 — and each
// widening rewrites the cached system-prompt prefix, so the whole prompt is paid
// for again. Following the instruction was billed as failing.
func TestThenDescriptionsNameOnlyOfferedTools(t *testing.T) {
	offered := []string{"read_files", "write_files", "shell_commands", "repo_map"}
	inEnum := make(map[string]bool, len(offered))
	for _, name := range offered {
		inEnum[name] = true
	}

	schema := buildThenSchema(offered)

	descriptions := descriptionsIn(schema)
	if len(descriptions) == 0 {
		t.Fatal("found no descriptions in the then schema — this test would pass without checking anything")
	}

	for _, description := range descriptions {
		for _, named := range toolNameShaped.FindAllString(description, -1) {
			if !inEnum[named] {
				t.Errorf("a then-schema description tells the model to use %q, which is not in the enum of dispatchable tools %v — the model cannot obey it, and the failed chain is billed as an error\ndescription: %s", named, offered, description)
			}
		}
	}
}

// TestThenDescriptionsNameNoToolAtAll is the stronger form, and it is the one
// that survives a change to which tools an agent happens to hold. The enum is
// built per agent from the tools it was given; prose is fixed at compile time.
// So a tool name in prose is correct for at most one tool set and wrong for
// every other one, whatever today's set contains.
func TestThenDescriptionsNameNoToolAtAll(t *testing.T) {
	// A tool set sharing no name with any other, so nothing in prose can be
	// accidentally justified by happening to appear in this enum.
	schema := buildThenSchema([]string{"alpha_one", "beta_two"})

	for _, description := range descriptionsIn(schema) {
		if named := toolNameShaped.FindAllString(description, -1); len(named) > 0 {
			t.Errorf("a then-schema description names tools %v in prose; the enum is per-agent and prose is not, so a name written here is wrong for every tool set that lacks it\ndescription: %s", named, description)
		}
	}
}

// TestTurnTerminatorIsNotRegisteredAnywhere pins the fact the special case in
// AddChainAndSidebandFields depends on, so that registering end_turn — a
// model-visible contract change, and the open half of todo 87399239 — cannot
// happen silently. If this test starts failing, the special case has become
// live and the comment above the constant needs rewriting, not deleting.
//
// package agent cannot import package tools (tools imports agent), so this
// asserts the property from agent's own side: the constant names a tool the
// chain builder never sees, so the branch that withholds the chain field from
// it has never been taken.
func TestTurnTerminatorSpecialCaseIsDormant(t *testing.T) {
	// Every tool the engine can hand this package today. Restated here rather
	// than imported because of the dependency direction — package tools imports
	// package agent, so agent cannot import it back.
	//
	// A restated list can go stale, so it is not what protects the property.
	// engine.TestChainIsOfferedOnEveryToolTheEngineBuilds derives the real set
	// from the constructors and asserts the same thing there; this test's job is
	// to state the property on agent's own side, where the branch lives.
	engineTools := []string{
		"shell_commands", "read_files", "write_files", "edit_files", "list_files",
		"repo_map", "recent_files", "deploy", "spawn_agent", "task_plan",
		"scratchpad", "web_search", "web_fetch", "browser", "scheduler",
	}

	for _, name := range engineTools {
		if name == turnTerminatorToolName {
			t.Fatalf("%q is now a tool the engine builds, so the chain-withholding special case is live; update the constant's comment rather than removing it", turnTerminatorToolName)
		}
	}

	// And the branch's observable effect: with no end_turn in the set, every
	// tool gets the chain field.
	tools := make([]Tool, 0, len(engineTools))
	for _, name := range engineTools {
		tools = append(tools, Tool{Name: name, InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{"path": map[string]any{"type": "string"}},
		}})
	}
	for _, prepared := range AddChainAndSidebandFields(tools) {
		props, ok := prepared.InputSchema.Properties.(map[string]any)
		if !ok {
			t.Fatalf("%s: properties are not a map after preparation", prepared.Name)
		}
		if _, present := props[chainField]; !present {
			t.Errorf("%s did not get the %q field, but only %q is meant to be withheld", prepared.Name, chainField, turnTerminatorToolName)
		}
	}
}

// TestChainFieldStaysOptional guards the reason "omit this field" is honest
// advice. If "then" ever became required, telling the model to omit it would be
// a second instruction it cannot follow.
func TestChainFieldStaysOptional(t *testing.T) {
	prepared := AddChainAndSidebandFields([]Tool{{
		Name: "read_files",
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{"path": map[string]any{"type": "string"}},
			Required:   []string{"path"},
		},
	}})

	for _, required := range prepared[0].InputSchema.Required {
		if required == chainField {
			t.Fatalf("%q is a required property, so the descriptions telling the model to omit it are instructing it to send an invalid call", chainField)
		}
	}

	description, _ := buildThenSchema([]string{"read_files"})["description"].(string)
	if !strings.Contains(description, "Omit") {
		t.Errorf("the then description no longer tells the model what to do when there is no follow-up: %q", description)
	}
}
