package agent

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

// anthropicStyle413 renders the error string the Anthropic SDK actually
// produces for the 32MB cap on /v1/messages. apierror.Error.Error() formats
// method, URL, status code, http.StatusText and the raw JSON body, and
// Cloudflare emits this one before the request reaches the API. Building it from
// http.StatusText rather than hand-typing the phrase keeps the fixture honest if
// Go ever restates it.
func anthropicStyle413() error {
	raw := `{"type":"error","error":{"type":"request_too_large","message":"Request body is too large"}}`
	return fmt.Errorf("api call failed: %w", fmt.Errorf(`POST "https://api.anthropic.com/v1/messages": %d %s %s`,
		http.StatusRequestEntityTooLarge, http.StatusText(http.StatusRequestEntityTooLarge), raw))
}

// TestARequestRefusedForItsBytesIsClassified pins the class that had no
// classifier at all. Every positive here is a refusal denominated in bytes; a
// gateway writes it in its own prose, so the fixtures are wordings rather than a
// single API string.
func TestARequestRefusedForItsBytesIsClassified(t *testing.T) {
	positives := []struct {
		name string
		err  error
	}{
		{"anthropic 413 through the SDK", anthropicStyle413()},
		{"the error type alone", errors.New(`{"type":"request_too_large"}`)},
		{"a gateway 400 naming the body", errors.New("400 Bad Request: request body exceeds 10485760 bytes")},
		{"a gateway naming content length", errors.New("upstream refused: content length 41 MB exceeds the configured maximum")},
		{"a proxy saying payload too large", errors.New("HTTP 413: Payload Too Large")},
		{"payload size, over the limit", errors.New("payload size too large for this endpoint")},
		{"wrapped, as callAPI returns it", fmt.Errorf("api call failed: %w", errors.New("request size exceeds limit"))},
	}
	for _, c := range positives {
		t.Run(c.name, func(t *testing.T) {
			if !IsRequestByteSizeLimitError(c.err) {
				t.Fatalf("a byte-size refusal was not classified, so it still reaches model-store as a "+
					"provider fault and demotes the model for every session on this host: %q", c.err)
			}
		})
	}
}

// TestAnErrorThatIsNotAboutBytesIsLeftAlone is the other direction, and it is
// the one that matters most: a false positive here silently discards real
// evidence about a failing model, which is the signal failover exists to read.
func TestAnErrorThatIsNotAboutBytesIsLeftAlone(t *testing.T) {
	negatives := []struct {
		name string
		err  error
	}{
		{"nil", nil},
		{"a token overflow", errors.New("prompt is too long: 250000 tokens > 200000 maximum")},
		{"the openai token wording", errors.New("context_length_exceeded")},
		{"an ordinary provider failure", errors.New("500 Internal Server Error: overloaded_error")},
		{"a rate limit", errors.New("429 Too Many Requests: rate_limit_error")},
		{"an auth failure", errors.New("401 Unauthorized: invalid x-api-key")},
		{"a size noun with no refusal in it", errors.New("logging request body for debugging")},
		{"a refusal with no size noun in it", errors.New("the tool result was too large to render")},
	}
	for _, c := range negatives {
		t.Run(c.name, func(t *testing.T) {
			if IsRequestByteSizeLimitError(c.err) {
				t.Fatalf("classified %q as a byte-size refusal; a false positive here stops model-store "+
					"recording a genuine fault, which is the evidence selectModel fails over on", c.err)
			}
		})
	}
}

// TestTheByteClassDoesNotArmTheTokenPruner is a pin on the omission, not on the
// behaviour. Todo 70ae784b reserves what the recovery for a byte overflow should
// be, and the reason it is a reserved question is that inber's only recovery —
// the prune-and-retry in callAPI, gated on isContextLengthError — is denominated
// in tokens while the overflow is denominated in bytes. So the two predicates
// must stay disjoint until that question is answered: an answer arrived at by
// widening isContextLengthError would be an answer nobody decided.
func TestTheByteClassDoesNotArmTheTokenPruner(t *testing.T) {
	byteRefusals := []error{
		anthropicStyle413(),
		errors.New("request body exceeds 10485760 bytes"),
		errors.New("HTTP 413: Payload Too Large"),
	}
	for _, err := range byteRefusals {
		if isContextLengthError(err) {
			t.Fatalf("isContextLengthError now fires on %q, so callAPI answers a byte overflow with a "+
				"token head-drop — the recovery todo 70ae784b deliberately leaves open", err)
		}
	}

	tokenOverflows := []error{
		errors.New("prompt is too long: 250000 tokens > 200000 maximum"),
		errors.New("context_length_exceeded"),
		errors.New("maximum context length is 128000 tokens"),
		errors.New("too many tokens"),
	}
	for _, err := range tokenOverflows {
		if !isContextLengthError(err) {
			t.Fatalf("isContextLengthError stopped firing on %q — the pruner this predicate arms is "+
				"unrelated to the byte class and must not have moved", err)
		}
	}
}
