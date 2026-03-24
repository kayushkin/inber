package server

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/kayushkin/bus/messages"
	"github.com/kayushkin/inber/bus"
)

func debugLog(msg string, args ...interface{}) {
	f, _ := os.OpenFile("/tmp/inber-bus-debug.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if f != nil {
		fmt.Fprintf(f, "[%s] %s\n", time.Now().Format("15:04:05.000"), fmt.Sprintf(msg, args...))
		f.Close()
	}
}

// BusManager handles message bus integration for the server.
type BusManager struct {
	server *Server
}

// NewBusManager creates a new bus manager for the given server.
func NewBusManager(server *Server) *BusManager {
	return &BusManager{server: server}
}

// ListenBus subscribes to the bus and processes inbound messages.
func (bm *BusManager) ListenBus(ctx context.Context) error {
	if bm.server.bus == nil {
		log.Printf("[server] bus not configured, skipping bus listener")
		return nil
	}

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
	if msg.Orchestrator != "" && msg.Orchestrator != "inber" {
		return // not for us — other orchestrators have their own adapters
	}

	agent := msg.Agent
	if agent == "" {
		agent = bm.server.config.DefaultAgent
	}
	if agent == "" {
		log.Printf("[server] no agent specified and no default, dropping message")
		return
	}

	// Session ID for bus events — use "main" for now, spawns will get their own
	sessionID := "main"
	debugLog("handleBusMessage: agent=%s text=%s", agent, truncate(msg.Text, 80))
	log.Printf("[server] bus → %s: %s", agent, truncate(msg.Text, 80))

	req := RunRequest{
		Agent:   agent,
		Message: msg.Text,
		Channel: msg.Channel,
		Author:  msg.Author,
	}

	var fullText strings.Builder

	onEvent := func(ev StreamEvent) {
		debugLog("onEvent: kind=%s text=%s", ev.Kind, truncate(ev.Text, 60))
		delta := messages.NewChatDelta(agent, "inber", sessionID, ev.Kind)
		switch ev.Kind {
		case "delta":
			delta.Type = "text"
			delta.Text = ev.Text
			fullText.WriteString(ev.Text)
		case "thinking":
			delta.Text = ev.Text
		case "tool_call":
			delta.Type = "tool"
			delta.Tool = ev.Tool
			delta.ToolInput = ev.Text
		case "tool_result":
			delta.Tool = ev.Tool
			delta.ToolOutput = ev.Text
		case "done":
			return // handled after Stream() returns
		default:
			return
		}
		bm.server.bus.PublishDelta(delta)
	}

	debugLog("calling Stream() for %s", agent)
	err := bm.server.Stream(ctx, req, onEvent)
	debugLog("Stream() returned for %s, err=%v", agent, err)
	if err != nil {
		log.Printf("[server] bus message error: %v", err)
	}

	// Publish done on chat.stream
	debugLog("publishing done delta for %s", agent)
	done := messages.NewDoneDelta(agent, "inber", sessionID, nil)
	bm.server.bus.PublishDelta(done)

	debugLog("handleBusMessage done for %s, fullText=%d bytes", agent, fullText.Len())
	if fullText.Len() > 0 {
		// Publish to JetStream for persistence (chat.outbound is the authoritative
		// completed-response channel; do NOT also publish a "completed" delta on
		// chat.stream — that causes downstream consumers to deliver the response twice).
		debugLog("publishing outbound for %s", agent)
		bm.server.bus.PublishOutbound(messages.ChatOutbound{
			Agent:        agent,
			Orchestrator: "inber",
			SessionID:    sessionID,
			Text:         fullText.String(),
			Timestamp:    time.Now(),
		})
	}
}
