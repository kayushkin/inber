// Package bus provides a client for communicating with the message bus via NATS,
// handling subscriptions for inbound messages and publishing for outbound responses.
package bus

import (
	"context"
	"encoding/json"
	"log"
	"time"

	natsbus "github.com/kayushkin/bus"
	"github.com/kayushkin/bus/messages"
)

// Client subscribes to the bus for inbound messages and publishes
// outbound responses and events via NATS.
type Client struct {
	nc *natsbus.Client
}

// Type aliases — keep existing names for backward compatibility within inber.
type BusMessage = messages.BusEnvelope
type InboundMessage = messages.ChatInbound
type OutboundMessage = messages.ChatOutbound
type OutboundMeta = messages.OutboundMeta
type ToolEventMeta = messages.ToolEventMeta

// NewClient creates a bus client. Returns nil if natsURL is empty.
func NewClient(natsURL, consumer string) *Client {
	if natsURL == "" {
		return nil
	}

	if consumer == "" {
		consumer = "inber-server"
	}

	nc, err := natsbus.Connect(natsbus.Options{
		URL:  natsURL,
		Name: consumer,
	})
	if err != nil {
		log.Printf("[bus] failed to connect to NATS at %s: %v", natsURL, err)
		return nil
	}

	log.Printf("[bus] connected to NATS at %s", natsURL)
	return &Client{nc: nc}
}

// Subscribe connects to NATS and delivers inbound messages to the returned channel.
// The topics parameter is kept for API compatibility but messages are received on "chat.inbound".
// Blocks until ctx is cancelled.
func (c *Client) Subscribe(ctx context.Context, topics []string) <-chan InboundMessage {
	ch := make(chan InboundMessage, 64)

	go func() {
		defer close(ch)

		handler := func(subject string, data []byte) {
			var msg InboundMessage
			if err := json.Unmarshal(data, &msg); err != nil {
				log.Printf("[bus] unmarshal error: %v", err)
				return
			}

			// Filter: only process messages for "inber" orchestrator.
			if msg.Orchestrator != "" && msg.Orchestrator != "inber" {
				log.Printf("[bus] skipping message for orchestrator %q", msg.Orchestrator)
				return
			}

			log.Printf("[bus] ← [%s] %s → %s: %s", msg.Channel, msg.Author, msg.Agent, truncateBus(msg.Text, 80))

			select {
			case ch <- msg:
			default:
				log.Printf("[bus] warning: inbound channel full, dropping message")
			}
		}

		sub, err := c.nc.Subscribe("chat.inbound", handler)
		if err != nil {
			log.Printf("[bus] subscribe chat.inbound error: %v", err)
			return
		}
		defer sub.Unsubscribe()

		log.Printf("[bus] subscribed to chat.inbound")

		<-ctx.Done()
	}()

	return ch
}

// Publish sends a message to a NATS subject.
func (c *Client) Publish(topic string, payload any) error {
	if c == nil {
		return nil
	}
	return c.nc.Publish(topic, payload)
}

// PublishOutbound publishes an agent response to the "chat.outbound" subject.
func (c *Client) PublishOutbound(msg OutboundMessage) error {
	msg.Timestamp = time.Now()
	if msg.Orchestrator == "" {
		msg.Orchestrator = "inber"
	}

	agent := msg.Agent
	if agent == "" {
		agent = "unknown"
	}

	// Route to subject based on stream type, matching openclaw-adapter conventions.
	switch msg.Stream {
	case "delta", "thinking", "tool_call", "tool_result":
		// Ephemeral streaming events → chat.stream.{agent}
		return c.Publish("chat.stream."+agent, msg)
	case "status":
		return c.Publish("chat.status."+agent, msg)
	case "done":
		// Final completed message → JetStream chat.completed
		// Also publish to webchat.completed if channel is webchat
		if err := c.Publish("chat.completed", msg); err != nil {
			return err
		}
		if msg.Channel == "webchat" {
			return c.Publish("webchat.completed", msg)
		}
		return nil
	default:
		// Fallback for other/unset stream types
		return c.Publish("chat.completed", msg)
	}
}

// PublishEvent publishes a system event to the "events" subject.
func (c *Client) PublishEvent(event any) error {
	return c.Publish("events", event)
}

// Reply subscribes to a subject using request/reply pattern.
// The handler receives raw bytes and returns a response to marshal back.
func (c *Client) Reply(subject string, handler func(data []byte) (any, error)) error {
	if c == nil {
		return nil
	}
	_, err := c.nc.Reply(subject, handler)
	return err
}

// Close closes the NATS connection.
func (c *Client) Close() {
	if c != nil && c.nc != nil {
		c.nc.Close()
	}
}

func truncateBus(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
