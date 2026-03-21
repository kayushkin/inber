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
	bm.server.bus.PublishOutbound(bus.NewOutboundFull(agent, msg.Channel, "status", streamID, "received"))

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
			bm.server.bus.PublishOutbound(bus.NewOutboundFull(agent, msg.Channel, "delta", streamID, ev.Text))

		case "thinking":
			bm.server.bus.PublishOutbound(bus.NewOutboundFull(agent, msg.Channel, "thinking", streamID, ev.Text))

		case "tool_call":
			m := bus.NewOutboundFull(agent, msg.Channel, "tool_call", streamID, ev.Text)
			m.Tool = ev.Tool
			m.Meta = &bus.OutboundMeta{
				Tools: []bus.ToolEventMeta{{Tool: ev.Tool, Input: ev.Text}},
			}
			bm.server.bus.PublishOutbound(m)

		case "tool_result":
			m := bus.NewOutboundFull(agent, msg.Channel, "tool_result", streamID, ev.Text)
			m.Tool = ev.Tool
			m.Meta = &bus.OutboundMeta{
				Tools: []bus.ToolEventMeta{{Tool: ev.Tool, Output: ev.Text}},
			}
			bm.server.bus.PublishOutbound(m)

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
	bm.server.bus.PublishOutbound(bus.NewOutboundFull(agent, msg.Channel, "status", streamID, "api_call"))

	err := bm.server.Stream(ctx, req, onEvent)
	if err != nil {
		log.Printf("[server] bus message error: %v", err)
		// Publish error response.
		bm.server.bus.PublishOutbound(bus.NewOutboundFull(agent, msg.Channel, "done", streamID, fmt.Sprintf("error: %v", err)))
		return
	}

	// Publish final "done" message.
	done := bus.NewOutboundFull(agent, msg.Channel, "done", streamID, finalText)
	done.Meta = &bus.OutboundMeta{
		InputTokens:         finalTokens.Input,
		OutputTokens:        finalTokens.Output,
		CacheReadTokens:     finalTokens.CacheRead,
		CacheCreationTokens: finalTokens.CacheWrite,
		Cost:                finalTokens.Cost,
	}
	bm.server.bus.PublishOutbound(done)
}