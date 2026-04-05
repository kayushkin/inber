// Package bus provides a client for communicating with the message bus via NATS,
// handling subscriptions for inbound messages and publishing for outbound responses.
package bus

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/nats-io/nats.go"
	natsbus "github.com/kayushkin/bus"
	"github.com/kayushkin/bus/messages"
)

// Client subscribes to the bus for inbound messages and publishes
// outbound responses and events via NATS.
type Client struct {
	nc *natsbus.Client
}

// Type aliases for convenience.
type InboundMessage = messages.ChatInbound

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

// Subscribe connects to NATS and delivers inbound messages on chat.inbound.
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
				return
			}

			log.Printf("[bus] ← [%s] %s → %s: %s", msg.Channel, msg.Author, msg.Agent, truncateBus(msg.Text, 80))

			select {
			case ch <- msg:
			default:
				log.Printf("[bus] warning: inbound channel full, dropping message")
			}
		}

		// Subscribe to all requested topics (e.g. "chat.inbound.inber").
		// Do NOT also subscribe to base "chat.inbound" — causes duplicate delivery.
		allTopics := topics
		var subs []*nats.Subscription
		for _, topic := range allTopics {
			s, err := c.nc.QueueSubscribe(topic, "inber-server", handler)
			if err != nil {
				log.Printf("[bus] subscribe %s error: %v", topic, err)
				continue
			}
			subs = append(subs, s)
			log.Printf("[bus] subscribed to %s (queue: inber-server)", topic)
		}
		defer func() {
			for _, s := range subs {
				s.Unsubscribe()
			}
		}()
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

// PublishDelta publishes a streaming delta to chat.stream.
func (c *Client) PublishDelta(delta messages.ChatDelta) error {
	return c.Publish("chat.stream", delta)
}

// PublishOutbound publishes a completed response to chat.outbound (JetStream).
func (c *Client) PublishOutbound(msg messages.ChatOutbound) error {
	msg.Timestamp = time.Now()
	if msg.Orchestrator == "" {
		msg.Orchestrator = "inber"
	}
	if c == nil || c.nc == nil {
		return nil
	}
	return c.nc.JetPublish("chat.outbound", msg)
}

// Reply subscribes to a subject using request/reply pattern.
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
