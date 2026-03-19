// Package bus provides a client for communicating with the message bus,
// handling WebSocket subscriptions for inbound messages and HTTP publishing
// for outbound responses and events.
package bus

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kayushkin/bus/messages"
)

// Client subscribes to the bus for inbound messages and publishes
// outbound responses and events. This replaces bus-agent as the bridge
// between bus and the agent runtime.
type Client struct {
	busURL   string
	wsURL    string
	token    string
	consumer string
	http     *http.Client
}

// Type aliases — keep existing names for backward compatibility within inber.
type BusMessage = messages.BusEnvelope
type InboundMessage = messages.ChatInbound
type OutboundMessage = messages.ChatOutbound
type OutboundMeta = messages.OutboundMeta

// NewClient creates a bus client. Returns nil if busURL is empty.
func NewClient(busURL, token, consumer string) *Client {
	if busURL == "" {
		return nil
	}

	wsURL := strings.Replace(busURL, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)

	// Ensure we have an HTTP URL for REST calls (ack, publish).
	httpURL := strings.Replace(busURL, "wss://", "https://", 1)
	httpURL = strings.Replace(httpURL, "ws://", "http://", 1)

	if consumer == "" {
		consumer = "inber-server"
	}

	return &Client{
		busURL:   httpURL,
		wsURL:    wsURL,
		token:    token,
		consumer: consumer,
		http:     &http.Client{Timeout: 10 * time.Second},
	}
}

// Subscribe connects to the bus and delivers inbound messages to the returned channel.
// Reconnects automatically on failure. Blocks until ctx is cancelled.
func (c *Client) Subscribe(ctx context.Context, topics []string) <-chan InboundMessage {
	ch := make(chan InboundMessage, 64)

	go func() {
		defer close(ch)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			if err := c.subscribeLoop(ctx, topics, ch); err != nil {
				log.Printf("[bus] subscribe error: %v, reconnecting in 3s...", err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(3 * time.Second):
				}
			}
		}
	}()

	return ch
}

func (c *Client) subscribeLoop(ctx context.Context, topics []string, ch chan<- InboundMessage) error {
	url := fmt.Sprintf("%s/subscribe?consumer=%s&topics=%s&token=%s",
		c.wsURL, c.consumer, strings.Join(topics, ","), c.token)

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	log.Printf("[bus] subscribed to %s", strings.Join(topics, ","))

	// Handle server pings.
	conn.SetPingHandler(func(data string) error {
		conn.SetReadDeadline(time.Now().Add(120 * time.Second))
		return conn.WriteControl(websocket.PongMessage, []byte(data), time.Now().Add(10*time.Second))
	})

	// Client-side keepalive.
	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second))
			}
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		conn.SetReadDeadline(time.Now().Add(120 * time.Second))
		_, data, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}

		var busMsg BusMessage
		if err := json.Unmarshal(data, &busMsg); err != nil {
			log.Printf("[bus] unmarshal error: %v", err)
			continue
		}

		if busMsg.Topic != "inbound" {
			// For now we only process inbound; ack and skip others.
			go c.ack(busMsg.Topic, busMsg.ID)
			continue
		}

		var msg InboundMessage
		if err := json.Unmarshal(busMsg.Payload, &msg); err != nil {
			log.Printf("[bus] payload unmarshal error: %v", err)
			go c.ack(busMsg.Topic, busMsg.ID)
			continue
		}

		// Filter: only process messages for "inber" or known proxy targets.
		// Messages for other orchestrators are left for their own subscribers.
		if msg.Orchestrator != "" && msg.Orchestrator != "inber" {
			log.Printf("[bus] skipping message for orchestrator %q", msg.Orchestrator)
			go c.ack(busMsg.Topic, busMsg.ID)
			continue
		}

		log.Printf("[bus] ← [%s] %s → %s: %s", msg.Channel, msg.Author, msg.Agent, truncateBus(msg.Text, 80))

		select {
		case ch <- msg:
		default:
			log.Printf("[bus] warning: inbound channel full, dropping message")
		}

		go c.ack(busMsg.Topic, busMsg.ID)
	}
}

// Publish sends a message to a bus topic.
func (c *Client) Publish(topic string, payload any) error {
	if c == nil {
		return nil
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	body := map[string]interface{}{
		"topic":   topic,
		"payload": json.RawMessage(payloadJSON),
		"source":  "inber-server",
	}
	data, _ := json.Marshal(body)

	url := fmt.Sprintf("%s/publish?token=%s", c.busURL, c.token)
	resp, err := c.http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("publish: %w", err)
	}
	resp.Body.Close()
	return nil
}

// PublishOutbound publishes an agent response to the "outbound" topic.
func (c *Client) PublishOutbound(msg OutboundMessage) error {
	msg.Timestamp = time.Now()
	if msg.Orchestrator == "" {
		msg.Orchestrator = "inber"
	}
	return c.Publish("outbound", msg)
}

// PublishEvent publishes a system event to the "events" topic.
func (c *Client) PublishEvent(event any) error {
	return c.Publish("events", event)
}

func (c *Client) ack(topic string, id int64) {
	body := map[string]interface{}{
		"consumer":   c.consumer,
		"topic":      topic,
		"message_id": id,
	}
	data, _ := json.Marshal(body)
	url := fmt.Sprintf("%s/ack?token=%s", c.busURL, c.token)
	resp, err := c.http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		log.Printf("[bus] ack error: %v", err)
		return
	}
	resp.Body.Close()
}

func truncateBus(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
