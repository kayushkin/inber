package redact

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
)

// ObserveFunc is called once per request that had something removed. It is how
// a redaction becomes visible: a gate that fires silently is indistinguishable
// from one that was never wired. It may be nil.
type ObserveFunc func(findings []Finding)

// RedactRequest rewrites a request's body in place, removing every secret the
// redactor recognises.
//
// The body is fully buffered. A provider request is a JSON document that was
// already assembled in memory, so there is no stream to preserve, and both the
// Anthropic SDK and the OpenAI client below hand over a body that is already
// complete. ContentLength and GetBody are reset together with Body: a retry
// replays through GetBody, and a retry that replayed the unredacted body would
// leak exactly the payload the first attempt had cleaned.
func RedactRequest(request *http.Request, redactor *Redactor, observe ObserveFunc) error {
	if request == nil || request.Body == nil || redactor == nil {
		return nil
	}
	body, err := io.ReadAll(request.Body)
	request.Body.Close()
	if err != nil {
		// The body is consumed either way, so it cannot be sent as it was.
		// Failing here is the honest outcome: the alternative is a request
		// with a truncated body and no complaint.
		return fmt.Errorf("redact: read request body: %w", err)
	}

	cleaned, findings := redactor.RedactPayload(body)
	setRequestBody(request, cleaned)
	if len(findings) > 0 && observe != nil {
		observe(findings)
	}
	return nil
}

func setRequestBody(request *http.Request, body []byte) {
	request.Body = io.NopCloser(bytes.NewReader(body))
	request.ContentLength = int64(len(body))
	request.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
}

// Middleware returns a request interceptor in the shape the Anthropic SDK's
// option.WithMiddleware takes. The signature is written out in stdlib terms
// rather than importing the SDK: option.Middleware is a type alias, so a plain
// function of this shape satisfies it, and this package stays free of any
// provider dependency.
func Middleware(redactor *Redactor, observe ObserveFunc) func(*http.Request, func(*http.Request) (*http.Response, error)) (*http.Response, error) {
	return func(request *http.Request, next func(*http.Request) (*http.Response, error)) (*http.Response, error) {
		if err := RedactRequest(request, redactor, observe); err != nil {
			return nil, err
		}
		return next(request)
	}
}

// RoundTripper wraps a transport so every request through it is redacted. It
// is the same gate as Middleware for clients that take an http.Client instead
// of SDK options. A nil base means http.DefaultTransport.
func RoundTripper(base http.RoundTripper, redactor *Redactor, observe ObserveFunc) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &redactingRoundTripper{base: base, redactor: redactor, observe: observe}
}

type redactingRoundTripper struct {
	base     http.RoundTripper
	redactor *Redactor
	observe  ObserveFunc
}

func (t *redactingRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	// RoundTrip must not modify the request it is given. Clone first, then
	// rewrite the clone's body.
	clone := request.Clone(request.Context())
	if err := RedactRequest(clone, t.redactor, t.observe); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(clone)
}
