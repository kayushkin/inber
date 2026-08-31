package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// The exchange cut in BackgroundExtractMemories is asked two separate questions
// here, in two tests with two fixtures, because the call site makes two
// separable claims and one test that reddens for either cannot say which one
// broke.
//
// Neither test names textutil. Both read the cut where it lands — in the prompt
// that arrives at the provider — so renaming or reimplementing the helper
// leaves them green, and reverting the CALL SITE does not.
//
// Why this site: commit 9ce1666 ranked its own repairs, and "the cuts that
// reach the model are the ones that matter". This one is two lines above a
// client.Messages.New, so what it cuts is sent verbatim.
//
// Why they exist: measured 2026-08-31 against branch
// test/the-truncatewith-call-sites-are-asked at 2c597ad, reverting this call
// site to the pre-9ce1666 byte cut reddened nothing in the whole repository,
// and neither did removing the truncation altogether. Card 3388f950, residual
// 3bec041f.
//
// No live egress: the client is a parameter, and these tests hand it a base URL
// pointing at a local recorder.

// promptSentToTheProvider drives BackgroundExtractMemories against a local
// recorder and returns the single text block the provider actually received.
//
// The recorder answers with an empty text block on purpose: the function then
// returns before it touches the memory store, so these tests exercise the
// prompt-building half and nothing else.
func promptSentToTheProvider(t *testing.T, assistantResponse string) string {
	t.Helper()

	var body []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"msg_1","type":"message","role":"assistant","model":"claude-test",`+
			`"content":[{"type":"text","text":""}],"stop_reason":"end_turn",`+
			`"usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer server.Close()

	client := anthropic.NewClient(
		option.WithAPIKey("test-key"),
		option.WithBaseURL(server.URL),
	)

	cfg := DefaultExtractionConfig()
	cfg.Model = "claude-test"
	BackgroundExtractMemories(context.Background(), &client,
		"please summarise everything that happened in this session",
		assistantResponse, nil, "session-under-test", nil, cfg)

	if body == nil {
		t.Fatal("BackgroundExtractMemories sent no request, so this test observed nothing; " +
			"the exchange no longer clears MinExchangeTokens or the 500-token prompt budget")
	}

	var sent struct {
		Messages []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("could not read the request the provider received: %v\nbody %q", err, body)
	}
	var texts []string
	for _, m := range sent.Messages {
		for _, c := range m.Content {
			if c.Type == "text" {
				texts = append(texts, c.Text)
			}
		}
	}
	if len(texts) != 1 {
		t.Fatalf("expected one text block in the extraction request, got %d", len(texts))
	}
	return texts[0]
}

// TestTheExtractionPromptIsCutOnARuneBoundary is the rune-boundary claim on its
// own.
//
// The cut position is a constant this test deliberately does not predict: it is
// derived inside the function from the length of extractionPrompt, so a test
// that hardcoded it would go quiet the day the prompt is reworded. Instead the
// fixture is built from three-byte runes behind 0, 1 and 2 filler bytes, which
// covers every residue of the cut index modulo three. A rune-safe cut is on a
// boundary in all three; a plain s[:n] is inside a rune in two of them,
// whatever the constant happens to be.
//
// The assertion is not utf8.ValidString on the request body. encoding/json
// rewrites invalid bytes as U+FFFD while marshalling, so a body carrying a
// split rune is still valid UTF-8 by the time it is on the wire — the damage
// shows up as the replacement character in the decoded prompt, which is what a
// model would read.
func TestTheExtractionPromptIsCutOnARuneBoundary(t *testing.T) {
	for filler := 0; filler < 3; filler++ {
		t.Run(fmt.Sprintf("filler-%d", filler), func(t *testing.T) {
			response := strings.Repeat("x", filler) + strings.Repeat("字", 400)

			prompt := promptSentToTheProvider(t, response)

			if strings.ContainsRune(prompt, utf8.RuneError) {
				t.Errorf("the extraction prompt that reached the provider carries a "+
					"replacement character: the exchange cut ended inside a rune.\n"+
					"prompt tail %q", tailOf(prompt, 40))
			}
		})
	}
}

// TestTheExtractionPromptIsCutToItsBudget is the does-it-cut-at-all claim on
// its own. The fixture is pure ASCII, so a byte cut and a rune-safe cut agree
// on it and this test is blind to the boundary question the one above asks.
func TestTheExtractionPromptIsCutToItsBudget(t *testing.T) {
	const tail = "LAST-LINE-OF-THE-ASSISTANT-TURN"
	response := strings.Repeat("a", 2000) + tail

	prompt := promptSentToTheProvider(t, response)

	if strings.Contains(prompt, tail) {
		t.Errorf("a %d-byte exchange reached the provider whole: the cut that keeps "+
			"the extraction prompt under its token budget did not happen.\n"+
			"prompt is %d bytes", len(response), len(prompt))
	}
	if !strings.Contains(prompt, "...") {
		t.Errorf("the exchange was shortened without saying it had been cut, so the "+
			"extracting model reads a truncated turn as a complete one.\n"+
			"prompt tail %q", tailOf(prompt, 40))
	}
}

func tailOf(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
