package session

import (
	"fmt"
	"strings"
	"time"
)

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