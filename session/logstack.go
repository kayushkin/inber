package session

import (
	"fmt"
	"log"

	logstackclient "github.com/kayushkin/logstack/client"
	logstackmodels "github.com/kayushkin/logstack/models"
)

// LogstackAdapter handles integration with logstack centralized logging.
type LogstackAdapter struct {
	client    *logstackclient.Client
	agentName string
	sessionID string
}

// NewLogstackAdapter creates a new logstack adapter if URL is provided.
func NewLogstackAdapter(url, agentName, sessionID string) *LogstackAdapter {
	if url == "" {
		return nil
	}
	return &LogstackAdapter{
		client:    logstackclient.New(url),
		agentName: agentName,
		sessionID: sessionID,
	}
}

// Log sends an entry to logstack (best-effort, async).
func (la *LogstackAdapter) Log(e Entry) {
	if la == nil || la.client == nil {
		return
	}

	go func() {
		entry := e.toLogstackEntry(la.agentName, la.sessionID)
		if err := la.client.Log(entry); err != nil {
			log.Printf("[logstack] failed to log: %v", err)
		}
	}()
}

// toLogstackEntry converts a session Entry to a logstack LogEntry.
func (e Entry) toLogstackEntry(agentName, sessionID string) logstackmodels.LogEntry {
	// Map session role to logstack type
	entryType := logstackmodels.TypeMessage
	switch e.Role {
	case "tool_call":
		entryType = logstackmodels.TypeToolCall
	case "tool_result":
		entryType = logstackmodels.TypeToolResult
	case "system":
		entryType = logstackmodels.TypeLifecycle
	}

	// Map to logstack level
	level := logstackmodels.LevelInfo
	if e.IsError {
		level = logstackmodels.LevelError
	}

	// Build content - include tool info for tool entries
	content := e.Content
	if e.Role == "tool_call" {
		content = string(e.ToolInput)
	}

	return logstackmodels.LogEntry{
		Timestamp:   e.Timestamp,
		Orchestrator: "inber",
		Agent:       agentName,
		SessionID:   sessionID,
		TurnID:      fmt.Sprintf("%d", e.Turn),
		Model:       e.Model,
		Level:       level,
		Type:        entryType,
		Content:     content,
		TokensIn:    e.InputTokens,
		TokensOut:   e.OutputTokens,
		Metadata: map[string]interface{}{
			"turn":      e.Turn,
			"role":      e.Role,
			"tool_name": e.ToolName,
			"tool_id":   e.ToolID,
			"cost_usd":  e.TotalCost,
			"is_error":  e.IsError,
		},
	}
}