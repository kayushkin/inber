package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	modelstore "github.com/kayushkin/model-store"
)

// ReconstructTimelineFromJSONL reads a session.jsonl file and reconstructs the timeline.
//
// The log records the model each turn ran on, so the store is what turns that
// id into the prices it was billed at; with a nil store every turn in the
// rebuilt timeline reads at the unknown-model flat rate whatever it actually
// cost.
func ReconstructTimelineFromJSONL(logFilePath string, store *modelstore.Store) ([]TimelineEvent, time.Time, error) {
	file, err := os.Open(logFilePath)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("open log file: %w", err)
	}
	defer file.Close()

	var events []TimelineEvent
	var startTime time.Time
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB lines
	turnToolCount := make(map[int]int) // turn -> tool count

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var entry Entry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		// First entry defines start time
		if startTime.IsZero() {
			startTime = entry.Timestamp
		}

		switch entry.Role {
		case "request":
			// Extract user message from request
			var req struct {
				Messages []struct {
					Role    string `json:"role"`
					Content []struct {
						Type string `json:"type"`
						Text string `json:"text,omitempty"`
					} `json:"content"`
				} `json:"messages"`
			}
			if err := json.Unmarshal(entry.Request, &req); err == nil {
				userMsg := "(tool results)"
				// Find the last user message
				for i := len(req.Messages) - 1; i >= 0; i-- {
					if req.Messages[i].Role == "user" {
						for _, block := range req.Messages[i].Content {
							if block.Type == "text" && block.Text != "" {
								userMsg = block.Text
								break
							}
						}
						break
					}
				}
				events = append(events, TimelineEvent{
					Type:        "prompt",
					Timestamp:   entry.Timestamp,
					TurnNumber:  entry.Turn,
					UserMessage: truncateStr(userMsg, 120),
					PromptFile:  fmt.Sprintf("prompts/turn-%d.md", entry.Turn),
				})
				turnToolCount[entry.Turn] = 0
			}

		case "thinking":
			// Could add thinking events if desired
			// For now, skip to keep timeline concise

		case "tool_call":
			turnToolCount[entry.Turn]++
			events = append(events, TimelineEvent{
				Type:      "tool_call",
				Timestamp: entry.Timestamp,
				ToolName:  entry.ToolName,
				ToolInput: summarizeToolInput(entry.ToolName, string(entry.ToolInput)),
				ToolCount: turnToolCount[entry.Turn],
			})

		case "tool_result":
			events = append(events, TimelineEvent{
				Type:        "tool_result",
				Timestamp:   entry.Timestamp,
				ToolName:    entry.ToolName,
				ToolOutput:  summarizeToolOutput(entry.Content),
				ToolIsError: entry.IsError,
			})

		case "assistant":
			events = append(events, TimelineEvent{
				Type:         "response",
				Timestamp:    entry.Timestamp,
				ResponseText: truncateStr(entry.Content, 120),
			})

			// Add stats after response. A turn whose whole prompt was served
			// from the cache has no fresh input at all, so gating on the input
			// and output counts alone would drop the stats line for the turns
			// the cache worked best on.
			if entry.InputTokens > 0 || entry.OutputTokens > 0 || entry.CacheRead > 0 || entry.CacheWrite > 0 {
				// Count tool calls in this turn
				toolCalls := 0
				for j := len(events) - 1; j >= 0; j-- {
					if events[j].Type == "tool_call" {
						toolCalls++
					}
					if events[j].Type == "prompt" {
						break
					}
				}
				
				events = append(events, TimelineEvent{
					Type:         "stats",
					Timestamp:    entry.Timestamp,
					InputTokens:  entry.InputTokens,
					OutputTokens: entry.OutputTokens,
					CacheRead:    entry.CacheRead,
					CacheWrite:   entry.CacheWrite,
					ToolCalls:    toolCalls,
					// The rebuilt timeline is the one cost figure a user reads
					// directly, so it prices the cache the same way the live
					// path does. CalcCost has no cache terms and was charging
					// this turn for its uncached remainder only.
					Cost:  CalcCostWithCache(entry.Model, entry.InputTokens, entry.OutputTokens, entry.CacheRead, entry.CacheWrite, store),
					Model: entry.Model,
				})
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, time.Time{}, fmt.Errorf("read log file: %w", err)
	}

	if startTime.IsZero() {
		startTime = time.Now()
	}

	return events, startTime, nil
}

// ReadTimelineFromJSONL reads a session.jsonl file and generates a timeline markdown.
func ReadTimelineFromJSONL(logsDir, sessionID string, store *modelstore.Store) (string, error) {
	// Find the session.jsonl file
	logFile := findSessionJSONL(logsDir, sessionID)
	if logFile == "" {
		return "", fmt.Errorf("session log not found: %s", sessionID)
	}

	events, startTime, err := ReconstructTimelineFromJSONL(logFile, store)
	if err != nil {
		return "", fmt.Errorf("failed to reconstruct timeline from %s: %w", logFile, err)
	}

	return FormatTimeline(events, startTime), nil
}

// findSessionJSONL finds the session.jsonl file for a given session ID.
func findSessionJSONL(logsDir, sessionID string) string {
	var found string
	filepath.WalkDir(logsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if d.Name() == "session.jsonl" && strings.Contains(path, sessionID) {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	return found
}