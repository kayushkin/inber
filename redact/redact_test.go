package redact

import (
	"encoding/json"
	"strings"
	"testing"
)

// The literal half is the precise one: it matches a string because that string
// is a live credential on this box, not because it is shaped like one.
func TestALiveEnvironmentSecretIsRemovedFromThePayload(t *testing.T) {
	redactor := NewFromEnvironment([]string{
		"NOTEBOARD_API_TOKEN=zPq4L8xVn2Kd7Rm5Tw9Hb3Jc",
		"HOME=/home/kayushkincom",
	})

	body := []byte(`{"messages":[{"text":"the token is zPq4L8xVn2Kd7Rm5Tw9Hb3Jc, use it"}]}`)
	cleaned, findings := redactor.RedactPayload(body)

	if strings.Contains(string(cleaned), "zPq4L8xVn2Kd7Rm5Tw9Hb3Jc") {
		t.Fatalf("the secret survived redaction: %s", cleaned)
	}
	if !strings.Contains(string(cleaned), "[redacted: NOTEBOARD_API_TOKEN]") {
		t.Fatalf("expected the marker to name the variable, got: %s", cleaned)
	}
	if len(findings) != 1 || findings[0].Label != "NOTEBOARD_API_TOKEN" || findings[0].Count != 1 {
		t.Fatalf("findings did not report the one removal: %+v", findings)
	}
}

// A secret holding a quote is the one most worth catching and the one a raw
// substring search walks past, because the body carries it JSON-escaped.
func TestASecretIsFoundInItsJSONEscapedForm(t *testing.T) {
	secret := `pa"ss\word-with-quotes`
	redactor := NewFromEnvironment([]string{"DATABASE_PASSWORD=" + secret})

	body := []byte(`{"text":"connect with pa\"ss\\word-with-quotes now"}`)
	cleaned, findings := redactor.RedactPayload(body)

	if strings.Contains(string(cleaned), `pa\"ss\\word`) {
		t.Fatalf("the escaped form survived redaction: %s", cleaned)
	}
	if len(findings) != 1 {
		t.Fatalf("expected one finding, got %+v", findings)
	}
}

// The rules that keep redaction from eating the agent's own text. Each of
// these variable names looks like a credential's; none of the values is one.
func TestValuesThatWouldCorruptTheConversationAreLeftAlone(t *testing.T) {
	cases := []struct {
		name  string
		entry string
		text  string
	}{
		{"a short value would redact a common word", "SECRET_MODE=on", "the session is on and running"},
		{"a path names where a secret lives, not the secret", "GOOGLE_TOKEN_FILE=/home/kayushkincom/.config/token.json", "open /home/kayushkincom/.config/token.json"},
		{"a home-relative path is a path too", "AUTH_KEYFILE=~/.ssh/id_ed25519.pub", "cat ~/.ssh/id_ed25519.pub"},
		{"an endpoint URL is not a credential", "AUTH_STORE_URL=http://127.0.0.1:8303", "GET http://127.0.0.1:8303/api/resolve"},
		{"a value with spaces is prose, not a key", "KEY_DESCRIPTION=the primary signing key", "the primary signing key is rotated"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			redactor := NewFromEnvironment([]string{c.entry})
			cleaned, findings := redactor.RedactPayload([]byte(c.text))
			if string(cleaned) != c.text {
				t.Fatalf("text was rewritten: %q -> %q", c.text, cleaned)
			}
			if len(findings) != 0 {
				t.Fatalf("expected no findings, got %+v", findings)
			}
		})
	}
}

// A variable whose name says nothing about credentials is not a secret source,
// however long its value.
func TestAnOrdinaryVariableIsNotTreatedAsASecret(t *testing.T) {
	redactor := NewFromEnvironment([]string{"LS_COLORS=rs=0:di=01;34:ln=01;36:mh=00"})
	if redactor.LiteralCount() != 0 {
		t.Fatalf("expected no literals, got %d", redactor.LiteralCount())
	}
}

// The shape half catches a credential inber never held — one read out of a
// file, or printed by someone else's tool.
func TestVendorShapedCredentialsAreRemovedWithoutAnyEnvironment(t *testing.T) {
	redactor := New(nil)
	cases := []struct {
		name  string
		text  string
		label string
	}{
		{"anthropic", "key: sk-ant-api03-AbCdEfGhIjKlMnOpQrStUvWxYz0123456789", "anthropic-api-key"},
		{"openai", "key: sk-AbCdEfGhIjKlMnOpQrStUvWxYz0123456789abcdefgh", "openai-api-key"},
		{"openai project", "key: sk-proj-AbCdEfGh_IjKlMnOpQrStUvWxYz0123456789", "openai-project-key"},
		{"github", "token ghp_AbCdEfGhIjKlMnOpQrStUvWxYz0123", "github-token"},
		{"aws", "id AKIAIOSFODNN7EXAMPLE here", "aws-access-key-id"},
		{"google", "key AIzaSyC1234567890abcdefghijklmnopqrstuvw done", "google-api-key"},
		{"slack", "xoxb-123456789012-abcdefABCDEF", "slack-token"},
		{"jwt", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dBjftJeZ4CVPmB92K27uhbUJU1p1r_wW1gFWFOEjXk", "json-web-token"},
		{"bearer", "Authorization: Bearer abcdefghijklmnopqrstuvwxyz012345", "bearer-token"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cleaned, findings := redactor.RedactPayload([]byte(c.text))
			if len(findings) == 0 {
				t.Fatalf("nothing was redacted from %q", c.text)
			}
			if findings[0].Label != c.label {
				t.Fatalf("expected label %q, got %q", c.label, findings[0].Label)
			}
			if !strings.Contains(string(cleaned), "[redacted: "+c.label+"]") {
				t.Fatalf("marker missing from %q", cleaned)
			}
		})
	}
}

// Half a private key on the wire is still a private key on the wire.
func TestAPrivateKeyBlockIsRemovedWhole(t *testing.T) {
	block := "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAA\nQyNTUxOQAAACD\n-----END OPENSSH PRIVATE KEY-----"
	cleaned, findings := New(nil).RedactPayload([]byte("here it is:\n" + block + "\nthat was it"))

	if strings.Contains(string(cleaned), "b3BlbnNzaC1rZXktdjEAAAAA") {
		t.Fatalf("the key body survived: %s", cleaned)
	}
	if strings.Contains(string(cleaned), "BEGIN OPENSSH PRIVATE KEY") {
		t.Fatalf("the header survived, so only part of the block was removed: %s", cleaned)
	}
	if len(findings) != 1 || findings[0].Label != "private-key-block" {
		t.Fatalf("unexpected findings: %+v", findings)
	}
}

// The pattern that nearly shipped without a word boundary. Hyphenated prose is
// everywhere in this repository, and `sk-[A-Za-z0-9_-]{20,}` matches inside it.
func TestHyphenatedProseIsNotMistakenForAnAPIKey(t *testing.T) {
	text := "the risk-assessment-framework-2026 and the task-completion-loop-board"
	cleaned, findings := New(nil).RedactPayload([]byte(text))
	if string(cleaned) != text {
		t.Fatalf("prose was rewritten: %q", cleaned)
	}
	if len(findings) != 0 {
		t.Fatalf("expected no findings, got %+v", findings)
	}
}

// Prompt caching reuses a byte-identical prefix. A redactor that produced a
// different payload for the same input would miss the cache on every turn, so
// determinism is a cost property here, not only a correctness one.
func TestRedactionIsDeterministicAndIdempotent(t *testing.T) {
	redactor := NewFromEnvironment([]string{"SERVICE_API_KEY=Q7wE9rT2yU4iO6pA8sD0fG1h"})
	body := []byte(`{"text":"Q7wE9rT2yU4iO6pA8sD0fG1h and sk-ant-api03-AbCdEfGhIjKlMnOpQrStUvWxYz01"}`)

	first, _ := redactor.RedactPayload(body)
	second, _ := redactor.RedactPayload(body)
	if string(first) != string(second) {
		t.Fatalf("two runs differed:\n%s\n%s", first, second)
	}
	third, findings := redactor.RedactPayload(first)
	if string(third) != string(first) {
		t.Fatalf("redacting an already-redacted payload changed it:\n%s\n%s", first, third)
	}
	if len(findings) != 0 {
		t.Fatalf("a redacted payload still reported findings: %+v", findings)
	}
}

// A value that contains another value must not be left half-redacted.
func TestTheLongerOfTwoOverlappingSecretsIsRemovedWhole(t *testing.T) {
	redactor := NewFromEnvironment([]string{
		"SHORT_TOKEN=abcdefghijkl",
		"LONG_TOKEN=abcdefghijklmnopqrstuvwx",
	})
	cleaned, _ := redactor.RedactPayload([]byte("value abcdefghijklmnopqrstuvwx end"))
	if strings.Contains(string(cleaned), "mnopqrstuvwx") {
		t.Fatalf("the tail of the longer secret survived: %s", cleaned)
	}
}

// Redaction rewrites; it never refuses. Nothing here may turn into a reason a
// turn fails.
func TestAnEmptyOrUnmatchedPayloadIsReturnedUnchanged(t *testing.T) {
	redactor := NewFromEnvironment([]string{"API_KEY=Q7wE9rT2yU4iO6pA8sD0fG1h"})

	cleaned, findings := redactor.RedactPayload(nil)
	if cleaned != nil || findings != nil {
		t.Fatalf("an empty payload was altered: %v %+v", cleaned, findings)
	}

	body := []byte(`{"text":"nothing secret here"}`)
	cleaned, findings = redactor.RedactPayload(body)
	if string(cleaned) != string(body) || findings != nil {
		t.Fatalf("an unmatched payload was altered: %s %+v", cleaned, findings)
	}
}

// A nil redactor is the "no gate" case, and it must pass text through rather
// than panic: a caller that forgot to build one gets no redaction, not a crash
// in the middle of a turn.
func TestANilRedactorPassesTextThrough(t *testing.T) {
	var redactor *Redactor
	text := "sk-ant-api03-AbCdEfGhIjKlMnOpQrStUvWxYz0123456789"
	if out, _ := redactor.RedactString(text); out != text {
		t.Fatalf("nil redactor altered text: %q", out)
	}
	if out, _ := redactor.RedactPayload([]byte(text)); string(out) != text {
		t.Fatalf("nil redactor altered payload: %q", out)
	}
	if redactor.LiteralCount() != 0 {
		t.Fatal("nil redactor reported literals")
	}
}

// The marker goes into a JSON document, so it must never contain a character
// that would have to be escaped there.
func TestTheMarkerIsSafeInsideAJSONString(t *testing.T) {
	redactor := NewFromEnvironment([]string{`WEIRD"NAME_TOKEN=Q7wE9rT2yU4iO6pA8sD0fG1h`})
	cleaned, _ := redactor.RedactPayload([]byte(`{"text":"Q7wE9rT2yU4iO6pA8sD0fG1h"}`))

	var document map[string]string
	if err := json.Unmarshal(cleaned, &document); err != nil {
		t.Fatalf("redaction produced invalid JSON (%v): %s", err, cleaned)
	}
	if document["text"] != "[redacted: WEIRD_NAME_TOKEN]" {
		t.Fatalf("unexpected marker: %q", document["text"])
	}
}

func TestDescribeNamesLabelsAndCountsOnly(t *testing.T) {
	got := Describe([]Finding{{Label: "API_KEY", Count: 2}, {Label: "github-token", Count: 1}})
	if got != "API_KEY×2, github-token×1" {
		t.Fatalf("unexpected description: %q", got)
	}
}

// The environment is not where inber's secrets are on this host: inber-server
// is started with --api-key-from-auth-store and resolves its provider key over
// HTTP, so the redactor has to be told about it at the moment it is resolved.
func TestARuntimeRegisteredCredentialIsRedacted(t *testing.T) {
	redactor := NewFromEnvironment(nil)
	if redactor.LiteralCount() != 0 {
		t.Fatalf("expected an empty redactor, got %d literals", redactor.LiteralCount())
	}

	// An OpenRouter key: no built-in shape matches it, which is exactly why
	// the literal half has to carry it.
	const key = "or-v1-9f3c2ab84d1e77605b2ac3de1f4890ba"
	if !redactor.AddLiteral("openrouter-credential", key) {
		t.Fatal("the credential was refused")
	}
	if redactor.AddLiteral("openrouter-credential", key) {
		t.Fatal("the same credential was registered twice")
	}

	cleaned, findings := redactor.RedactPayload([]byte(`{"text":"key is ` + key + `"}`))
	if strings.Contains(string(cleaned), key) {
		t.Fatalf("the credential survived: %s", cleaned)
	}
	if len(findings) != 1 || findings[0].Label != "openrouter-credential" {
		t.Fatalf("unexpected findings: %+v", findings)
	}
}

// An implausible value must be refused rather than registered, whatever the
// caller claims it is: a provider whose key resolves to "none" would otherwise
// redact every "none" in every payload.
func TestAnImplausibleCredentialIsRefused(t *testing.T) {
	redactor := New(nil)
	for _, value := range []string{"", "none", "/home/kayushkincom/.config/key", "http://127.0.0.1:8303"} {
		if redactor.AddLiteral("provider-credential", value) {
			t.Fatalf("%q was accepted as a secret", value)
		}
	}
}

// Sessions start while other sessions send. Registration and redaction touch
// the same instance, so the race detector has to see them overlap.
func TestRegistrationAndRedactionRunConcurrently(t *testing.T) {
	redactor := New(nil)
	done := make(chan struct{})

	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			redactor.AddLiteral("provider-credential", strings.Repeat("a", 12+i%9)+string(rune('A'+i%26)))
		}
	}()
	for i := 0; i < 200; i++ {
		redactor.RedactPayload([]byte(`{"text":"aaaaaaaaaaaaA and sk-ant-api03-AbCdEfGhIjKlMnOpQrStUvWxYz01"}`))
	}
	<-done
}

// Every request pays this, so the cost has to be known rather than assumed.
// A large turn is a few hundred kilobytes of JSON.
func BenchmarkRedactPayloadOnALargeTurn(b *testing.B) {
	redactor := NewFromEnvironment([]string{
		"SERVICE_API_KEY=Q7wE9rT2yU4iO6pA8sD0fG1h",
		"DATABASE_PASSWORD=hunter2-and-then-some-more",
	})
	body := []byte(strings.Repeat(
		`{"role":"user","content":[{"type":"text","text":"read the file and report what changed"}]},`, 2500))

	b.SetBytes(int64(len(body)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		redactor.RedactPayload(body)
	}
}

// Each shape runs behind a cheap literal prefilter, and a prefilter that does
// not appear in everything its expression matches is a gate that silently
// narrowed — the fast path would decide, and it would decide by skipping. This
// pins the pairing: every prefilter needs a sample the expression really
// matches, and every pattern needs a prefilter or it pays a full scan on every
// request.
func TestEveryPrefilterAppearsInSomethingItsExpressionMatches(t *testing.T) {
	// One sample per prefilter, so an alternation cannot be half covered.
	samplesByPrefilter := map[string]string{
		"sk-ant-":     "sk-ant-api03-AbCdEfGhIjKlMnOpQrStUvWxYz0123456789",
		"sk-proj-":    "sk-proj-AbCdEfGh_IjKlMnOpQrStUvWxYz0123456789",
		"sk-svcacct-": "sk-svcacct-AbCdEfGh_IjKlMnOpQrStUvWxYz0123456789",
		"sk-admin-":   "sk-admin-AbCdEfGh_IjKlMnOpQrStUvWxYz0123456789",
		"sk-":         "sk-AbCdEfGhIjKlMnOpQrStUvWxYz0123456789abcdefgh",
		"ghp_":        "ghp_AbCdEfGhIjKlMnOpQrStUvWxYz0123",
		"gho_":        "gho_AbCdEfGhIjKlMnOpQrStUvWxYz0123",
		"ghu_":        "ghu_AbCdEfGhIjKlMnOpQrStUvWxYz0123",
		"ghs_":        "ghs_AbCdEfGhIjKlMnOpQrStUvWxYz0123",
		"ghr_":        "ghr_AbCdEfGhIjKlMnOpQrStUvWxYz0123",
		"github_pat_": "github_pat_11ABCDEFG0AbCdEfGhIjKlMnOpQrStUvWxYz",
		"AKIA":        "AKIAIOSFODNN7EXAMPLE",
		"ASIA":        "ASIAIOSFODNN7EXAMPLE",
		"AIza":        "AIzaSyC1234567890abcdefghijklmnopqrstuvw",
		"xox":         "xoxb-123456789012-abcdefABCDEF",
		"eyJ":         "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dBjftJeZ4CVPmB92K27uhbUJU1p1r_wW1gFWFOEjXk",
		"PRIVATE KEY": "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaA\n-----END OPENSSH PRIVATE KEY-----",
		"Bearer":      "Bearer abcdefghijklmnopqrstuvwxyz012345",
		"bearer":      "bearer abcdefghijklmnopqrstuvwxyz012345",
		"BEARER":      "BEARER abcdefghijklmnopqrstuvwxyz012345",
	}

	for _, pattern := range builtinPatterns() {
		if len(pattern.prefilters) == 0 {
			t.Errorf("%s has no prefilter, so it pays a full scan on every request", pattern.name)
			continue
		}
		for _, prefilter := range pattern.prefilters {
			sample, ok := samplesByPrefilter[prefilter]
			if !ok {
				t.Errorf("%s: prefilter %q has no sample here, so nothing checks that the fast path still reaches it", pattern.name, prefilter)
				continue
			}
			if !strings.Contains(sample, prefilter) {
				t.Errorf("%s: sample for %q does not contain it", pattern.name, prefilter)
			}
			if len(pattern.matches("value: "+sample+" end")) == 0 {
				t.Errorf("%s: prefilter %q admits text its expression then rejects, so the prefilter is doing the deciding", pattern.name, prefilter)
			}
		}
	}
}

// The variant that made the case-insensitive expression a lie once the
// prefilter arrived.
func TestAMixedCaseBearerSchemeIsHandledConsistently(t *testing.T) {
	for _, scheme := range []string{"Bearer", "bearer", "BEARER"} {
		text := "Authorization: " + scheme + " abcdefghijklmnopqrstuvwxyz012345"
		cleaned, findings := New(nil).RedactPayload([]byte(text))
		if len(findings) != 1 {
			t.Fatalf("%s was not redacted: %s", scheme, cleaned)
		}
		if !strings.Contains(string(cleaned), scheme+" [redacted: bearer-token]") {
			t.Fatalf("the scheme word was not kept: %s", cleaned)
		}
	}
}
