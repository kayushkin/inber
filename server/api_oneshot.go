package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/kayushkin/inber/agent"
	"github.com/kayushkin/inber/logger"
)

// defaultOneShotModel is used when the caller omits Model in a OneShotRequest.
const defaultOneShotModel = "claude-haiku-4-5-20251001"

// defaultOneShotMaxTokens caps the response when the caller names no limit.
const defaultOneShotMaxTokens = 4096

// OneShotRequest is a stateless single-turn LLM call.
//
// When Schema is set, the response is hard-forced via tool_choice: the model
// MUST call a synthetic tool named "output" whose input schema is the provided
// Schema. The tool's input is returned in OneShotResponse.Parsed.
//
// Schema is a whole JSON Schema object, which is what the canonical wire type
// says it is (msg.OneShotRequest in ~/repos/llm-bridge). It is not a bare
// properties map: this handler used to assign it straight to the SDK's
// Properties field, so a caller honouring the documented contract sent
// {"properties":{"type":"object","properties":{...},"required":[...]}} and got
// a 400 back. See toolInputSchema.
type OneShotRequest struct {
	Prompt       string          `json:"prompt"`
	SystemPrompt string          `json:"system_prompt,omitempty"`
	Model        string          `json:"model,omitempty"`
	Schema       json.RawMessage `json:"schema,omitempty"`
	MaxTokens    int             `json:"max_tokens,omitempty"`
}

// OneShotResponse carries the result of a stateless single-turn call.
type OneShotResponse struct {
	Text       string          `json:"text,omitempty"`
	Parsed     json.RawMessage `json:"parsed,omitempty"`
	Usage      OneShotUsage    `json:"usage"`
	DurationMs int64           `json:"duration_ms"`
	StopReason string          `json:"stop_reason"`
	Model      string          `json:"model"`
}

type OneShotUsage struct {
	InputTokens         int `json:"input_tokens"`
	OutputTokens        int `json:"output_tokens"`
	CacheCreationTokens int `json:"cache_creation_tokens,omitempty"`
	CacheReadTokens     int `json:"cache_read_tokens,omitempty"`
}

// POST /api/oneshot
//
// Stateless single-turn LLM call. Bypasses sessions, queue, memory, and the
// inber agent loop entirely. Uses ANTHROPIC_API_KEY from the process env.
func (g *Server) handleOneShot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req OneShotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Prompt == "" {
		jsonError(w, "prompt required", http.StatusBadRequest)
		return
	}

	params, err := buildOneShotParams(req)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// This handler builds its own client instead of taking one from the
	// engine, so it has to install the egress gate itself. It is the one
	// anthropic.NewClient call outside agent.newAnthropicClient, and the one
	// that would silently post an unredacted prompt if this were dropped.
	client := anthropic.NewClient(agent.EgressRedactionRequestOption())

	start := time.Now()
	resp, err := client.Messages.New(r.Context(), params)
	if err != nil {
		logger.WithComponent("oneshot").Error("api call failed", map[string]interface{}{
			"error": err.Error(),
			"model": string(params.Model),
		})
		jsonError(w, "api call failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	out := OneShotResponse{
		DurationMs: time.Since(start).Milliseconds(),
		StopReason: string(resp.StopReason),
		Model:      string(resp.Model),
		Usage: OneShotUsage{
			InputTokens:         int(resp.Usage.InputTokens),
			OutputTokens:        int(resp.Usage.OutputTokens),
			CacheCreationTokens: int(resp.Usage.CacheCreationInputTokens),
			CacheReadTokens:     int(resp.Usage.CacheReadInputTokens),
		},
	}

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			out.Text += block.Text
		case "tool_use":
			if block.Name == "output" {
				out.Parsed = json.RawMessage([]byte(block.Input))
			}
		}
	}

	// Schema was requested but the model didn't call the output tool — fail loud.
	if len(req.Schema) > 0 && len(out.Parsed) == 0 {
		jsonError(w, fmt.Sprintf("schema-forced call did not return a tool_use block (stop_reason=%s)", out.StopReason), http.StatusBadGateway)
		return
	}

	logger.WithComponent("oneshot").Info("call completed", map[string]interface{}{
		"model":        out.Model,
		"stop_reason":  out.StopReason,
		"input_tokens": out.Usage.InputTokens,
		"output_tokens": out.Usage.OutputTokens,
		"duration_ms":  out.DurationMs,
		"schema":       len(req.Schema) > 0,
	})

	jsonResponse(w, out)
}
