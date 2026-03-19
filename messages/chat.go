// Package messages defines shared payload structs for bus topics.
// These types are imported by other services (e.g. openclaw-bus).
package messages

import "time"

// ChatInbound represents an incoming chat message arriving via bus.
type ChatInbound struct {
	ID           string    `json:"id,omitempty"`
	Text         string    `json:"text"`
	Author       string    `json:"author,omitempty"`
	Agent        string    `json:"agent,omitempty"`
	Orchestrator string    `json:"orchestrator,omitempty"` // "inber", "openclaw", etc.
	Channel      string    `json:"channel,omitempty"`
	ReplyTo      string    `json:"reply_to,omitempty"`
	MediaURL     string    `json:"media_url,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
}

// ChatOutbound represents an outgoing agent response published to bus.
type ChatOutbound struct {
	Text         string        `json:"text"`
	Agent        string        `json:"agent"`
	Author       string        `json:"author"`
	Channel      string        `json:"channel"`
	Orchestrator string        `json:"orchestrator,omitempty"`
	Stream       string        `json:"stream,omitempty"`
	StreamID     string        `json:"stream_id,omitempty"`
	Tool         string        `json:"tool,omitempty"`
	Timestamp    time.Time     `json:"timestamp"`
	Meta         *OutboundMeta `json:"meta,omitempty"`
}

// OutboundMeta holds token/cost statistics for a response.
type OutboundMeta struct {
	InputTokens         int     `json:"input_tokens,omitempty"`
	OutputTokens        int     `json:"output_tokens,omitempty"`
	CacheReadTokens     int     `json:"cache_read_tokens,omitempty"`
	CacheCreationTokens int     `json:"cache_creation_tokens,omitempty"`
	ToolCalls           int     `json:"tool_calls,omitempty"`
	Cost                float64 `json:"cost,omitempty"`
	DurationMs          int64   `json:"duration_ms,omitempty"`
	Model               string  `json:"model,omitempty"`
}

// BusEnvelope is the raw message wrapper from the bus WebSocket.
type BusEnvelope struct {
	ID      int64  `json:"id"`
	Topic   string `json:"topic"`
	Payload []byte `json:"payload"` // raw JSON
	Source  string `json:"source"`
}
