package tools

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// deployDouble stands in for the service that serves POST /api/forge/deploy,
// routing the way forge's own mux routes: only that path answers.
//
// A double that answers every path alike cannot fail on a wrong URL, and a
// wrong URL is half of what this file is about — the tool's default pointed at
// a service with no such route. It asserts on RequestURI rather than URL.Path
// because Go's server decodes %2F back into a slash in the latter.
func deployDouble(t *testing.T, status int, contentType, body string) (*httptest.Server, *[]string) {
	t.Helper()
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked = append(asked, r.Method+" "+r.RequestURI)
		if r.RequestURI != "/api/forge/deploy" || r.Method != http.MethodPost {
			http.Error(w, "no such route", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	return srv, &asked
}

// A 200 carrying HTML is refused rather than reported as a started deploy.
//
// This is the live shape: dash has no /api/forge/deploy handler, so its
// single-page-app catch-all answers 200 text/html. The decode error used to be
// discarded, the status was 200, and the tool told the model "Deploy started
// (id: 0)" — a success message for work that had not happened.
func TestAnHTMLBodyIsRefusedRatherThanReportedAsAStartedDeploy(t *testing.T) {
	srv, _ := deployDouble(t, http.StatusOK, "text/html; charset=utf-8", "<!doctype html>\n<html></html>")
	defer srv.Close()
	t.Setenv("BUS_AGENT_URL", srv.URL)

	out, err := postDeploy(srv.URL, "probe", 1, "nightly-test")
	if err == nil {
		t.Fatalf("an HTML body was reported as success: %q", out)
	}
	if !strings.Contains(err.Error(), "text/html") {
		t.Errorf("the error does not say what came back instead: %v", err)
	}
	if strings.Contains(out, "Deploy started") {
		t.Errorf("still reporting a started deploy: %q", out)
	}
}

// ⭐ A 200 with no deploy_id is refused. forge's own handler is a stub that
// answers 200 with {"status":"stub","message":"deploy not implemented yet"},
// so this is not a hypothetical body — it is what the route that really owns
// this path returns today.
func TestAStubAnswerIsRefusedRatherThanReportedAsDeployZero(t *testing.T) {
	srv, _ := deployDouble(t, http.StatusOK, "application/json",
		`{"status":"stub","message":"deploy not implemented yet"}`)
	defer srv.Close()

	out, err := postDeploy(srv.URL, "probe", 1, "nightly-test")
	if err == nil {
		t.Fatalf("a stub answer was reported as success: %q", out)
	}
	if !strings.Contains(err.Error(), "deploy_id") {
		t.Errorf("the error does not say why it refused: %v", err)
	}
}

// A non-200 reaches the message with its status, rather than being reported as
// a bare empty error string.
func TestANon200ReachesTheDeployError(t *testing.T) {
	srv, _ := deployDouble(t, http.StatusBadGateway, "application/json", `{"error":"pool is busy"}`)
	defer srv.Close()

	_, err := postDeploy(srv.URL, "probe", 1, "nightly-test")
	if err == nil {
		t.Fatal("a 502 was reported as success")
	}
	if !strings.Contains(err.Error(), "502") {
		t.Errorf("the status did not reach the error: %v", err)
	}
	if !strings.Contains(err.Error(), "pool is busy") {
		t.Errorf("the upstream's reason did not reach the error: %v", err)
	}
}

// Cry-wolf control: a real deploy result is still reported as success, and the
// id reaches the message. Without this the three refusals above are satisfied
// by a function that refuses everything.
func TestARealDeployResultIsStillReported(t *testing.T) {
	srv, _ := deployDouble(t, http.StatusOK, "application/json", `{"deploy_id":4171}`)
	defer srv.Close()

	out, err := postDeploy(srv.URL, "probe", 7, "nightly-test")
	if err != nil {
		t.Fatalf("a well-formed deploy result was refused: %v", err)
	}
	if !strings.Contains(out, "4171") {
		t.Errorf("the deploy id did not reach the message: %q", out)
	}
	if !strings.Contains(out, "7.dev.kayushkin.com") {
		t.Errorf("the slot did not reach the message: %q", out)
	}
}

// ⭐ The request target is asserted directly, because refusing a wrong path is
// not the same as sending the right one.
//
// The first version of this test posted to base+"/api/forge" and checked that
// the call was refused. It could not fail: postDeploy appends its own path, so
// any literal — right or wrong — produced a route the double 404s, and the
// test passed either way. Scoring caught it (the arm that shortens the literal
// came back caught by four other tests and not by this one). Reading back what
// the server was actually asked for is the only assertion here that observes
// the literal.
func TestTheRequestTargetIsTheForgeDeployRoute(t *testing.T) {
	srv, asked := deployDouble(t, http.StatusOK, "application/json", `{"deploy_id":9}`)
	defer srv.Close()

	if _, err := postDeploy(srv.URL, "probe", 1, "nightly-test"); err != nil {
		t.Fatalf("a well-formed deploy result was refused: %v", err)
	}
	if len(*asked) != 1 {
		t.Fatalf("the server was asked %d times, want 1: %v", len(*asked), *asked)
	}
	if got := (*asked)[0]; got != "POST /api/forge/deploy" {
		t.Fatalf("the tool asked for %q, want %q", got, "POST /api/forge/deploy")
	}
}

// The base URL comes out of the environment, with a trailing slash trimmed so
// the path this builds cannot become a doubled separator.
func TestDeployBaseURLReadsTheEnvironmentAndTrimsASlash(t *testing.T) {
	t.Setenv("BUS_AGENT_URL", "http://example.invalid:8150/")
	if got := deployBaseURL(); got != "http://example.invalid:8150" {
		t.Fatalf("deployBaseURL() = %q, want the base with no trailing slash", got)
	}
}
