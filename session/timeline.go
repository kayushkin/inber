package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

// Timeline accumulates events during a session and writes a timeline markdown file.
type Timeline struct {
	mu        sync.Mutex
	sessionID string
	logsDir   string // directory containing the session log file
	startTime time.Time
	events    []TimelineEvent
	turnTool  int // running tool count for current turn
}

// NewTimeline creates a new timeline for a session.
func NewTimeline(logFilePath, sessionID string) *Timeline {
	return &Timeline{
		sessionID: sessionID,
		logsDir:   filepath.Dir(logFilePath),
		startTime: time.Now(),
	}
}

// AddPrompt adds a prompt event.
func (tl *Timeline) AddPrompt(userMessage string, turnNumber int, promptFile string) {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	tl.turnTool = 0
	tl.events = append(tl.events, TimelineEvent{
		Type:        "prompt",
		Timestamp:   time.Now(),
		UserMessage: truncateStr(userMessage, 120),
		TurnNumber:  turnNumber,
		PromptFile:  promptFile,
	})
}

// AddToolCall adds a tool_call event.
func (tl *Timeline) AddToolCall(name, input string) {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	tl.turnTool++
	tl.events = append(tl.events, TimelineEvent{
		Type:      "tool_call",
		Timestamp: time.Now(),
		ToolName:  name,
		ToolInput: summarizeToolInput(name, input),
		ToolCount: tl.turnTool,
	})
}

// AddToolResult adds a tool_result event.
func (tl *Timeline) AddToolResult(name, output string, isError bool) {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	tl.events = append(tl.events, TimelineEvent{
		Type:        "tool_result",
		Timestamp:   time.Now(),
		ToolName:    name,
		ToolOutput:  summarizeToolOutput(output),
		ToolIsError: isError,
	})
}

// AddResponse adds a response event.
func (tl *Timeline) AddResponse(text string) {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	tl.events = append(tl.events, TimelineEvent{
		Type:         "response",
		Timestamp:    time.Now(),
		ResponseText: truncateStr(text, 120),
	})
}

// AddStats adds a stats event.
func (tl *Timeline) AddStats(model string, inTok, outTok, toolCalls int) {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	cost := CalcCost(model, inTok, outTok)
	tl.events = append(tl.events, TimelineEvent{
		Type:         "stats",
		Timestamp:    time.Now(),
		InputTokens:  inTok,
		OutputTokens: outTok,
		ToolCalls:    toolCalls,
		Cost:         cost,
		Model:        model,
	})
}

// Events returns a copy of the timeline events.
func (tl *Timeline) Events() []TimelineEvent {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	out := make([]TimelineEvent, len(tl.events))
	copy(out, tl.events)
	return out
}

// CalcCost calculates cost from model and token counts.
func CalcCost(model string, inTok, outTok int) float64 {
	info, ok := agent.Models[model]
	if !ok {
		return 0
	}
	return (float64(inTok)*info.InputCostPer1M + float64(outTok)*info.OutputCostPer1M) / 1_000_000
}

// Format produces the markdown timeline file content.
func (tl *Timeline) Format() string {
	tl.mu.Lock()
	events := make([]TimelineEvent, len(tl.events))
	copy(events, tl.events)
	startTime := tl.startTime
	tl.mu.Unlock()

	return FormatTimeline(events, startTime)
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

// FormatTerminal formats the last stats event as ANSI-colored terminal output.
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

// WriteFile writes the timeline markdown to disk.
func (tl *Timeline) WriteFile() error {
	tl.mu.Lock()
	dir := tl.logsDir
	sid := tl.sessionID
	tl.mu.Unlock()

	path := filepath.Join(dir, sid+"-timeline.md")
	content := tl.Format()
	return os.WriteFile(path, []byte(content), 0644)
}

// TimelinePath returns the path where the timeline file is written.
func (tl *Timeline) TimelinePath() string {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	return filepath.Join(tl.logsDir, tl.sessionID+"-timeline.md")
}

// ReadTimelineFile reads a timeline markdown file from disk given logs dir and session ID.
func ReadTimelineFile(logsDir, sessionID string) (string, error) {
	// Search in logsDir and subdirs
	var found string
	filepath.Walk(logsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.Contains(info.Name(), sessionID) && strings.HasSuffix(info.Name(), "-timeline.md") {
			found = path
			return filepath.SkipAll
		}
		return nil
	})

	if found == "" {
		return "", fmt.Errorf("timeline not found for session: %s", sessionID)
	}

	data, err := os.ReadFile(found)
	if err != nil {
		return "", err
	}
	return string(data), nil
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
		if idx := strings.Index(raw, "command"); idx >= 0 {
			// Extract command value
			start := strings.Index(raw[idx:], ":")
			if start >= 0 {
				val := strings.TrimSpace(raw[idx+start+1:])
				val = strings.Trim(val, "\"}")
				if len(val) > 100 {
					val = val[:100] + "..."
				}
				return fmt.Sprintf("`%s`", val)
			}
		}
	}
	// For file tools, try to extract the path
	if strings.Contains(name, "file") || strings.Contains(name, "write") || strings.Contains(name, "read") || strings.Contains(name, "edit") {
		if idx := strings.Index(raw, "path"); idx >= 0 {
			start := strings.Index(raw[idx:], ":")
			if start >= 0 {
				val := strings.TrimSpace(raw[idx+start+1:])
				val = strings.Trim(val, "\",}")
				return fmt.Sprintf("`%s`", val)
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
