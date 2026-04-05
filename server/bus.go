package server

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/kayushkin/bus/messages"
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
func (bm *BusManager) ListenBus(ctx context.Context) error {
	if bm.server.bus == nil {
		log.Printf("[server] bus not configured, skipping bus listener")
		return nil
	}

	bm.server.setupAgentRunHandler()

	inbound := bm.server.bus.Subscribe(ctx, []string{"chat.inbound.inber"})
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
	// Recover from panics so a single bad message doesn't crash the server.
	defer func() {
		if r := recover(); r != nil {
			agent := msg.Agent
			if agent == "" {
				agent = bm.server.config.DefaultAgent
			}
			sessionID := "main"
			log.Printf("[bus] PANIC handling message from %s: %v", msg.Author, r)
			errDelta := messages.NewChatDelta(agent, "inber", sessionID, "error")
			errDelta.Text = fmt.Sprintf("internal error: %v", r)
			bm.server.bus.PublishDelta(errDelta)
			done := messages.NewDoneDelta(agent, "inber", sessionID, nil)
			bm.server.bus.PublishDelta(done)
		}
	}()

	// Task 4: Add entrance logging BEFORE orchestrator filter
	log.Printf("[bus] received: orchestrator=%s agent=%s channel=%s text=%s", 
		msg.Orchestrator, msg.Agent, msg.Channel, truncate(msg.Text, 50))

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
	log.Printf("[server] bus → %s: %s", agent, truncate(msg.Text, 80))
	
	// Task 1: Publish "received" acknowledgment event 
	ack := messages.NewChatDelta(agent, "inber", sessionID, "status")
	ack.Text = "processing"
	if err := bm.server.bus.PublishDelta(ack); err != nil {
		bm.server.logWarning(agent, sessionID, msg.Text, "failed to publish processing ack: "+err.Error())
	}

	req := RunRequest{
		Agent:   agent,
		Message: msg.Text,
		Channel: msg.Channel,
		Author:  msg.Author,
	}

	var fullText strings.Builder

	// publishStatus sends a status delta to signal progress to the UI.
	publishStatus := func(text string) {
		s := messages.NewChatDelta(agent, "inber", sessionID, "status")
		s.Text = text
		bm.server.bus.PublishDelta(s)
	}

	// Track which status phases we've already emitted to avoid duplicates.
	seenThinking := false
	seenDelta := false

	onEvent := func(ev StreamEvent) {
		// Emit granular status deltas at key phase transitions.
		switch ev.Kind {
		case "thinking":
			if !seenThinking {
				seenThinking = true
				publishStatus("thinking")
			}
		case "delta":
			if !seenDelta {
				seenDelta = true
				publishStatus("responding")
			}
		case "tool_call":
			toolName := ev.Tool
			if toolName == "" {
				toolName = "tool"
			}
			publishStatus("tool:" + toolName)
		case "tool_result":
			publishStatus("responding")
		}

		// Original delta publishing.
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
		if err := bm.server.bus.PublishDelta(delta); err != nil {
			bm.server.logWarning(agent, sessionID, msg.Text, "failed to publish delta: "+err.Error())
		}
	}

	// Publish loading status before calling Stream.
	publishStatus("loading_agent")

	// Task 3: Add request timeout
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	err := bm.server.Stream(ctx, req, onEvent)
	if err != nil {
		// Publish error status + error delta.
		publishStatus("error")
		errDelta := messages.NewChatDelta(agent, "inber", sessionID, "error")
		errDelta.Text = err.Error()
		if pubErr := bm.server.bus.PublishDelta(errDelta); pubErr != nil {
			bm.server.logWarning(agent, sessionID, msg.Text, "failed to publish error delta: "+pubErr.Error())
		}

		bm.server.logError(agent, sessionID, msg.Text, err)
		log.Printf("[server] bus message error: %v", err)
	}

	// Publish done status + done delta.
	publishStatus("done")
	done := messages.NewDoneDelta(agent, "inber", sessionID, nil)
	if err := bm.server.bus.PublishDelta(done); err != nil {
		bm.server.logWarning(agent, sessionID, msg.Text, "failed to publish done delta: "+err.Error())
	}

	if fullText.Len() > 0 {
		// Publish to JetStream for persistence (chat.outbound is the authoritative
		// completed-response channel; do NOT also publish a "completed" delta on
		// chat.stream — that causes downstream consumers to deliver the response twice).
		bm.server.bus.PublishOutbound(messages.ChatOutbound{
			Agent:        agent,
			Orchestrator: "inber",
			SessionID:    sessionID,
			Text:         fullText.String(),
			Timestamp:    time.Now(),
		})
	}
}
