package server

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/kayushkin/inber/bus"
)

// BusManager handles message bus integration for the server.
type BusManager struct {
	server *Server
}

// NewBusManager creates a new bus manager for the given server.
func NewBusManager(server *Server) *BusManager {
	return &BusManager{server: server}
}

// ListenBus subscribes to the bus and processes inbound messages.
// Each message is routed to the appropriate agent and processed via Run().
// Responses are published back to bus "outbound" topic.
// Blocks until ctx is cancelled.
func (bm *BusManager) ListenBus(ctx context.Context) error {
	if bm.server.bus == nil {
		log.Printf("[server] bus not configured, skipping bus listener")
		return nil
	}

	// Set up NATS request/reply handlers.
	bm.server.setupAgentRunHandler()

	inbound := bm.server.bus.Subscribe(ctx, []string{"chat.inbound"})
	log.Printf("[server] listening for bus inbound messages")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case msg, ok := <-inbound:
			if !ok {
				return nil
			}
			go bm.handleBusMessage(ctx, msg)
		}
	}
}

// handleBusMessage routes an inbound bus message to the correct agent.
func (bm *BusManager) handleBusMessage(ctx context.Context, msg bus.InboundMessage) {
	// Proxy openclaw messages to OpenClaw API.
	if msg.Orchestrator == "openclaw" {
		bm.server.proxyToOpenClaw(ctx, msg)
		return
	}

	agent := msg.Agent
	if agent == "" {
		agent = bm.server.config.DefaultAgent
	}
	if agent == "" {
		log.Printf("[server] no agent specified and no default, dropping message")
		return
	}

	log.Printf("[server] bus → %s: %s", agent, truncate(msg.Text, 80))

	// Use streaming so we can publish deltas to bus in real-time.
	streamID := fmt.Sprintf("s-%d", time.Now().UnixMilli())

	// Publish status: orchestrator received the message
	bm.server.bus.PublishOutbound(bus.OutboundMessage{
		Text:     "received",
		Agent:    agent,
		Author:   agent,
		Channel:  msg.Channel,
		Stream:   "status",
		StreamID: streamID,
	})

	req := RunRequest{
		Agent:   agent,
		Message: msg.Text,
		Channel: msg.Channel,
		Author:  msg.Author,
	}

	var finalText string
	var finalTokens TokenUsage

	onEvent := func(ev StreamEvent) {
		switch ev.Kind {
		case "delta":
			bm.server.bus.PublishOutbound(bus.OutboundMessage{
				Text:     ev.Text,
				Agent:    agent,
				Author:   agent,
				Channel:  msg.Channel,
				Stream:   "delta",
				StreamID: streamID,
			})

		case "thinking":
			bm.server.bus.PublishOutbound(bus.OutboundMessage{
				Text:     ev.Text,
				Agent:    agent,
				Author:   agent,
				Channel:  msg.Channel,
				Stream:   "thinking",
				StreamID: streamID,
			})

		case "tool_call":
			bm.server.bus.PublishOutbound(bus.OutboundMessage{
				Text:     ev.Text,
				Agent:    agent,
				Author:   agent,
				Channel:  msg.Channel,
				Stream:   "tool_call",
				StreamID: streamID,
				Tool:     ev.Tool,
				Meta: &bus.OutboundMeta{
					Tools: []bus.ToolEventMeta{{Tool: ev.Tool, Input: ev.Text}},
				},
			})

		case "tool_result":
			bm.server.bus.PublishOutbound(bus.OutboundMessage{
				Text:     ev.Text,
				Agent:    agent,
				Author:   agent,
				Channel:  msg.Channel,
				Stream:   "tool_result",
				StreamID: streamID,
				Tool:     ev.Tool,
				Meta: &bus.OutboundMeta{
					Tools: []bus.ToolEventMeta{{Tool: ev.Tool, Output: ev.Text}},
				},
			})

		case "done":
			finalText = ev.Text
			if data, ok := ev.Data.(map[string]any); ok {
				if tokens, ok := data["tokens"].(TokenUsage); ok {
					finalTokens = tokens
				}
			}
		}
	}

	// Publish status: calling API
	bm.server.bus.PublishOutbound(bus.OutboundMessage{
		Agent:    agent,
		Author:   agent,
		Channel:  msg.Channel,
		Stream:   "status",
		StreamID: streamID,
		Text:     "api_call",
	})

	err := bm.server.Stream(ctx, req, onEvent)
	if err != nil {
		log.Printf("[server] bus message error: %v", err)
		// Publish error response.
		bm.server.bus.PublishOutbound(bus.OutboundMessage{
			Text:    fmt.Sprintf("error: %v", err),
			Agent:   agent,
			Author:  agent,
			Channel: msg.Channel,
		})
		return
	}

	// Publish final "done" message.
	bm.server.bus.PublishOutbound(bus.OutboundMessage{
		Text:     finalText,
		Agent:    agent,
		Author:   agent,
		Channel:  msg.Channel,
		Stream:   "done",
		StreamID: streamID,
		Meta: &bus.OutboundMeta{
			InputTokens:         finalTokens.Input,
			OutputTokens:        finalTokens.Output,
			CacheReadTokens:     finalTokens.CacheRead,
			CacheCreationTokens: finalTokens.CacheWrite,
			Cost:                finalTokens.Cost,
		},
	})
}