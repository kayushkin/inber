package agent

import (
	"testing"
	"time"
)

// TestTheOpenAICompatibleClientCarriesItsOwnDeadline pins a fact that is
// load-bearing in another package.
//
// engine's errorIsEvidenceAboutTheModel decides whether a failed turn says
// anything about the provider. It excludes context.DeadlineExceeded only when
// the turn context is done, because there is exactly one deadline in inber that
// is NOT on a context: the one below. When it fires, net/http hands back an
// error satisfying errors.Is(err, context.DeadlineExceeded) while the turn
// context is still live, and that is the only signal a hung provider on this
// path — openai, google, openrouter, ollama and the catch-all — ever produces.
//
// Remove this timeout and that reasoning silently stops describing the code: a
// request would hang forever instead, and the comment in engine/failover.go
// would be explaining a case that can no longer happen. The test fails here, in
// the package that owns the number, rather than leaving the stale explanation to
// be found by whoever next debugs failover.
//
// It deliberately does not assert the VALUE. Whether 120s is right — cline
// raised its Ollama timeout to 5 minutes for cold model loads, and this is a
// total-request deadline rather than a response-start one — is the open question
// on the todo this comment's sibling was filed under, and pinning a number here
// would look like an answer.
func TestTheOpenAICompatibleClientCarriesItsOwnDeadline(t *testing.T) {
	client := NewOpenAIClient("https://example.invalid", "test-key", "test-model")

	if client.client.Timeout == 0 {
		t.Fatal("the OpenAI-compatible http.Client no longer has a Timeout. engine's model-health " +
			"filter treats a deadline as inber's own unless the turn context is live, on the premise " +
			"that this client's timeout is the one deadline that is not on a context — update " +
			"errorIsEvidenceAboutTheModel's reasoning, or put the timeout back")
	}
	if client.client.Timeout < time.Second {
		t.Fatalf("the OpenAI-compatible client's timeout is %s, short enough that ordinary "+
			"generations will read as a provider that stopped answering", client.client.Timeout)
	}
}
