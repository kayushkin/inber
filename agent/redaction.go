package agent

import (
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/kayushkin/inber/redact"
)

// The egress redactor is built once per process, from the environment the
// process was started with. One instance rather than one per client so that
// the "armed" line is logged once and every provider client — Anthropic,
// OpenAI-compatible, and any added later — is measured against the same set of
// live secrets.
var (
	egressRedactorOnce sync.Once
	egressRedactor     *redact.Redactor
)

// EgressRedactor returns the process-wide redactor applied to every provider
// request. See package redact for what it removes and what it deliberately
// leaves alone.
func EgressRedactor() *redact.Redactor {
	egressRedactorOnce.Do(func() {
		egressRedactor = redact.NewFromEnvironment(os.Environ())
		log.Printf("[redact] egress redaction armed: %d live secret values from the environment, plus built-in credential shapes",
			egressRedactor.LiteralCount())
	})
	return egressRedactor
}

// registerProviderCredentialWithEgressRedactor adds a resolved provider key to
// the set of literals the gate removes. The label names the provider, never
// the key, and is what appears in the payload and in the log line.
func registerProviderCredentialWithEgressRedactor(provider, apiKey string) {
	if provider == "" {
		provider = "provider"
	}
	if EgressRedactor().AddLiteral(provider+"-credential", apiKey) {
		log.Printf("[redact] egress redaction now covers the %s credential this process sends with", provider)
	}
}

// logEgressRedaction reports a redacted request. It names the labels and
// counts and never the values: a log line about a leaked credential must not
// be where the credential finally lands.
func logEgressRedaction(findings []redact.Finding) {
	log.Printf("[redact] removed secrets from an outgoing provider request: %s", redact.Describe(findings))
}

// EgressRedactionRequestOption is the redaction gate in the form the Anthropic
// SDK takes. Every anthropic.NewClient call in this repository passes it, and
// TestEveryAnthropicClientIsRedacted fails the build if one stops.
func EgressRedactionRequestOption() option.RequestOption {
	return option.WithMiddleware(redact.Middleware(EgressRedactor(), logEgressRedaction))
}

// EgressRedactionTransport is the same gate for a client that takes an
// http.Client rather than SDK options. base may be nil.
func EgressRedactionTransport(base http.RoundTripper) http.RoundTripper {
	return redact.RoundTripper(base, EgressRedactor(), logEgressRedaction)
}
