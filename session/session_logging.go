package session

import (
	"encoding/json"
	"fmt"
	"time"
)

// LogUser logs a user message to the session.
func (s *Session) LogUser(text string) {
	turn := s.CurrentTurn()
	s.write(Entry{
		Timestamp: time.Now(),
		Turn:      turn,
		Role:      "user",
		Content:   text,
	})
	// Set task on first user message (turn is 0 before LogRequest increments it)
	if turn == 0 && s.store != nil {
		// Ignore error - this is a best-effort operation for task tracking
		s.store.SetTask(s.sessionID, text)
	}
}

// LogAssistant logs an assistant response with token counts.
func (s *Session) LogAssistant(text string, inTokens, outTokens, toolCalls int) {
	s.mu.Lock()
	s.totalIn += inTokens
	s.totalOut += outTokens
	cost := s.cost()
	turn := s.turn
	s.mu.Unlock()

	s.write(Entry{
		Timestamp:    time.Now(),
		Turn:         turn,
		Role:         "assistant",
		Content:      text,
		Model:        s.model,
		InputTokens:  inTokens,
		OutputTokens: outTokens,
		TotalCost:    cost,
	})
}

// LogToolCall logs a tool invocation to the session.
func (s *Session) LogToolCall(toolID, name string, input json.RawMessage) {
	s.write(Entry{
		Timestamp: time.Now(),
		Turn:      s.CurrentTurn(),
		Role:      "tool_call",
		ToolID:    toolID,
		ToolName:  name,
		ToolInput: input,
	})
}

// TruncateToolResult returns the truncated version of tool output if it was truncated,
// otherwise returns empty string if no truncation was needed.
func (s *Session) TruncateToolResult(name, output string, isError bool) string {
	s.mu.Lock()
	cfg := s.truncateCfg
	s.mu.Unlock()

	result := TruncateToolResult(name, output, cfg)
	if result.Truncated {
		return result.Displayed
	}
	return "" // No modification needed
}

// LogToolResult logs a tool result to the session with optional truncation.
func (s *Session) LogToolResult(toolID, name, output string, isError bool) {
	s.mu.Lock()
	cfg := s.truncateCfg
	s.mu.Unlock()

	// Truncate if needed.
	//
	// The untruncated output is NOT retained. It used to be held in a
	// map[string]string on the Session so it could be "recovered later", and
	// nothing ever recovered it: the only reader was an exported getter with no
	// callers, so a session doing two hundred large ripgreps kept two hundred
	// full outputs resident for the life of the process and handed none of them
	// back. Holding a copy of the thing you just truncated, where no code path
	// can reach it, is the truncation undone and the memory spent twice.
	//
	// If a truncated result should be recoverable, the recovery has to be a tool
	// the model actually holds — that is what conversation.StashLargeContent and
	// memory_expand are, and a pointer to them is only honest when that tool is
	// on the wire. Do not reintroduce a side map here.
	result := TruncateToolResult(name, output, cfg)

	s.write(Entry{
		Timestamp: time.Now(),
		Turn:      s.CurrentTurn(),
		Role:      "tool_result",
		ToolID:    toolID,
		ToolName:  name,
		Content:   result.Displayed,
		IsError:   isError,
	})
}

// LogThinking logs an assistant's thinking content to the session.
func (s *Session) LogThinking(text string) {
	s.write(Entry{
		Timestamp: time.Now(),
		Turn:      s.CurrentTurn(),
		Role:      "thinking",
		Content:   text,
	})
}

// LogRequest logs an API request payload and increments the turn counter.
func (s *Session) LogRequest(payload json.RawMessage) {
	now := time.Now()
	s.mu.Lock()
	s.turn++
	turn := s.turn
	s.mu.Unlock()

	s.write(Entry{
		Timestamp: now,
		Turn:      turn,
		Role:      "request",
		Request:   payload,
	})

	if s.store != nil {
		s.store.InsertTurn(&TurnRow{
			SessionID: s.sessionID,
			Turn:      turn,
			StartedAt: now,
		})
	}
}

// LogCompaction logs memory compaction operations to the session.
func (s *Session) LogCompaction(original []string, newID string, tags []string) {
	data := map[string]interface{}{
		"original_ids": original,
		"new_id":       newID,
		"tags":         tags,
	}
	raw, _ := json.Marshal(data)
	s.write(Entry{
		Timestamp: time.Now(),
		Role:      "compaction",
		Content:   fmt.Sprintf("compacted %d memories into %s", len(original), newID),
		Request:   json.RawMessage(raw),
	})
}

// LogSummarize logs conversation summarization operations to the session.
func (s *Session) LogSummarize(turns int, summaryTokens int, keptMessages int, memoryID string) {
	data := map[string]interface{}{
		"summarized_turns": turns,
		"summary_tokens":   summaryTokens,
		"kept_messages":    keptMessages,
		"memory_id":        memoryID,
	}
	raw, _ := json.Marshal(data)
	s.write(Entry{
		Timestamp: time.Now(),
		Role:      "summarize",
		Content:   fmt.Sprintf("summarized %d turns → %d token summary (kept %d recent messages, memory: %s)", turns, summaryTokens, keptMessages, memoryID),
		Request:   json.RawMessage(raw),
	})
}

// LogStash logs message stashing operations to the session.
func (s *Session) LogStash(messageType string, blockCount int, tokens int) {
	data := map[string]interface{}{
		"message_type": messageType,
		"block_count":  blockCount,
		"tokens":       tokens,
	}
	raw, _ := json.Marshal(data)
	s.write(Entry{
		Timestamp: time.Now(),
		Role:      "stash",
		Content:   fmt.Sprintf("stashed %d large blocks from %s message (%d tokens)", blockCount, messageType, tokens),
		Request:   json.RawMessage(raw),
	})
}

// LogPrune logs message pruning operations to the session.
func (s *Session) LogPrune(removed int, tokensFreed int, strategy string) {
	data := map[string]interface{}{
		"removed":      removed,
		"tokens_freed": tokensFreed,
		"strategy":     strategy,
	}
	raw, _ := json.Marshal(data)
	s.write(Entry{
		Timestamp: time.Now(),
		Role:      "prune",
		Content:   fmt.Sprintf("pruned %d messages (%d tokens freed, strategy: %s)", removed, tokensFreed, strategy),
		Request:   json.RawMessage(raw),
	})
}

// EndTurn records turn completion in the DB (called after each API response).
//
// It used to also append the turn to a timeline.md beside the session log. That
// file had no reader: the one consumer, GET /api/sessions/{key}/timeline, calls
// ReadTimelineFromJSONL and regenerates the timeline from session.jsonl, which
// is the source of truth the derived file was copied from. Producing it cost a
// Sync plus a parse and format of the *entire* log on every turn, to slice out
// the one turn that had just finished — so turn latency grew with the length of
// the session in service of a file nobody opened.
func (s *Session) EndTurn(inTokens, outTokens, toolCalls int, stopReason, errMsg string) {
	s.mu.Lock()
	turn := s.turn
	s.mu.Unlock()

	if s.store != nil {
		// Calculate cost for just this turn
		cost := s.calculateTurnCost(inTokens, outTokens)
		s.store.EndTurn(s.sessionID, turn, inTokens, outTokens, toolCalls, cost, stopReason, errMsg)
	}
}