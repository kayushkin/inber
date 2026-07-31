package redact

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testRedactor() *Redactor {
	return NewFromEnvironment([]string{"PROVIDER_API_TOKEN=Q7wE9rT2yU4iO6pA8sD0fG1h"})
}

// The gate is only worth anything if the bytes on the socket are the redacted
// ones, so this asserts what the server received rather than what the redactor
// returned.
func TestTheServerNeverSeesTheSecretThroughARedactingTransport(t *testing.T) {
	var received string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received = string(body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &http.Client{Transport: RoundTripper(nil, testRedactor(), nil)}
	response, err := client.Post(server.URL, "application/json",
		strings.NewReader(`{"text":"token Q7wE9rT2yU4iO6pA8sD0fG1h"}`))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	response.Body.Close()

	if strings.Contains(received, "Q7wE9rT2yU4iO6pA8sD0fG1h") {
		t.Fatalf("the secret reached the server: %s", received)
	}
	if !strings.Contains(received, "[redacted: PROVIDER_API_TOKEN]") {
		t.Fatalf("the server did not receive the marker: %s", received)
	}
}

// A shortened body with the old Content-Length is a request the server rejects
// or reads short, so the two have to move together.
func TestTheContentLengthMatchesTheRedactedBody(t *testing.T) {
	var receivedLength int64
	var receivedBytes int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedLength = r.ContentLength
		body, _ := io.ReadAll(r.Body)
		receivedBytes = len(body)
	}))
	defer server.Close()

	client := &http.Client{Transport: RoundTripper(nil, testRedactor(), nil)}
	response, err := client.Post(server.URL, "application/json",
		strings.NewReader(`{"text":"Q7wE9rT2yU4iO6pA8sD0fG1h"}`))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	response.Body.Close()

	if receivedLength != int64(receivedBytes) {
		t.Fatalf("Content-Length %d did not match the %d bytes sent", receivedLength, receivedBytes)
	}
}

// A retry replays through GetBody. A retry that replayed the original body
// would leak exactly the payload the first attempt had cleaned, and it would
// do it on the attempt nobody is watching.
func TestAReplayedRequestCarriesTheRedactedBody(t *testing.T) {
	request, err := http.NewRequest(http.MethodPost, "https://example.invalid/v1/messages",
		strings.NewReader(`{"text":"Q7wE9rT2yU4iO6pA8sD0fG1h"}`))
	if err != nil {
		t.Fatalf("building the request failed: %v", err)
	}
	if err := RedactRequest(request, testRedactor(), nil); err != nil {
		t.Fatalf("redaction failed: %v", err)
	}

	replay, err := request.GetBody()
	if err != nil {
		t.Fatalf("GetBody failed: %v", err)
	}
	replayed, _ := io.ReadAll(replay)
	if strings.Contains(string(replayed), "Q7wE9rT2yU4iO6pA8sD0fG1h") {
		t.Fatalf("the replayed body still held the secret: %s", replayed)
	}
}

// Redaction observes; it must not swallow the fact that it fired.
func TestAFiringGateIsObservable(t *testing.T) {
	var seen []Finding
	request, _ := http.NewRequest(http.MethodPost, "https://example.invalid/",
		strings.NewReader(`{"text":"Q7wE9rT2yU4iO6pA8sD0fG1h"}`))

	if err := RedactRequest(request, testRedactor(), func(f []Finding) { seen = f }); err != nil {
		t.Fatalf("redaction failed: %v", err)
	}
	if len(seen) != 1 || seen[0].Label != "PROVIDER_API_TOKEN" {
		t.Fatalf("the observer was not told what was removed: %+v", seen)
	}
}

// A request with nothing to send must pass through untouched rather than gain
// an empty body or an error.
func TestARequestWithoutABodyPassesThrough(t *testing.T) {
	request, _ := http.NewRequest(http.MethodGet, "https://example.invalid/v1/models", nil)
	if err := RedactRequest(request, testRedactor(), nil); err != nil {
		t.Fatalf("redaction failed on a bodyless request: %v", err)
	}
	if request.Body != nil {
		t.Fatal("a bodyless request came back with a body")
	}
}

// The middleware form is what the Anthropic SDK takes; it must redact and then
// call through, not replace the call.
func TestTheMiddlewareRedactsAndCallsThrough(t *testing.T) {
	var forwarded string
	called := false
	middleware := Middleware(testRedactor(), nil)

	request, _ := http.NewRequest(http.MethodPost, "https://example.invalid/",
		strings.NewReader(`{"text":"Q7wE9rT2yU4iO6pA8sD0fG1h"}`))

	_, err := middleware(request, func(r *http.Request) (*http.Response, error) {
		called = true
		body, _ := io.ReadAll(r.Body)
		forwarded = string(body)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(""))}, nil
	})
	if err != nil {
		t.Fatalf("middleware returned an error: %v", err)
	}
	if !called {
		t.Fatal("the middleware did not call through")
	}
	if strings.Contains(forwarded, "Q7wE9rT2yU4iO6pA8sD0fG1h") {
		t.Fatalf("the forwarded body still held the secret: %s", forwarded)
	}
}
