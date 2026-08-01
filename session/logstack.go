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

// entryType maps a session role to the logstack type that describes it.
//
// The two that matter are inbound and outbound: logstack's Usage and MaxUsage
// both select outbound and read nothing else, so the type is what decides
// whether a turn is counted at all. Assistant turns used to land on
// logstackmodels.TypeMessage, which no reader queries, and inber therefore
// reported no tokens, no dollars and no API calls for three months while every
// entry was written to disk and acked.
//
// The roles left on TypeMessage are the ones that carry no usage — the request
// payload, thinking text, and the conversation-management markers (compaction,
// summarize, stash, prune). System entries stay lifecycle deliberately: the
// session's closing entry is a system entry holding the session's *cumulative*
// totals, so counting it as outbound would add a whole session's usage on top
// of the per-turn assistant entries that already reported it.
func entryType(role string) string {
	switch role {
	case "user":
		return logstackmodels.TypeInbound
	case "assistant":
		return logstackmodels.TypeOutbound
	case "tool_call":
		return logstackmodels.TypeToolCall
	case "tool_result":
		return logstackmodels.TypeToolResult
	case "system":
		return logstackmodels.TypeLifecycle
	default:
		return logstackmodels.TypeMessage
	}
}

// turnStats is the structured usage block logstack reads, or nil for an entry
// that reports no usage.
//
// logstack marks TokensIn, TokensOut and Metadata deprecated and computes usage
// from Stats alone. When Stats is nil it tries to parse the entry's Content as
// a JSON object holding a stats block — and inber's Content is the assistant's
// reply as a plain string, so that parse fails and the entry is dropped from
// usage entirely, cache counts and all. The deprecated fields stay filled for
// consumers that still read them; Stats is what makes the entry count.
//
// Three of TurnStats' fields are deliberately left zero, because inber has no
// honest value for them and a wrong number here is worse than an absent one:
//
//   - Cost: Entry.TotalCost is the session's running total, not this turn's, so
//     any consumer summing it over a session would count the session's cost
//     once per turn. It stays in Metadata where it is labelled cumulative.
//     MaxUsage prices the tokens itself, so nothing observable is lost.
//   - DurationMs and ToolCalls: session.Entry carries neither.
//
// CacheCreationTokens carries the cache writes and CacheWriteTokens stays zero:
// MaxUsage reads the two as a sum, so filling both would double the figure.
func (e Entry) turnStats() *logstackmodels.TurnStats {
	if e.InputTokens == 0 && e.OutputTokens == 0 && e.CacheRead == 0 && e.CacheWrite == 0 {
		return nil
	}
	return &logstackmodels.TurnStats{
		InputTokens:         e.InputTokens,
		OutputTokens:        e.OutputTokens,
		CacheReadTokens:     e.CacheRead,
		CacheCreationTokens: e.CacheWrite,
		Model:               e.Model,
	}
}

// toLogstackEntry converts a session Entry to a logstack LogEntry.
func (e Entry) toLogstackEntry(agentName, sessionID string) logstackmodels.LogEntry {

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
		Timestamp:    e.Timestamp,
		Orchestrator: "inber",
		Agent:        agentName,
		SessionID:    sessionID,
		TurnID:       fmt.Sprintf("%d", e.Turn),
		Model:        e.Model,
		Level:        level,
		Type:         entryType(e.Role),
		Content:      content,
		Stats:        e.turnStats(),
		TokensIn:     e.InputTokens,
		TokensOut:    e.OutputTokens,
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
