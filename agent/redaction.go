package agent

import (
	"log"
	"net/http"
	"sync"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/kayushkin/inber/redact"
)

// The egress redactor is built once per process, from the environment the
// command hands to ArmEgressRedactor. One instance rather than one per client so
// that the "armed" line is logged once and every provider client — Anthropic,
// OpenAI-compatible, and any added later — is measured against the same set of
// live secrets.
var (
	egressRedactorMutex sync.Mutex
	egressRedactor      *redact.Redactor
)

// ArmEgressRedactor builds the process-wide redactor from environment, in
// os.Environ form, and teaches it every configured provider key. The command
// calls it once, before anything builds a provider client. It takes the whole
// environment rather than the declared settings because its job is every secret
// the process holds, including one nobody declared.
//
// It panics if called twice: a second environment would silently replace the
// set of secrets every client already sends through.
func ArmEgressRedactor(environment []string, keys ProviderAPIKeys) {
	egressRedactorMutex.Lock()
	defer egressRedactorMutex.Unlock()
	if egressRedactor != nil {
		panic("agent.ArmEgressRedactor called twice")
	}
	egressRedactor = newEgressRedactor(environment, keys)
	log.Printf("[redact] egress redaction armed: %d live secret values from the environment and the configured provider keys, plus built-in credential shapes",
		egressRedactor.LiteralCount())
}

// newEgressRedactor is the redactor ArmEgressRedactor installs: the secrets in
// environment, and each configured provider key labelled by its provider.
func newEgressRedactor(environment []string, keys ProviderAPIKeys) *redact.Redactor {
	redactor := redact.NewFromEnvironment(environment)
	for provider, key := range map[string]string{
		"anthropic":  keys.Anthropic,
		"openai":     keys.OpenAI,
		"google":     keys.Google,
		"openrouter": keys.OpenRouter,
	} {
		if key != "" {
			redactor.AddLiteral(provider+"-credential", key)
		}
	}
	return redactor
}

// EgressRedactor returns the process-wide redactor applied to every provider
// request. See package redact for what it removes and what it deliberately
// leaves alone.
//
// It panics before ArmEgressRedactor. Building an empty redactor instead would
// send every request through a gate that knows none of the process's secrets,
// and nothing would say so.
func EgressRedactor() *redact.Redactor {
	egressRedactorMutex.Lock()
	defer egressRedactorMutex.Unlock()
	if egressRedactor == nil {
		panic("agent.EgressRedactor used before agent.ArmEgressRedactor: the command must arm it at start")
	}
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
