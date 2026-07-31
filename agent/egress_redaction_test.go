package agent

import (
	"context"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// The secret used here is vendor-shaped rather than taken from the
// environment, so these tests do not depend on the process-wide redactor
// having been built before or after some other test set a variable.
const testAnthropicKeyInPrompt = "sk-ant-api03-AbCdEfGhIjKlMnOpQrStUvWxYz0123456789"

// The live canary for the Anthropic path: a real SDK client, built with the
// real option, talking to a server that records what actually arrived.
func TestTheAnthropicSDKPathRedactsBeforeItSends(t *testing.T) {
	var received string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received = string(body)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"msg_1","type":"message","role":"assistant","model":"claude-test","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer server.Close()

	client := anthropic.NewClient(
		EgressRedactionRequestOption(),
		option.WithAPIKey("test-key"),
		option.WithBaseURL(server.URL),
	)
	_, err := client.Messages.New(context.Background(), anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-test"),
		MaxTokens: 16,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("here is the key: " + testAnthropicKeyInPrompt)),
		},
	})
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if strings.Contains(received, testAnthropicKeyInPrompt) {
		t.Fatalf("the key reached the provider: %s", received)
	}
	if !strings.Contains(received, "[redacted: anthropic-api-key]") {
		t.Fatalf("the provider did not receive the marker: %s", received)
	}
}

// The OpenAI-compatible client is hand-rolled rather than an SDK, and it
// carries openai, google, openrouter, ollama and the unnamed-provider
// catch-all. It gets the same canary.
func TestTheOpenAICompatiblePathRedactsBeforeItSends(t *testing.T) {
	var received string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received = string(body)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"id":"1","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()

	client := NewOpenAIClient(server.URL, "test-key", "test-model")
	_, err := client.ChatCompletion(context.Background(), OpenAIRequest{
		Messages: []OpenAIMessage{{Role: "user", Content: "here is the key: " + testAnthropicKeyInPrompt}},
	})
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if strings.Contains(received, testAnthropicKeyInPrompt) {
		t.Fatalf("the key reached the provider: %s", received)
	}
	if !strings.Contains(received, "[redacted: anthropic-api-key]") {
		t.Fatalf("the provider did not receive the marker: %s", received)
	}
}

// One gate is only one gate while every client passes through it. Inber had
// two Anthropic client constructions in different packages — the shared one in
// this file's package and a second inside the one-shot HTTP handler — and a
// gate on the shared one alone is not a gate. This walks the repository and
// fails on any anthropic.NewClient call that does not carry the option, so a
// third construction site cannot be added quietly.
func TestEveryAnthropicClientIsRedacted(t *testing.T) {
	repositoryRoot := ".."
	fileSet := token.NewFileSet()
	found := 0

	err := filepath.Walk(repositoryRoot, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", "vendor", "node_modules", "logs":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, parseErr := parser.ParseFile(fileSet, path, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			return nil // not this test's business to police unparseable files
		}

		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "NewClient" {
				return true
			}
			pkg, ok := selector.X.(*ast.Ident)
			if !ok || pkg.Name != "anthropic" {
				return true
			}
			found++
			if !callCarriesRedaction(fileSet, call) {
				t.Errorf("%s:%d builds an Anthropic client without EgressRedactionRequestOption — "+
					"every provider request must pass the egress gate",
					path, fileSet.Position(call.Pos()).Line)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walking the repository failed: %v", err)
	}
	if found == 0 {
		t.Fatal("found no anthropic.NewClient calls at all — this test has stopped checking anything")
	}
}

func callCarriesRedaction(fileSet *token.FileSet, call *ast.CallExpr) bool {
	for _, argument := range call.Args {
		var rendered strings.Builder
		if err := printer.Fprint(&rendered, fileSet, argument); err != nil {
			continue
		}
		if strings.Contains(rendered.String(), "EgressRedactionRequestOption") {
			return true
		}
	}
	return false
}

// The SDK is not the only way out. Two of inber's provider paths are
// hand-rolled http.Clients — the OpenAI-compatible client here and the
// OpenClaw proxy in package server — and neither is caught by the check above,
// because neither calls anthropic.NewClient. Any file that names a chat
// endpoint is sending a conversation somewhere and has to install the gate.
func TestEveryHandRolledChatEndpointInstallsTheGate(t *testing.T) {
	endpointMarkers := []string{"chat/completions", "/v1/messages"}
	checked := 0

	err := filepath.Walk("..", func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", "vendor", "node_modules", "logs", "docs", "examples":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		source, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		text := string(source)
		named := false
		for _, marker := range endpointMarkers {
			if strings.Contains(text, marker) {
				named = true
			}
		}
		if !named {
			return nil
		}
		checked++
		if !strings.Contains(text, "EgressRedaction") {
			t.Errorf("%s posts to a chat endpoint without installing the egress gate", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the repository failed: %v", err)
	}
	if checked == 0 {
		t.Fatal("found no chat endpoints at all — this test has stopped checking anything")
	}
}

// The redactor is built once and shared, so a second caller gets an armed gate
// rather than a fresh empty one.
func TestTheProcessRedactorIsBuiltOnce(t *testing.T) {
	if EgressRedactor() != EgressRedactor() {
		t.Fatal("EgressRedactor returned two different redactors")
	}
}

// The streaming call is the one production actually uses whenever a display
// hook is set, and a middleware that buffered the wrong side of the exchange
// would break it while the non-streaming test above still passed. This asserts
// both halves: the request was redacted, and the response still streamed.
func TestTheStreamingPathRedactsAndStillStreams(t *testing.T) {
	var received string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received = string(body)
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range []string{
			`event: message_start` + "\n" + `data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-test","content":[],"stop_reason":null,"usage":{"input_tokens":1,"output_tokens":0}}}`,
			`event: content_block_start` + "\n" + `data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
			`event: content_block_delta` + "\n" + `data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ok"}}`,
			`event: content_block_stop` + "\n" + `data: {"type":"content_block_stop","index":0}`,
			`event: message_stop` + "\n" + `data: {"type":"message_stop"}`,
		} {
			io.WriteString(w, event+"\n\n")
			w.(http.Flusher).Flush()
		}
	}))
	defer server.Close()

	client := anthropic.NewClient(
		EgressRedactionRequestOption(),
		option.WithAPIKey("test-key"),
		option.WithBaseURL(server.URL),
	)
	stream := client.Messages.NewStreaming(context.Background(), anthropic.MessageNewParams{
		Model:     anthropic.Model("claude-test"),
		MaxTokens: 16,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("here is the key: " + testAnthropicKeyInPrompt)),
		},
	})

	events := 0
	for stream.Next() {
		events++
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream failed: %v", err)
	}
	if events == 0 {
		t.Fatal("the stream delivered no events")
	}
	if strings.Contains(received, testAnthropicKeyInPrompt) {
		t.Fatalf("the key reached the provider on the streaming path: %s", received)
	}
}
