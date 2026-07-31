// Package redact removes credentials from the bytes inber sends to a model
// provider.
//
// Before this package existed inber had no egress redaction of any kind. A
// session that ran `cat .env`, or read `~/.aws/credentials`, or printed
// `env`, put the live value of every secret on this box straight into
// params.Messages and posted it to Anthropic or to an OpenAI-compatible
// endpoint. Nothing looked at the payload on the way out.
//
// The gate sits at the HTTP boundary — the last point before bytes leave the
// process — rather than at each place that produces text. Tool output is not
// the only way a secret gets into a conversation: memory recall, a summarizer
// re-sending an older turn, a pasted prompt and a loaded context file all
// reach the same socket, and a gate per producer would have to be repeated at
// every one of them and at every one added later. One gate per client
// constructor covers all of them, including callers that do not exist yet.
//
// Redaction never blocks a request and never fails one. It rewrites matched
// spans and sends everything else through untouched: a redactor that could
// refuse a call would be a second, unreviewed way for a turn to die.
//
// Two kinds of secret are recognised:
//
//   - Literals — credentials this process actually holds: the values of its
//     secret-looking environment variables, plus every provider key it
//     resolves at runtime (registered via AddLiteral). This is the precise
//     half: it matches a string because that string *is* a credential on this
//     box right now, not because it looks like one. The runtime half is not
//     optional on this host — inber-server is started with
//     --api-key-from-auth-store and its environment holds no provider
//     credential at all, so an environment-only redactor would know none of
//     the keys inber sends with.
//   - Patterns — vendor-shaped credentials (`sk-ant-…`, `ghp_…`, `AKIA…`,
//     PEM private key blocks, JWTs). This is the half that catches a secret
//     inber never held: one read out of a file, a teammate's key in a log.
//
// What it deliberately does NOT do: decide whether a file may be read at all.
// A denylist on read_files is a fail-closed policy with a real cost to a
// working agent, and picking one is a decision, not a cleanup. Redaction is
// the fail-open half and it is safe to run unattended.
package redact

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Finding records one redacted span, for logging. Value is never included —
// a log line about a leaked secret must not be the leak.
type Finding struct {
	// Label names what was removed: the environment variable name for a
	// literal, or the pattern name for a shape match.
	Label string
	// Count is how many times this label matched in one payload.
	Count int
}

// Redactor rewrites secrets out of text. The zero value redacts nothing; use
// New or NewFromEnvironment. A Redactor is safe for concurrent use: sessions
// start and send at the same time, and AddLiteral is called on the same
// instance that in-flight requests are reading.
type Redactor struct {
	mutex sync.RWMutex
	// literals are the exact secret values, longest first so that a value
	// containing another value is replaced whole.
	literals []literalSecret
	patterns []shapePattern
}

type literalSecret struct {
	value string
	label string
}

type shapePattern struct {
	name string
	re   *regexp.Regexp
	// replacement is the whole-match replacement. Some patterns keep a
	// prefix (`Bearer `) so the text still reads as what it was.
	replacement string
	// prefilters are literal substrings, any one of which must be present
	// before the expression is worth running. Every shape here starts with
	// \b, which stops the regexp engine from using a literal prefix to skip
	// ahead, so an unfiltered scan tests every byte position: measured at
	// 5-11ms per pattern on a 227KB turn, 56ms for the set, on every request
	// and growing with the conversation. A strings.Contains pass over the
	// same payload costs microseconds and skips almost all of them, because
	// almost no request contains a credential at all.
	prefilters []string
}

// matches returns the spans this pattern finds, or nil if the cheap prefilter
// says the expensive scan cannot find anything.
func (p shapePattern) matches(text string) [][]int {
	if len(p.prefilters) > 0 && !containsAny(text, p.prefilters) {
		return nil
	}
	return p.re.FindAllStringIndex(text, -1)
}

func containsAny(text string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

// New builds a Redactor over the given literal secrets. Each entry maps a
// label (an environment variable name, say) to the secret value it names.
// Values that fail the plausibility rules in IsPlausibleSecretValue are
// dropped: redacting a short or common value would corrupt unrelated text.
func New(literals map[string]string) *Redactor {
	r := &Redactor{patterns: builtinPatterns()}
	for label, value := range literals {
		r.AddLiteral(label, value)
	}
	return r
}

// AddLiteral registers one more live secret value, and reports whether it was
// taken. It is refused when the value fails IsPlausibleSecretValue or is
// already known.
//
// This exists because on this host the environment is not where inber's
// secrets are. inber-server runs with --api-key-from-auth-store and resolves
// its provider credential over HTTP at startup, so a redactor built from
// os.Environ alone knows nothing about the one credential the process actually
// holds. The value is registered where it is resolved, by the code that has it
// in hand.
func (r *Redactor) AddLiteral(label, value string) bool {
	if r == nil || !IsPlausibleSecretValue(value) {
		return false
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	for _, existing := range r.literals {
		if existing.value == value {
			return false
		}
	}
	r.literals = append(r.literals, literalSecret{value: value, label: sanitizeLabel(label)})
	// Longest first: if one secret is a prefix of another, replacing the
	// short one first would leave the tail of the long one on the wire.
	sort.Slice(r.literals, func(i, j int) bool {
		if len(r.literals[i].value) != len(r.literals[j].value) {
			return len(r.literals[i].value) > len(r.literals[j].value)
		}
		return r.literals[i].value < r.literals[j].value
	})
	return true
}

// snapshotLiterals copies the literal set under the read lock, so a long
// redaction pass does not hold the lock while it scans a payload.
func (r *Redactor) snapshotLiterals() []literalSecret {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	snapshot := make([]literalSecret, len(r.literals))
	copy(snapshot, r.literals)
	return snapshot
}

// NewFromEnvironment builds a Redactor from an environment listing in
// os.Environ form ("NAME=value"). Only variables whose name looks like a
// credential's are considered, and only values that pass
// IsPlausibleSecretValue are kept.
func NewFromEnvironment(environ []string) *Redactor {
	literals := make(map[string]string)
	for _, entry := range environ {
		name, value, ok := strings.Cut(entry, "=")
		if !ok || !IsSecretVariableName(name) {
			continue
		}
		literals[name] = value
	}
	return New(literals)
}

// secretVariableNameFragments are the substrings that mark an environment
// variable as holding a credential. Matching is case-insensitive on the
// variable name only — the value's own shape is judged separately.
var secretVariableNameFragments = []string{
	"key", "token", "secret", "password", "passwd", "credential",
	"apikey", "auth", "session_key", "private",
}

// IsSecretVariableName reports whether an environment variable's name marks
// its value as a credential.
func IsSecretVariableName(name string) bool {
	lower := strings.ToLower(name)
	for _, fragment := range secretVariableNameFragments {
		if strings.Contains(lower, fragment) {
			return true
		}
	}
	return false
}

// minimumLiteralSecretLength is the shortest value worth treating as a
// secret. Below it the risk runs the wrong way: `SECRET_MODE=on` would
// redact every "on" in the payload, and a payload with every "on" removed is
// worse than one carrying a two-character value that is not a credential.
const minimumLiteralSecretLength = 12

// IsPlausibleSecretValue reports whether a value is worth redacting as a
// literal. The rules are deliberately conservative in the direction of
// leaving text alone: a false positive here rewrites the agent's own working
// text, which is a correctness bug for every turn, while a false negative
// leaves one value to the shape patterns below.
func IsPlausibleSecretValue(value string) bool {
	if len(value) < minimumLiteralSecretLength {
		return false
	}
	if strings.ContainsAny(value, " \t\r\n") {
		return false
	}
	// A filesystem path is the name of where a secret lives, not the secret.
	// Redacting it would rewrite every mention of a real path in the
	// conversation, which is how a coding agent loses its bearings.
	if strings.HasPrefix(value, "/") || strings.HasPrefix(value, "~") ||
		strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../") {
		return false
	}
	// Likewise a bare endpoint URL. A URL carrying its own credential is not
	// covered here; see the note in the package doc.
	if urlSchemePattern.MatchString(value) {
		return false
	}
	return true
}

var urlSchemePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*://`)

// labelSafeCharacters keeps a replacement marker free of anything that would
// have to be escaped when the payload is JSON.
var labelSafeCharacters = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

func sanitizeLabel(label string) string {
	cleaned := labelSafeCharacters.ReplaceAllString(label, "_")
	if cleaned == "" {
		return "secret"
	}
	return cleaned
}

// builtinPatterns are credential shapes recognised wherever they appear, so a
// key inber never held is still caught. Each is anchored on a vendor prefix
// and a length no ordinary word reaches, because a shape match has no second
// piece of evidence behind it.
func builtinPatterns() []shapePattern {
	patterns := []struct {
		name        string
		expression  string
		replacement string
		prefilters  []string
	}{
		// Every shape is anchored with \b. Without it `sk-[A-Za-z0-9_-]{20,}`
		// matches inside ordinary hyphenated prose — "risk-assessment-…",
		// "task-completion-loop" — and a redactor that eats the agent's own
		// words is worse than the leak it was added to stop. The plain
		// OpenAI form is alphanumeric-only for the same reason; the project
		// and service-account forms need dashes, and earn them with a prefix.
		{"anthropic-api-key", `\bsk-ant-[A-Za-z0-9_-]{20,}`, "", []string{"sk-ant-"}},
		{"openai-project-key", `\bsk-(?:proj|svcacct|admin)-[A-Za-z0-9_-]{20,}`, "", []string{"sk-proj-", "sk-svcacct-", "sk-admin-"}},
		{"openai-api-key", `\bsk-[A-Za-z0-9]{32,}`, "", []string{"sk-"}},
		{"github-token", `\bgh[pousr]_[A-Za-z0-9]{20,}`, "", []string{"ghp_", "gho_", "ghu_", "ghs_", "ghr_"}},
		{"github-fine-grained-token", `\bgithub_pat_[A-Za-z0-9_]{20,}`, "", []string{"github_pat_"}},
		{"aws-access-key-id", `\b(?:AKIA|ASIA)[0-9A-Z]{16}`, "", []string{"AKIA", "ASIA"}},
		{"google-api-key", `\bAIza[0-9A-Za-z_-]{35}`, "", []string{"AIza"}},
		{"slack-token", `\bxox[baprs]-[A-Za-z0-9-]{10,}`, "", []string{"xox"}},
		{"json-web-token", `\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`, "", []string{"eyJ"}},
		// The whole armoured block, not the header: half a private key on
		// the wire is still a private key on the wire. [\s\S] rather than
		// (?s) so the class is explicit about crossing newlines.
		{"private-key-block", `-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`, "", []string{"PRIVATE KEY"}},
		// Keeps the scheme so the text still says what kind of header it was.
		// The scheme word is spelled out rather than matched with (?i), so
		// the expression and its prefilter accept exactly the same strings.
		// A case-insensitive expression behind a case-sensitive prefilter
		// would quietly stop catching whatever the prefilter missed: the
		// fast path would narrow the gate, which is the worst way for a
		// safety check to be wrong. A case-insensitive prefilter is not the
		// answer either — it means lowercasing the whole payload, which is
		// the copy the prefilter exists to avoid.
		{"bearer-token", `\b((?:Bearer|bearer|BEARER)\s+)[A-Za-z0-9._~+/=-]{20,}`, "${1}" + marker("bearer-token"), []string{"Bearer", "bearer", "BEARER"}},
	}
	compiled := make([]shapePattern, 0, len(patterns))
	for _, p := range patterns {
		replacement := p.replacement
		if replacement == "" {
			replacement = marker(p.name)
		}
		compiled = append(compiled, shapePattern{
			name:        p.name,
			re:          regexp.MustCompile(p.expression),
			replacement: replacement,
			prefilters:  p.prefilters,
		})
	}
	return compiled
}

// marker is the one wording for a removed secret. It names what was taken out
// so the model can tell a redaction from a value the file really held, and it
// contains no character that JSON would escape, so redacting a serialized
// body cannot produce invalid JSON.
func marker(label string) string {
	return fmt.Sprintf("[redacted: %s]", label)
}

// RedactString removes every recognised secret from s and reports what it
// removed. The result is deterministic: the same input always produces the
// same output, which is what lets a redacted prompt prefix stay byte-identical
// across turns and keep its cache hit.
func (r *Redactor) RedactString(s string) (string, []Finding) {
	if r == nil {
		return s, nil
	}
	counts := make(map[string]int)
	for _, literal := range r.snapshotLiterals() {
		if n := strings.Count(s, literal.value); n > 0 {
			s = strings.ReplaceAll(s, literal.value, marker(literal.label))
			counts[literal.label] += n
		}
	}
	for _, pattern := range r.patterns {
		if n := len(pattern.matches(s)); n > 0 {
			s = pattern.re.ReplaceAllString(s, pattern.replacement)
			counts[pattern.name] += n
		}
	}
	return s, findingsFromCounts(counts)
}

// RedactPayload removes secrets from a serialized request body.
//
// It repeats each literal search against the value's JSON-escaped form as
// well as its raw form. Most credentials are alphanumeric and survive JSON
// encoding unchanged, but a password holding a quote or a backslash appears
// in the body as `\"` or `\\` and a raw search walks straight past the one
// value most worth catching.
func (r *Redactor) RedactPayload(body []byte) ([]byte, []Finding) {
	if r == nil || len(body) == 0 {
		return body, nil
	}
	text := string(body)
	counts := make(map[string]int)

	for _, literal := range r.snapshotLiterals() {
		for _, form := range literalForms(literal.value) {
			if n := strings.Count(text, form); n > 0 {
				text = strings.ReplaceAll(text, form, marker(literal.label))
				counts[literal.label] += n
			}
		}
	}
	for _, pattern := range r.patterns {
		if n := len(pattern.matches(text)); n > 0 {
			text = pattern.re.ReplaceAllString(text, pattern.replacement)
			counts[pattern.name] += n
		}
	}
	if len(counts) == 0 {
		return body, nil
	}
	return []byte(text), findingsFromCounts(counts)
}

// literalForms returns the ways one secret value can appear in a serialized
// body: as itself, and as its JSON-escaped self. The two are the same string
// for any ordinary API key, and the duplicate is dropped rather than searched
// twice.
func literalForms(value string) []string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return []string{value}
	}
	escaped := string(encoded[1 : len(encoded)-1])
	if escaped == value {
		return []string{value}
	}
	// Escaped first: it is the longer form, and replacing the raw form first
	// would leave the escape characters around a marker.
	return []string{escaped, value}
}

func findingsFromCounts(counts map[string]int) []Finding {
	if len(counts) == 0 {
		return nil
	}
	findings := make([]Finding, 0, len(counts))
	for label, count := range counts {
		findings = append(findings, Finding{Label: label, Count: count})
	}
	sort.Slice(findings, func(i, j int) bool { return findings[i].Label < findings[j].Label })
	return findings
}

// Describe renders findings for a log line: labels and counts, never values.
func Describe(findings []Finding) string {
	parts := make([]string, 0, len(findings))
	for _, f := range findings {
		parts = append(parts, fmt.Sprintf("%s×%d", f.Label, f.Count))
	}
	return strings.Join(parts, ", ")
}

// LiteralCount reports how many live secret values this redactor knows. It
// exists so a caller can log that the gate is armed and with how much, which
// is the difference between a gate that is on and a gate nobody wired.
func (r *Redactor) LiteralCount() int {
	if r == nil {
		return 0
	}
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	return len(r.literals)
}
