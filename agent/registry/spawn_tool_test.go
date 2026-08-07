package registry

import (
	"net/http"
	"strings"
	"testing"
)

// The response dash actually returns for registryAgentsURL. Kept verbatim rather than
// as a generic "not json" string because the specific failure this pins is a 200 whose
// body is a single-page-app shell: the status says success, so a decoder that trusts
// the status and not the body reads it as an empty registry.
const dashSPAFallbackBody = `<!doctype html>
<html lang="en">
  <head><title>Dash — Agent Dashboard</title></head>
  <body><div id="root"></div></body>
</html>`

func TestDecodeRegistryAgentsRejectsSPAFallback(t *testing.T) {
	agents, err := decodeRegistryAgents(http.StatusOK, []byte(dashSPAFallbackBody))
	if err == nil {
		t.Fatalf("a 200 carrying HTML must be an error, got agents=%#v", agents)
	}
	if agents != nil {
		t.Errorf("no agents may be returned alongside an error, got %#v", agents)
	}
	// The message has to name the status, because "200" is the surprising half of this
	// failure and a reader who sees only "invalid character '<'" will go looking for a
	// broken server rather than a missing route.
	if !strings.Contains(err.Error(), "200") {
		t.Errorf("error must name the status code it accepted, got %q", err)
	}
}

func TestDecodeRegistryAgentsRejectsNon2xx(t *testing.T) {
	// The old code read the body of every response identically, so a 401 whose body
	// failed to parse was reported the same as an empty registry. Each of these must
	// now be an error even when the body is a perfectly valid empty agent list.
	for _, status := range []int{
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusInternalServerError,
		http.StatusBadGateway,
	} {
		agents, err := decodeRegistryAgents(status, []byte(`[]`))
		if err == nil {
			t.Errorf("HTTP %d must be an error even with a decodable body, got agents=%#v", status, agents)
		}
		if agents != nil {
			t.Errorf("HTTP %d must return no agents, got %#v", status, agents)
		}
	}
}

func TestDecodeRegistryAgentsAcceptsSuccess(t *testing.T) {
	body := []byte(`[{"name":"alpha","orchestrator":"claude","enabled":true},
	                 {"name":"beta","orchestrator":"","enabled":false}]`)
	agents, err := decodeRegistryAgents(http.StatusOK, body)
	if err != nil {
		t.Fatalf("a 200 carrying a JSON agent list must decode, got %v", err)
	}
	if len(agents) != 2 {
		t.Fatalf("want 2 agents, got %d (%#v)", len(agents), agents)
	}
	if agents[0].Name != "alpha" || !agents[0].Enabled {
		t.Errorf("first agent decoded wrong: %#v", agents[0])
	}
	if agents[1].Name != "beta" || agents[1].Enabled {
		t.Errorf("second agent decoded wrong: %#v", agents[1])
	}
}

func TestDecodeRegistryAgentsAcceptsEmptyRegistry(t *testing.T) {
	// An empty registry is a real answer and must be distinguishable from a failure —
	// it is the one case where "no agents" carries no error.
	agents, err := decodeRegistryAgents(http.StatusOK, []byte(`[]`))
	if err != nil {
		t.Fatalf("an empty agent list is a valid answer, got %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("want no agents, got %#v", agents)
	}
}

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
	if got := validAgentsDescription(); strings.Contains(got, "Valid options") {
		// validAgentsDescription consults the live registry, which on this host does not
		// answer; either way it must never emit an empty option list.
		if strings.HasSuffix(strings.TrimSpace(got), ":") {
			t.Errorf("description offered an empty option list: %q", got)
		}
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
