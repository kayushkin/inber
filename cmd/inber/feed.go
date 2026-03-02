package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/websocket"
)

// FeedMessage matches sí's si.Message format (subset we care about).
type FeedMessage struct {
	Text      string    `json:"text"`
	Author    string    `json:"author,omitempty"`
	Channel   string    `json:"channel,omitempty"`
	Timestamp time.Time `json:"timestamp,omitempty"`
}

// RunFeed connects to a sí feed WebSocket and runs the engine in feed mode.
// Incoming messages become RunTurn calls; responses are sent back.
func RunFeed(feedURL string, eng *Engine) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	Log.Info("connecting to feed: %s", feedURL)

	conn, _, err := websocket.DefaultDialer.DialContext(ctx, feedURL, nil)
	if err != nil {
		return fmt.Errorf("feed connect failed: %w", err)
	}
	defer conn.Close()

	Log.Info("feed connected")

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return nil // clean shutdown
			}
			return fmt.Errorf("feed read error: %w", err)
		}

		var msg FeedMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			Log.Warn("bad feed message: %v", err)
			continue
		}

		if msg.Text == "" {
			continue
		}

		// Format input with author context if available
		input := msg.Text
		if msg.Author != "" {
			input = fmt.Sprintf("[%s] %s", msg.Author, msg.Text)
		}

		Log.Info("feed ← %s: %s", msg.Author, truncateStr(msg.Text, 80))

		result, err := eng.RunTurn(input)
		if err != nil {
			Log.Errorf("RunTurn error: %v", err)
			// Send error back
			resp := FeedMessage{Text: fmt.Sprintf("error: %v", err)}
			if data, err := json.Marshal(resp); err == nil {
				conn.WriteMessage(websocket.TextMessage, data)
			}
			continue
		}

		if result.Text != "" {
			resp := FeedMessage{Text: result.Text}
			data, _ := json.Marshal(resp)
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return fmt.Errorf("feed write error: %w", err)
			}
			Log.Info("feed → %s", truncateStr(result.Text, 80))
		}
	}
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
