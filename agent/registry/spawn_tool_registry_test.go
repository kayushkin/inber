package registry

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// inberAgentsBody is the body inber's own GET /api/agents serves, copied from
// the live route rather than from this package's struct. server.handleAgents
// builds it: name, orchestrator, enabled.
const inberAgentsBody = `[{"name":"dagda","orchestrator":"inber","enabled":true},` +
	`{"name":"goibniu","orchestrator":"inber","enabled":true},` +
	`{"name":"oisin","orchestrator":"openclaw","enabled":true}]`

// agentStoreBody is what agent-store's GET /agents actually serves, field names
// and all. It is here because it is the trap: it is valid JSON and a valid
// array, so it decodes into []RegistryAgent without error — into blanks.
const agentStoreBody = `[{"slug":"dagda","display_name":"Dagda","role":"builder","enabled":true},` +
	`{"slug":"goibniu","display_name":"Goibniu","role":"smith","enabled":true}]`

// routingRegistry stands in for the inber server the way that server routes:
// only /api/agents answers, everything else 404s.
//
// A double that answers every path alike cannot fail on a wrong URL, and a
// wrong URL is the whole defect this file is about — the literal used to name
// a route on a different service entirely. It asserts on RequestURI rather
// than URL.Path because Go's server decodes %2F back into a slash in the
// latter, so a path assertion there passes either way.
func routingRegistry(t *testing.T, status int, contentType, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.RequestURI != "/api/agents" {
			http.Error(w, "no such route", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

// captureLog collects what the package writes to the standard logger while fn
// runs. The warnings are the observable half of "a failure is no longer
// silent", so they have to be read, not assumed.
func captureLog(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	prevOut, prevFlags := log.Writer(), log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() { log.SetOutput(prevOut); log.SetFlags(prevFlags) })
	fn()
	return buf.String()
}

// The agent list comes off inber's own /api/agents, with the names and the
// orchestrators filled in.
//
// This is the route server/api_models.go's handleAgents serves and sorts
// specifically for this function, and the route server/agent_names.go names as
// this function's source. Before this test the literal pointed at :8101, which
// is dash — a service with no /api/agents handler at all.
func TestFetchRegistryAgentsReadsInbersOwnAgentsRoute(t *testing.T) {
	srv := routingRegistry(t, http.StatusOK, "application/json", inberAgentsBody)
	defer srv.Close()
	t.Setenv("INBER_SERVER_URL", srv.URL)

	out := captureLog(t, func() {
		agents := fetchRegistryAgents()
		if len(agents) != 3 {
			t.Fatalf("got %d agents, want 3 — the double routes only /api/agents, so a wrong path reads as an empty registry", len(agents))
		}
		for _, a := range agents {
			if a.Name == "" {
				t.Errorf("agent %+v has no name; the description this feeds would list a blank", a)
			}
			if a.Orchestrator == "" {
				t.Errorf("agent %+v has no orchestrator; validOrchestrators drops it", a)
			}
		}
	})

	// Cry-wolf control: a registry that answered correctly must not warn.
	if out != "" {
		t.Errorf("a healthy registry logged a warning:\n%s", out)
	}
}

// A trailing slash on INBER_SERVER_URL does not produce a doubled slash.
func TestRegistryBaseURLTrimsATrailingSlash(t *testing.T) {
	t.Setenv("INBER_SERVER_URL", "http://example.invalid:8200/")
	if got := registryBaseURL(); got != "http://example.invalid:8200" {
		t.Fatalf("registryBaseURL() = %q, want the base with no trailing slash", got)
	}
}

// With nothing in the environment the default is inber's own listen address,
// which cmd/inber-server and server/config.go both put at :8200.
func TestRegistryBaseURLDefaultsToTheInberServer(t *testing.T) {
	t.Setenv("INBER_SERVER_URL", "")
	if got := registryBaseURL(); got != "http://127.0.0.1:8200" {
		t.Fatalf("registryBaseURL() = %q, want inber's own default listen address", got)
	}
}

// A 200 carrying a body this cannot read is a fault, and it says so.
//
// This is the exact live shape of the defect: dash's single-page-app catch-all
// answers 200 text/html for a route it does not have, so the read succeeded,
// the decode failed, and the caller read the nil as "no registry". Returning
// nil is unchanged — the fail-open half is deliberately not decided here — but
// it is now reported instead of swallowed.
func TestAnUnreadableBodyIsReportedRatherThanReadAsAnEmptyRegistry(t *testing.T) {
	srv := routingRegistry(t, http.StatusOK, "text/html; charset=utf-8", "<!doctype html>\n<html></html>")
	defer srv.Close()
	t.Setenv("INBER_SERVER_URL", srv.URL)

	var agents []RegistryAgent
	out := captureLog(t, func() { agents = fetchRegistryAgents() })

	if agents != nil {
		t.Fatalf("got %d agents from an HTML body, want nil", len(agents))
	}
	if out == "" {
		t.Fatal("a 200 with an unreadable body was swallowed silently — that is the defect, not the repair")
	}
	if !strings.Contains(out, "text/html") {
		t.Errorf("the warning does not say what came back instead:\n%s", out)
	}
	if !strings.Contains(out, "validation is OFF") {
		t.Errorf("the warning does not say the check is now off:\n%s", out)
	}
}

// A non-200 reaches the message, rather than failing one step later in the
// decoder and being reported as a malformed body.
func TestANon200StatusReachesTheWarning(t *testing.T) {
	srv := routingRegistry(t, http.StatusServiceUnavailable, "application/json", `{"error":"down"}`)
	defer srv.Close()
	t.Setenv("INBER_SERVER_URL", srv.URL)

	var agents []RegistryAgent
	out := captureLog(t, func() { agents = fetchRegistryAgents() })

	if agents != nil {
		t.Fatalf("got %d agents from a 503, want nil", len(agents))
	}
	if !strings.Contains(out, "503") {
		t.Errorf("the status did not reach the warning; a reader cannot tell a refused read from a malformed one:\n%s", out)
	}
}

// An unreachable registry is reported, and the name check goes off rather than
// on. The fail-open behaviour is asserted so that changing it is a decision
// somebody has to make against a red test, not a silent drift.
func TestAnUnreachableRegistryIsReportedAndLeavesTheCheckOpen(t *testing.T) {
	srv := routingRegistry(t, http.StatusOK, "application/json", inberAgentsBody)
	srv.Close() // nothing is listening on that port now
	t.Setenv("INBER_SERVER_URL", srv.URL)

	var agents []RegistryAgent
	out := captureLog(t, func() { agents = fetchRegistryAgents() })

	if agents != nil {
		t.Fatalf("got %d agents from an unreachable registry, want nil", len(agents))
	}
	if !strings.Contains(out, "unreachable") {
		t.Errorf("an unreachable registry did not say so:\n%s", out)
	}
}

// ⭐ agent-store's rows decode into []RegistryAgent cleanly and produce nothing.
//
// This is why the route matters and why "some service that lists agents" is not
// interchangeable. agent-store is the box's source of truth for agent identity,
// but its rows carry `slug` and `display_name` and no `orchestrator` — so
// json.Unmarshal reports no error, len(agents) is non-zero, and the caller's
// `if len(agents) > 0` guard turns the name check ON against a list in which
// every name is empty. Pointing this client there would convert a check that is
// silently off into a check that silently rejects every agent.
func TestAgentStoreRowsDecodeIntoBlanksRatherThanFailing(t *testing.T) {
	var agents []RegistryAgent
	if err := json.Unmarshal([]byte(agentStoreBody), &agents); err != nil {
		t.Fatalf("agent-store's shape failed to decode: %v — the point of this test is that it does NOT fail", err)
	}
	if len(agents) == 0 {
		t.Fatal("agent-store's body decoded to nothing; the trap is that it decodes to something")
	}
	for _, a := range agents {
		if a.Name != "" {
			t.Errorf("agent-store row decoded a name %q; if it ever does, re-read this test and the card behind it", a.Name)
		}
	}
}

// The description the model is shown lists the agents when the registry answers.
func TestValidAgentsDescriptionNamesTheAgents(t *testing.T) {
	srv := routingRegistry(t, http.StatusOK, "application/json", inberAgentsBody)
	defer srv.Close()
	t.Setenv("INBER_SERVER_URL", srv.URL)

	got := validAgentsDescription()
	for _, name := range []string{"dagda", "goibniu", "oisin"} {
		if !strings.Contains(got, name) {
			t.Errorf("description %q does not name %q", got, name)
		}
	}
}

// validOrchestrators reports each distinct orchestrator once.
func TestValidOrchestratorsAreDeduplicated(t *testing.T) {
	srv := routingRegistry(t, http.StatusOK, "application/json", inberAgentsBody)
	defer srv.Close()
	t.Setenv("INBER_SERVER_URL", srv.URL)

	got := validOrchestrators()
	if len(got) != 2 {
		t.Fatalf("got %v, want the two distinct orchestrators in the fixture", got)
	}
}
