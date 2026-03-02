package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kayushkin/inber/agent"
)

// TimelineEvent represents a single event in the session timeline.
type TimelineEvent struct {
	Type      string    // "prompt", "thinking", "tool_call", "tool_result", "response", "stats"
	Timestamp time.Time

	// Prompt events
	UserMessage string // truncated
	PromptFile  string // path to full prompt breakdown file
	TurnNumber  int

	// Tool events
	ToolName    string
	ToolInput   string // summarized
	ToolOutput  string // summarized
	ToolIsError bool
	ToolCount   int // running count for this turn

	// Response events
	ResponseText string // truncated

	// Stats
	InputTokens  int
	OutputTokens int
	ToolCalls    int
	Cost         float64
	Model        string
}

// CalcCost calculates cost from model and token counts.
func CalcCost(model string, inTok, outTok int) float64 {
	info, ok := agent.Models[model]
	if !ok {
		return 0
	}
	return (float64(inTok)*info.InputCostPer1M + float64(outTok)*info.OutputCostPer1M) / 1_000_000
}

// FormatTimeline formats a slice of events into markdown.
func FormatTimeline(events []TimelineEvent, startTime time.Time) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# Session Timeline — %s\n", startTime.Format("2006-01-02 15:04")))

	currentTurn := 0
	for i := 0; i < len(events); i++ {
		ev := events[i]
		switch ev.Type {
		case "prompt":
			if ev.TurnNumber > currentTurn {
				currentTurn = ev.TurnNumber
				sb.WriteString(fmt.Sprintf("\n## Turn %d\n", currentTurn))
			}
			if ev.UserMessage != "" && ev.UserMessage != "(tool results)" {
				sb.WriteString(fmt.Sprintf("📤 **Prompt:** \"%s\"\n", ev.UserMessage))
			} else {
				sb.WriteString("📤 **Prompt:** (tool results)\n")
			}
			if ev.PromptFile != "" {
				sb.WriteString(fmt.Sprintf("   → [full context](%s)\n", ev.PromptFile))
			}
			sb.WriteString("\n")

		case "tool_call":
			// Look ahead for the matching tool_result
			resultStr := ""
			if i+1 < len(events) && events[i+1].Type == "tool_result" && events[i+1].ToolName == ev.ToolName {
				result := events[i+1]
				if result.ToolIsError {
					resultStr = fmt.Sprintf("   → ✗ %s\n", result.ToolOutput)
				} else {
					resultStr = fmt.Sprintf("   → %s\n", result.ToolOutput)
				}
				i++ // skip the tool_result
			}
			sb.WriteString(fmt.Sprintf("⚡ **%s** %s\n", ev.ToolName, ev.ToolInput))
			if resultStr != "" {
				sb.WriteString(resultStr)
			}
			sb.WriteString("\n")

		case "tool_result":
			// Standalone (not paired with call above)
			if ev.ToolIsError {
				sb.WriteString(fmt.Sprintf("   → ✗ %s\n\n", ev.ToolOutput))
			} else {
				sb.WriteString(fmt.Sprintf("   → %s\n\n", ev.ToolOutput))
			}

		case "response":
			sb.WriteString(fmt.Sprintf("💬 **Response:** \"%s\"\n", ev.ResponseText))

		case "stats":
			parts := []string{
				fmt.Sprintf("in=%d", ev.InputTokens),
				fmt.Sprintf("out=%d", ev.OutputTokens),
			}
			if ev.ToolCalls > 0 {
				parts = append(parts, fmt.Sprintf("tools=%d", ev.ToolCalls))
			}
			if ev.Cost > 0 {
				parts = append(parts, fmt.Sprintf("$%.4f", ev.Cost))
			}
			sb.WriteString(fmt.Sprintf("📊 [%s]\n", strings.Join(parts, " | ")))
		}
	}

	return sb.String()
}

// FormatTerminalStats formats the last stats event as ANSI-colored terminal output.
func FormatTerminalStats(ev TimelineEvent) string {
	parts := []string{
		fmt.Sprintf("in=%d", ev.InputTokens),
		fmt.Sprintf("out=%d", ev.OutputTokens),
	}
	if ev.ToolCalls > 0 {
		parts = append(parts, fmt.Sprintf("tools=%d", ev.ToolCalls))
	}
	if ev.Cost > 0 {
		parts = append(parts, fmt.Sprintf("$%.4f", ev.Cost))
	}
	return fmt.Sprintf("\033[2m[%s]\033[0m", strings.Join(parts, " | "))
}

// ReconstructTimelineFromJSONL reads a session.jsonl file and reconstructs the timeline.
func ReconstructTimelineFromJSONL(logFilePath string) ([]TimelineEvent, time.Time, error) {
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

			// Add stats after response
			if entry.InputTokens > 0 || entry.OutputTokens > 0 {
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
					ToolCalls:    toolCalls,
					Cost:         CalcCost(entry.Model, entry.InputTokens, entry.OutputTokens),
					Model:        entry.Model,
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
func ReadTimelineFromJSONL(logsDir, sessionID string) (string, error) {
	// Find the session.jsonl file
	logFile := findSessionJSONL(logsDir, sessionID)
	if logFile == "" {
		return "", fmt.Errorf("session log not found: %s", sessionID)
	}

	events, startTime, err := ReconstructTimelineFromJSONL(logFile)
	if err != nil {
		return "", err
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

func truncateStr(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func summarizeToolInput(name, raw string) string {
	// Extract a concise representation
	raw = strings.TrimSpace(raw)
	
	// For shell/exec, try to extract the command
	if name == "shell" || name == "bash" {
		var input struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal([]byte(raw), &input); err == nil && input.Command != "" {
			if len(input.Command) > 100 {
				return fmt.Sprintf("`%s...`", input.Command[:100])
			}
			return fmt.Sprintf("`%s`", input.Command)
		}
	}
	
	// For file tools, try to extract the path
	if strings.Contains(name, "file") || strings.Contains(name, "write") || strings.Contains(name, "read") || strings.Contains(name, "edit") {
		var input struct {
			Path     string `json:"path"`
			FilePath string `json:"file_path"`
		}
		if err := json.Unmarshal([]byte(raw), &input); err == nil {
			path := input.Path
			if path == "" {
				path = input.FilePath
			}
			if path != "" {
				return fmt.Sprintf("`%s`", path)
			}
		}
	}
	
	// Generic: truncate
	s := raw
	if len(s) > 80 {
		s = s[:80] + "..."
	}
	return s
}

func summarizeToolOutput(output string) string {
	bytes := len(output)
	if bytes == 0 {
		return "0 bytes"
	}
	lines := strings.Count(output, "\n") + 1
	if lines > 1 {
		return fmt.Sprintf("%d lines, %s bytes", lines, formatNumber(bytes))
	}
	return fmt.Sprintf("%s bytes", formatNumber(bytes))
}

func formatNumber(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%d,%03d", n/1000, n%1000)
}
