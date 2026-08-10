package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// These tests judge resolveAnthropicFromAuthStore against auth-store's ACTUAL
// route rather than against its own doc comment. Derived from
// auth-store/internal/server/server.go and confirmed against a real auth-store
// binary on a throwaway database (2026-08-10):
//
//	GET /api/resolve/{provider}   keyAccess (bearer + X-Auth-App + X-Auth-Reason)
//
// keyAccess answers 400 with the plain-text body
// "X-Auth-App and X-Auth-Reason are required" when either header is missing —
// before the handler runs, so the reply mentions no provider. An unknown
// provider is 404 with a JSON {"error": ...} body; a bad bearer is 401.
//
// This function sets ANTHROPIC_API_KEY in the process environment as its whole
// purpose, so every test uses t.Setenv, which restores the previous value.

type capture struct {
	method string
	path   string
	query  string
	header http.Header
}

func newRecordingAuthStore(t *testing.T, status int, body string) *capture {
	t.Helper()
	got := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method = r.Method
		got.path = r.URL.Path
		got.query = r.URL.RawQuery
		got.header = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	t.Setenv("AUTH_STORE_URL", srv.URL)
	t.Setenv("AUTH_STORE_TOKEN", "probe-token")
	t.Setenv("ANTHROPIC_API_KEY", "")
	return got
}

// resolveFixtureFromRealAuthStore is a byte-for-byte capture of what a real
// auth-store answered for GET /api/resolve/anthropic on an api_key credential.
const resolveFixtureFromRealAuthStore = `{
  "id": "cred_90b0ca4b2f7b15bada54605c",
  "provider": "anthropic",
  "owner": "probe",
  "account": "default",
  "auth_type": "api_key",
  "refresh_mode": "server",
  "api_key": "sk-ant-SECRET-VALUE",
  "leased": false,
  "intended_app": "tool-store"
}`

func TestResolveUsesTheRouteAuthStoreServes(t *testing.T) {
	got := newRecordingAuthStore(t, 200, resolveFixtureFromRealAuthStore)

	if err := resolveAnthropicFromAuthStore(context.Background(), "inber-server"); err != nil {
		t.Fatalf("resolveAnthropicFromAuthStore: %v", err)
	}
	if got.method != "GET" {
		t.Errorf("method = %q, want GET", got.method)
	}
	if got.path != "/api/resolve/anthropic" {
		t.Errorf("path = %q, want /api/resolve/anthropic", got.path)
	}
	if got.query != "" {
		t.Errorf("query = %q, want none — inber resolves without an account filter", got.query)
	}
}

// TestResolveCarriesTheHeadersKeyAccessDemands is the sharpest thing here:
// without these two headers inber-server refuses to start, with a 400 that
// names no provider and no credential.
func TestResolveCarriesTheHeadersKeyAccessDemands(t *testing.T) {
	got := newRecordingAuthStore(t, 200, resolveFixtureFromRealAuthStore)

	if err := resolveAnthropicFromAuthStore(context.Background(), "inber-server"); err != nil {
		t.Fatalf("resolveAnthropicFromAuthStore: %v", err)
	}
	if got.header.Get("X-Auth-App") != "inber-server" {
		t.Errorf("X-Auth-App = %q, want the app argument; auth-store answers 400 without it",
			got.header.Get("X-Auth-App"))
	}
	if got.header.Get("X-Auth-Reason") == "" {
		t.Error("X-Auth-Reason is absent; auth-store answers 400 without it")
	}
	if got.header.Get("Authorization") != "Bearer probe-token" {
		t.Errorf("Authorization = %q", got.header.Get("Authorization"))
	}
}

// TestTheAppArgumentReachesAuthStoreAsTheIntendedApp records a consequence
// that is easy to miss. handleResolveByProvider defaults intended_app to
// X-Auth-App, so the app name this function is called with decides WHICH
// anthropic credential is preferred — auth-store tries the app-matched one
// first and only then falls back to any. The app argument is a credential
// selector, not just an audit label.
func TestTheAppArgumentReachesAuthStoreAsTheIntendedApp(t *testing.T) {
	got := newRecordingAuthStore(t, 200, resolveFixtureFromRealAuthStore)

	if err := resolveAnthropicFromAuthStore(context.Background(), "inber-bench"); err != nil {
		t.Fatalf("resolveAnthropicFromAuthStore: %v", err)
	}
	if got.header.Get("X-Auth-App") != "inber-bench" {
		t.Errorf("X-Auth-App = %q, want inber-bench", got.header.Get("X-Auth-App"))
	}
}

func TestResolveSetsTheEnvironmentVariableFromTheSecret(t *testing.T) {
	newRecordingAuthStore(t, 200, resolveFixtureFromRealAuthStore)

	if err := resolveAnthropicFromAuthStore(context.Background(), "inber-server"); err != nil {
		t.Fatalf("resolveAnthropicFromAuthStore: %v", err)
	}
	if got := os.Getenv("ANTHROPIC_API_KEY"); got != "sk-ant-SECRET-VALUE" {
		t.Errorf("ANTHROPIC_API_KEY = %q, want the api_key from the payload", got)
	}
}

func TestResolveAcceptsAnOAuthCredentialAsAnAccessToken(t *testing.T) {
	// auth-store's toResolved fills access_token rather than api_key for
	// oauth and token credentials, and never fills both.
	newRecordingAuthStore(t, 200, `{"auth_type":"oauth","access_token":"tok-oauth"}`)

	if err := resolveAnthropicFromAuthStore(context.Background(), "inber-server"); err != nil {
		t.Fatalf("resolveAnthropicFromAuthStore: %v", err)
	}
	if got := os.Getenv("ANTHROPIC_API_KEY"); got != "tok-oauth" {
		t.Errorf("ANTHROPIC_API_KEY = %q, want the access_token", got)
	}
}

// TestResolveFailsLoudWithoutTouchingTheEnvironment pins the single-source-of-
// truth contract in the doc comment: on any failure the process must not be
// left holding a stale or ambient ANTHROPIC_API_KEY that this function did not
// put there.
func TestResolveFailsLoudWithoutTouchingTheEnvironment(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		// wantInError is a substring the message must carry. For the HTTP
		// failures this is the status code, and asserting it is what makes
		// the test meaningful: auth-store's 404 body is VALID JSON that
		// decodes to a credential with no secret, so a client that stopped
		// checking the status would still fail — with "no api_key or
		// access_token" instead of "404", blaming the credential for a
		// lookup that never found one.
		wantInError string
	}{
		{"missing audit headers", 400, "X-Auth-App and X-Auth-Reason are required", "400"},
		{"bad bearer", 401, "unauthorized", "401"},
		{"unknown provider", 404, `{"error":"no credentials for provider anthropic account=\"\" intended_app=\"\""}`, "404"},
		{"a credential with no usable secret", 200, `{"auth_type":"password","username":"u"}`, "no api_key or access_token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			newRecordingAuthStore(t, tc.status, tc.body)
			t.Setenv("ANTHROPIC_API_KEY", "ambient-value-that-must-not-be-trusted")

			err := resolveAnthropicFromAuthStore(context.Background(), "inber-server")
			if err == nil {
				t.Fatalf("status %d with body %q was treated as success", tc.status, tc.body)
			}
			if !strings.Contains(err.Error(), tc.wantInError) {
				t.Errorf("error = %q, want it to carry %q", err, tc.wantInError)
			}
			if got := os.Getenv("ANTHROPIC_API_KEY"); got != "ambient-value-that-must-not-be-trusted" {
				t.Errorf("ANTHROPIC_API_KEY = %q; a failed resolve must not write the variable", got)
			}
		})
	}
}

func TestResolveRefusesToRunWithoutTheConfigurationItNeeds(t *testing.T) {
	t.Run("empty app", func(t *testing.T) {
		newRecordingAuthStore(t, 200, resolveFixtureFromRealAuthStore)
		if err := resolveAnthropicFromAuthStore(context.Background(), ""); err == nil {
			t.Fatal("an empty app was accepted; it becomes X-Auth-App, which auth-store requires")
		}
	})
	t.Run("missing token", func(t *testing.T) {
		newRecordingAuthStore(t, 200, resolveFixtureFromRealAuthStore)
		t.Setenv("AUTH_STORE_TOKEN", "")
		err := resolveAnthropicFromAuthStore(context.Background(), "inber-server")
		if err == nil {
			t.Fatal("a missing AUTH_STORE_TOKEN was accepted")
		}
		if !strings.Contains(err.Error(), "AUTH_STORE_TOKEN") {
			t.Errorf("error = %q, want it to name the missing variable", err)
		}
	})
}
