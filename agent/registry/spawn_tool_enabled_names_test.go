package registry

import (
	"strings"
	"testing"
)

func TestEnabledAgentNamesLeavesNoEmptySlots(t *testing.T) {
	// The exact regression: the old slice was sized to ALL agents and written only at
	// the enabled indices, so every disabled agent left a zero value behind and the
	// joined message read "alpha, , gamma".
	agents := []RegistryAgent{
		{Name: "alpha", Enabled: true},
		{Name: "beta", Enabled: false},
		{Name: "gamma", Enabled: true},
	}
	names := enabledAgentNames(agents)
	got := strings.Join(names, ", ")
	if got != "alpha, gamma" {
		t.Fatalf("want %q, got %q", "alpha, gamma", got)
	}
	for i, n := range names {
		if n == "" {
			t.Errorf("names[%d] is empty — a disabled agent left a hole in the list", i)
		}
	}
}

func TestEnabledAgentNamesExcludesDisabled(t *testing.T) {
	// The schema description used to list every agent while the validator accepted only
	// enabled ones, so the tool advertised names it would then reject. One function now
	// answers both questions, and this pins that it answers with the validator's rule.
	agents := []RegistryAgent{
		{Name: "live", Enabled: true},
		{Name: "retired", Enabled: false},
	}
	for _, name := range enabledAgentNames(agents) {
		if name == "retired" {
			t.Fatalf("a disabled agent must never be offered as a valid option: %#v", agents)
		}
	}
}

func TestEnabledAgentNamesEmptyWhenNoneEnabled(t *testing.T) {
	// Distinct from "the registry did not answer", but both must reach the generic
	// description rather than print "Valid options: " with nothing after it.
	agents := []RegistryAgent{
		{Name: "one", Enabled: false},
		{Name: "two", Enabled: false},
	}
	if names := enabledAgentNames(agents); len(names) != 0 {
		t.Fatalf("want no names when nothing is enabled, got %#v", names)
	}
	registry := &Registry{listRegisteredAgents: func() []RegistryAgent { return agents }}
	if got := registry.validAgentsDescription(); strings.Contains(got, "Valid options") {
		t.Errorf("description offered an empty option list: %q", got)
	}
}

func TestEnabledAgentNamesPreservesRegistryOrder(t *testing.T) {
	agents := []RegistryAgent{
		{Name: "c", Enabled: true},
		{Name: "a", Enabled: true},
		{Name: "b", Enabled: true},
	}
	if got := strings.Join(enabledAgentNames(agents), ","); got != "c,a,b" {
		t.Errorf("registry order must survive the filter, want %q got %q", "c,a,b", got)
	}
}

// The description the model is shown offers only the names the validator accepts.
func TestValidAgentsDescriptionOffersOnlyEnabledAgents(t *testing.T) {
	registry := &Registry{listRegisteredAgents: func() []RegistryAgent {
		return []RegistryAgent{{Name: "live", Enabled: true}, {Name: "retired", Enabled: false}}
	}}
	got := registry.validAgentsDescription()
	if !strings.Contains(got, "live") || strings.Contains(got, "retired") {
		t.Errorf("description = %q, want it to offer live and not retired", got)
	}
}
