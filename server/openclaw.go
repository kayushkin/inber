package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// proxyToOpenClaw forwards a bus message to OpenClaw's chat completions API
// and publishes the response back to bus outbound.
func (g *Server) proxyToOpenClaw(ctx context.Context, msg InboundMessage) {
	if g.config.OpenClawURL == "" {
		log.Printf("[openclaw] no OpenClaw URL configured, dropping message for %s", msg.Agent)
		return
	}

	agent := msg.Agent
	if agent == "" {
		agent = "main"
	}

	log.Printf("[openclaw] bus → %s: %s", agent, truncate(msg.Text, 80))

	// Map agent name to openclaw agent ID (some differ).
	openclawAgent := agent

	// Build chat completions request.
	content := msg.Text
	if msg.Author != "" {
		content = fmt.Sprintf("[%s] %s", msg.Author, msg.Text)
	}

	reqBody := openclawRequest{
		Model: "openclaw",
		Messages: []openclawMessage{
			{Role: "user", Content: content},
		},
		Stream: true,
		StreamOptions: &openclawStreamOpts{IncludeUsage: true},
	}
	data, _ := json.Marshal(reqBody)

	url := strings.TrimRight(g.config.OpenClawURL, "/") + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		log.Printf("[openclaw] request error: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if g.config.OpenClawToken != "" {
		req.Header.Set("Authorization", "Bearer "+g.config.OpenClawToken)
	}
	req.Header.Set("x-openclaw-agent-id", openclawAgent)
	req.Header.Set("x-openclaw-session-key", fmt.Sprintf("agent:%s:main", openclawAgent))

	client := &http.Client{Timeout: 5 * time.Minute}
	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[openclaw] request failed: %v", err)
		g.publishOpenClawError(msg, fmt.Sprintf("openclaw unavailable: %v", err))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[openclaw] HTTP %d: %s", resp.StatusCode, string(body)[:200])
		g.publishOpenClawError(msg, fmt.Sprintf("openclaw error: HTTP %d", resp.StatusCode))
		return
	}

	// Stream SSE response.
	streamID := fmt.Sprintf("s-%d", start.UnixMilli())
	var fullText string
	var usage openclawUsage

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			break
		}

		var chunk openclawChunk
		if json.Unmarshal([]byte(payload), &chunk) != nil {
			continue
		}

		if chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 {
			usage = chunk.Usage
		}

		if len(chunk.Choices) > 0 {
			delta := chunk.Choices[0].Delta.Content
			if delta != "" {
				fullText += delta
				g.bus.PublishOutbound(OutboundMessage{
					Text:     delta,
					Agent:    agent,
					Author:   agent,
					Channel:  msg.Channel,
					Stream:   "delta",
					StreamID: streamID,
				})
			}
		}
	}

	duration := time.Since(start)

	// Read usage from sessions file if SSE didn't provide it.
	if usage.PromptTokens == 0 && usage.CompletionTokens == 0 {
		if u, ok := readOpenClawUsage(openclawAgent); ok {
			usage = u
		}
	}

	log.Printf("[openclaw] → %s: %s (%.1fs, %d in, %d out)",
		agent, truncate(fullText, 80), duration.Seconds(), usage.PromptTokens, usage.CompletionTokens)

	// Publish final done message.
	g.bus.PublishOutbound(OutboundMessage{
		Text:     fullText,
		Agent:    agent,
		Author:   agent,
		Channel:  msg.Channel,
		Stream:   "done",
		StreamID: streamID,
		Meta: &OutboundMeta{
			InputTokens:  usage.PromptTokens,
			OutputTokens: usage.CompletionTokens,
			DurationMs:   duration.Milliseconds(),
		},
	})
}

func (g *Server) publishOpenClawError(msg InboundMessage, errMsg string) {
	g.bus.PublishOutbound(OutboundMessage{
		Text:    "⚠️ " + errMsg,
		Agent:   msg.Agent,
		Author:  msg.Agent,
		Channel: msg.Channel,
	})
}

// readOpenClawUsage reads session usage from OpenClaw's sessions.json.
func readOpenClawUsage(agent string) (openclawUsage, bool) {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".openclaw", "sessions.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return openclawUsage{}, false
	}
	var store map[string]struct {
		InputTokens  int `json:"inputTokens"`
		OutputTokens int `json:"outputTokens"`
	}
	if json.Unmarshal(data, &store) != nil {
		return openclawUsage{}, false
	}
	key := fmt.Sprintf("agent:%s:main", agent)
	entry, ok := store[key]
	if !ok {
		return openclawUsage{}, false
	}
	return openclawUsage{
		PromptTokens:     entry.InputTokens,
		CompletionTokens: entry.OutputTokens,
	}, true
}

// OpenClaw API types.
type openclawRequest struct {
	Model         string               `json:"model"`
	Messages      []openclawMessage    `json:"messages"`
	Stream        bool                 `json:"stream,omitempty"`
	StreamOptions *openclawStreamOpts  `json:"stream_options,omitempty"`
}

type openclawStreamOpts struct {
	IncludeUsage bool `json:"include_usage"`
}

type openclawMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openclawUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type openclawChunk struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
	Usage openclawUsage `json:"usage"`
}
